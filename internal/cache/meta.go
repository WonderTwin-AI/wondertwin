package cache

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// CacheMeta stores metadata about a cached content payload.
type CacheMeta struct {
	Twin            string    `json:"twin"`
	Version         string    `json:"version"`
	FetchedAt       time.Time `json:"fetched_at"`
	TTLHours        int       `json:"ttl_hours"`
	ExpiresAt       time.Time `json:"expires_at"`
	TaxonomyVersion string    `json:"taxonomy_version,omitempty"`
}

// IsExpired returns true if the cache entry has passed its expiration time.
func (m *CacheMeta) IsExpired() bool {
	return time.Now().After(m.ExpiresAt)
}

// ReadMeta reads a CacheMeta from the given file path.
func ReadMeta(path string) (*CacheMeta, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading cache meta: %w", err)
	}

	var meta CacheMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("parsing cache meta: %w", err)
	}
	return &meta, nil
}

// WriteMeta writes a CacheMeta to the given file path.
func WriteMeta(path string, meta *CacheMeta) error {
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling cache meta: %w", err)
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o600)
}
