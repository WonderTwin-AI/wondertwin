package store

import (
	"encoding/json"

	pkgstore "github.com/wondertwin-ai/wondertwin/twinkit/store"
)

// MemoryStore holds all twin state in memory.
// Add a pkgstore.Store field for each resource type your twin implements.
type MemoryStore struct {
	Resources *pkgstore.Store[Resource]
	Clock     *pkgstore.Clock
}

// New creates a new MemoryStore with empty state.
func New() *MemoryStore {
	return &MemoryStore{
		Resources: pkgstore.New[Resource]("res"), // Prefix for generated IDs (e.g., "res_abc123")
		Clock:     pkgstore.NewClock(),
	}
}

// stateSnapshot is the JSON-serializable state for admin endpoints.
// Include every resource collection so snapshots are complete.
type stateSnapshot struct {
	Resources map[string]Resource `json:"resources"`
}

// Snapshot returns the full state as a JSON-serializable value.
// Used by the admin /state endpoint.
func (s *MemoryStore) Snapshot() any {
	return stateSnapshot{
		Resources: s.Resources.Snapshot(),
	}
}

// LoadState replaces the full state from a JSON body.
// Used by admin /state/load and seed data loading.
func (s *MemoryStore) LoadState(data []byte) error {
	var snap stateSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return err
	}
	s.Resources.LoadSnapshot(snap.Resources)
	return nil
}

// Reset clears all state.
// Used by the admin /reset endpoint.
func (s *MemoryStore) Reset() {
	s.Resources.Reset()
	s.Clock.Reset()
}
