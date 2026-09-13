package zabbixreceiver

import (
	"bufio"
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/wieso/zabbixreceiver/receiver/zabbixreceiver/internal/discovery"
	otelmetrics "github.com/wieso/zabbixreceiver/receiver/zabbixreceiver/internal/metrics"
	"github.com/wieso/zabbixreceiver/receiver/zabbixreceiver/internal/zabbix"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.uber.org/zap"
)

func (r *zabbixReceiver) startStreaming() error {
	listener, err := net.Listen("tcp", r.config.Streaming.Endpoint)
	if err != nil {
		return err
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/history", r.handleHistory)
	r.server = &http.Server{Handler: mux, ReadTimeout: r.config.Streaming.Timeout, ReadHeaderTimeout: r.config.Streaming.Timeout, WriteTimeout: r.config.Streaming.Timeout, IdleTimeout: time.Minute}
	go func() {
		if err := r.server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			r.logger.Error("Zabbix streaming server failed", zap.Error(err))
		}
	}()
	return nil
}

// History is newline-delimited JSON, not a JSON array. Pointer fields distinguish
// absent required fields from valid zero-valued numeric samples.
type historyRecord struct {
	Host struct {
		Host string `json:"host"`
		Name string `json:"name"`
	} `json:"host"`
	Groups   []string        `json:"groups"`
	ItemTags []zabbix.Tag    `json:"item_tags"`
	Name     string          `json:"name"`
	ItemID   json.Number     `json:"itemid"`
	Clock    *int64          `json:"clock"`
	NS       int64           `json:"ns"`
	Value    json.RawMessage `json:"value"`
	Type     *int            `json:"type"`
}

func streamReply(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	key := "error"
	if status == http.StatusOK {
		key = "response"
	}
	_ = json.NewEncoder(w).Encode(map[string]string{key: message})
}

