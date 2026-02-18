// Package lockfile reads and writes wondertwin-lock.json for reproducible twin installs.
package lockfile

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const Filename = "wondertwin-lock.json"

// LockFile represents the contents of wondertwin-lock.json.
type LockFile struct {
	GeneratedAt       time.Time             `json:"generated_at"`
	RegistryFetchedAt time.Time             `json:"registry_fetched_at"`
	Twins             map[string]LockedTwin `json:"twins"`
}

// LockedTwin captures the resolved state of a single twin.
type LockedTwin struct {
	Version      string `json:"version"`
	ResolvedFrom string `json:"resolved_from"`
	SDKPackage   string `json:"sdk_package,omitempty"`
	SDKVersion   string `json:"sdk_version,omitempty"`
	Checksum     string `json:"checksum,omitempty"`
	BinaryURL    string `json:"binary_url,omitempty"`
}

// Load reads and parses a lock file from the given directory.
func Load(dir string) (*LockFile, error) {
	path := filepath.Join(dir, Filename)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var lf LockFile
	if err := json.Unmarshal(data, &lf); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", Filename, err)
	}
	return &lf, nil
}

// Save writes the lock file to the given directory with indented JSON.
func Save(dir string, lf *LockFile) error {
	data, err := json.MarshalIndent(lf, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling %s: %w", Filename, err)
	}
	data = append(data, '\n')
	path := filepath.Join(dir, Filename)
	return os.WriteFile(path, data, 0o644)
}

// Exists returns true if a lock file exists in the given directory.
func Exists(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, Filename))
	return err == nil
}
