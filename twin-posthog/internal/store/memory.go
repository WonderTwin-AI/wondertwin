package store

import (
	"encoding/json"
	"sync"

	pkgstore "github.com/wondertwin-ai/wondertwin/twinkit/store"
)

// MemoryStore holds all PostHog twin state in memory.
type MemoryStore struct {
	Events  *pkgstore.Store[CapturedEvent]
	Persons *pkgstore.Store[Person]
	Aliases *pkgstore.Store[AliasMapping]
	Groups  *pkgstore.Store[Group]
	Clock   *pkgstore.Clock

	mu           sync.RWMutex
	FeatureFlags map[string]FeatureFlag
}

// New creates a new MemoryStore with empty state.
func New() *MemoryStore {
	return &MemoryStore{
		Events:       pkgstore.New[CapturedEvent]("evt"),
		Persons:      pkgstore.New[Person]("per"),
		Aliases:      pkgstore.New[AliasMapping]("als"),
		Groups:       pkgstore.New[Group]("grp"),
		Clock:        pkgstore.NewClock(),
		FeatureFlags: make(map[string]FeatureFlag),
	}
}

// GetFeatureFlags returns a copy of all feature flags.
func (s *MemoryStore) GetFeatureFlags() map[string]FeatureFlag {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]FeatureFlag, len(s.FeatureFlags))
	for k, v := range s.FeatureFlags {
		out[k] = v
	}
	return out
}

// SetFeatureFlag sets or updates a feature flag.
func (s *MemoryStore) SetFeatureFlag(flag FeatureFlag) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.FeatureFlags[flag.Key] = flag
}

// SetFeatureFlags replaces all feature flags.
func (s *MemoryStore) SetFeatureFlags(flags []FeatureFlag) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.FeatureFlags = make(map[string]FeatureFlag, len(flags))
	for _, f := range flags {
		s.FeatureFlags[f.Key] = f
	}
}

// stateSnapshot is the JSON-serializable state for admin endpoints.
type stateSnapshot struct {
	Events       map[string]CapturedEvent `json:"events"`
	Persons      map[string]Person        `json:"persons"`
	Aliases      map[string]AliasMapping  `json:"aliases"`
	Groups       map[string]Group         `json:"groups"`
	FeatureFlags map[string]FeatureFlag   `json:"feature_flags"`
}

// Snapshot returns the full state as a JSON-serializable value.
func (s *MemoryStore) Snapshot() any {
	return stateSnapshot{
		Events:       s.Events.Snapshot(),
		Persons:      s.Persons.Snapshot(),
		Aliases:      s.Aliases.Snapshot(),
		Groups:       s.Groups.Snapshot(),
		FeatureFlags: s.GetFeatureFlags(),
	}
}

// LoadState replaces the full state from a JSON body.
func (s *MemoryStore) LoadState(data []byte) error {
	var snap stateSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return err
	}
	s.Events.LoadSnapshot(snap.Events)
	s.Persons.LoadSnapshot(snap.Persons)
	s.Aliases.LoadSnapshot(snap.Aliases)
	s.Groups.LoadSnapshot(snap.Groups)
	s.mu.Lock()
	defer s.mu.Unlock()
	if snap.FeatureFlags != nil {
		s.FeatureFlags = snap.FeatureFlags
	}
	return nil
}

// Reset clears all state.
func (s *MemoryStore) Reset() {
	s.Events.Reset()
	s.Persons.Reset()
	s.Aliases.Reset()
	s.Groups.Reset()
	s.Clock.Reset()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.FeatureFlags = make(map[string]FeatureFlag)
}
