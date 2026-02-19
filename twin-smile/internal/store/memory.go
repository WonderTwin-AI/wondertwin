package store

import (
	"encoding/json"

	pkgstore "github.com/wondertwin-ai/wondertwin/twinkit/store"
)

// MemoryStore holds all Smile.io twin state in memory.
type MemoryStore struct {
	Customers   *pkgstore.Store[Customer]
	Redemptions *pkgstore.Store[Redemption]
	Clock       *pkgstore.Clock
}

// New creates a new MemoryStore with empty state.
func New() *MemoryStore {
	return &MemoryStore{
		Customers:   pkgstore.New[Customer]("cust"),
		Redemptions: pkgstore.New[Redemption]("red"),
		Clock:       pkgstore.NewClock(),
	}
}

// stateSnapshot is the JSON-serializable state for admin endpoints.
type stateSnapshot struct {
	Customers   map[string]Customer   `json:"customers"`
	Redemptions map[string]Redemption `json:"redemptions"`
}

// Snapshot returns the full state as a JSON-serializable value.
func (s *MemoryStore) Snapshot() any {
	return stateSnapshot{
		Customers:   s.Customers.Snapshot(),
		Redemptions: s.Redemptions.Snapshot(),
	}
}

// LoadState replaces the full state from a JSON body.
func (s *MemoryStore) LoadState(data []byte) error {
	var snap stateSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return err
	}
	if snap.Customers != nil {
		s.Customers.LoadSnapshot(snap.Customers)
	}
	if snap.Redemptions != nil {
		s.Redemptions.LoadSnapshot(snap.Redemptions)
	}
	return nil
}

// Reset clears all state.
func (s *MemoryStore) Reset() {
	s.Customers.Reset()
	s.Redemptions.Reset()
	s.Clock.Reset()
}

// FindRedemptionByIdempotencyKey returns the first redemption matching the given key, if any.
func (s *MemoryStore) FindRedemptionByIdempotencyKey(key string) *Redemption {
	if key == "" {
		return nil
	}
	_, items := s.Redemptions.FilterWithIDs(func(_ string, r Redemption) bool {
		return r.IdempotencyKey == key
	})
	if len(items) == 0 {
		return nil
	}
	return &items[0]
}
