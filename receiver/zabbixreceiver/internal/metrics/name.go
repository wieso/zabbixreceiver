// Package metrics converts Zabbix item values into OpenTelemetry metrics.
package metrics

import (
	"fmt"
	"strings"
)

var symbolWords = strings.NewReplacer("%", "_percent_", "°", "_degree_", "+", "_plus_", "&", "_and_")

// Name returns a lowercase, snake_case Prometheus identifier.
func Name(prefix, source string) (string, error) {
	name := identifier(prefix + source)
	if name == "" {
		return "", fmt.Errorf("metric name is empty after sanitizing source %q", source)
	}
	if name[0] >= '0' && name[0] <= '9' {
		name = "_" + name
	}
	return name, nil
}

// identifier also normalizes metadata keys. Values are never normalized.
func identifier(source string) string {
	chars := []rune(symbolWords.Replace(source))
	var out strings.Builder
	separator := false
	for i, ch := range chars {
		upper := ch >= 'A' && ch <= 'Z'
		if !upper && !lower(ch) && !(ch >= '0' && ch <= '9') {
			separator = out.Len() > 0
			continue
		}
		if upper && i > 0 && (lower(chars[i-1]) || (chars[i-1] >= '0' && chars[i-1] <= '9') || (chars[i-1] >= 'A' && chars[i-1] <= 'Z' && i+1 < len(chars) && lower(chars[i+1]))) {
			separator = out.Len() > 0
		}
		if separator {
			out.WriteByte('_')
			separator = false
		}
		if upper {
			ch += 'a' - 'A'
		}
		out.WriteRune(ch)
	}
	return out.String()
}

func lower(ch rune) bool { return ch >= 'a' && ch <= 'z' }
