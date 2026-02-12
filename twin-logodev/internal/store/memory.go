package store

import (
	"encoding/json"
	"time"

	pkgstore "github.com/wondertwin-ai/wondertwin/pkg/store"
)

// MemoryStore holds all logo twin state.
type MemoryStore struct {
	Requests *pkgstore.Store[LogoRequest]
	// CustomLogos maps domain -> SVG content for specific test domains
	CustomLogos map[string][]byte
	Clock       *pkgstore.Clock
}

// New creates a new MemoryStore.
func New() *MemoryStore {
	return &MemoryStore{
		Requests:    pkgstore.New[LogoRequest]("logo"),
		CustomLogos: make(map[string][]byte),
		Clock:       pkgstore.NewClock(),
	}
}

// RecordRequest logs a logo request.
func (s *MemoryStore) RecordRequest(domain string, size int, format string, greyscale bool) {
	id := s.Requests.NextID()
	s.Requests.Set(id, LogoRequest{
		Domain:    domain,
		Size:      size,
		Format:    format,
		Greyscale: greyscale,
		Timestamp: time.Now(),
	})
}

type stateSnapshot struct {
	Requests    map[string]LogoRequest `json:"requests"`
	CustomLogos map[string]string      `json:"custom_logos,omitempty"` // domain -> base64 SVG
}

func (s *MemoryStore) Snapshot() any {
	return stateSnapshot{
		Requests: s.Requests.Snapshot(),
	}
}

func (s *MemoryStore) LoadState(data []byte) error {
	var snap stateSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return err
	}
	s.Requests.LoadSnapshot(snap.Requests)
	return nil
}

func (s *MemoryStore) Reset() {
	s.Requests.Reset()
	s.CustomLogos = make(map[string][]byte)
	s.Clock.Reset()
}
