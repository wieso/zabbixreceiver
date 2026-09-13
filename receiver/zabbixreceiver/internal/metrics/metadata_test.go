package metrics

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/wieso/zabbixreceiver/receiver/zabbixreceiver/internal/zabbix"
	"go.opentelemetry.io/collector/pdata/pcommon"
)

func TestMetadataPreservesGroupNames(t *testing.T) {
	for _, tc := range []struct {
		name   string
		groups []string
		want   map[string]any
	}{
		{name: "no groups", want: map[string]any{}},
		{name: "single group", groups: []string{"Linux servers"}, want: map[string]any{"host_groups": "Linux servers"}},
		{name: "sorted unique original names", groups: []string{"Серверы/Москва", "Production", "Linux servers", "Production", "a-b", "a_b"}, want: map[string]any{"host_groups": "Linux servers, Production, a-b, a_b, Серверы/Москва"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			original := slices.Clone(tc.groups)
			attrs := pcommon.NewMap()
			PutMetadata(attrs, zabbix.Metadata{Groups: tc.groups})
			require.Equal(t, tc.want, attrs.AsRaw())
			require.Equal(t, original, tc.groups, "shared source metadata must not be mutated")
		})
	}
}
