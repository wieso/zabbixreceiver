package discovery

import (
	"maps"
	"slices"
	"sync/atomic"
)

// Snapshot is an immutable collection of discovered item metadata.
type Snapshot struct {
	items []ItemMeta
	byID  map[string]int
}

// NewSnapshot creates an immutable snapshot of items.
func NewSnapshot(items []ItemMeta) *Snapshot {
	s := &Snapshot{items: cloneItems(items), byID: make(map[string]int, len(items))}
	for i, item := range items {
		s.byID[item.ID] = i
	}
	return s
}

// Lookup returns an independent copy without copying the entire discovery cache.
func (s *Snapshot) Lookup(id string) (ItemMeta, bool) {
	if s == nil {
		return ItemMeta{}, false
	}
	i, ok := s.byID[id]
	if !ok {
		return ItemMeta{}, false
	}
	return cloneItems(s.items[i : i+1])[0], true
}

// Items returns an independent copy of the snapshot's item metadata.
func (s *Snapshot) Items() []ItemMeta {
	if s == nil {
		return nil
	}
	return cloneItems(s.items)
}

// Store atomically publishes immutable discovery snapshots.
type Store struct {
	snapshot atomic.Pointer[Snapshot]
}

// Load returns the most recently published snapshot, or nil when none has been published.
func (s *Store) Load() *Snapshot {
	return s.snapshot.Load()
}

// Replace atomically publishes snapshot.
func (s *Store) Replace(snapshot *Snapshot) {
	s.snapshot.Store(snapshot)
}

func cloneItems(items []ItemMeta) []ItemMeta {
	cloned := append([]ItemMeta(nil), items...)
	for i := range cloned {
		cloned[i].Metadata.ItemTags = slices.Clone(items[i].Metadata.ItemTags)
		cloned[i].Metadata.HostTags = slices.Clone(items[i].Metadata.HostTags)
		cloned[i].Metadata.InheritedHostTags = slices.Clone(items[i].Metadata.InheritedHostTags)
		cloned[i].Metadata.Groups = slices.Clone(items[i].Metadata.Groups)
		cloned[i].Metadata.Inventory = maps.Clone(items[i].Metadata.Inventory)
	}
	return cloned
}
