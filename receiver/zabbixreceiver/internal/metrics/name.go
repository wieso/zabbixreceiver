// Package metrics converts Zabbix item values into OpenTelemetry metrics.
package metrics

import (
	"fmt"
	"strings"
)

// Name returns the Prometheus-compatible metric name for an item key.
func Name(prefix, itemKey string) (string, error) {
	var sanitized strings.Builder
	for _, r := range itemKey {
		if isMetricCharacter(r) {
			sanitized.WriteRune(r)
		} else {
			sanitized.WriteByte('_')
		}
	}

	name := strings.TrimRight(prefix+sanitized.String(), "_")
	if name == "" {
		return "", fmt.Errorf("metric name is empty after sanitizing item key %q", itemKey)
	}
	if !isMetricStart(rune(name[0])) {
		name = "_" + name
	}
	return name, nil
}

func isMetricCharacter(r rune) bool {
	return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == ':'
}

func isMetricStart(r rune) bool {
	return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r == '_' || r == ':'
}
