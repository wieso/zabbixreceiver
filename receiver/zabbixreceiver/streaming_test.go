package zabbixreceiver

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"context"
	"errors"
	"fmt"
	"github.com/golang/snappy"
	"github.com/klauspost/compress/zstd"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/wieso/zabbixreceiver/receiver/zabbixreceiver/internal/zabbix"
	"go.opentelemetry.io/collector/receiver/receivertest"
)

func TestStreamingHistory(t *testing.T) {
	cfg := validConfig()
	cfg.Mode = "streaming"
	cfg.Streaming.Token = "stream-secret"
	next := newRecordingConsumer(t)
	r := newTestReceiver(t, cfg, &fakeAPI{}, next)
	body := `{"host":{"host":"stream-host"},"name":"CPU load","itemid":1,"clock":1700000000,"ns":123,"value":12.5,"type":0}`
	send := func(body, token string) int {
		req := httptest.NewRequest("POST", "/v1/history", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/x-ndjson")
		w := httptest.NewRecorder()
		r.handleHistory(w, req)
		return w.Code
	}
	require.Equal(t, 401, send(body, "wrong"))
	require.Empty(t, next.snapshot())
	require.Equal(t, 200, send("{", "stream-secret"))
	require.Empty(t, next.snapshot())
	require.Equal(t, 200, send(body+"\n"+body, "stream-secret"))
	batches := next.snapshot()
	require.Len(t, batches, 1)
	metrics := batches[0].ResourceMetrics().At(0).ScopeMetrics().At(0).Metrics()
	require.Equal(t, 2, metrics.Len())
	require.Equal(t, "zabbix_cpu_load", metrics.At(0).Name())
	require.Equal(t, map[string]any{"host": "stream-host", "itemid": "1", "item_name": "CPU load", "value_type": "0"}, metrics.At(0).Gauge().DataPoints().At(0).Attributes().AsRaw())
	require.EqualValues(t, 1700000000000000123, metrics.At(0).Gauge().DataPoints().At(0).Timestamp())
	require.Equal(t, 12.5, metrics.At(0).Gauge().DataPoints().At(0).DoubleValue())
	next.err = errors.New("backpressure")
	require.Equal(t, 503, send(body, "stream-secret"))
}

func TestStreamingRequestValidation(t *testing.T) {
	for _, tc := range []struct {
		name, method, contentType, body string
		limit                           int64
		code                            int
	}{
		{"method", "GET", "application/x-ndjson", "", 1024, 405},
		{"content type", "POST", "application/json", "{}", 1024, 415},
		{"size", "POST", "application/x-ndjson", strings.Repeat(" ", 1025), 1024, 413},
		{"missing fields", "POST", "application/x-ndjson", `{"host":{"host":"stream-host"},"name":"CPU load","itemid":1}`, 1024, 200},
		{"negative ns", "POST", "application/x-ndjson", `{"host":{"host":"stream-host"},"name":"CPU load","itemid":1,"clock":1,"ns":-1,"value":1,"type":0}`, 1024, 200},
		{"NaN", "POST", "application/x-ndjson", `{"host":{"host":"stream-host"},"name":"CPU load","itemid":1,"clock":1,"value":"NaN","type":0}`, 1024, 200},
		{"event", "POST", "application/x-ndjson", `{"eventid":1,"clock":1,"value":1}`, 1024, 200},
		{"missing host", "POST", "application/x-ndjson", `{"name":"CPU load","itemid":99,"clock":1,"value":1,"type":0}`, 1024, 200},
		{"text", "POST", "application/x-ndjson", `{"host":{"host":"stream-host"},"name":"CPU load","itemid":1,"clock":1,"value":"text","type":4}`, 1024, 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := validConfig()
			cfg.Streaming.MaxRequestBodySize = tc.limit
			next := newRecordingConsumer(t)
			r := newTestReceiver(t, cfg, &fakeAPI{}, next)
			req := httptest.NewRequest(tc.method, "/v1/history", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", tc.contentType)
			w := httptest.NewRecorder()
			r.handleHistory(w, req)
			require.Equal(t, tc.code, w.Code)
			require.Empty(t, next.snapshot())
		})
	}
}

func TestStreamingConfig(t *testing.T) {
	cfg := validConfig()
	cfg.Mode = "streaming"
	require.NoError(t, cfg.validateResolved())
	cfg.Schedule.Jobs.Discover.Enabled = false
	cfg.Zabbix = ZabbixConfig{}
	require.NoError(t, cfg.validateResolved())
	cfg.Schedule.Jobs.Discover.Enabled = true
	cfg.Streaming.Endpoint = "no-port"
	cfg.Streaming.MaxRequestBodySize = 0
	cfg.Streaming.Timeout = 0
	err := cfg.validateResolved()
	require.ErrorContains(t, err, "streaming.endpoint")
	require.ErrorContains(t, err, "streaming.max_request_body_size")
	require.ErrorContains(t, err, "streaming.timeout")
	cfg.Mode = "other"
	require.ErrorContains(t, cfg.validateResolved(), "mode")
}

func TestStreamingLifecycleNeverPollsValues(t *testing.T) {
	cfg := validConfig()
	cfg.Mode = "streaming"
	cfg.Streaming.Endpoint = "127.0.0.1:0"
	cfg.Schedule.Jitter = 0
	cfg.Schedule.Jobs.Discover = JobConfig{Enabled: true, RunOnStart: true, Interval: time.Millisecond, Timeout: time.Second}
	cfg.Schedule.Jobs.Values = JobConfig{Enabled: true, RunOnStart: true, Interval: time.Millisecond, Timeout: time.Second}
	var calls atomic.Int64
	api := &fakeAPI{hosts: func(context.Context) ([]zabbix.Host, error) { calls.Add(1); return nil, nil }, values: func(context.Context, []string) ([]zabbix.Value, error) { calls.Add(1); return nil, nil }}
	r := newTestReceiver(t, cfg, api, newRecordingConsumer(t))
	require.NoError(t, r.Start(context.Background(), nil))
	require.NoError(t, r.Start(context.Background(), nil))
	time.Sleep(20 * time.Millisecond)
	require.Zero(t, calls.Load())
	require.NoError(t, r.Shutdown(context.Background()))
	require.NoError(t, r.Shutdown(context.Background()))
}

func TestStreamingBindFailure(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()
	cfg := validConfig()
	cfg.Mode = "streaming"
	cfg.Streaming.Endpoint = listener.Addr().String()
	r := newTestReceiver(t, cfg, &fakeAPI{}, newRecordingConsumer(t))
	require.Error(t, r.Start(context.Background(), nil))
	require.NoError(t, r.Shutdown(context.Background()))
}

func TestStreamingRealHTTP(t *testing.T) {
	cfg := validConfig()
	next := newRecordingConsumer(t)
	r := newTestReceiver(t, cfg, &fakeAPI{}, next)
	server := httptest.NewServer(http.HandlerFunc(r.handleHistory))
	defer server.Close()
	response, err := http.Post(server.URL+"/v1/history", "application/x-ndjson", strings.NewReader(`{"host":{"host":"stream-host"},"name":"CPU load","itemid":1,"clock":0,"ns":7,"value":"18446744073709551615","type":3}`))
	require.NoError(t, err)
	defer response.Body.Close()
	require.Equal(t, 200, response.StatusCode)
	require.Len(t, next.snapshot(), 1)
}

func TestStreamingFactoryWithoutAPI(t *testing.T) {
	cfg := createDefaultConfig().(*Config)
	cfg.Mode = "streaming"
	cfg.Zabbix.Filters.HostIncludeRegex = "[invalid"
	cfg.Schedule.Jitter = -time.Second
	cfg.Streaming.Endpoint = "127.0.0.1:0"
	// Even invalid global API overrides must not affect streaming.
	t.Setenv("ZABBIX_TIMEOUT", "invalid")
	t.Setenv("ZABBIX_ITEMS_PER_REQUEST", "invalid")
	require.NoError(t, cfg.Validate())
	factory := NewFactory()
	r, err := factory.CreateMetrics(context.Background(), receivertest.NewNopSettings(factory.Type()), cfg, newRecordingConsumer(t).metrics)
	require.NoError(t, err)
	require.NoError(t, r.Start(context.Background(), nil))
	require.NoError(t, r.Shutdown(context.Background()))
}

func TestStreamingCompressedPartialHistory(t *testing.T) {
	record := `{"host":{"host":"h"},"name":"load","itemid":1,"clock":1700000000,"ns":1000000000,"value":12.5,"type":0}`
	for _, encoding := range []string{"", "identity", "none", "gzip", "deflate", "zstd", "snappy"} {
		t.Run(encoding, func(t *testing.T) {
			payload := []byte(record + "\n{broken\n" + record)
			encoded := compressHistory(t, encoding, payload)
			cfg := validConfig()
			next := newRecordingConsumer(t)
			r := newTestReceiver(t, cfg, &fakeAPI{}, next)
			req := httptest.NewRequest("POST", "/v1/history", bytes.NewReader(encoded))
			req.Header.Set("Content-Type", "application/x-ndjson")
			req.Header.Set("Content-Encoding", encoding)
			w := httptest.NewRecorder()
			r.handleHistory(w, req)
			require.Equal(t, 200, w.Code, w.Body.String())
			batches := next.snapshot()
			require.Len(t, batches, 1)
			metrics := batches[0].ResourceMetrics().At(0).ScopeMetrics().At(0).Metrics()
			require.Equal(t, 2, metrics.Len())
			for i := 0; i < metrics.Len(); i++ {
				point := metrics.At(i).Gauge().DataPoints().At(0)
				require.EqualValues(t, 1700000001000000000, point.Timestamp())
				require.Equal(t, 12.5, point.DoubleValue())
			}
		})
	}
}

func compressHistory(t *testing.T, encoding string, data []byte) []byte {
	t.Helper()
	var b bytes.Buffer
	switch encoding {
	case "gzip":
		w := gzip.NewWriter(&b)
		_, err := w.Write(data)
		require.NoError(t, err)
		require.NoError(t, w.Close())
	case "deflate":
		w := zlib.NewWriter(&b)
		_, err := w.Write(data)
		require.NoError(t, err)
		require.NoError(t, w.Close())
	case "zstd":
		w, err := zstd.NewWriter(&b)
		require.NoError(t, err)
		_, err = w.Write(data)
		require.NoError(t, err)
		require.NoError(t, w.Close())
	case "snappy":
		return snappy.Encode(nil, data)
	default:
		return data
	}
	return b.Bytes()
}

func TestStreamingCompressedFailures(t *testing.T) {
	record := `{"host":{"host":"h"},"name":"load","itemid":1,"clock":1,"value":1,"type":0}`
	for _, encoding := range []string{"gzip", "deflate", "zstd", "snappy"} {
		for _, failure := range []string{"corrupt", "decoded limit", "encoded limit"} {
			t.Run(encoding+"/"+failure, func(t *testing.T) {
				cfg := validConfig()
				encoded := compressHistory(t, encoding, []byte(record+"\n"+strings.Repeat(" ", 4096)))
				code := 400
				switch failure {
				case "corrupt":
					encoded = encoded[:len(encoded)-1]
				case "decoded limit":
					cfg.Streaming.MaxRequestBodySize = 1024
					code = 413
				case "encoded limit":
					cfg.Streaming.MaxRequestBodySize = 8
					code = 413
				}
				next := newRecordingConsumer(t)
				r := newTestReceiver(t, cfg, &fakeAPI{}, next)
				req := httptest.NewRequest("POST", "/v1/history", bytes.NewReader(encoded))
				req.Header.Set("Content-Type", "application/x-ndjson")
				req.Header.Set("Content-Encoding", encoding)
				w := httptest.NewRecorder()
				r.handleHistory(w, req)
				require.Equal(t, code, w.Code, w.Body.String())
				require.Empty(t, next.snapshot())
			})
		}
	}
}

func TestStreamingTimestampBoundaries(t *testing.T) {
	for _, tc := range []struct {
		clock, ns int64
		accepted  bool
		timestamp uint64
	}{
		{1, 1000000001, true, 2000000001},
		{9223372036, 854775807, true, 9223372036854775807},
		{9223372036, 854775808, false, 0},
		{1, 9223372036854775807, false, 0},
		{-1, 1000000000, false, 0},
	} {
		t.Run(fmt.Sprintf("%d/%d", tc.clock, tc.ns), func(t *testing.T) {
			next := newRecordingConsumer(t)
			r := newTestReceiver(t, validConfig(), &fakeAPI{}, next)
			data := fmt.Sprintf(`{"host":{"host":"h"},"name":"load","itemid":1,"clock":%d,"ns":%d,"value":1,"type":0}`, tc.clock, tc.ns)
			req := httptest.NewRequest("POST", "/v1/history", strings.NewReader(data))
			req.Header.Set("Content-Type", "application/x-ndjson")
			w := httptest.NewRecorder()
			r.handleHistory(w, req)
			require.Equal(t, 200, w.Code)
			if !tc.accepted {
				require.Empty(t, next.snapshot())
				return
			}
			batches := next.snapshot()
			require.Len(t, batches, 1)
			require.EqualValues(t, tc.timestamp, batches[0].ResourceMetrics().At(0).ScopeMetrics().At(0).Metrics().At(0).Gauge().DataPoints().At(0).Timestamp())
		})
	}
}

func TestStreamingUnsupportedEncoding(t *testing.T) {
	for _, encoding := range []string{"br", "gzip, deflate"} {
		t.Run(encoding, func(t *testing.T) {
			next := newRecordingConsumer(t)
			r := newTestReceiver(t, validConfig(), &fakeAPI{}, next)
			req := httptest.NewRequest("POST", "/v1/history", strings.NewReader("{}"))
			req.Header.Set("Content-Type", "application/x-ndjson")
			req.Header.Set("Content-Encoding", encoding)
			w := httptest.NewRecorder()
			r.handleHistory(w, req)
			require.Equal(t, 415, w.Code)
			require.Empty(t, next.snapshot())
		})
	}
}
