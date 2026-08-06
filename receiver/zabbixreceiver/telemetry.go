package zabbixreceiver

import (
	"fmt"

	"go.opentelemetry.io/otel/metric"
)

type receiverTelemetry struct {
	discoverAttempts metric.Int64Counter
	discoverErrors   metric.Int64Counter
	discoverDuration metric.Float64Histogram
	valuesAttempts   metric.Int64Counter
	valuesErrors     metric.Int64Counter
	valuesDuration   metric.Float64Histogram
	emittedPoints    metric.Int64Counter
	invalidValues    metric.Int64Counter
	filteredItems    metric.Int64Counter
	limitedItems     metric.Int64Counter
}

func newTelemetry(meter metric.Meter) (*receiverTelemetry, error) {
	telemetry := &receiverTelemetry{}
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
	)
	if err != nil {
		return nil, fmt.Errorf("create values duration histogram: %w", err)
	}
	telemetry.emittedPoints, err = meter.Int64Counter(
		"otelcol_receiver_zabbix_emitted_points",
		metric.WithDescription("Number of metric points emitted from Zabbix values."),
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

	return telemetry, nil
}
