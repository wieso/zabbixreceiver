package zabbixreceiver

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/wieso/zabbixreceiver/receiver/zabbixreceiver/internal/discovery"
	"github.com/wieso/zabbixreceiver/receiver/zabbixreceiver/internal/zabbix"
)

func TestRepresentationAcrossModes(t *testing.T) {
	for _, mode := range []string{"api", "streaming", "enriched"} {
		t.Run(mode, func(t *testing.T) {
			cfg := validConfig()
			cfg.Mode = mode
			if mode == "enriched" {
				cfg.Mode = "streaming"
				cfg.Streaming.EnrichWithAPI = true
			}
			next := newRecordingConsumer(t)
			meta := zabbix.Metadata{ItemName: "CPU Usage (%)", ValueType: "0", ItemTags: []zabbix.Tag{
				{Tag: "a.b", Value: "literal [value] / %"}, {Tag: "empty", Value: ""}, {Tag: "json", Value: `["x"]`},
			}}
			api := &fakeAPI{values: func(context.Context, []string) ([]zabbix.Value, error) {
				return []zabbix.Value{{ItemID: "1", LastValue: "2", LastClock: "1"}}, nil
			}}
			r := newTestReceiver(t, cfg, api, next)
			r.store.Replace(discovery.NewSnapshot([]discovery.ItemMeta{{ID: "1", HostID: "10", Host: "srv", Key: "System.CPUUsage[%]", Name: meta.ItemName, Metadata: meta}}))
			if mode == "api" {
				require.NoError(t, r.values(context.Background()))
			} else {
				req := httptest.NewRequest("POST", "/v1/history", strings.NewReader(`{"host":{"host":"srv"},"name":"CPU Usage (%)","itemid":1,"clock":1,"value":2,"type":0,"item_tags":[{"tag":"a.b","value":"literal [value] / %"},{"tag":"empty","value":""},{"tag":"json","value":"[\"x\"]"}]}`))
				req.Header.Set("Content-Type", "application/x-ndjson")
				w := httptest.NewRecorder()
				r.handleHistory(w, req)
				require.Equal(t, 200, w.Code)
			}
			m := next.snapshot()[0].ResourceMetrics().At(0).ScopeMetrics().At(0).Metrics().At(0)
			want := "zabbix_system_cpu_usage_percent"
			if mode == "streaming" {
				want = "zabbix_cpu_usage_percent"
			}
			require.Equal(t, want, m.Name())
			attrs := m.Gauge().DataPoints().At(0).Attributes().AsRaw()
			require.Equal(t, "literal [value] / %", attrs["item_tag_a_b"])
			require.Equal(t, "", attrs["item_tag_empty"])
			require.Equal(t, `["x"]`, attrs["item_tag_json"])
			require.Equal(t, "CPU Usage (%)", attrs["item_name"])
			for key := range attrs {
				require.False(t, strings.HasPrefix(key, "zabbix_"))
				require.NotContains(t, key, "encoded_")
			}
		})
	}
}
