package metrics

import (
	"math"
	"strconv"

	"github.com/wieso/zabbixreceiver/receiver/zabbixreceiver/internal/discovery"
	"github.com/wieso/zabbixreceiver/receiver/zabbixreceiver/internal/zabbix"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
)

// Config controls the OpenTelemetry metric representation of Zabbix items.
type Config struct {
	Prefix          string
	ConstLabels     map[string]string
	ScopeName       string
	MetadataEnabled bool
}

// Stats reports how many Zabbix values were converted, malformed, or unmatched.
type Stats struct {
	Emitted int64
	Invalid int64
	Missing int64
}

// Build converts matched Zabbix values to gauge metrics.
func Build(items []discovery.ItemMeta, values []zabbix.Value, config Config) (pmetric.Metrics, Stats) {
	metrics := pmetric.NewMetrics()
	scopeMetrics := metrics.ResourceMetrics().AppendEmpty().ScopeMetrics().AppendEmpty()
	scopeMetrics.Scope().SetName(config.ScopeName)

	itemsByID := make(map[string]discovery.ItemMeta, len(items))
	for _, item := range items {
		itemsByID[item.ID] = item
	}

	var stats Stats
	for _, value := range values {
		item, ok := itemsByID[value.ItemID]
		if !ok {
			stats.Missing++
			continue
		}

		number, err := strconv.ParseFloat(value.LastValue, 64)
		if err != nil {
			stats.Invalid++
			continue
		}
		lastClock, err := strconv.ParseInt(value.LastClock, 10, 64)
		if err != nil || lastClock < 0 || lastClock > math.MaxInt64/1_000_000_000 {
			stats.Invalid++
			continue
		}
		name, err := Name(config.Prefix, item.Key)
		if err != nil {
			stats.Invalid++
			continue
		}

		metric := scopeMetrics.Metrics().AppendEmpty()
		metric.SetName(name)
		metric.SetDescription(item.Name)
		point := metric.SetEmptyGauge().DataPoints().AppendEmpty()
		point.SetDoubleValue(number)
		point.SetTimestamp(pcommon.Timestamp(lastClock * 1_000_000_000))
		for key, label := range config.ConstLabels {
			point.Attributes().PutStr(key, label)
		}
		point.Attributes().PutStr("host", item.Host)
		point.Attributes().PutStr("hostid", item.HostID)
		point.Attributes().PutStr("item_key", item.Key)
		point.Attributes().PutStr("itemid", item.ID)
		if config.MetadataEnabled {
			PutMetadata(point.Attributes(), item.Metadata)
		}
		stats.Emitted++
	}
	return metrics, stats
}
