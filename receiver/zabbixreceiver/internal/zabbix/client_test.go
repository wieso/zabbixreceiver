package zabbix

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const testToken = "secret-token"

type capturedRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
	Auth    *string         `json:"auth"`
	ID      uint64          `json:"id"`
	Fields  map[string]json.RawMessage
}

func decodeRequest(t *testing.T, r *http.Request) capturedRequest {
	t.Helper()
	requireEqual(t, "application/json-rpc", r.Header.Get("Content-Type"))
	body, err := io.ReadAll(r.Body)
	requireNoError(t, err)
	var request capturedRequest
	requireNoError(t, json.Unmarshal(body, &request))
	requireNoError(t, json.Unmarshal(body, &request.Fields))
	requireEqual(t, "2.0", request.JSONRPC)
	if request.ID == 0 {
		t.Fatal("request ID must be non-zero")
	}
	return request
}

func writeResult(t *testing.T, w http.ResponseWriter, id uint64, result any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json-rpc")
	requireNoError(t, json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": id, "result": result}))
}

func writeRPCError(t *testing.T, w http.ResponseWriter, id uint64, code int, message, data string) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json-rpc")
	requireNoError(t, json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": id, "error": RPCError{Code: code, Message: message, Data: data}}))
}

func TestHostsUsesBearerAndExactFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requireEqual(t, "Bearer "+testToken, r.Header.Get("Authorization"))
		request := decodeRequest(t, r)
		requireExactKeys(t, request.Fields, "id", "jsonrpc", "method", "params")
		requireEqual(t, "host.get", request.Method)
		requireCanonicalJSON(t, request.Params, `{"output":["hostid","host"],"sortfield":"hostid"}`)
		writeResult(t, w, request.ID, []map[string]string{{"hostid": "2", "host": "second"}, {"hostid": "1", "host": "first"}})
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	hosts, err := client.Hosts(context.Background())
	requireNoError(t, err)
	requireEqual(t, []Host{{ID: "2", Name: "second"}, {ID: "1", Name: "first"}}, hosts)
}

func TestItemsUsesSupportedSortAndPreservesDeterministicOrder(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		request := decodeRequest(t, r)
		requireExactKeys(t, request.Fields, "id", "jsonrpc", "method", "params")
		requireEqual(t, "item.get", request.Method)
		requireCanonicalJSON(t, request.Params, `{"filter":{"value_type":["0","3"]},"hostids":["10","11"],"output":["itemid","hostid","name","key_","value_type"],"sortfield":"itemid"}`)
		writeResult(t, w, request.ID, []map[string]string{
			{"itemid": "5", "hostid": "11", "name": "Memory", "key_": "vm.memory", "value_type": "3"},
			{"itemid": "6", "hostid": "10", "name": "CPU", "key_": "system.cpu", "value_type": "0"},
		})
	}))
	defer server.Close()

	items, err := newTestClient(t, server.URL).Items(context.Background(), []string{"10", "11"})
	requireNoError(t, err)
	requireEqual(t, []Item{
		{ID: "5", HostID: "11", Name: "Memory", Key: "vm.memory", ValueType: "3"},
		{ID: "6", HostID: "10", Name: "CPU", Key: "system.cpu", ValueType: "0"},
	}, items)
}

func TestValuesRequestsLastFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		request := decodeRequest(t, r)
		requireExactKeys(t, request.Fields, "id", "jsonrpc", "method", "params")
		requireEqual(t, "item.get", request.Method)
		requireCanonicalJSON(t, request.Params, `{"itemids":["5","6"],"output":["itemid","lastvalue","lastclock"],"sortfield":"itemid"}`)
		writeResult(t, w, request.ID, []map[string]string{{"itemid": "5", "lastvalue": "12.5", "lastclock": "1234"}})
	}))
	defer server.Close()

	values, err := newTestClient(t, server.URL).Values(context.Background(), []string{"5", "6"})
	requireNoError(t, err)
	requireEqual(t, []Value{{ItemID: "5", LastValue: "12.5", LastClock: "1234"}}, values)
}

func TestEmptyIDSlicesSkipNetworkCalls(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }))
	defer server.Close()
	client := newTestClient(t, server.URL)

	items, err := client.Items(context.Background(), nil)
	requireNoError(t, err)
	requireEqual(t, []Item(nil), items)
	values, err := client.Values(context.Background(), []string{})
	requireNoError(t, err)
	requireEqual(t, []Value(nil), values)
	requireEqual(t, int32(0), calls.Load())
}