func (r *zabbixReceiver) handleHistory(w http.ResponseWriter, req *http.Request) {
	started := time.Now()
	obsCtx := r.obs.StartMetricsOp(req.Context())
	req = req.WithContext(obsCtx)
	var received int
	var receiveErr error
	defer func() { r.obs.EndMetricsOp(obsCtx, "zabbix", received, receiveErr) }()
	r.telemetry.valuesAttempts.Add(req.Context(), 1, r.telemetry.attrs)
	status := http.StatusOK
	defer func() {
		r.telemetry.valuesDuration.Record(req.Context(), time.Since(started).Seconds(), r.telemetry.attrs)
		r.telemetry.streamingRequests.Add(req.Context(), 1, r.telemetry.attrs, metric.WithAttributes(attribute.Int("http.response.status_code", status)))
		if status != http.StatusOK {
			r.telemetry.valuesErrors.Add(req.Context(), 1, r.telemetry.attrs)
		}
	}()
	fail := func(code int, message string) {
		status = code
		receiveErr = errors.New(message)
		fields := []zap.Field{zap.Int("status_code", code), zap.String("reason", message), zap.Duration("duration", time.Since(started))}
		if code >= 500 {
			r.logger.Error("Zabbix streaming request failed", fields...)
		} else {
			r.logger.Warn("Zabbix streaming request rejected", fields...)
		}
		streamReply(w, code, message)
	}
	if req.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		fail(405, "POST required")
		return
	}
	if token := string(r.config.Streaming.Token); token != "" && subtle.ConstantTimeCompare([]byte(req.Header.Get("Authorization")), []byte("Bearer "+token)) != 1 {
		fail(401, "unauthorized")
		return
	}
	mediaType, _, err := mime.ParseMediaType(req.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/x-ndjson" {
		fail(415, "application/x-ndjson required")
		return
	}
	ctx, cancel := context.WithTimeout(req.Context(), r.config.Streaming.Timeout)
	defer cancel()
	body := http.MaxBytesReader(w, req.Body, r.config.Streaming.MaxRequestBodySize)
	defer body.Close()
	readFailure := func(err error) {
		var tooLarge *http.MaxBytesError
		switch {
		case errors.As(err, &tooLarge):
			fail(413, "request too large")
		case errors.Is(err, errUnsupportedEncoding):
			fail(415, "unsupported content encoding")
		default:
			fail(400, "cannot read request")
		}
	}
	decoded, err := decodeHistoryBody(body, req.Header.Get("Content-Encoding"), r.config.Streaming.MaxRequestBodySize)
	if err != nil {
		readFailure(err)
		return
	}
	defer decoded.Close()
	reader := bufio.NewReader(http.MaxBytesReader(w, decoded, r.config.Streaming.MaxRequestBodySize))
	batch := pmetric.NewMetrics()
	scope := batch.ResourceMetrics().AppendEmpty().ScopeMetrics().AppendEmpty()
	scope.Scope().SetName(r.settings.ID.String())
	var emitted int64
	// One snapshot for the entire request prevents mixed generations of labels.
	snapshot := r.store.Load()
	var skipped int64
	skip := func() { skipped++; r.telemetry.invalidValues.Add(ctx, 1, r.telemetry.attrs) }
	for {
		if ctx.Err() != nil {
			fail(503, "request timeout")
			return
		}
		line, readErr := reader.ReadBytes('\n')
		if readErr != nil && readErr != io.EOF {
			readFailure(readErr)
			return
		}
		line = bytes.TrimSpace(line)
		if len(line) > 0 {
			var record historyRecord
			if err := json.Unmarshal(line, &record); err != nil || record.ItemID == "" || record.Clock == nil || record.Type == nil || len(record.Value) == 0 {
				skip()
				continue
			}
			if _, err := strconv.ParseUint(string(record.ItemID), 10, 64); err != nil {
				skip()
				continue
			}
			if *record.Type == 0 || *record.Type == 3 {
				value := string(record.Value)
				if len(value) > 0 && value[0] == '"' {
					if json.Unmarshal(record.Value, &value) != nil {
						skip()
						continue
					}
				}
				number, timestamp, err := otelmetrics.ParseSample(value, *record.Clock, record.NS)
				if err != nil {
					skip()
					continue
				}
				if record.Host.Host == "" || record.Name == "" {
					skip()
					continue
				}
				meta := zabbix.Metadata{}
				var hostID, itemKey string
				if r.config.Streaming.EnrichWithAPI {
					item, ok := snapshot.Lookup(string(record.ItemID))
					if !ok {
						fail(503, "item metadata unavailable; retry after discovery")
						return
					}
					meta, hostID, itemKey = item.Metadata, item.HostID, item.Key
				}
				meta.ItemName, meta.ValueType = record.Name, strconv.Itoa(*record.Type)
				if record.Host.Name != "" {
					meta.HostName = record.Host.Name
				}
				if record.Groups != nil {
					meta.Groups = record.Groups
				}
				if record.ItemTags != nil {
					meta.ItemTags = record.ItemTags
				}
				item := discovery.ItemMeta{ID: string(record.ItemID), Host: record.Host.Host, HostID: hostID, Key: itemKey, Name: record.Name, Metadata: meta}
				if !otelmetrics.AppendGauge(scope.Metrics(), item, number, timestamp, otelmetrics.Config{Prefix: r.config.Prom.Prefix, ConstLabels: r.config.Prom.ConstLabels, MetadataEnabled: r.config.Metadata.Enabled}) {
					skip()
					continue
				}
				emitted++
			}
		}
		if readErr == io.EOF {
			break
		}
		if ctx.Err() != nil {
			fail(503, "request timeout")
			return
		}
	}
	// Some decompressors stop at the end of their stream without consuming the
	// complete HTTP body. MaxBytesReader keeps read errors (including limits)
	// sticky, so draining it also catches errors swallowed during read-ahead.
	if _, err := io.Copy(io.Discard, body); err != nil {
		readFailure(err)
		return
	}
	if ctx.Err() != nil {
		fail(503, "request timeout")
		return
	}
	received = int(emitted)
	if err := r.deliver(ctx, batch); err != nil {
		fail(503, "downstream unavailable")
		receiveErr = err
		return
	}
	r.logger.Debug("Zabbix streaming request completed", zap.Int64("points", emitted), zap.Int64("invalid_records", skipped), zap.Duration("duration", time.Since(started)))
	streamReply(w, http.StatusOK, "success")
}
