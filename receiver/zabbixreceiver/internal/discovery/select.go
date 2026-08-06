// Package discovery selects Zabbix items and exposes immutable discovery snapshots.
package discovery

import (
	"regexp"

	"github.com/wieso/zabbixreceiver/receiver/zabbixreceiver/internal/zabbix"
)

// Filters controls which Zabbix hosts and item keys are discovered.
type Filters struct {
	HostInclude, HostExclude, ItemKeyInclude, ItemKeyExclude *regexp.Regexp
}

// ItemMeta is the static Zabbix metadata needed to retrieve and emit an item.
type ItemMeta struct {
	ID, HostID, Host, Name, Key, ValueType string
}

// Select filters items by their hosts and keys, then retains at most max items
// for each host. A non-positive max applies no per-host limit.
func Select(hosts []zabbix.Host, items []zabbix.Item, filters Filters, max int) (selected []ItemMeta, filtered, limited int) {
	hostNames := make(map[string]string, len(hosts))
	for _, host := range hosts {
		hostNames[host.ID] = host.Name
	}

	selectedPerHost := make(map[string]int, len(hosts))
	for _, item := range items {
		hostName, ok := hostNames[item.HostID]
		if !ok || excluded(hostName, filters.HostInclude, filters.HostExclude) || excluded(item.Key, filters.ItemKeyInclude, filters.ItemKeyExclude) {
			filtered++
			continue
		}
		if max > 0 && selectedPerHost[item.HostID] >= max {
			limited++
			continue
		}

		selected = append(selected, ItemMeta{
			ID: item.ID, HostID: item.HostID, Host: hostName, Name: item.Name, Key: item.Key, ValueType: item.ValueType,
		})
		selectedPerHost[item.HostID]++
	}
	return selected, filtered, limited
}

func excluded(value string, include, exclude *regexp.Regexp) bool {
	return include != nil && !include.MatchString(value) || exclude != nil && exclude.MatchString(value)
}