func TestTransportErrorsAreSafeAndAnnotated(t *testing.T) {
	t.Run("HTTP status", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusServiceUnavailable) }))
		defer server.Close()
		_, err := newTestClient(t, server.URL+"?token="+testToken).Hosts(context.Background())
		requireErrorContains(t, err, "zabbix host.get at "+server.URL+": unexpected HTTP status 503")
		requireNotContains(t, err, testToken)
	})
	t.Run("malformed JSON", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "{") }))
		defer server.Close()
		_, err := newTestClient(t, server.URL).Hosts(context.Background())
		requireErrorContains(t, err, "zabbix host.get at "+server.URL)
		requireNotContains(t, err, testToken)
	})
	t.Run("mismatched response ID", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			request := decodeRequest(t, r)
			writeResult(t, w, request.ID+1, []any{})
		}))
		defer server.Close()
		_, err := newTestClient(t, server.URL).Hosts(context.Background())
		requireErrorContains(t, err, "zabbix host.get at "+server.URL+": response ID mismatch")
	})
	t.Run("RPC error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			request := decodeRequest(t, r)
			writeRPCError(t, w, request.ID, -32602, "Invalid params.", "invalid parameter")
		}))
		defer server.Close()
		_, err := newTestClient(t, server.URL).Hosts(context.Background())
		requireErrorContains(t, err, "zabbix host.get at "+server.URL+": zabbix API error -32602: Invalid params.: invalid parameter")
		var rpcError *RPCError
		if !errors.As(err, &rpcError) {
			t.Fatal("error does not expose RPCError")
		}
	})
}

func TestContextCancellationAndClientTimeout(t *testing.T) {
	t.Run("context cancellation", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
		defer server.Close()
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := newTestClient(t, server.URL).Hosts(ctx)
		requireErrorContains(t, err, "zabbix host.get at "+server.URL)
		if !errors.Is(err, context.Canceled) {
			t.Fatal("error does not wrap context cancellation")
		}
	})
	t.Run("client timeout", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { time.Sleep(100 * time.Millisecond) }))
		defer server.Close()
		client, err := NewClient(ClientConfig{URL: server.URL, Token: testToken, Timeout: time.Millisecond}, nil)
		requireNoError(t, err)
		_, err = client.Hosts(context.Background())
		requireErrorContains(t, err, "zabbix host.get at "+server.URL)
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatal("error does not wrap client timeout")
		}
	})
}

func TestLegacyAuthRetryAndCache(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call := calls.Add(1)
		request := decodeRequest(t, r)
		switch call {
		case 1:
			requireExactKeys(t, request.Fields, "id", "jsonrpc", "method", "params")
			requireEqual(t, "Bearer "+testToken, r.Header.Get("Authorization"))
			writeRPCError(t, w, request.ID, -32602, "Invalid params.", "Not authorized.")
		case 2:
			requireExactKeys(t, request.Fields, "auth", "id", "jsonrpc", "method", "params")
			requireEqual(t, "", r.Header.Get("Authorization"))
			if request.Auth == nil {
				t.Fatal("legacy retry must include auth")
			}
			requireEqual(t, testToken, *request.Auth)
			writeResult(t, w, request.ID, []any{})
		case 3:
			requireExactKeys(t, request.Fields, "auth", "id", "jsonrpc", "method", "params")
			requireEqual(t, "", r.Header.Get("Authorization"))
			if request.Auth == nil {
				t.Fatal("cached legacy request must include auth")
			}
			requireEqual(t, testToken, *request.Auth)
			writeResult(t, w, request.ID, []any{})
		default:
			t.Fatalf("unexpected request %d", call)
		}
	}))
	defer server.Close()
	client := newTestClient(t, server.URL)

	_, err := client.Hosts(context.Background())
	requireNoError(t, err)
	_, err = client.Hosts(context.Background())
	requireNoError(t, err)
	requireEqual(t, int32(3), calls.Load())
}

func TestLegacyAuthFailedRetryDoesNotCacheAndRetriesOnlyOnce(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call := calls.Add(1)
		request := decodeRequest(t, r)
		switch call {
		case 1:
			requireExactKeys(t, request.Fields, "id", "jsonrpc", "method", "params")
			requireEqual(t, "Bearer "+testToken, r.Header.Get("Authorization"))
			writeRPCError(t, w, request.ID, -32602, "Invalid params.", "Not authorized.")
		case 2:
			requireExactKeys(t, request.Fields, "auth", "id", "jsonrpc", "method", "params")
			requireEqual(t, "", r.Header.Get("Authorization"))
			writeRPCError(t, w, request.ID, -32602, "Invalid params.", "Not authorized.")
		case 3:
			requireExactKeys(t, request.Fields, "id", "jsonrpc", "method", "params")
			requireEqual(t, "Bearer "+testToken, r.Header.Get("Authorization"))
			writeResult(t, w, request.ID, []any{})
		default:
			t.Fatalf("unexpected request %d", call)
		}
	}))
	defer server.Close()
	client := newTestClient(t, server.URL)

	_, err := client.Hosts(context.Background())
	requireErrorContains(t, err, "zabbix API error -32602: Invalid params.: Not authorized.")
	requireEqual(t, int32(2), calls.Load())
	_, err = client.Hosts(context.Background())
	requireNoError(t, err)
	requireEqual(t, int32(3), calls.Load())
}

