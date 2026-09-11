package zabbix

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

const maxResponseBytes = 8 << 20

var quotedErrorText = regexp.MustCompile(`"(?:[^"\\]|\\.)*"`)

type authMode uint8

const (
	bearerAuth authMode = iota
	legacyAuth
)

// Client is a concurrency-safe JSON-RPC client for the Zabbix API.
type Client struct {
	endpoint          *url.URL
	safeEndpoint      string
	token             string
	httpClient        *http.Client
	nextID            atomic.Uint64
	metadataEnabled   bool
	inheritedHostTags bool
	inventoryFields   []string
	legacyGroups      atomic.Bool

	authMu   sync.RWMutex
	authMode authMode
}

type requestEnvelope struct {
	JSONRPC string  `json:"jsonrpc"`
	Method  string  `json:"method"`
	Params  any     `json:"params"`
	Auth    *string `json:"auth,omitempty"`
	ID      uint64  `json:"id"`
}

type responseEnvelope struct {
	JSONRPC string          `json:"jsonrpc"`
	Result  json.RawMessage `json:"result"`
	Error   *RPCError       `json:"error"`
	ID      uint64          `json:"id"`
}

func NewClient(config ClientConfig, httpClient *http.Client) (*Client, error) {
	endpoint, err := url.Parse(config.URL)
	if err != nil || endpoint.Scheme == "" || endpoint.Host == "" || (endpoint.Scheme != "http" && endpoint.Scheme != "https") {
		return nil, errors.New("invalid Zabbix URL")
	}
	if config.Token == "" {
		return nil, errors.New("Zabbix token must not be empty")
	}

	if httpClient == nil {
		httpClient = &http.Client{}
	}
	clientCopy := *httpClient
	if config.Timeout > 0 {
		clientCopy.Timeout = config.Timeout
	}

	return &Client{
		endpoint:          endpoint,
		safeEndpoint:      endpoint.Scheme + "://" + endpoint.Host + endpoint.EscapedPath(),
		token:             config.Token,
		httpClient:        &clientCopy,
		authMode:          bearerAuth,
		metadataEnabled:   config.MetadataEnabled,
		inheritedHostTags: config.InheritedHostTags,
		inventoryFields:   append([]string(nil), config.InventoryFields...),
	}, nil
}

func (c *Client) Hosts(ctx context.Context) ([]Host, error) {
	var result []struct {
		ID            string `json:"hostid"`
		Name          string `json:"host"`
		VisibleName   string `json:"name"`
		Tags          []Tag  `json:"tags"`
		InheritedTags []Tag  `json:"inheritedTags"`
		Groups        []struct {
			Name string `json:"name"`
		} `json:"hostgroups"`
		LegacyGroups []struct {
			Name string `json:"name"`
		} `json:"groups"`
		Inventory json.RawMessage `json:"inventory"`
	}
	params := map[string]any{
		"output":    []string{"hostid", "host"},
		"sortfield": "hostid",
	}
	if c.metadataEnabled {
		params["output"] = []string{"hostid", "host", "name"}
		params["selectTags"] = []string{"tag", "value"}
		params["selectHostGroups"] = []string{"name"}
		if c.legacyGroups.Load() {
			delete(params, "selectHostGroups")
			params["selectGroups"] = []string{"name"}
		}
		if c.inheritedHostTags {
			params["selectInheritedTags"] = []string{"tag", "value"}
		}
		if len(c.inventoryFields) > 0 {
			params["selectInventory"] = c.inventoryFields
		}
	}
	err := c.call(ctx, "host.get", params, &result)
	var rpcError *RPCError
	unsupportedGroups := errors.As(err, &rpcError) && rpcError.Code == -32602 && strings.Contains(rpcError.Data, `unexpected parameter "selectHostGroups"`)
	// Zabbix 6.0 also silently ignores unknown selectors. A present empty array
	// is valid; an absent property on a returned host requires the legacy query.
	if err == nil {
		for _, host := range result {
			if host.Groups == nil {
				unsupportedGroups = true
				break
			}
		}
	}
	if _, modern := params["selectHostGroups"]; modern && unsupportedGroups {
		delete(params, "selectHostGroups")
		params["selectGroups"] = []string{"name"}
		result = nil
		err = c.call(ctx, "host.get", params, &result)
		if err == nil {
			c.legacyGroups.Store(true)
		}
	}
	if err != nil {
		return nil, err
	}
	hosts := make([]Host, len(result))
	for i, host := range result {
		hosts[i] = Host{ID: host.ID, Name: host.Name}
		if c.metadataEnabled {
			hosts[i].VisibleName = host.VisibleName
			hosts[i].Tags = host.Tags
			if c.inheritedHostTags {
				hosts[i].InheritedTags = host.InheritedTags
			}
			for _, group := range host.Groups {
				hosts[i].Groups = append(hosts[i].Groups, group.Name)
			}
			for _, group := range host.LegacyGroups {
				hosts[i].Groups = append(hosts[i].Groups, group.Name)
			}
			if len(c.inventoryFields) > 0 && len(host.Inventory) > 0 && string(host.Inventory) != "[]" {
				var inventory map[string]string
				if err := json.Unmarshal(host.Inventory, &inventory); err != nil {
					return nil, c.operationError("host.get", fmt.Errorf("decode inventory: %w", err))
				}
				for _, key := range c.inventoryFields {
					if value, ok := inventory[key]; ok {
						if hosts[i].Inventory == nil {
							hosts[i].Inventory = make(map[string]string)
						}
						hosts[i].Inventory[key] = value
					}
				}
			}
		}
	}
	return hosts, nil
}

