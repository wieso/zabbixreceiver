package discovery

import (
	"reflect"
	"regexp"
	"testing"

	"github.com/wieso/zabbixreceiver/receiver/zabbixreceiver/internal/zabbix"
)

func TestSelectAppliesIncludesBeforeExcludes(t *testing.T) {
	hosts := []zabbix.Host{{ID: "1", Name: "prod-a"}, {ID: "2", Name: "prod-a-backup"}, {ID: "3", Name: "dev-a"}}
	items := []zabbix.Item{
		{ID: "1", HostID: "1", Name: "CPU", Key: "system.cpu.util", ValueType: "0"},
		{ID: "2", HostID: "1", Name: "Memory", Key: "vm.memory.size", ValueType: "3"},
		{ID: "3", HostID: "1", Name: "Log", Key: "system.log", ValueType: "0"},
		{ID: "4", HostID: "2", Name: "CPU", Key: "system.cpu.util", ValueType: "0"},
		{ID: "5", HostID: "3", Name: "CPU", Key: "system.cpu.util", ValueType: "0"},
	}
	filters := Filters{
		HostInclude:    regexp.MustCompile(`prod-.*`),
		HostExclude:    regexp.MustCompile(`.*-backup`),
		ItemKeyInclude: regexp.MustCompile(`system\..*|vm\..*`),
		ItemKeyExclude: regexp.MustCompile(`.*\.log`),
	}

	selected, filtered, limited := Select(hosts, items, filters, 0)

	want := []ItemMeta{
		{ID: "1", HostID: "1", Host: "prod-a", Name: "CPU", Key: "system.cpu.util", ValueType: "0"},
		{ID: "2", HostID: "1", Host: "prod-a", Name: "Memory", Key: "vm.memory.size", ValueType: "3"},
	}
	if !equalItemMetas(selected, want) {
		t.Fatalf("selected = %#v, want %#v", selected, want)
	}
	if filtered != 3 {
		t.Fatalf("filtered = %d, want 3", filtered)
	}
	if limited != 0 {
		t.Fatalf("limited = %d, want 0", limited)
	}
}

func TestSelectLimitsEachHostInInputOrder(t *testing.T) {
	hosts := []zabbix.Host{{ID: "1", Name: "prod-a"}, {ID: "2", Name: "prod-b"}}
	items := []zabbix.Item{
		{ID: "2", HostID: "1", Name: "second", Key: "k.second", ValueType: "0"},
		{ID: "1", HostID: "1", Name: "first", Key: "k.first", ValueType: "0"},
		{ID: "3", HostID: "2", Name: "third", Key: "k.third", ValueType: "0"},
		{ID: "4", HostID: "2", Name: "fourth", Key: "k.fourth", ValueType: "0"},
	}

	selected, filtered, limited := Select(hosts, items, Filters{}, 1)

	want := []ItemMeta{
		{ID: "2", HostID: "1", Host: "prod-a", Name: "second", Key: "k.second", ValueType: "0"},
		{ID: "3", HostID: "2", Host: "prod-b", Name: "third", Key: "k.third", ValueType: "0"},
	}
	if !equalItemMetas(selected, want) {
		t.Fatalf("selected = %#v, want %#v", selected, want)
	}
	if filtered != 0 {
		t.Fatalf("filtered = %d, want 0", filtered)
	}
	if limited != 2 {
		t.Fatalf("limited = %d, want 2", limited)
	}
}

func TestSelectFiltersUnknownHosts(t *testing.T) {
	items := []zabbix.Item{{ID: "1", HostID: "missing", Name: "CPU", Key: "system.cpu.util", ValueType: "0"}}

	selected, filtered, limited := Select(nil, items, Filters{}, 0)

	if selected != nil {
		t.Fatalf("selected = %#v, want nil", selected)
	}
	if filtered != 1 {
		t.Fatalf("filtered = %d, want 1", filtered)
	}
	if limited != 0 {
		t.Fatalf("limited = %d, want 0", limited)
	}
}

func equalItemMetas(got, want []ItemMeta) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		want[i].Metadata.ItemName = want[i].Name
		want[i].Metadata.ValueType = want[i].ValueType
		if !reflect.DeepEqual(got[i], want[i]) {
			return false
		}
	}
	return true
}
