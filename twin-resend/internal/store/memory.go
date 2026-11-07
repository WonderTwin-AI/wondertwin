package store

import (
	"encoding/json"
	"sync"

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

	// Idempotency holds Idempotency-Key records, keyed by method, path and key.
	Idempotency *pkgstate.Store[IdempotencyRecord]

	Clock *pkgstate.Clock

	// Rand is the deterministic per-twin random source. Set it through Seed
	// so Reset can restore it; test setups may also assign it directly.
	Rand *sim.Rand

	// randMu guards swapping Rand on Reset against concurrent id draws.
	randMu sync.RWMutex
	// seed is the seed Rand was created from by Seed; nil when Rand was
	// assigned directly, in which case Reset leaves Rand as it is.
	seed *int64
}

// Seed gives the store a fresh random source from seed, which Reset restores,
// so a run after /admin/reset issues the same ids as a fresh start.
func (s *MemoryStore) Seed(seed int64) {
	s.randMu.Lock()
	defer s.randMu.Unlock()
	s.Rand = sim.NewRand(seed)
	s.seed = &seed
}

// New creates a new MemoryStore with empty state.
func New() *MemoryStore {
	return &MemoryStore{
		Emails:      pkgstate.New[Email]("email"),
		Domains:     pkgstate.New[Domain]("domain"),
		APIKeys:     pkgstate.New[APIKey]("apikey"),
		Audiences:   pkgstate.New[Audience]("audience"),
		Contacts:    pkgstate.New[Contact]("contact"),
		Broadcasts:  pkgstate.New[Broadcast]("broadcast"),
		Idempotency: pkgstate.New[IdempotencyRecord]("idem"),
		Clock:       pkgstate.NewClock(),
	}
}

// stateSnapshot is the JSON-serializable state for admin endpoints.
type stateSnapshot struct {
	Emails      map[string]Email             `json:"emails"`
	Domains     map[string]Domain            `json:"domains,omitempty"`
	APIKeys     map[string]APIKey            `json:"api_keys,omitempty"`
	Audiences   map[string]Audience          `json:"audiences,omitempty"`
	Contacts    map[string]Contact           `json:"contacts,omitempty"`
	Broadcasts  map[string]Broadcast         `json:"broadcasts,omitempty"`
	Idempotency map[string]IdempotencyRecord `json:"idempotency,omitempty"`
}

// Snapshot returns the full state as a JSON-serializable value.
func (s *MemoryStore) Snapshot() any {
	return stateSnapshot{
		Emails:      s.Emails.Snapshot(),
		Domains:     s.Domains.Snapshot(),
		APIKeys:     s.APIKeys.Snapshot(),
		Audiences:   s.Audiences.Snapshot(),
		Contacts:    s.Contacts.Snapshot(),
		Broadcasts:  s.Broadcasts.Snapshot(),
		Idempotency: s.Idempotency.Snapshot(),
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
	if snap.Idempotency != nil {
		// A request in flight when the state was taken never finishes in
		// this process; keeping its record would answer 409 for that key
		// for 24 hours.
		for k, rec := range snap.Idempotency {
			if rec.InFlight {
				delete(snap.Idempotency, k)
			}
		}
		s.Idempotency.LoadSnapshot(snap.Idempotency)
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
	s.Idempotency.Reset()
	s.Clock.Reset()
	s.randMu.Lock()
	if s.seed != nil {
		s.Rand = sim.NewRand(*s.seed)
	}
	s.randMu.Unlock()
}
