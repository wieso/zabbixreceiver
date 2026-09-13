package zabbixreceiver

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/wieso/zabbixreceiver/receiver/zabbixreceiver/internal/zabbix"
	"go.opentelemetry.io/collector/receiver/receivertest"
)

func TestStreamingPreservesMetadataAndTagIdentity(t *testing.T) {
	cfg := validConfig()
	cfg.Mode = "streaming"
	next := newRecordingConsumer(t)
	r := newTestReceiver(t, cfg, nil, next)
	req := httptest.NewRequest("POST", "/v1/history", strings.NewReader(`{"host":{"host":"srv","name":"Server"},"name":"CPU","itemid":1,"clock":1,"value":2,"type":0,"groups":["Linux","Linux"],"item_tags":[{"tag":"app","value":"web"},{"tag":"app","value":"api"},{"tag":"app","value":"api"},{"tag":"empty","value":""},{"tag":"a.b","value":"dot"},{"tag":"a_b","value":"underscore"},{"tag":"encoded_612e62","value":"literal"},{"tag":"json","value":"[\"x\"]"}]}`))
	req.Header.Set("Content-Type", "application/x-ndjson")
	w := httptest.NewRecorder()
	r.handleHistory(w, req)
	require.Equal(t, 200, w.Code)
	attrs := next.snapshot()[0].ResourceMetrics().At(0).ScopeMetrics().At(0).Metrics().At(0).Gauge().DataPoints().At(0).Attributes().AsRaw()
	require.Equal(t, "Server", attrs["host_name"])
	require.Equal(t, "CPU", attrs["item_name"])
	require.Equal(t, "0", attrs["value_type"])
	require.Equal(t, "true", attrs["host_group_linux"])
	require.Equal(t, "api, web", attrs["item_tag_app"])
	require.Equal(t, "", attrs["item_tag_empty"])
	require.Equal(t, "dot, underscore", attrs["item_tag_a_b"])
	require.Equal(t, "literal", attrs["item_tag_encoded_612e62"])
	require.Equal(t, `["x"]`, attrs["item_tag_json"])
}

