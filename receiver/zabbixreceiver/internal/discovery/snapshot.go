package discovery

import "sync/atomic"

// Snapshot is an immutable collection of discovered item metadata.
type Snapshot struct {
	items []ItemMeta
}

// NewSnapshot creates an immutable snapshot of items.
func NewSnapshot(items []ItemMeta) *Snapshot {
	return &Snapshot{items: cloneItems(items)}
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
	return append([]ItemMeta(nil), items...)
}
