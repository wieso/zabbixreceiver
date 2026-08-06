package main

import (
	"github.com/open-telemetry/opentelemetry-collector-contrib/exporter/prometheusremotewriteexporter"
	"github.com/open-telemetry/opentelemetry-collector-contrib/extension/healthcheckextension"
	"github.com/wieso/zabbixreceiver/receiver/zabbixreceiver"
	"go.opentelemetry.io/collector/otelcol"
	"go.opentelemetry.io/collector/processor"
	"go.opentelemetry.io/collector/processor/batchprocessor"
	"go.opentelemetry.io/collector/processor/memorylimiterprocessor"
	"go.opentelemetry.io/collector/service/telemetry/otelconftelemetry"
)

func components() (otelcol.Factories, error) {
	receivers, err := otelcol.MakeFactoryMap(zabbixreceiver.NewFactory())
	if err != nil {
		return otelcol.Factories{}, err
	}

	processors, err := otelcol.MakeFactoryMap(
		batchprocessor.NewFactory(),
		processor.Factory(memorylimiterprocessor.NewFactory()),
	)
	if err != nil {
		return otelcol.Factories{}, err
	}

	exporters, err := otelcol.MakeFactoryMap(prometheusremotewriteexporter.NewFactory())
	if err != nil {
		return otelcol.Factories{}, err
	}

	extensions, err := otelcol.MakeFactoryMap(healthcheckextension.NewFactory())
	if err != nil {
		return otelcol.Factories{}, err
	}

	return otelcol.Factories{
		Receivers:  receivers,
		Processors: processors,
		Exporters:  exporters,
		Extensions: extensions,
		Telemetry:  otelconftelemetry.NewFactory(),
	}, nil
}
