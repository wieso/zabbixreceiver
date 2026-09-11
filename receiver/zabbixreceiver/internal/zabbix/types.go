// Package zabbix provides a small, typed client for the Zabbix JSON-RPC API.
package zabbix

import (
	"context"
	"fmt"
	"time"
)

// API is the subset of the Zabbix API used by the receiver.
type API interface {
	Hosts(context.Context) ([]Host, error)
	Items(context.Context, []string) ([]Item, error)
	Values(context.Context, []string) ([]Value, error)
}

type Host struct {
	ID            string
	Name          string
	VisibleName   string
	Tags          []Tag
	InheritedTags []Tag
	Groups        []string
	Inventory     map[string]string
}

type Item struct {
	ID        string
	HostID    string
	Name      string
	Key       string
	ValueType string
	Units     string
	Tags      []Tag
}

// Tag remains a list entry: Zabbix allows multiple values for one tag name.
type Tag struct {
	Tag   string `json:"tag"`
	Value string `json:"value"`
}

// Metadata is shared by polling and streaming before label encoding.
type Metadata struct {
	HostName, ItemName, ValueType, Units  string
	ItemTags, HostTags, InheritedHostTags []Tag
	Groups                                []string
	Inventory                             map[string]string
}

type Value struct {
	ItemID    string
	LastValue string
	LastClock string
}

type ClientConfig struct {
	URL               string
	Token             string
	Timeout           time.Duration
	MetadataEnabled   bool
	InheritedHostTags bool
	InventoryFields   []string
}

// RPCError is an error returned by the Zabbix JSON-RPC endpoint.
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    string `json:"data"`
}

func (e *RPCError) Error() string {
	return fmt.Sprintf("zabbix API error %d: %s: %s", e.Code, e.Message, e.Data)
}
