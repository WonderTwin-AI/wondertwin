// Package store holds the Xero app emulator's state in memory.
package store

import (
	"encoding/json"

	"github.com/wondertwin-ai/wondertwin/twinkit/sim"
	pkgstate "github.com/wondertwin-ai/wondertwin/twinkit/state"
)

// MemoryStore holds all Xero emulator state in memory. Each slice that adds a
// record type adds its store here, with its snapshot, load and reset.
type MemoryStore struct {
	Clock        *pkgstate.Clock
	AccessTokens *pkgstate.Store[AccessToken]
	// Rand is the deterministic random source behind NewID. New seeds it so a
	// bare store works; main.go replaces it with twin.Rand.
	Rand *sim.Rand
}

// New creates an empty MemoryStore.
func New() *MemoryStore {
	return &MemoryStore{
		Clock:        pkgstate.NewClock(),
		AccessTokens: pkgstate.New[AccessToken]("at"),
		Rand:         sim.DefaultRand(),
	}
}

// snapshot is the JSON form of the store. A field a state file leaves out
// leaves that store as it is.
type snapshot struct {
	AccessTokens map[string]AccessToken `json:"access_tokens"`
}

// Snapshot returns the full state for GET /admin/state.
func (s *MemoryStore) Snapshot() any {
	return snapshot{AccessTokens: s.AccessTokens.Snapshot()}
}

// LoadState loads a snapshot or seed file for POST /admin/state.
func (s *MemoryStore) LoadState(data []byte) error {
	var snap snapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return err
	}
	if snap.AccessTokens != nil {
		s.AccessTokens.LoadSnapshot(snap.AccessTokens)
	}
	return nil
}

// Reset clears every store and the simulated clock.
func (s *MemoryStore) Reset() {
	s.Clock.Reset()
	s.AccessTokens.Reset()
}
