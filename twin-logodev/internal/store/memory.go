package store

import (
	"encoding/base64"
	"encoding/json"
	"time"

	pkgstore "github.com/wondertwin-ai/wondertwin/twinkit/store"
)

// MemoryStore holds all logo twin state.
type MemoryStore struct {
	Requests    *pkgstore.Store[LogoRequest]
	CustomLogos map[string]CustomLogo
	Clock       *pkgstore.Clock
}

// New creates a new MemoryStore.
func New() *MemoryStore {
	return &MemoryStore{
		Requests:    pkgstore.New[LogoRequest]("logo"),
		CustomLogos: make(map[string]CustomLogo),
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

type logoSnapshot struct {
	ContentType string `json:"content_type"`
	Data        string `json:"data"` // base64-encoded
}

type stateSnapshot struct {
	Requests    map[string]LogoRequest    `json:"requests"`
	CustomLogos map[string]logoSnapshot   `json:"custom_logos,omitempty"`
}

func (s *MemoryStore) Snapshot() any {
	logos := make(map[string]logoSnapshot, len(s.CustomLogos))
	for domain, logo := range s.CustomLogos {
		logos[domain] = logoSnapshot{
			ContentType: logo.ContentType,
			Data:        base64.StdEncoding.EncodeToString(logo.Data),
		}
	}
	return stateSnapshot{
		Requests:    s.Requests.Snapshot(),
		CustomLogos: logos,
	}
}

func (s *MemoryStore) LoadState(data []byte) error {
	var snap stateSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return err
	}
	s.Requests.LoadSnapshot(snap.Requests)
	for domain, logo := range snap.CustomLogos {
		decoded, err := base64.StdEncoding.DecodeString(logo.Data)
		if err != nil {
			return err
		}
		ct := logo.ContentType
		if ct == "" {
			ct = "image/svg+xml"
		}
		s.CustomLogos[domain] = CustomLogo{ContentType: ct, Data: decoded}
	}
	return nil
}

func (s *MemoryStore) Reset() {
	s.Requests.Reset()
	s.CustomLogos = make(map[string]CustomLogo)
	s.Clock.Reset()
}
