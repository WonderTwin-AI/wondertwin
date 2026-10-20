// Package store holds the Plaid app emulator's state in memory.
package store

import (
	"encoding/json"

	"github.com/wondertwin-ai/wondertwin/twinkit/sim"
	pkgstate "github.com/wondertwin-ai/wondertwin/twinkit/state"
)

// MemoryStore holds all Plaid emulator state in memory.
type MemoryStore struct {
	Clock        *pkgstate.Clock
	Items        *pkgstate.Store[Item]
	Accounts     *pkgstate.Store[Account]
	Institutions *pkgstate.Store[Institution]
	// Rand is the deterministic random source. main.go replaces the default
	// with twin.Rand, the source the rest of the emulator draws from.
	Rand *sim.Rand
}

// New creates an empty MemoryStore.
func New() *MemoryStore {
	return &MemoryStore{
		Clock:        pkgstate.NewClock(),
		Items:        pkgstate.New[Item]("item"),
		Accounts:     pkgstate.New[Account]("acct"),
		Institutions: pkgstate.New[Institution]("ins"),
		Rand:         sim.DefaultRand(),
	}
}

// snapshot is the JSON form of the store. A field a state file leaves out
// leaves that store as it is.
type snapshot struct {
	Items        map[string]Item        `json:"items,omitempty"`
	Accounts     map[string]Account     `json:"accounts,omitempty"`
	Institutions map[string]Institution `json:"institutions,omitempty"`
}

// Snapshot returns the full state for GET /admin/state.
func (s *MemoryStore) Snapshot() any {
	return snapshot{
		Items:        s.Items.Snapshot(),
		Accounts:     s.Accounts.Snapshot(),
		Institutions: s.Institutions.Snapshot(),
	}
}

// LoadState loads a snapshot or seed file for POST /admin/state.
func (s *MemoryStore) LoadState(data []byte) error {
	var snap snapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return err
	}
	if snap.Items != nil {
		s.Items.LoadSnapshot(snap.Items)
	}
	if snap.Accounts != nil {
		s.Accounts.LoadSnapshot(snap.Accounts)
	}
	if snap.Institutions != nil {
		s.Institutions.LoadSnapshot(snap.Institutions)
	}
	return nil
}

// Reset clears every store and the simulated clock.
func (s *MemoryStore) Reset() {
	s.Clock.Reset()
	s.Items.Reset()
	s.Accounts.Reset()
	s.Institutions.Reset()
}
