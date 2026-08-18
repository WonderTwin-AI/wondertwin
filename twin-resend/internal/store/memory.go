package store

import (
	"encoding/hex"
	"encoding/json"

	"github.com/wondertwin-ai/wondertwin/twinkit/sim"
	pkgstate "github.com/wondertwin-ai/wondertwin/twinkit/state"
)

// MemoryStore holds all Resend twin state in memory.
type MemoryStore struct {
	Emails     *pkgstate.Store[Email]
	Domains    *pkgstate.Store[Domain]
	APIKeys    *pkgstate.Store[APIKey]
	Audiences  *pkgstate.Store[Audience]
	Contacts   *pkgstate.Store[Contact]
	Broadcasts *pkgstate.Store[Broadcast]
	Clock      *pkgstate.Clock

	// Rand is the deterministic per-twin random source. Wired by
	// main.go (and test setups) to twin.Rand so handler-level
	// randomness shares the simulator's seed.
	Rand *sim.Rand
}

// RandHex returns a deterministic hex string of n bytes drawn from
// the store's Rand. Handlers use it in place of crypto/rand so
// token-shaped output is reproducible per seed.
func (s *MemoryStore) RandHex(n int) string {
	if s == nil || s.Rand == nil || n <= 0 {
		return ""
	}
	buf := make([]byte, n)
	for i := range buf {
		//nolint:gosec // G115: Intn(256) returns 0..255 by contract; the
		// conversion to byte cannot overflow.
		buf[i] = byte(s.Rand.Intn(256))
	}
	return hex.EncodeToString(buf)
}

// New creates a new MemoryStore with empty state.
func New() *MemoryStore {
	return &MemoryStore{
		Emails:     pkgstate.New[Email]("email"),
		Domains:    pkgstate.New[Domain]("domain"),
		APIKeys:    pkgstate.New[APIKey]("apikey"),
		Audiences:  pkgstate.New[Audience]("audience"),
		Contacts:   pkgstate.New[Contact]("contact"),
		Broadcasts: pkgstate.New[Broadcast]("broadcast"),
		Clock:      pkgstate.NewClock(),
	}
}

// stateSnapshot is the JSON-serializable state for admin endpoints.
type stateSnapshot struct {
	Emails     map[string]Email     `json:"emails"`
	Domains    map[string]Domain    `json:"domains,omitempty"`
	APIKeys    map[string]APIKey    `json:"api_keys,omitempty"`
	Audiences  map[string]Audience  `json:"audiences,omitempty"`
	Contacts   map[string]Contact   `json:"contacts,omitempty"`
	Broadcasts map[string]Broadcast `json:"broadcasts,omitempty"`
}

// Snapshot returns the full state as a JSON-serializable value.
func (s *MemoryStore) Snapshot() any {
	return stateSnapshot{
		Emails:     s.Emails.Snapshot(),
		Domains:    s.Domains.Snapshot(),
		APIKeys:    s.APIKeys.Snapshot(),
		Audiences:  s.Audiences.Snapshot(),
		Contacts:   s.Contacts.Snapshot(),
		Broadcasts: s.Broadcasts.Snapshot(),
	}
}

// LoadState replaces the full state from a JSON body.
func (s *MemoryStore) LoadState(data []byte) error {
	var snap stateSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return err
	}
	s.Emails.LoadSnapshot(snap.Emails)
	if snap.Domains != nil {
		s.Domains.LoadSnapshot(snap.Domains)
	}
	if snap.APIKeys != nil {
		s.APIKeys.LoadSnapshot(snap.APIKeys)
	}
	if snap.Audiences != nil {
		s.Audiences.LoadSnapshot(snap.Audiences)
	}
	if snap.Contacts != nil {
		s.Contacts.LoadSnapshot(snap.Contacts)
	}
	if snap.Broadcasts != nil {
		s.Broadcasts.LoadSnapshot(snap.Broadcasts)
	}
	return nil
}

// Reset clears all state.
func (s *MemoryStore) Reset() {
	s.Emails.Reset()
	s.Domains.Reset()
	s.APIKeys.Reset()
	s.Audiences.Reset()
	s.Contacts.Reset()
	s.Broadcasts.Reset()
	s.Clock.Reset()
}
