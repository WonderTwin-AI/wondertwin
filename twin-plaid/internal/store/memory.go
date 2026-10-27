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
	Clock           *pkgstate.Clock
	Items           *pkgstate.Store[Item]
	Accounts        *pkgstate.Store[Account]
	Institutions    *pkgstate.Store[Institution]
	PublicTokens    *pkgstate.Store[PublicToken]
	AccessTokens    *pkgstate.Store[AccessToken]
	LinkTokens      *pkgstate.Store[LinkToken]
	ProcessorTokens *pkgstate.Store[ProcessorToken]
	Transactions    *pkgstate.Store[Transaction]
	// ChangeLogs is each Item's transaction history, by item id; logMu guards it.
	ChangeLogs map[string][]Change
	// Rand is the deterministic random source. main.go replaces the default
	// with twin.Rand, the source the rest of the emulator draws from.
	Rand *sim.Rand

	tokenMu sync.Mutex
	itemMu  sync.Mutex
	logMu   sync.Mutex

	// defaults restores the fixtures a fresh store starts with; see SetDefaults.
	defaults func(*MemoryStore)
	// onReset runs after every Reset, for state that lives outside the store
	// (the webhook dispatcher); see OnReset.
	onReset []func()
}

// New creates an empty MemoryStore.
func New() *MemoryStore {
	return &MemoryStore{
		Clock:           pkgstate.NewClock(),
		Items:           pkgstate.New[Item]("item"),
		Accounts:        pkgstate.New[Account]("acct"),
		Institutions:    pkgstate.New[Institution]("ins"),
		PublicTokens:    pkgstate.New[PublicToken]("public"),
		AccessTokens:    pkgstate.New[AccessToken]("access"),
		LinkTokens:      pkgstate.New[LinkToken]("link"),
		ProcessorTokens: pkgstate.New[ProcessorToken]("processor"),
		Transactions:    pkgstate.New[Transaction]("txn"),
		ChangeLogs:      map[string][]Change{},
		Rand:            sim.DefaultRand(),
	}
}

// snapshot is the JSON form of the store. A field a state file leaves out
// leaves that store as it is.
type snapshot struct {
	Items           map[string]Item           `json:"items,omitempty"`
	Accounts        map[string]Account        `json:"accounts,omitempty"`
	Institutions    map[string]Institution    `json:"institutions,omitempty"`
	PublicTokens    map[string]PublicToken    `json:"public_tokens,omitempty"`
	AccessTokens    map[string]AccessToken    `json:"access_tokens,omitempty"`
	LinkTokens      map[string]LinkToken      `json:"link_tokens,omitempty"`
	ProcessorTokens map[string]ProcessorToken `json:"processor_tokens,omitempty"`
	Transactions    map[string]Transaction    `json:"transactions,omitempty"`
	ChangeLogs      map[string][]Change       `json:"change_logs,omitempty"`
}

// Snapshot returns the full state for GET /admin/state.
func (s *MemoryStore) Snapshot() any {
	return snapshot{
		Items:           s.Items.Snapshot(),
		Accounts:        s.Accounts.Snapshot(),
		Institutions:    s.Institutions.Snapshot(),
		PublicTokens:    s.PublicTokens.Snapshot(),
		AccessTokens:    s.AccessTokens.Snapshot(),
		LinkTokens:      s.LinkTokens.Snapshot(),
		ProcessorTokens: s.ProcessorTokens.Snapshot(),
		Transactions:    s.Transactions.Snapshot(),
		ChangeLogs:      s.changeLogsCopy(),
	}
}

// LoadState loads a snapshot or seed file for POST /admin/state.
func (s *MemoryStore) LoadState(data []byte) error {
	var snap snapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return err
	}
	if err := snap.validateTransactions(); err != nil {
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
	if snap.Transactions != nil {
		s.Transactions.LoadSnapshot(snap.Transactions)
	}
	if snap.ChangeLogs != nil {
		s.logMu.Lock()
		s.ChangeLogs = snap.ChangeLogs
		s.logMu.Unlock()
	}
	if snap.ProcessorTokens != nil {
		s.ProcessorTokens.LoadSnapshot(snap.ProcessorTokens)
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

// OnReset registers fn to run at the end of every Reset, after the defaults are
// reloaded. State the store does not own, such as queued webhooks, clears here
// so /admin/reset returns the whole emulator to a fresh start.
func (s *MemoryStore) OnReset(fn func()) {
	s.onReset = append(s.onReset, fn)
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
	s.ProcessorTokens.Reset()
	s.Transactions.Reset()
	s.logMu.Lock()
	s.ChangeLogs = map[string][]Change{}
	s.logMu.Unlock()
	if s.defaults != nil {
		s.defaults(s)
	}
	for _, fn := range s.onReset {
		fn()
	}
}

// changeLogsCopy is a copy of every Item's log, for the snapshot.
func (s *MemoryStore) changeLogsCopy() map[string][]Change {
	s.logMu.Lock()
	defer s.logMu.Unlock()
	out := make(map[string][]Change, len(s.ChangeLogs))
	for k, v := range s.ChangeLogs {
		out[k] = append([]Change(nil), v...)
	}
	return out
}

// validateTransactions rejects a snapshot whose transaction bodies would break
// the SDK, before any of it is loaded.
func (snap snapshot) validateTransactions() error {
	for _, t := range snap.Transactions {
		if err := t.Body.Validate(); err != nil {
			return err
		}
	}
	for _, log := range snap.ChangeLogs {
		for _, c := range log {
			if c.Body == nil {
				continue
			}
			if err := c.Body.Validate(); err != nil {
				return err
			}
		}
	}
	return nil
}
