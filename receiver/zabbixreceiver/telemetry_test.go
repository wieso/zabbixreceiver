package zabbixreceiver

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wieso/zabbixreceiver/receiver/zabbixreceiver/internal/discovery"
	"github.com/wieso/zabbixreceiver/receiver/zabbixreceiver/internal/zabbix"
	"go.opentelemetry.io/collector/receiver/receivertest"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func TestTelemetryRecordsCountersAndDuration(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { require.NoError(t, provider.Shutdown(context.Background())) })
	telemetry, err := newTelemetry(provider.Meter("zabbixreceiver-test"))
	require.NoError(t, err)

	ctx := context.Background()
	telemetry.discoverAttempts.Add(ctx, 1)
	telemetry.emittedPoints.Add(ctx, 3)
	telemetry.discoverDuration.Record(ctx, 0.25)

	metrics := collectTelemetry(t, reader)
	assert.Equal(t, int64(1), sumValue(t, metrics, "otelcol_receiver_zabbix_discover_attempts"))
	assert.Equal(t, int64(3), sumValue(t, metrics, "otelcol_receiver_zabbix_emitted_points"))
	assert.Equal(t, uint64(1), histogramCount(t, metrics, "otelcol_receiver_zabbix_discover_duration"))
}

func TestReceiverRecordsCycleSelectionAndConversionTelemetry(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { require.NoError(t, provider.Shutdown(context.Background())) })
	telemetry, err := newTelemetry(provider.Meter("zabbixreceiver-test"))
	require.NoError(t, err)

	cfg := validConfig()
	cfg.Zabbix.Limits.MaxMetricsPerHost = 1
	cfg.Zabbix.Filters.ItemKeyExcludeRegex = `^skip`
	api := &fakeAPI{
		hosts: func(context.Context) ([]zabbix.Host, error) {
			return []zabbix.Host{{ID: "10", Name: "prod-a"}}, nil
		},
		items: func(context.Context, []string) ([]zabbix.Item, error) {
			return []zabbix.Item{
				{ID: "1", HostID: "10", Key: "keep.first"},
				{ID: "2", HostID: "10", Key: "keep.limited"},
				{ID: "3", HostID: "10", Key: "skip.filtered"},
			}, nil
		},
		values: func(context.Context, []string) ([]zabbix.Value, error) {
			return []zabbix.Value{
				{ItemID: "1", LastValue: "12.5", LastClock: "1700000000"},
				{ItemID: "2", LastValue: "invalid", LastClock: "1700000000"},
			}, nil
		},
	}
	settings := receivertest.NewNopSettings(componentType)
	settings.MeterProvider = provider
	receiver, err := newReceiver(settings, cfg, newRecordingConsumer(t).metrics, api, telemetry)
	require.NoError(t, err)

	require.NoError(t, receiver.discover(context.Background()))
	api.hosts = func(context.Context) ([]zabbix.Host, error) { return nil, errors.New("discover failed") }
	require.Error(t, receiver.discover(context.Background()))

	receiver.store.Replace(discovery.NewSnapshot(testItems(2)))
	require.NoError(t, receiver.values(context.Background()))
	api.values = func(context.Context, []string) ([]zabbix.Value, error) { return nil, errors.New("values failed") }
	require.Error(t, receiver.values(context.Background()))

	metrics := collectTelemetry(t, reader)
	assert.Equal(t, int64(2), sumValue(t, metrics, "otelcol_receiver_zabbix_discover_attempts"))
	assert.Equal(t, int64(1), sumValue(t, metrics, "otelcol_receiver_zabbix_discover_errors"))
	assert.Equal(t, uint64(2), histogramCount(t, metrics, "otelcol_receiver_zabbix_discover_duration"))
	assert.Equal(t, int64(2), sumValue(t, metrics, "otelcol_receiver_zabbix_values_attempts"))
	assert.Equal(t, int64(1), sumValue(t, metrics, "otelcol_receiver_zabbix_values_errors"))
	assert.Equal(t, uint64(2), histogramCount(t, metrics, "otelcol_receiver_zabbix_values_duration"))
	assert.Equal(t, int64(1), sumValue(t, metrics, "otelcol_receiver_zabbix_emitted_points"))
	assert.Equal(t, int64(1), sumValue(t, metrics, "otelcol_receiver_zabbix_invalid_values"))
	assert.Equal(t, int64(1), sumValue(t, metrics, "otelcol_receiver_zabbix_filtered_items"))
	assert.Equal(t, int64(1), sumValue(t, metrics, "otelcol_receiver_zabbix_limited_items"))
}

func collectTelemetry(t *testing.T, reader *sdkmetric.ManualReader) metricdata.ResourceMetrics {
	t.Helper()
	var metrics metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &metrics))
	return metrics
}

func sumValue(t *testing.T, resourceMetrics metricdata.ResourceMetrics, name string) int64 {
	t.Helper()
	for _, scopeMetrics := range resourceMetrics.ScopeMetrics {
		for _, metric := range scopeMetrics.Metrics {
			if metric.Name != name {
				continue
			}
			sum, ok := metric.Data.(metricdata.Sum[int64])
			require.True(t, ok, "%s has data type %T", name, metric.Data)
			require.Len(t, sum.DataPoints, 1)
			return sum.DataPoints[0].Value
		}
	}
	t.Fatalf("metric %q was not collected", name)
	return 0
}

func histogramCount(t *testing.T, resourceMetrics metricdata.ResourceMetrics, name string) uint64 {
	t.Helper()
	for _, scopeMetrics := range resourceMetrics.ScopeMetrics {
		for _, metric := range scopeMetrics.Metrics {
			if metric.Name != name {
				continue
			}
			histogram, ok := metric.Data.(metricdata.Histogram[float64])
			require.True(t, ok, "%s has data type %T", name, metric.Data)
			require.Len(t, histogram.DataPoints, 1)
			return histogram.DataPoints[0].Count
		}
	}
	t.Fatalf("metric %q was not collected", name)
	return 0
}
