package zabbixreceiver

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel/metric"
)

type receiverTelemetry struct {
	streamingRequests   metric.Int64Counter
	attrs               metric.MeasurementOption
	discoveredHosts     metric.Int64Gauge
	discoveredItems     metric.Int64Gauge
	discoverLastSuccess metric.Float64Gauge
	valuesLastSuccess   metric.Float64Gauge
	discoverAttempts    metric.Int64Counter
	discoverErrors      metric.Int64Counter
	discoverDuration    metric.Float64Histogram
	valuesAttempts      metric.Int64Counter
	valuesErrors        metric.Int64Counter
	valuesDuration      metric.Float64Histogram
	emittedPoints       metric.Int64Counter
	invalidValues       metric.Int64Counter
	filteredItems       metric.Int64Counter
	limitedItems        metric.Int64Counter
}

func newTelemetry(meter metric.Meter) (*receiverTelemetry, error) {
	telemetry := &receiverTelemetry{attrs: metric.WithAttributes()}
	var err error

	telemetry.discoverAttempts, err = meter.Int64Counter(
		"otelcol_receiver_zabbix_discover_attempts",
		metric.WithDescription("Number of Zabbix discovery cycles attempted."),
	)
	if err != nil {
		return nil, fmt.Errorf("create discover attempts counter: %w", err)
	}
	telemetry.discoverErrors, err = meter.Int64Counter(
		"otelcol_receiver_zabbix_discover_errors",
		metric.WithDescription("Number of Zabbix discovery cycles that returned an error."),
	)
	if err != nil {
		return nil, fmt.Errorf("create discover errors counter: %w", err)
	}
	telemetry.discoverDuration, err = meter.Float64Histogram(
		"otelcol_receiver_zabbix_discover_duration",
		metric.WithDescription("Duration of Zabbix discovery cycles."),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(0.05, 0.1, 0.5, 1, 2.5, 5, 10, 30, 60),
	)
	if err != nil {
		return nil, fmt.Errorf("create discover duration histogram: %w", err)
	}
	telemetry.valuesAttempts, err = meter.Int64Counter(
		"otelcol_receiver_zabbix_values_attempts",
		metric.WithDescription("Number of Zabbix value collection cycles attempted."),
	)
	if err != nil {
		return nil, fmt.Errorf("create values attempts counter: %w", err)
	}
	telemetry.valuesErrors, err = meter.Int64Counter(
		"otelcol_receiver_zabbix_values_errors",
		metric.WithDescription("Number of Zabbix value collection cycles that returned an error."),
	)
	if err != nil {
		return nil, fmt.Errorf("create values errors counter: %w", err)
	}
	telemetry.valuesDuration, err = meter.Float64Histogram(
		"otelcol_receiver_zabbix_values_duration",
		metric.WithDescription("Duration of Zabbix value collection cycles."),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(0.05, 0.1, 0.5, 1, 2.5, 5, 10, 30, 60),
	)
	if err != nil {
		return nil, fmt.Errorf("create values duration histogram: %w", err)
	}
	telemetry.emittedPoints, err = meter.Int64Counter(
		"otelcol_receiver_zabbix_emitted_points",
		metric.WithDescription("Number of metric points successfully handed to the next consumer (not backend delivery)."),
	)
	if err != nil {
		return nil, fmt.Errorf("create emitted points counter: %w", err)
	}
	telemetry.invalidValues, err = meter.Int64Counter(
		"otelcol_receiver_zabbix_invalid_values",
		metric.WithDescription("Number of invalid Zabbix values skipped during conversion."),
	)
	if err != nil {
		return nil, fmt.Errorf("create invalid values counter: %w", err)
	}
	telemetry.filteredItems, err = meter.Int64Counter(
		"otelcol_receiver_zabbix_filtered_items",
		metric.WithDescription("Number of Zabbix items excluded by discovery filters."),
	)
	if err != nil {
		return nil, fmt.Errorf("create filtered items counter: %w", err)
	}
	telemetry.limitedItems, err = meter.Int64Counter(
		"otelcol_receiver_zabbix_limited_items",
		metric.WithDescription("Number of Zabbix items excluded by per-host limits."),
	)
	if err != nil {
		return nil, fmt.Errorf("create limited items counter: %w", err)
	}

	telemetry.streamingRequests, err = meter.Int64Counter("otelcol_receiver_zabbix_streaming_requests", metric.WithDescription("Streaming requests completed, by HTTP response status code."))
	if err != nil {
		return nil, fmt.Errorf("create streaming requests counter: %w", err)
	}
	telemetry.discoveredHosts, err = meter.Int64Gauge("otelcol_receiver_zabbix_discovered_hosts", metric.WithDescription("Hosts with selected items in the last successful discovery snapshot."))
	if err != nil {
		return nil, fmt.Errorf("create discovered hosts gauge: %w", err)
	}
	telemetry.discoveredItems, err = meter.Int64Gauge("otelcol_receiver_zabbix_discovered_items", metric.WithDescription("Items in the last successful discovery snapshot."))
	if err != nil {
		return nil, fmt.Errorf("create discovered items gauge: %w", err)
	}
	telemetry.discoverLastSuccess, err = meter.Float64Gauge("otelcol_receiver_zabbix_discover_last_success_timestamp", metric.WithUnit("s"), metric.WithDescription("Unix timestamp of last successful discovery, or zero before success."))
	if err != nil {
		return nil, fmt.Errorf("create discovery last success gauge: %w", err)
	}
	telemetry.valuesLastSuccess, err = meter.Float64Gauge("otelcol_receiver_zabbix_values_last_success_timestamp", metric.WithUnit("s"), metric.WithDescription("Unix timestamp of last successful value collection or streaming request; zero before success. Skipped API cycles do not update it."))
	if err != nil {
		return nil, fmt.Errorf("create values last success gauge: %w", err)
	}
	return telemetry, nil
}

// Initialize only applicable instruments so idle receivers are visible to scrapers.
func (t *receiverTelemetry) initialize(ctx context.Context, mode string) {
	for _, c := range []metric.Int64Counter{t.valuesAttempts, t.valuesErrors, t.emittedPoints, t.invalidValues} {
		c.Add(ctx, 0, t.attrs)
	}
	t.valuesLastSuccess.Record(ctx, 0, t.attrs)
	if mode != "streaming" {
		for _, c := range []metric.Int64Counter{t.discoverAttempts, t.discoverErrors, t.filteredItems, t.limitedItems} {
			c.Add(ctx, 0, t.attrs)
		}
		t.discoveredHosts.Record(ctx, 0, t.attrs)
		t.discoveredItems.Record(ctx, 0, t.attrs)
		t.discoverLastSuccess.Record(ctx, 0, t.attrs)
	}
}
