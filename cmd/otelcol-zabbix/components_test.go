package main

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/component"
)

func TestComponents(t *testing.T) {
	factories, err := components()
	if !assert.NoError(t, err) {
		return
	}

	assert.Equal(t, []string{"zabbix"}, keys(factories.Receivers))
	assert.Equal(t, []string{"batch", "memory_limiter"}, keys(factories.Processors))
	assert.Equal(t, []string{"prometheus_remote_write", "prometheusremotewrite"}, keys(factories.Exporters))
	assert.Equal(t, []string{"health_check"}, keys(factories.Extensions))
}

func keys[T any](factories map[component.Type]T) []string {
	keys := make([]string, 0, len(factories))
	for key := range factories {
		keys = append(keys, key.String())
	}
	sort.Strings(keys)
	return keys
}
