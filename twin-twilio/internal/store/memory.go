package store

import (
	"encoding/json"

	pkgstate "github.com/wondertwin-ai/wondertwin/twinkit/state"
)

// MemoryStore holds all Twilio twin state in memory.
type MemoryStore struct {
	Messages       *pkgstate.Store[Message]
	Verifications  *pkgstate.Store[Verification]
	VerifyServices *pkgstate.Store[VerifyService]
	Clock          *pkgstate.Clock
	OTPTTLSeconds  int // verification code TTL, default 600 (10 min)
}

// New creates a new MemoryStore with empty state.
func New() *MemoryStore {
	return &MemoryStore{
		Messages:       pkgstate.New[Message]("SM"),
		Verifications:  pkgstate.New[Verification]("VE"),
		VerifyServices: pkgstate.New[VerifyService]("VA"),
		Clock:          pkgstate.NewClock(),
		OTPTTLSeconds:  600,
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
		Messages       map[string]Message       `json:"messages"`
		Verifications  map[string]Verification  `json:"verifications"`
		VerifyServices map[string]VerifyService `json:"verify_services,omitempty"`
	}{
		Messages:       s.Messages.Snapshot(),
		Verifications:  s.Verifications.Snapshot(),
		VerifyServices: s.VerifyServices.Snapshot(),
	}
}

// LoadState replaces the full state from a JSON body.
func (s *MemoryStore) LoadState(data []byte) error {
	var snap struct {
		Messages       map[string]Message       `json:"messages"`
		Verifications  map[string]Verification  `json:"verifications"`
		VerifyServices map[string]VerifyService `json:"verify_services"`
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
	if snap.VerifyServices != nil {
		s.VerifyServices.LoadSnapshot(snap.VerifyServices)
	}
	return nil
}

// Reset clears all state.
func (s *MemoryStore) Reset() {
	s.Messages.Reset()
	s.Verifications.Reset()
	s.VerifyServices.Reset()
	s.Clock.Reset()
}
