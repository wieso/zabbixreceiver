package zabbixreceiver

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"github.com/wieso/zabbixreceiver/receiver/zabbixreceiver/internal/zabbix"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/receiver"
	"go.opentelemetry.io/otel/metric/noop"
)

var componentType = component.MustNewType("zabbix")

func NewFactory() receiver.Factory {
	return receiver.NewFactory(
		componentType,
		createDefaultConfig,
		receiver.WithMetrics(createMetricsReceiver, component.StabilityLevelDevelopment),
	)
}

func createMetricsReceiver(
	_ context.Context,
	settings receiver.Settings,
	baseConfig component.Config,
	next consumer.Metrics,
) (receiver.Metrics, error) {
	config, ok := baseConfig.(*Config)
	if !ok || config == nil {
		return nil, fmt.Errorf("expected *zabbixreceiver.Config, got %T", baseConfig)
	}

	resolved := config.Clone()
	if err := resolved.ResolveEnv(os.LookupEnv); err != nil {
		return nil, fmt.Errorf("resolve Zabbix receiver environment: %w", err)
	}
	if err := resolved.validateResolved(); err != nil {
		return nil, fmt.Errorf("validate Zabbix receiver config: %w", err)
	}

	var api zabbix.API
	if resolved.Mode == "api" || resolved.Streaming.EnrichWithAPI {
		httpClient := &http.Client{Timeout: resolved.Zabbix.Timeout}
		client, err := zabbix.NewClient(zabbix.ClientConfig{
			URL:               resolved.Zabbix.URL,
			Token:             string(resolved.Zabbix.Token),
			Timeout:           resolved.Zabbix.Timeout,
			MetadataEnabled:   resolved.Metadata.Enabled,
			InheritedHostTags: resolved.Metadata.InheritedHostTags,
			InventoryFields:   resolved.Metadata.InventoryFields,
		}, httpClient)
		if err != nil {
			return nil, fmt.Errorf("create Zabbix client: %w", err)
		}

		api = client
	}
	meterProvider := settings.MeterProvider
	if meterProvider == nil {
		meterProvider = noop.NewMeterProvider()
	}
	telemetry, err := newTelemetry(meterProvider.Meter("github.com/wieso/zabbixreceiver/receiver/zabbixreceiver"))
	if err != nil {
		return nil, fmt.Errorf("create Zabbix receiver telemetry: %w", err)
	}

	return newReceiver(settings, resolved, next, api, telemetry)
}
