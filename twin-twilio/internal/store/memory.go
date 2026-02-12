package store

import (
	"encoding/json"

	pkgstore "github.com/wondertwin-ai/wondertwin/pkg/store"
)

// MemoryStore holds all Twilio twin state in memory.
type MemoryStore struct {
	Messages      *pkgstore.Store[Message]
	Verifications *pkgstore.Store[Verification]
	Clock         *pkgstore.Clock
	OTPTTLSeconds int // verification code TTL, default 600 (10 min)
}

// New creates a new MemoryStore with empty state.
func New() *MemoryStore {
	return &MemoryStore{
		Messages:      pkgstore.New[Message]("SM"),
		Verifications: pkgstore.New[Verification]("VE"),
		Clock:         pkgstore.NewClock(),
		OTPTTLSeconds: 600,
	}
}

// stateSnapshot is the JSON-serializable state for admin endpoints.
type stateSnapshot struct {
	Messages      map[string]Verification `json:"messages"`
	Verifications map[string]Verification `json:"verifications"`
}

// Snapshot returns the full state as a JSON-serializable value.
func (s *MemoryStore) Snapshot() any {
	return struct {
		Messages      map[string]Message      `json:"messages"`
		Verifications map[string]Verification `json:"verifications"`
	}{
		Messages:      s.Messages.Snapshot(),
		Verifications: s.Verifications.Snapshot(),
	}
}

// LoadState replaces the full state from a JSON body.
func (s *MemoryStore) LoadState(data []byte) error {
	var snap struct {
		Messages      map[string]Message      `json:"messages"`
		Verifications map[string]Verification `json:"verifications"`
	}
	if err := json.Unmarshal(data, &snap); err != nil {
		return err
	}
	if snap.Messages != nil {
		s.Messages.LoadSnapshot(snap.Messages)
	}
	if snap.Verifications != nil {
		s.Verifications.LoadSnapshot(snap.Verifications)
	}
	return nil
}

// Reset clears all state.
func (s *MemoryStore) Reset() {
	s.Messages.Reset()
	s.Verifications.Reset()
	s.Clock.Reset()
}
