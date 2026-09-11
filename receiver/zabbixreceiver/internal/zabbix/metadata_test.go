package zabbix

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHostMetadataLegacyGroupsAndInventory(t *testing.T) {
	var modern, legacy int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		req := decodeRequest(t, r)
		var params map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(req.Params, &params))
		require.NotContains(t, params, "selectInheritedTags")
		require.JSONEq(t, `["os","location"]`, string(params["selectInventory"]))
		if _, ok := params["selectHostGroups"]; ok {
			modern++
			writeRPCError(t, w, req.ID, -32602, "Invalid params.", `Invalid parameter "/": unexpected parameter "selectHostGroups".`)
			return
		}
		legacy++
		require.JSONEq(t, `["name"]`, string(params["selectGroups"]))
		writeResult(t, w, req.ID, json.RawMessage(`[{"hostid":"1","host":"srv","name":"Server","groups":[{"name":"Linux"}],"tags":[{"tag":"env","value":"prod"}],"inheritedTags":[{"tag":"hidden","value":"ignored"}],"inventory":{"os":"Linux","location":"","secret":"must not be copied"}},{"hostid":"2","host":"no-inventory","inventory":[]}]`))
	}))
	defer server.Close()
	fields := []string{"os", "location"}
	client, err := NewClient(ClientConfig{URL: server.URL, Token: testToken, MetadataEnabled: true, InventoryFields: fields}, nil)
	require.NoError(t, err)
	fields[0] = "secret"
	for range 2 {
		hosts, err := client.Hosts(context.Background())
		require.NoError(t, err)
		require.Equal(t, []string{"Linux"}, hosts[0].Groups)
		require.Equal(t, map[string]string{"os": "Linux", "location": ""}, hosts[0].Inventory)
		require.Empty(t, hosts[0].InheritedTags)
		require.Empty(t, hosts[1].Inventory)
	}
	require.Equal(t, 1, modern, "remember legacy selector after successful fallback")
	require.Equal(t, 2, legacy)
}

func TestHostMetadataDoesNotRetryUnrelatedErrors(t *testing.T) {
	for _, data := range []string{"permission denied", `Invalid parameter "selectInventory".`} {
		t.Run(data, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				req := decodeRequest(t, r)
				writeRPCError(t, w, req.ID, -32602, "Invalid params.", data)
			}))
			defer server.Close()
			client, err := NewClient(ClientConfig{URL: server.URL, Token: testToken, MetadataEnabled: true}, nil)
			require.NoError(t, err)
			_, err = client.Hosts(context.Background())
			require.Error(t, err)
			require.Equal(t, 1, calls)
		})
	}
}

func TestHostMetadataFallsBackWhenLegacyAPIIgnoresModernSelector(t *testing.T) {
	var modern, legacy int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		req := decodeRequest(t, r)
		var params map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(req.Params, &params))
		if _, ok := params["selectHostGroups"]; ok {
			modern++
			writeResult(t, w, req.ID, json.RawMessage(`[{"hostid":"1","host":"srv"}]`))
			return
		}
		legacy++
		require.Contains(t, params, "selectGroups")
		writeResult(t, w, req.ID, json.RawMessage(`[{"hostid":"1","host":"srv","groups":[{"name":"Linux"}]}]`))
	}))
	defer server.Close()
	client, err := NewClient(ClientConfig{URL: server.URL, Token: testToken, MetadataEnabled: true}, nil)
	require.NoError(t, err)
	for range 2 {
		hosts, err := client.Hosts(context.Background())
		require.NoError(t, err)
		require.Equal(t, []string{"Linux"}, hosts[0].Groups)
	}
	require.Equal(t, 1, modern)
	require.Equal(t, 2, legacy)
}

func TestHostMetadataAcceptsExplicitEmptyModernGroups(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		req := decodeRequest(t, r)
		writeResult(t, w, req.ID, json.RawMessage(`[{"hostid":"1","host":"srv","hostgroups":[]}]`))
	}))
	defer server.Close()
	client, err := NewClient(ClientConfig{URL: server.URL, Token: testToken, MetadataEnabled: true}, nil)
	require.NoError(t, err)
	hosts, err := client.Hosts(context.Background())
	require.NoError(t, err)
	require.Empty(t, hosts[0].Groups)
	require.Equal(t, 1, calls)
}
