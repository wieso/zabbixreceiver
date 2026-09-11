package zabbixreceiver

import (
	"context"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wieso/zabbixreceiver/receiver/zabbixreceiver/internal/discovery"
	otelmetrics "github.com/wieso/zabbixreceiver/receiver/zabbixreceiver/internal/metrics"
	"github.com/wieso/zabbixreceiver/receiver/zabbixreceiver/internal/zabbix"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/receiver"
	"go.opentelemetry.io/otel/metric/noop"
)

// Conversion only: excludes API HTTP, discovery, processors and remote write.
func BenchmarkScalingAPIBuild(b *testing.B) {
	for _, n := range []int{1000, 10000, 100000} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			items := make([]discovery.ItemMeta, n)
			values := make([]zabbix.Value, n)
			for i := range items {
				id := fmt.Sprint(i + 1)
				items[i] = discovery.ItemMeta{ID: id, HostID: "1", Host: "demo-host", Name: "CPU load", Key: "system.cpu.load", ValueType: "0"}
				values[i] = zabbix.Value{ItemID: id, LastValue: "12.5", LastClock: "1700000000"}
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				data, stats := otelmetrics.Build(items, values, otelmetrics.Config{Prefix: "zabbix_", ConstLabels: map[string]string{"env": "bench"}})
				if stats.Emitted != int64(n) || data.DataPointCount() != n {
					b.Fatal("unexpected point count")
				}
			}
			b.ReportMetric(float64(n)*float64(b.N)/b.Elapsed().Seconds(), "points/s")
		})
	}
}

// Full HTTP handler, with no-op telemetry and a non-retaining consumer.
// It excludes socket I/O, processors, export and concurrent requests.
func BenchmarkScalingStreaming(b *testing.B) {
	for _, n := range []int{100, 1000, 10000} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			body := strings.Repeat(`{"host":{"host":"demo-host"},"name":"CPU load","itemid":1,"clock":1700000000,"ns":0,"value":12.5,"type":0}`+"\n", n)
			cfg := createDefaultConfig().(*Config)
			cfg.Mode = "streaming"
			cfg.Streaming.Token = "benchmark-token"
			cfg.Prom.ConstLabels = map[string]string{"env": "bench"}
			next, err := consumer.NewMetrics(func(_ context.Context, data pmetric.Metrics) error {
				if data.DataPointCount() != n {
					return fmt.Errorf("unexpected point count: %d", data.DataPointCount())
				}
				return nil
			})
			if err != nil {
				b.Fatal(err)
			}
			telemetry, err := newTelemetry(noop.NewMeterProvider().Meter("bench"))
			if err != nil {
				b.Fatal(err)
			}
			r, err := newReceiver(receiver.Settings{ID: component.MustNewID("zabbix")}, cfg, next, nil, telemetry)
			if err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.SetBytes(int64(len(body)))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				req := httptest.NewRequest("POST", "/v1/history", strings.NewReader(body))
				req.Header.Set("Content-Type", "application/x-ndjson")
				req.Header.Set("Authorization", "Bearer benchmark-token")
				w := httptest.NewRecorder()
				r.handleHistory(w, req)
				if w.Code != 200 {
					b.Fatalf("HTTP %d: %s", w.Code, w.Body.String())
				}
			}
			b.ReportMetric(float64(n)*float64(b.N)/b.Elapsed().Seconds(), "points/s")
		})
	}
}
