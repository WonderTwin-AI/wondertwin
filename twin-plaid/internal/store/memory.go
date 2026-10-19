// Package store holds the Plaid app emulator's state in memory.
package store

import (
	"encoding/json"

	"github.com/wondertwin-ai/wondertwin/twinkit/sim"
	pkgstate "github.com/wondertwin-ai/wondertwin/twinkit/state"
)

// MemoryStore holds all Plaid emulator state in memory. It carries no record
// types yet; each slice that adds one adds its store here, with its snapshot,
// load and reset.
type MemoryStore struct {
	Clock *pkgstate.Clock
	// Rand is the deterministic random source. main.go wires it to twin.Rand;
	// left nil, a draw through it yields nothing.
	Rand *sim.Rand
}

// New creates an empty MemoryStore.
func New() *MemoryStore {
	return &MemoryStore{Clock: pkgstate.NewClock()}
}

// snapshot is the JSON form of the store. A field a state file leaves out
// leaves that store as it is.
type snapshot struct{}

// Snapshot returns the full state for GET /admin/state.
func (s *MemoryStore) Snapshot() any {
	return snapshot{}
}

// LoadState loads a snapshot or seed file for POST /admin/state.
func (s *MemoryStore) LoadState(data []byte) error {
	var snap snapshot
	return json.Unmarshal(data, &snap)
}

// Reset clears every store and the simulated clock.
func (s *MemoryStore) Reset() {
	s.Clock.Reset()
}