func (c *Client) Items(ctx context.Context, hostIDs []string) ([]Item, error) {
	if len(hostIDs) == 0 {
		return nil, nil
	}
	var result []struct {
		ID        string `json:"itemid"`
		HostID    string `json:"hostid"`
		Name      string `json:"name"`
		Key       string `json:"key_"`
		ValueType string `json:"value_type"`
		Units     string `json:"units"`
		Tags      []Tag  `json:"tags"`
	}
	params := map[string]any{
		"output":    []string{"itemid", "hostid", "name", "key_", "value_type"},
		"hostids":   hostIDs,
		"filter":    map[string][]string{"value_type": {"0", "3"}},
		"sortfield": "itemid",
	}
	if c.metadataEnabled {
		params["output"] = []string{"itemid", "hostid", "name", "key_", "value_type", "units"}
		params["selectTags"] = []string{"tag", "value"}
	}
	err := c.call(ctx, "item.get", params, &result)
	if err != nil {
		return nil, err
	}
	items := make([]Item, len(result))
	for i, item := range result {
		items[i] = Item{ID: item.ID, HostID: item.HostID, Name: item.Name, Key: item.Key, ValueType: item.ValueType}
		if c.metadataEnabled {
			items[i].Units = item.Units
			items[i].Tags = item.Tags
		}
	}
	return items, nil
}

func (c *Client) Values(ctx context.Context, itemIDs []string) ([]Value, error) {
	if len(itemIDs) == 0 {
		return nil, nil
	}
	var result []struct {
		ItemID    string `json:"itemid"`
		LastValue string `json:"lastvalue"`
		LastClock string `json:"lastclock"`
	}
	err := c.call(ctx, "item.get", map[string]any{
		"output":    []string{"itemid", "lastvalue", "lastclock"},
		"itemids":   itemIDs,
		"sortfield": "itemid",
	}, &result)
	if err != nil {
		return nil, err
	}
	values := make([]Value, len(result))
	for i, value := range result {
		values[i] = Value{ItemID: value.ItemID, LastValue: value.LastValue, LastClock: value.LastClock}
	}
	return values, nil
}

func (c *Client) call(ctx context.Context, method string, params any, result any) error {
	mode := c.currentAuthMode()
	err := c.callWithMode(ctx, method, params, result, mode)
	if err == nil || mode == legacyAuth || !isAuthenticationError(err) {
		return err
	}

	retryErr := c.callWithMode(ctx, method, params, result, legacyAuth)
	if retryErr == nil {
		c.useLegacyAuth()
	}
	return retryErr
}

