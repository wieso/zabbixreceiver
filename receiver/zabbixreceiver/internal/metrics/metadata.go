package metrics

import (
	"encoding/hex"
	"encoding/json"
	"slices"
	"strings"

	"github.com/wieso/zabbixreceiver/receiver/zabbixreceiver/internal/zabbix"
	"go.opentelemetry.io/collector/pdata/pcommon"
)

// PutMetadata emits a flat, collision-free label representation. Call after
// constant labels so source metadata takes precedence.
func PutMetadata(attrs pcommon.Map, meta zabbix.Metadata) {
	for key, value := range map[string]string{
		"zabbix_host_name": meta.HostName, "zabbix_item_name": meta.ItemName,
		"zabbix_value_type": meta.ValueType, "zabbix_item_units": meta.Units,
	} {
		if value != "" {
			attrs.PutStr(key, value)
		}
	}
	putTags(attrs, "zabbix_item_tag_", meta.ItemTags)
	putTags(attrs, "zabbix_host_tag_", meta.HostTags)
	putTags(attrs, "zabbix_host_inherited_tag_", meta.InheritedHostTags)
	for _, group := range meta.Groups {
		attrs.PutStr("zabbix_host_group_"+labelKey(group), "true")
	}
	for key, value := range meta.Inventory {
		attrs.PutStr("zabbix_inventory_"+labelKey(key), labelValues([]string{value}))
	}
}

func putTags(attrs pcommon.Map, prefix string, tags []zabbix.Tag) {
	values := make(map[string][]string)
	for _, tag := range tags {
		values[tag.Tag] = append(values[tag.Tag], tag.Value)
	}
	for key, list := range values {
		attrs.PutStr(prefix+labelKey(key), labelValues(list))
	}
}

// Reserve encoded_ so encoded names cannot collide with literal tag names.
// Encode complete UTF-8 bytes, preserving otherwise ambiguous punctuation.
func labelKey(key string) string {
	valid := key != "" && !strings.HasPrefix(key, "encoded_")
	for _, r := range key {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_') {
			valid = false
			break
		}
	}
	if valid {
		return key
	}
	return "encoded_" + hex.EncodeToString([]byte(key))
}

// Values beginning with '[' are wrapped too, making the representation
// unambiguous. JSON encodes empty values without the exporter dropping them.
func labelValues(values []string) string {
	slices.Sort(values)
	values = slices.Compact(values)
	if len(values) == 1 && values[0] != "" && !strings.HasPrefix(values[0], "[") {
		return values[0]
	}
	encoded, _ := json.Marshal(values)
	return string(encoded)
}