func TestStreamingEnrichmentCacheAndRefresh(t *testing.T) {
	cfg := validConfig()
	cfg.Mode = "streaming"
	cfg.Streaming.EnrichWithAPI = true
	next := newRecordingConsumer(t)
	var fail bool
	env := "prod"
	api := &fakeAPI{
		hosts: func(context.Context) ([]zabbix.Host, error) {
			if fail {
				return nil, errors.New("API down")
			}
			return []zabbix.Host{{ID: "10", Name: "srv", VisibleName: "cached host", Tags: []zabbix.Tag{{Tag: "env", Value: env}}, Inventory: map[string]string{"os": "Linux"}}}, nil
		},
		items: func(context.Context, []string) ([]zabbix.Item, error) {
			return []zabbix.Item{{ID: "1", HostID: "10", Name: "cached item", Key: "cpu", ValueType: "0", Units: "%", Tags: []zabbix.Tag{{Tag: "app", Value: "cached"}}}}, nil
		},
	}
	r := newTestReceiver(t, cfg, api, next)
	send := func(body string) int {
		req := httptest.NewRequest("POST", "/v1/history", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/x-ndjson")
		w := httptest.NewRecorder()
		r.handleHistory(w, req)
		return w.Code
	}
	body := `{"host":{"host":"srv","name":"live host"},"name":"live CPU","itemid":1,"clock":1,"value":2,"type":0,"item_tags":[],"groups":[]}`
	require.Equal(t, 503, send(body), "must not emit without required metadata")
	require.Empty(t, next.snapshot())
	require.NoError(t, r.discover(context.Background()))
	require.Equal(t, 200, send(body))
	attrs := next.snapshot()[0].ResourceMetrics().At(0).ScopeMetrics().At(0).Metrics().At(0).Gauge().DataPoints().At(0).Attributes().AsRaw()
	require.Equal(t, "10", attrs["hostid"])
	require.Equal(t, "cpu", attrs["item_key"])
	require.Equal(t, "live host", attrs["host_name"])
	require.Equal(t, "live CPU", attrs["item_name"])
	require.Equal(t, "prod", attrs["host_tag_env"])
	require.Equal(t, "Linux", attrs["inventory_os"])
	require.NotContains(t, attrs, "item_tag_app", "explicit empty tags override cache")
	fail = true
	require.Error(t, r.discover(context.Background()))
	require.Equal(t, 200, send(body), "retain last successful snapshot on API error")
	fail = false
	env = "stage"
	require.NoError(t, r.discover(context.Background()))
	require.Equal(t, 200, send(body))
	attrs = next.snapshot()[2].ResourceMetrics().At(0).ScopeMetrics().At(0).Metrics().At(0).Gauge().DataPoints().At(0).Attributes().AsRaw()
	require.Equal(t, "stage", attrs["host_tag_env"])
	require.Equal(t, 503, send(body+"\n"+strings.Replace(body, `"itemid":1`, `"itemid":2`, 1)))
	require.Len(t, next.snapshot(), 3, "unknown item rejects the whole request")
}

func TestStreamingEnrichmentSchedulesOnlyDiscovery(t *testing.T) {
	cfg := validConfig()
	cfg.Mode = "streaming"
	cfg.Streaming.EnrichWithAPI = true
	cfg.Streaming.Endpoint = "127.0.0.1:0"
	cfg.Schedule.Jitter = 0
	cfg.Schedule.Jobs.Discover = JobConfig{Enabled: true, RunOnStart: true, Interval: time.Millisecond, Timeout: time.Second}
	cfg.Schedule.Jobs.Values = JobConfig{Enabled: true, RunOnStart: true, Interval: time.Millisecond, Timeout: time.Second}
	var discovers, values atomic.Int64
	api := &fakeAPI{hosts: func(context.Context) ([]zabbix.Host, error) { discovers.Add(1); return nil, nil }, items: func(context.Context, []string) ([]zabbix.Item, error) { return nil, nil }, values: func(context.Context, []string) ([]zabbix.Value, error) { values.Add(1); return nil, nil }}
	r := newTestReceiver(t, cfg, api, newRecordingConsumer(t))
	require.NoError(t, r.Start(context.Background(), nil))
	t.Cleanup(func() { require.NoError(t, r.Shutdown(context.Background())) })
	require.Eventually(t, func() bool { return discovers.Load() >= 2 }, time.Second, time.Millisecond)
	require.Zero(t, values.Load())
}

func TestMetadataConfiguration(t *testing.T) {
	cfg := validConfig()
	cfg.Mode = "streaming"
	cfg.Streaming.EnrichWithAPI = true
	require.NoError(t, cfg.validateResolved())
	cfg.Zabbix.Token = ""
	require.ErrorContains(t, cfg.validateResolved(), "zabbix.token")
	cfg.Zabbix.Token = "test"
	cfg.Schedule.Jobs.Discover.Enabled = false
	require.ErrorContains(t, cfg.validateResolved(), "discover")
	cfg.Schedule.Jobs.Discover.Enabled = true
	cfg.Metadata.Enabled = false
	require.ErrorContains(t, cfg.validateResolved(), "metadata.enabled")
	cfg.Metadata.Enabled = true
	cfg.Metadata.InventoryFields = []string{"os"}
	clone := cfg.Clone()
	clone.Metadata.InventoryFields[0] = "name"
	require.Equal(t, []string{"os"}, cfg.Metadata.InventoryFields)
	cfg.Metadata.InventoryFields = []string{""}
	require.ErrorContains(t, cfg.validateResolved(), "metadata.inventory_fields")
}

func TestMetadataCanBeDisabled(t *testing.T) {
	cfg := validConfig()
	cfg.Mode = "streaming"
	cfg.Metadata.Enabled = false
	next := newRecordingConsumer(t)
	r := newTestReceiver(t, cfg, nil, next)
	req := httptest.NewRequest("POST", "/v1/history", strings.NewReader(`{"host":{"host":"srv","name":"Server"},"name":"CPU","itemid":1,"clock":1,"value":2,"type":0,"groups":["Linux"],"item_tags":[{"tag":"app","value":"web"}]}`))
	req.Header.Set("Content-Type", "application/x-ndjson")
	w := httptest.NewRecorder()
	r.handleHistory(w, req)
	require.Equal(t, 200, w.Code)
	attrs := next.snapshot()[0].ResourceMetrics().At(0).ScopeMetrics().At(0).Metrics().At(0).Gauge().DataPoints().At(0).Attributes().AsRaw()
	require.Equal(t, map[string]any{"host": "srv", "itemid": "1"}, attrs)
}

func TestPollingMetadataFromAPIToDataPoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Method string
			Params map[string]json.RawMessage
			ID     uint64
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		var result string
		switch req.Method {
		case "host.get":
			require.JSONEq(t, `["tag","value"]`, string(req.Params["selectTags"]))
			require.JSONEq(t, `["tag","value"]`, string(req.Params["selectInheritedTags"]))
			require.JSONEq(t, `["name"]`, string(req.Params["selectHostGroups"]))
			require.JSONEq(t, `["os","location"]`, string(req.Params["selectInventory"]))
			result = `[{"hostid":"10","host":"srv","name":"Server","tags":[{"tag":"env","value":"prod"}],"inheritedTags":[{"tag":"env","value":"base"}],"hostgroups":[{"name":"Linux"}],"inventory":{"os":"Linux","location":"","secret":"not selected"}}]`
		case "item.get":
			if _, ok := req.Params["hostids"]; ok {
				require.JSONEq(t, `["tag","value"]`, string(req.Params["selectTags"]))
				result = `[{"itemid":"1","hostid":"10","name":"CPU","key_":"cpu","value_type":"0","units":"%","tags":[{"tag":"env","value":"item"}]}]`
			} else {
				result = `[{"itemid":"1","lastvalue":"2","lastclock":"1"}]`
			}
		default:
			t.Errorf("unexpected method %s", req.Method)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": json.RawMessage(result)})
	}))
	defer server.Close()
	cfg := validConfig()
	cfg.Zabbix.URL = server.URL
	cfg.Metadata.InventoryFields = []string{"os", "location"}
	next := newRecordingConsumer(t)
	factory := NewFactory()
	created, err := factory.CreateMetrics(context.Background(), receivertest.NewNopSettings(factory.Type()), cfg, next.metrics)
	require.NoError(t, err)
	r := created.(*zabbixReceiver)
	require.NoError(t, r.discover(context.Background()))
	require.NoError(t, r.values(context.Background()))
	attrs := next.snapshot()[0].ResourceMetrics().At(0).ScopeMetrics().At(0).Metrics().At(0).Gauge().DataPoints().At(0).Attributes().AsRaw()
	require.Equal(t, "prod", attrs["host_tag_env"])
	require.Equal(t, "base", attrs["host_inherited_tag_env"])
	require.Equal(t, "item", attrs["item_tag_env"])
	require.Equal(t, "Server", attrs["host_name"])
	require.Equal(t, "%", attrs["item_units"])
	require.Equal(t, "true", attrs["host_group_linux"])
	require.Equal(t, "Linux", attrs["inventory_os"])
	require.Equal(t, "", attrs["inventory_location"])
	require.NotContains(t, attrs, "inventory_secret")
}