func TestConcurrentLegacyAuthTransition(t *testing.T) {
	var bearerCalls atomic.Int32
	var legacyCalls atomic.Int32
	firstLegacyStarted := make(chan struct{})
	secondBearerSeen := make(chan struct{})
	releaseFirstLegacy := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		request := decodeRequest(t, r)
		if request.Auth == nil {
			requireExactKeys(t, request.Fields, "id", "jsonrpc", "method", "params")
			requireEqual(t, "Bearer "+testToken, r.Header.Get("Authorization"))
			if bearerCalls.Add(1) == 2 {
				close(secondBearerSeen)
			}
			writeRPCError(t, w, request.ID, -32602, "Invalid params.", "Not authorized.")
			return
		}

		requireExactKeys(t, request.Fields, "auth", "id", "jsonrpc", "method", "params")
		requireEqual(t, testToken, *request.Auth)
		if legacyCalls.Add(1) == 1 {
			close(firstLegacyStarted)
			<-releaseFirstLegacy
		}
		writeResult(t, w, request.ID, []any{})
	}))
	defer server.Close()
	client := newTestClient(t, server.URL)

	var results sync.WaitGroup
	errs := make(chan error, 2)
	results.Add(1)
	go func() { defer results.Done(); _, err := client.Hosts(context.Background()); errs <- err }()
	select {
	case <-firstLegacyStarted:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("first legacy retry did not start")
	}
	results.Add(1)
	go func() { defer results.Done(); _, err := client.Hosts(context.Background()); errs <- err }()
	sawSecondBearer := false
	select {
	case <-secondBearerSeen:
		sawSecondBearer = true
	case <-time.After(100 * time.Millisecond):
	}
	close(releaseFirstLegacy)
	results.Wait()
	close(errs)
	for err := range errs {
		requireNoError(t, err)
	}
	if !sawSecondBearer {
		t.Fatal("concurrent request did not retain Bearer mode before successful legacy retry")
	}

	_, err := client.Hosts(context.Background())
	requireNoError(t, err)
	requireEqual(t, int32(2), bearerCalls.Load())
	requireEqual(t, int32(3), legacyCalls.Load())
}

func TestNewClientRejectsInvalidURL(t *testing.T) {
	_, err := NewClient(ClientConfig{URL: "ftp://user:" + testToken + "@zabbix.example", Token: testToken}, nil)
	requireErrorContains(t, err, "invalid Zabbix URL")
	requireNotContains(t, err, testToken)
}

func TestRPCErrorRedactsConfiguredTokenFromEntireErrorChain(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		request := decodeRequest(t, r)
		writeRPCError(t, w, request.ID, -32602, "Invalid params.", testToken)
	}))
	defer server.Close()

	_, err := newTestClient(t, server.URL).Hosts(context.Background())
	requireErrorChainNotContains(t, err, testToken)
	var rpcError *RPCError
	if !errors.As(err, &rpcError) {
		t.Fatal("error does not expose RPCError")
	}
	requireNotContains(t, rpcError, testToken)
}

func TestResponseLimitAndBodyClosure(t *testing.T) {
	body := &trackingReadCloser{Reader: strings.NewReader(strings.Repeat("x", maxResponseBytes+1))}
	client, err := NewClient(ClientConfig{URL: "http://zabbix.example", Token: testToken}, &http.Client{
		Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Body: body, Header: make(http.Header)}, nil
		}),
	})
	requireNoError(t, err)

	_, err = client.Hosts(context.Background())
	requireErrorContains(t, err, "response exceeds 8 MiB limit")
	if !body.closed.Load() {
		t.Fatal("response body was not closed")
	}
}

func newTestClient(t *testing.T, rawURL string) *Client {
	t.Helper()
	client, err := NewClient(ClientConfig{URL: rawURL, Token: testToken, Timeout: time.Second}, nil)
	requireNoError(t, err)
	return client
}

func requireNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func requireEqual(t *testing.T, want, got any) {
	t.Helper()
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("want %#v, got %#v", want, got)
	}
}

func requireErrorContains(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("error %v does not contain %q", err, want)
	}
}

func requireNotContains(t *testing.T, err error, forbidden string) {
	t.Helper()
	if err != nil && strings.Contains(err.Error(), forbidden) {
		t.Fatalf("error exposes %q: %v", forbidden, err)
	}
}

func requireExactKeys(t *testing.T, values map[string]json.RawMessage, want ...string) {
	t.Helper()
	got := make([]string, 0, len(values))
	for key := range values {
		got = append(got, key)
	}
	sort.Strings(got)
	sort.Strings(want)
	requireEqual(t, want, got)
}

func requireCanonicalJSON(t *testing.T, got json.RawMessage, want string) {
	t.Helper()
	var gotValue, wantValue any
	requireNoError(t, json.Unmarshal(got, &gotValue))
	requireNoError(t, json.Unmarshal([]byte(want), &wantValue))
	gotJSON, err := json.Marshal(gotValue)
	requireNoError(t, err)
	wantJSON, err := json.Marshal(wantValue)
	requireNoError(t, err)
	requireEqual(t, string(wantJSON), string(gotJSON))
}

func requireErrorChainNotContains(t *testing.T, err error, forbidden string) {
	t.Helper()
	for err != nil {
		requireNotContains(t, err, forbidden)
		err = errors.Unwrap(err)
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

type trackingReadCloser struct {
	io.Reader
	closed atomic.Bool
}

func (r *trackingReadCloser) Close() error {
	r.closed.Store(true)
	return nil
}
