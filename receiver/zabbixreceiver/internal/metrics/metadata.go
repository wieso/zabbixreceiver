package metrics

import (
	"slices"
	"strings"

	"github.com/wieso/zabbixreceiver/receiver/zabbixreceiver/internal/zabbix"
	"go.opentelemetry.io/collector/pdata/pcommon"
)

// PutMetadata adds readable labels after constant labels so source metadata wins.
func PutMetadata(attrs pcommon.Map, meta zabbix.Metadata, format string) {
	for key, value := range map[string]string{"host_name": meta.HostName, "item_name": meta.ItemName, "value_type": meta.ValueType, "item_units": meta.Units} {
		if value != "" {
			attrs.PutStr(key, value)
		}
	}
	putTags(attrs, "item_tag_", meta.ItemTags)
	putTags(attrs, "host_tag_", meta.HostTags)
	putTags(attrs, "host_inherited_tag_", meta.InheritedHostTags)
	if format == "" || format == "flags" || format == "both" {
		for _, group := range meta.Groups {
			attrs.PutStr("host_group_"+labelKey(group), "true")
		}
	}
	if len(meta.Groups) > 0 && format != "flags" {
		groups := slices.Clone(meta.Groups)
		slices.Sort(groups)
		attrs.PutStr("host_groups", strings.Join(slices.Compact(groups), ", "))
	}
	inventory := make([]zabbix.Tag, 0, len(meta.Inventory))
	for key, value := range meta.Inventory {
		inventory = append(inventory, zabbix.Tag{Tag: key, Value: value})
	}
	putTags(attrs, "inventory_", inventory)
}

// Combine duplicate and colliding keys deterministically, without encoding values.
func putTags(attrs pcommon.Map, prefix string, tags []zabbix.Tag) {
	values := make(map[string][]string)
	for _, tag := range tags {
		key := labelKey(tag.Tag)
		values[key] = append(values[key], tag.Value)
	}
	for key, list := range values {
		slices.Sort(list)
		attrs.PutStr(prefix+key, strings.Join(slices.Compact(list), ", "))
	}
}

func labelKey(key string) string {
	if normalized := identifier(key); normalized != "" {
		return normalized
	}
	return "unnamed"
}
