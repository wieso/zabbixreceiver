package discovery

import (
	"github.com/stretchr/testify/require"
	"github.com/wieso/zabbixreceiver/receiver/zabbixreceiver/internal/zabbix"
	"sync"
	"sync/atomic"
	"testing"
)

func TestSnapshotCopiesAtInputAndOutputBoundaries(t *testing.T) {
	input := []ItemMeta{{ID: "1", Host: "prod-a", Key: "system.cpu.util"}}
	snapshot := NewSnapshot(input)

	input[0].Host = "changed-input"
	got := snapshot.Items()
	if got[0].Host != "prod-a" {
		t.Fatalf("stored host after input mutation = %q, want prod-a", got[0].Host)
	}

	got[0].Host = "changed-output"
	if snapshot.Items()[0].Host != "prod-a" {
		t.Fatalf("stored host after output mutation = %q, want prod-a", snapshot.Items()[0].Host)
	}
}

func TestSnapshotDeepCopiesMetadata(t *testing.T) {
	input := []ItemMeta{{ID: "1", Metadata: zabbix.Metadata{ItemTags: []zabbix.Tag{{Tag: "app", Value: "web"}}, Groups: []string{"Linux"}, Inventory: map[string]string{"os": "Linux"}}}}
	snapshot := NewSnapshot(input)
	input[0].Metadata.ItemTags[0].Value = "changed"
	input[0].Metadata.Groups[0] = "changed"
	input[0].Metadata.Inventory["os"] = "changed"
	got := snapshot.Items()[0]
	require.Equal(t, "web", got.Metadata.ItemTags[0].Value)
	require.Equal(t, "Linux", got.Metadata.Groups[0])
	require.Equal(t, "Linux", got.Metadata.Inventory["os"])
	got.Metadata.ItemTags[0].Value = "output"
	got.Metadata.Inventory["os"] = "output"
	require.Equal(t, "web", snapshot.Items()[0].Metadata.ItemTags[0].Value)
	require.Equal(t, "Linux", snapshot.Items()[0].Metadata.Inventory["os"])
	lookup, ok := snapshot.Lookup("1")
	require.True(t, ok)
	lookup.Metadata.ItemTags[0].Value = "lookup"
	lookup.Metadata.Groups[0] = "lookup"
	lookup.Metadata.Inventory["os"] = "lookup"
	again, ok := snapshot.Lookup("1")
	require.True(t, ok)
	require.Equal(t, "web", again.Metadata.ItemTags[0].Value)
	require.Equal(t, "Linux", again.Metadata.Groups[0])
	require.Equal(t, "Linux", again.Metadata.Inventory["os"])
	_, ok = snapshot.Lookup("missing")
	require.False(t, ok)
}

func TestStoreSupportsConcurrentLoadsAndReplaces(t *testing.T) {
	var store Store
	store.Replace(NewSnapshot([]ItemMeta{{ID: "initial"}}))

	const iterations = 1_000
	var writers sync.WaitGroup
	for writer := 0; writer < 4; writer++ {
		writers.Add(1)
		go func(writer int) {
			defer writers.Done()
			for i := 0; i < iterations; i++ {
				store.Replace(NewSnapshot([]ItemMeta{{ID: string(rune('a' + writer)), Host: "prod"}}))
			}
		}(writer)
	}

	var readers sync.WaitGroup
	var loadReturnedNil atomic.Bool
	for reader := 0; reader < 4; reader++ {
		readers.Add(1)
		go func() {
			defer readers.Done()
			for i := 0; i < iterations; i++ {
				snapshot := store.Load()
				if snapshot == nil {
					loadReturnedNil.Store(true)
					return
				}
				_ = snapshot.Items()
			}
		}()
	}
	writers.Wait()
	readers.Wait()
	if loadReturnedNil.Load() {
		t.Fatal("Load returned nil after Replace")
	}
}
