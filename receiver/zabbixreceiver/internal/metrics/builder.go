package metrics

import (
	"fmt"
	"math"
	"strconv"

	"github.com/wieso/zabbixreceiver/receiver/zabbixreceiver/internal/discovery"
	"github.com/wieso/zabbixreceiver/receiver/zabbixreceiver/internal/zabbix"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
)

// Config controls the OpenTelemetry metric representation of Zabbix items.
type Config struct {
	Prefix           string
	ConstLabels      map[string]string
	ScopeName        string
	MetadataEnabled  bool
	HostGroupsFormat string
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

		clock, err := strconv.ParseInt(value.LastClock, 10, 64)
		if err != nil {
			stats.Invalid++
			continue
		}
		number, timestamp, err := ParseSample(value.LastValue, clock, 0)
		if err != nil || !AppendGauge(scopeMetrics.Metrics(), item, number, timestamp, config) {
			stats.Invalid++
			continue
		}
		stats.Emitted++
	}
	return metrics, stats
}

// ParseSample validates numeric samples in both polling and streaming modes.
func ParseSample(value string, clock, ns int64) (float64, pcommon.Timestamp, error) {
	number, err := strconv.ParseFloat(value, 64)
	if err != nil || math.IsNaN(number) || math.IsInf(number, 0) {
		return 0, 0, fmt.Errorf("invalid numeric value")
	}
	if clock < 0 || ns < 0 || clock > (math.MaxInt64-ns)/1_000_000_000 {
		return 0, 0, fmt.Errorf("invalid timestamp")
	}
	return number, pcommon.Timestamp(clock*1_000_000_000 + ns), nil
}

// AppendGauge uses the item key when available and otherwise the display name.
// Streaming without API enrichment cannot supply hostid or item_key.
func AppendGauge(metrics pmetric.MetricSlice, item discovery.ItemMeta, number float64, timestamp pcommon.Timestamp, config Config) bool {
	source := item.Key
	if source == "" {
		source = item.Name
	}
	name, err := Name(config.Prefix, source)
	if err != nil {
		return false
	}
	m := metrics.AppendEmpty()
	m.SetName(name)
	m.SetDescription(item.Name)
	point := m.SetEmptyGauge().DataPoints().AppendEmpty()
	point.SetDoubleValue(number)
	point.SetTimestamp(timestamp)
	for key, value := range config.ConstLabels {
		point.Attributes().PutStr(key, value)
	}
	point.Attributes().PutStr("host", item.Host)
	point.Attributes().PutStr("itemid", item.ID)
	if item.HostID != "" {
		point.Attributes().PutStr("hostid", item.HostID)
	}
	if item.Key != "" {
		point.Attributes().PutStr("item_key", item.Key)
	}
	if config.MetadataEnabled {
		PutMetadata(point.Attributes(), item.Metadata, config.HostGroupsFormat)
	}
	return true
}
