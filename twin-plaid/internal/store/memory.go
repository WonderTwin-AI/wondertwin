// Package store holds the Plaid app emulator's state in memory.
package store

import (
	"encoding/json"
	"sync"

	"github.com/wondertwin-ai/wondertwin/twinkit/sim"
	pkgstate "github.com/wondertwin-ai/wondertwin/twinkit/state"
)

// MemoryStore holds all Plaid emulator state in memory.
type MemoryStore struct {
	Clock        *pkgstate.Clock
	Items        *pkgstate.Store[Item]
	Accounts     *pkgstate.Store[Account]
	Institutions *pkgstate.Store[Institution]
	PublicTokens *pkgstate.Store[PublicToken]
	AccessTokens *pkgstate.Store[AccessToken]
	LinkTokens   *pkgstate.Store[LinkToken]
	// Rand is the deterministic random source. main.go replaces the default
	// with twin.Rand, the source the rest of the emulator draws from.
	Rand *sim.Rand

	tokenMu sync.Mutex

	// defaults restores the fixtures a fresh store starts with; see SetDefaults.
	defaults func(*MemoryStore)
}

// New creates an empty MemoryStore.
func New() *MemoryStore {
	return &MemoryStore{
		Clock:        pkgstate.NewClock(),
		Items:        pkgstate.New[Item]("item"),
		Accounts:     pkgstate.New[Account]("acct"),
		Institutions: pkgstate.New[Institution]("ins"),
		PublicTokens: pkgstate.New[PublicToken]("public"),
		AccessTokens: pkgstate.New[AccessToken]("access"),
		LinkTokens:   pkgstate.New[LinkToken]("link"),
		Rand:         sim.DefaultRand(),
	}
}

// snapshot is the JSON form of the store. A field a state file leaves out
// leaves that store as it is.
type snapshot struct {
	Items        map[string]Item        `json:"items,omitempty"`
	Accounts     map[string]Account     `json:"accounts,omitempty"`
	Institutions map[string]Institution `json:"institutions,omitempty"`
	PublicTokens map[string]PublicToken `json:"public_tokens,omitempty"`
	AccessTokens map[string]AccessToken `json:"access_tokens,omitempty"`
	LinkTokens   map[string]LinkToken   `json:"link_tokens,omitempty"`
}

// Snapshot returns the full state for GET /admin/state.
func (s *MemoryStore) Snapshot() any {
	return snapshot{
		Items:        s.Items.Snapshot(),
		Accounts:     s.Accounts.Snapshot(),
		Institutions: s.Institutions.Snapshot(),
		PublicTokens: s.PublicTokens.Snapshot(),
		AccessTokens: s.AccessTokens.Snapshot(),
		LinkTokens:   s.LinkTokens.Snapshot(),
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
	if snap.PublicTokens != nil {
		s.PublicTokens.LoadSnapshot(snap.PublicTokens)
	}
	if snap.AccessTokens != nil {
		s.AccessTokens.LoadSnapshot(snap.AccessTokens)
	}
	if snap.LinkTokens != nil {
		s.LinkTokens.LoadSnapshot(snap.LinkTokens)
	}
	return nil
}

// SetDefaults registers the function that loads the fixtures a fresh store
// starts with, and runs it now. Reset runs it again, so a reset returns to the
// starting state, not to an empty one. The fixtures live in a package that
// imports this one, so they are handed in here.
func (s *MemoryStore) SetDefaults(fn func(*MemoryStore)) {
	s.defaults = fn
	fn(s)
}

// Reset clears every store and the simulated clock, then reloads the defaults.
func (s *MemoryStore) Reset() {
	s.Clock.Reset()
	s.Items.Reset()
	s.Accounts.Reset()
	s.Institutions.Reset()
	s.PublicTokens.Reset()
	s.AccessTokens.Reset()
	s.LinkTokens.Reset()
	if s.defaults != nil {
		s.defaults(s)
	}
}
