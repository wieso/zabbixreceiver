package zabbixreceiver

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/wieso/zabbixreceiver/receiver/zabbixreceiver/internal/discovery"
	"github.com/wieso/zabbixreceiver/receiver/zabbixreceiver/internal/zabbix"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/receiver/receivertest"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestObservabilityDelivery(t *testing.T) {
	for _, mode := range []string{"api", "streaming"} {
		t.Run(mode, func(t *testing.T) {
			reader := sdkmetric.NewManualReader()
			provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
			t.Cleanup(func() { require.NoError(t, provider.Shutdown(context.Background())) })
			settings := receivertest.NewNopSettings(componentType)
			settings.ID = component.NewIDWithName(componentType, mode)
			settings.MeterProvider = provider
			core, logs := observer.New(zap.DebugLevel)
			settings.Logger = zap.New(core)
			cfg := validConfig()
			cfg.Mode = mode
			created, err := createMetricsReceiver(context.Background(), settings, cfg, newRecordingConsumer(t).metrics)
			require.NoError(t, err)
			r := created.(*zabbixReceiver)
			next := newRecordingConsumer(t)
			r.next = next.metrics
			r.api = &fakeAPI{values: func(context.Context, []string) ([]zabbix.Value, error) {
				return []zabbix.Value{{ItemID: "1", LastValue: "1", LastClock: "1700000000"}}, nil
			}}
			r.store.Replace(discovery.NewSnapshot(testItems(1)))
			run := func() {
				if mode == "api" {
					_ = r.values(context.Background())
					return
				}
				req := httptest.NewRequest("POST", "/v1/history", strings.NewReader(`{"host":{"host":"h"},"name":"cpu","itemid":1,"clock":1700000000,"value":1,"type":0}`))
				req.Header.Set("Content-Type", "application/x-ndjson")
				r.handleHistory(httptest.NewRecorder(), req)
			}
			run()
			next.err = errors.New("downstream unavailable")
			run()
			got := collectTelemetry(t, reader)
			require.EqualValues(t, 1, sumValue(t, got, "otelcol_receiver_zabbix_emitted_points"))
			require.EqualValues(t, 1, sumValue(t, got, "otelcol_receiver_accepted_metric_points"))
			require.EqualValues(t, 1, sumValue(t, got, "otelcol_receiver_refused_metric_points"))
			for _, scope := range got.ScopeMetrics {
				for _, m := range scope.Metrics {
					if sum, ok := m.Data.(metricdata.Sum[int64]); ok {
						for _, p := range sum.DataPoints {
							v, ok := p.Attributes.Value(attribute.Key("receiver"))
							require.True(t, ok, m.Name)
							require.Equal(t, "zabbix/"+mode, v.AsString())
						}
					}
				}
			}
			require.NotEmpty(t, logs.All())
		})
	}
}

func TestObservabilityLifecycleLogs(t *testing.T) {
	cfg := validConfig()
	cfg.Schedule.Jobs.Discover.Enabled = false
	cfg.Schedule.Jobs.Values.Enabled = false
	r := newTestReceiver(t, cfg, &fakeAPI{}, newRecordingConsumer(t))
	core, logs := observer.New(zap.InfoLevel)
	r.logger = zap.New(core)
	require.NoError(t, r.Start(context.Background(), nil))
	require.NoError(t, r.Shutdown(context.Background()))
	require.Equal(t, 1, logs.FilterMessage("Zabbix receiver started").Len())
	require.Equal(t, 1, logs.FilterMessage("Zabbix receiver stopping").Len())
}

func TestDiscoveryTelemetryRetainsLastSuccessfulSnapshot(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { require.NoError(t, provider.Shutdown(context.Background())) })
	telemetry, err := newTelemetry(provider.Meter("test"))
	require.NoError(t, err)
	api := &fakeAPI{
		hosts: func(context.Context) ([]zabbix.Host, error) { return []zabbix.Host{{ID: "10", Name: "a"}}, nil },
		items: func(context.Context, []string) ([]zabbix.Item, error) {
			return []zabbix.Item{{ID: "1", HostID: "10", Key: "cpu"}}, nil
		},
	}
	r, err := newReceiver(receivertest.NewNopSettings(componentType), validConfig(), newRecordingConsumer(t).metrics, api, telemetry)
	require.NoError(t, err)
	require.NoError(t, r.discover(context.Background()))
	first := collectTelemetry(t, reader)
	api.hosts = func(context.Context) ([]zabbix.Host, error) { return nil, errors.New("offline") }
	require.Error(t, r.discover(context.Background()))
	second := collectTelemetry(t, reader)
	for _, got := range []metricdata.ResourceMetrics{first, second} {
		for _, scope := range got.ScopeMetrics {
			for _, m := range scope.Metrics {
				switch m.Name {
				case "otelcol_receiver_zabbix_discovered_hosts", "otelcol_receiver_zabbix_discovered_items":
					require.EqualValues(t, 1, m.Data.(metricdata.Gauge[int64]).DataPoints[0].Value)
				case "otelcol_receiver_zabbix_discover_last_success_timestamp":
					require.Positive(t, m.Data.(metricdata.Gauge[float64]).DataPoints[0].Value)
				}
			}
		}
	}
	// A successful empty snapshot must clear the gauges, unlike a failed refresh.
	api.hosts = func(context.Context) ([]zabbix.Host, error) { return nil, nil }
	api.items = func(context.Context, []string) ([]zabbix.Item, error) { return nil, nil }
	require.NoError(t, r.discover(context.Background()))
	for _, scope := range collectTelemetry(t, reader).ScopeMetrics {
		for _, m := range scope.Metrics {
			if m.Name == "otelcol_receiver_zabbix_discovered_items" {
				require.Zero(t, m.Data.(metricdata.Gauge[int64]).DataPoints[0].Value)
			}
		}
	}
}
