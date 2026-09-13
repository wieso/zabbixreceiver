package metrics

import (
	"testing"

	"github.com/wieso/zabbixreceiver/receiver/zabbixreceiver/internal/discovery"
	"github.com/wieso/zabbixreceiver/receiver/zabbixreceiver/internal/zabbix"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
)

func TestBuildConvertsMatchedNumericValuesToGauges(t *testing.T) {
	items := []discovery.ItemMeta{
		{ID: "1", HostID: "10", Host: "prod-a", Name: "CPU utilization", Key: "system.cpu.util"},
		{ID: "2", HostID: "10", Host: "prod-a", Name: "Free bytes", Key: "vfs.fs.size[/,free]"},
		{ID: "3", HostID: "11", Host: "prod-b", Name: "Broken", Key: "broken.value"},
	}
	values := []zabbix.Value{
		{ItemID: "1", LastValue: "12.5", LastClock: "1700000000"},
		{ItemID: "2", LastValue: "18446744073709551615", LastClock: "1700000000"},
		{ItemID: "3", LastValue: "not-a-number", LastClock: "1700000000"},
		{ItemID: "missing", LastValue: "1", LastClock: "1700000000"},
	}

	got, stats := Build(items, values, Config{
		Prefix:      "zabbix_",
		ConstLabels: map[string]string{"env": "production", "host": "configured-host"},
		ScopeName:   "zabbixreceiver",
	})

	resourceMetrics := got.ResourceMetrics()
	if resourceMetrics.Len() != 1 {
		t.Fatalf("resource metrics = %d, want 1", resourceMetrics.Len())
	}
	scopeMetrics := resourceMetrics.At(0).ScopeMetrics()
	if scopeMetrics.Len() != 1 {
		t.Fatalf("scope metrics = %d, want 1", scopeMetrics.Len())
	}
	if scopeMetrics.At(0).Scope().Name() != "zabbixreceiver" {
		t.Fatalf("scope name = %q, want %q", scopeMetrics.At(0).Scope().Name(), "zabbixreceiver")
	}

	metrics := scopeMetrics.At(0).Metrics()
	if metrics.Len() != 2 {
		t.Fatalf("metrics = %d, want 2", metrics.Len())
	}
	assertGauge(t, metrics.At(0), "zabbix_system_cpu_util", "CPU utilization", 12.5, "prod-a")
	assertGauge(t, metrics.At(1), "zabbix_vfs_fs_size_free", "Free bytes", 18446744073709551615.0, "prod-a")

	if stats != (Stats{Emitted: 2, Invalid: 1, Missing: 1}) {
		t.Fatalf("stats = %#v, want %#v", stats, Stats{Emitted: 2, Invalid: 1, Missing: 1})
	}
}

func TestBuildRejectsNegativeAndOverflowingClocks(t *testing.T) {
	items := []discovery.ItemMeta{{ID: "1", HostID: "10", Host: "prod-a", Name: "CPU", Key: "system.cpu.util"}}
	values := []zabbix.Value{
		{ItemID: "1", LastValue: "1", LastClock: "1700000000"},
		{ItemID: "1", LastValue: "2", LastClock: "-1"},
		{ItemID: "1", LastValue: "3", LastClock: "9223372037"},
	}

	got, stats := Build(items, values, Config{})

	if got.ResourceMetrics().At(0).ScopeMetrics().At(0).Metrics().Len() != 1 {
		t.Fatalf("metrics = %d, want 1", got.ResourceMetrics().At(0).ScopeMetrics().At(0).Metrics().Len())
	}
	if stats != (Stats{Emitted: 1, Invalid: 2}) {
		t.Fatalf("stats = %#v, want %#v", stats, Stats{Emitted: 1, Invalid: 2})
	}
}

func TestBuildEmitsOneMetricForEachDuplicateValue(t *testing.T) {
	items := []discovery.ItemMeta{{ID: "1", HostID: "10", Host: "prod-a", Name: "CPU", Key: "system.cpu.util"}}
	values := []zabbix.Value{
		{ItemID: "1", LastValue: "1", LastClock: "1700000000"},
		{ItemID: "1", LastValue: "2", LastClock: "1700000001"},
	}

	got, stats := Build(items, values, Config{})

	metrics := got.ResourceMetrics().At(0).ScopeMetrics().At(0).Metrics()
	if metrics.Len() != 2 {
		t.Fatalf("metrics = %d, want 2", metrics.Len())
	}
	if stats != (Stats{Emitted: 2}) {
		t.Fatalf("stats = %#v, want %#v", stats, Stats{Emitted: 2})
	}
}

func assertGauge(t *testing.T, metric pmetric.Metric, wantName, wantDescription string, wantValue float64, wantHost string) {
	t.Helper()
	if metric.Name() != wantName {
		t.Errorf("metric name = %q, want %q", metric.Name(), wantName)
	}
	if metric.Description() != wantDescription {
		t.Errorf("metric description = %q, want %q", metric.Description(), wantDescription)
	}
	if metric.Type() != pmetric.MetricTypeGauge {
		t.Fatalf("metric type = %s, want gauge", metric.Type())
	}

	points := metric.Gauge().DataPoints()
	if points.Len() != 1 {
		t.Fatalf("gauge data points = %d, want 1", points.Len())
	}
	point := points.At(0)
	if point.ValueType() != pmetric.NumberDataPointValueTypeDouble {
		t.Fatalf("data point value type = %s, want double", point.ValueType())
	}
	if point.DoubleValue() != wantValue {
		t.Errorf("data point value = %v, want %v", point.DoubleValue(), wantValue)
	}
	if point.Timestamp() != pcommon.Timestamp(1700000000*1_000_000_000) {
		t.Errorf("data point timestamp = %d, want %d", point.Timestamp(), pcommon.Timestamp(1700000000*1_000_000_000))
	}

	attributes := point.Attributes()
	assertStringAttribute(t, attributes, "env", "production")
	assertStringAttribute(t, attributes, "host", wantHost)
	assertStringAttribute(t, attributes, "hostid", "10")
	assertStringAttribute(t, attributes, "item_key", map[string]string{
		"zabbix_system_cpu_util":  "system.cpu.util",
		"zabbix_vfs_fs_size_free": "vfs.fs.size[/,free]",
	}[wantName])
	assertStringAttribute(t, attributes, "itemid", map[string]string{
		"zabbix_system_cpu_util":  "1",
		"zabbix_vfs_fs_size_free": "2",
	}[wantName])
}

func assertStringAttribute(t *testing.T, attributes pcommon.Map, key, want string) {
	t.Helper()
	got, ok := attributes.Get(key)
	if !ok {
		t.Fatalf("attribute %q is missing", key)
	}
	if got.Str() != want {
		t.Errorf("attribute %q = %q, want %q", key, got.Str(), want)
	}
}

func TestBuildRejectsNonFiniteValues(t *testing.T) {
	items := []discovery.ItemMeta{{ID: "1", Key: "cpu"}}
	values := []zabbix.Value{{ItemID: "1", LastValue: "NaN", LastClock: "1"}, {ItemID: "1", LastValue: "+Inf", LastClock: "1"}, {ItemID: "1", LastValue: "-Inf", LastClock: "1"}}
	got, stats := Build(items, values, Config{})
	if stats.Invalid != 3 || got.DataPointCount() != 0 {
		t.Fatalf("non-finite points accepted: %+v", stats)
	}
}