func (c *Client) callWithMode(ctx context.Context, method string, params, result any, mode authMode) error {
	id := c.nextID.Add(1)
	requestBody := requestEnvelope{JSONRPC: "2.0", Method: method, Params: params, ID: id}
	if mode == legacyAuth {
		requestBody.Auth = &c.token
	}
	body, err := json.Marshal(requestBody)
	if err != nil {
		return c.operationError(method, fmt.Errorf("encode request: %w", err))
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint.String(), bytes.NewReader(body))
	if err != nil {
		return c.operationError(method, fmt.Errorf("create request: %w", err))
	}
	request.Header.Set("Content-Type", "application/json-rpc")
	if mode == bearerAuth {
		request.Header.Set("Authorization", "Bearer "+c.token)
	}

	response, err := c.httpClient.Do(request)
	if err != nil {
		return c.operationError(method, err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return c.operationError(method, fmt.Errorf("unexpected HTTP status %d", response.StatusCode))
	}

	responseBody, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		return c.operationError(method, fmt.Errorf("read response: %w", err))
	}
	if len(responseBody) > maxResponseBytes {
		return c.operationError(method, errors.New("response exceeds 8 MiB limit"))
	}

	var envelope responseEnvelope
	if err := json.Unmarshal(responseBody, &envelope); err != nil {
		return c.operationError(method, fmt.Errorf("decode response: %w", err))
	}
	if envelope.ID != id {
		return c.operationError(method, errors.New("response ID mismatch"))
	}
	if envelope.Error != nil {
		return c.operationError(method, envelope.Error)
	}
	if envelope.Result == nil {
		return c.operationError(method, errors.New("response is missing result"))
	}
	if err := json.Unmarshal(envelope.Result, result); err != nil {
		return c.operationError(method, fmt.Errorf("decode result: %w", err))
	}
	return nil
}

func (c *Client) operationError(method string, err error) error {
	return &operationError{method: method, endpoint: c.safeEndpoint, err: redactError(err, c.token)}
}

type operationError struct {
	method   string
	endpoint string
	err      error
}

func (e *operationError) Error() string {
	return fmt.Sprintf("zabbix %s at %s: %s", e.method, e.endpoint, e.err)
}

func (e *operationError) Unwrap() error {
	return e.err
}

type redactedError struct {
	message  string
	original error
	cause    error
}

func (e *redactedError) Error() string {
	return e.message
}

func (e *redactedError) Unwrap() error {
	return e.cause
}

func (e *redactedError) Is(target error) bool {
	return errors.Is(e.original, target)
}

func redactError(err error, token string) error {
	redacted, _ := redactErrorChain(err, token)
	return redacted
}

func redactErrorChain(err error, token string) (error, bool) {
	if err == nil {
		return err, false
	}
	if urlError, ok := err.(*url.Error); ok {
		copy := *urlError
		copy.URL = redactToken(safeErrorURL(copy.URL), token)
		copy.Err, _ = redactErrorChain(urlError.Err, token)
		return &copy, copy.Error() != urlError.Error()
	}
	if rpcError, ok := err.(*RPCError); ok {
		copy := *rpcError
		copy.Message = redactToken(copy.Message, token)
		copy.Data = redactToken(copy.Data, token)
		return &copy, copy.Message != rpcError.Message || copy.Data != rpcError.Data
	}

	originalCause := errors.Unwrap(err)
	cause, causeRedacted := redactErrorChain(originalCause, token)
	message := err.Error()
	if causeRedacted {
		// A wrapping error also formats its cause; sanitizing only Unwrap would
		// still leak the original URL when the outer error is logged.
		message = strings.ReplaceAll(message, originalCause.Error(), cause.Error())
	}
	message = redactToken(message, token)
	// net/http embeds a malformed Location (including relative URLs) in a
	// quoted plain error with no unwrap chain. Sanitize those URL literals too.
	message = quotedErrorText.ReplaceAllStringFunc(message, func(quoted string) string {
		value, err := strconv.Unquote(quoted)
		if err != nil || !strings.ContainsAny(value, "?#@") {
			return quoted
		}
		return strconv.Quote(safeErrorURL(value))
	})
	if causeRedacted || message != err.Error() {
		return &redactedError{message: message, original: err, cause: cause}, true
	}
	return err, false
}

func safeErrorURL(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "[REDACTED URL]"
	}
	parsed.User = nil
	parsed.RawQuery, parsed.Fragment, parsed.RawFragment = "", "", ""
	parsed.ForceQuery = false
	return parsed.String()
}

func redactToken(value, token string) string {
	if token == "" {
		return value
	}
	return strings.ReplaceAll(value, token, "[REDACTED]")
}

func (c *Client) currentAuthMode() authMode {
	c.authMu.RLock()
	defer c.authMu.RUnlock()
	return c.authMode
}

func (c *Client) useLegacyAuth() {
	c.authMu.Lock()
	defer c.authMu.Unlock()
	c.authMode = legacyAuth
}

func isAuthenticationError(err error) bool {
	var rpcError *RPCError
	if !errors.As(err, &rpcError) {
		return false
	}
	message := strings.ToLower(rpcError.Message + " " + rpcError.Data)
	return strings.Contains(message, "not authorized") ||
		strings.Contains(message, "not authenticated") ||
		strings.Contains(message, "unauthorized") ||
		strings.Contains(message, "authentication") ||
		strings.Contains(message, "session terminated")
}
