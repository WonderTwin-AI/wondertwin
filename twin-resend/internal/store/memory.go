package store

import (
	"encoding/json"

	pkgstore "github.com/wondertwin-ai/wondertwin/pkg/store"
)

// MemoryStore holds all Resend twin state in memory.
type MemoryStore struct {
	Emails *pkgstore.Store[Email]
	Clock  *pkgstore.Clock
}

// New creates a new MemoryStore with empty state.
func New() *MemoryStore {
	return &MemoryStore{
		Emails: pkgstore.New[Email]("email"),
		Clock:  pkgstore.NewClock(),
	}
}

// stateSnapshot is the JSON-serializable state for admin endpoints.
type stateSnapshot struct {
	Emails map[string]Email `json:"emails"`
}

// Snapshot returns the full state as a JSON-serializable value.
func (s *MemoryStore) Snapshot() any {
	return stateSnapshot{
		Emails: s.Emails.Snapshot(),
	}
}

// LoadState replaces the full state from a JSON body.
func (s *MemoryStore) LoadState(data []byte) error {
	var snap stateSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return err
	}
	s.Emails.LoadSnapshot(snap.Emails)
	return nil
}

// Reset clears all state.
func (s *MemoryStore) Reset() {
	s.Emails.Reset()
	s.Clock.Reset()
}
