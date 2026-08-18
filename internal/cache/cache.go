package cache

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

// Store manages encrypted content cache files on disk.
type Store struct {
	Dir        string
	LicenseKey string
}

// NewStore creates a Store rooted at cacheDir using licenseKey for encryption.
func NewStore(cacheDir, licenseKey string) *Store {
	return &Store{Dir: cacheDir, LicenseKey: licenseKey}
}

// encPath returns the path to the encrypted payload file.
func (s *Store) encPath(twin, version string) string {
	return filepath.Join(s.Dir, "content", twin, version+".enc")
}

// metaPath returns the path to the metadata file.
func (s *Store) metaPath(twin, version string) string {
	return filepath.Join(s.Dir, "content", twin, version+".meta")
}

// Get returns the decrypted payload and metadata for a cached entry.
// If the cache is expired, it still returns the stale payload along with
// the metadata (caller can check meta.IsExpired() for graceful degradation).
// Returns os.ErrNotExist-wrapped error if no cache exists.
func (s *Store) Get(twin, version string) ([]byte, *CacheMeta, error) {
	meta, err := ReadMeta(s.metaPath(twin, version))
	if err != nil {
		return nil, nil, fmt.Errorf("cache miss for %s@%s: %w", twin, version, err)
	}

	enc, err := os.ReadFile(s.encPath(twin, version))
	if err != nil {
		return nil, nil, fmt.Errorf("reading cached payload for %s@%s: %w", twin, version, err)
	}

	key, err := DeriveKey(s.LicenseKey, twin, version)
	if err != nil {
		return nil, nil, err
	}

	payload, err := Decrypt(key, enc)
	if err != nil {
		return nil, nil, fmt.Errorf("decrypting cache for %s@%s: %w", twin, version, err)
	}

	return payload, meta, nil
}

// Put encrypts and writes the payload to the cache along with metadata.
func (s *Store) Put(twin, version string, payload []byte, ttlHours int) error {
	dir := filepath.Dir(s.encPath(twin, version))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("creating cache dir: %w", err)
	}
	if err := tightenPerms(dir, 0o700); err != nil {
		return fmt.Errorf("tightening cache dir perms: %w", err)
	}

	key, err := DeriveKey(s.LicenseKey, twin, version)
	if err != nil {
		return err
	}

	enc, err := Encrypt(key, payload)
	if err != nil {
		return fmt.Errorf("encrypting payload for %s@%s: %w", twin, version, err)
	}

	if err := os.WriteFile(s.encPath(twin, version), enc, 0o600); err != nil {
		return fmt.Errorf("writing cached payload: %w", err)
	}

	now := time.Now().UTC()
	meta := &CacheMeta{
		Twin:      twin,
		Version:   version,
		FetchedAt: now,
		TTLHours:  ttlHours,
		ExpiresAt: now.Add(time.Duration(ttlHours) * time.Hour),
	}
	if err := WriteMeta(s.metaPath(twin, version), meta); err != nil {
		return fmt.Errorf("writing cache meta: %w", err)
	}

	return nil
}

// IsValid returns true if a non-expired cache entry exists.
func (s *Store) IsValid(twin, version string) bool {
	meta, err := ReadMeta(s.metaPath(twin, version))
	if err != nil {
		return false
	}
	return !meta.IsExpired()
}

// Clear removes cached content. If both twin and version are empty, all
// content cache is removed. If only version is empty, all versions of the
// twin are removed.
func (s *Store) Clear(twin, version string) error {
	if twin == "" && version == "" {
		return os.RemoveAll(filepath.Join(s.Dir, "content"))
	}
	if version == "" {
		return os.RemoveAll(filepath.Join(s.Dir, "content", twin))
	}
	os.Remove(s.encPath(twin, version))
	os.Remove(s.metaPath(twin, version))
	return nil
}

// tightenPerms drops mode bits on a path that already exists. MkdirAll is a
// no-op on an existing directory and WriteFile/OpenFile do not chmod an
// existing file, so a path created by an older CLI version keeps its original,
// looser mode until something re-tightens it. Mirrors the follow-up chmod in
// internal/config. No-op on Windows, where Unix mode bits are inert.
func tightenPerms(path string, mode os.FileMode) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	//nolint:gosec // G302: callers pass 0700 for directories, which G302 reads
	// as a file mode; it is the tightest useful mode for a directory.
	return os.Chmod(path, mode)
}
