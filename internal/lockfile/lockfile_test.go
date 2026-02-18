package lockfile

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	now := time.Now().UTC().Truncate(time.Second)

	lf := &LockFile{
		GeneratedAt:       now,
		RegistryFetchedAt: now,
		Twins: map[string]LockedTwin{
			"stripe": {
				Version:      "0.1.0",
				ResolvedFrom: "latest",
				SDKPackage:   "github.com/stripe/stripe-go",
				SDKVersion:   "81.0.0",
				Checksum:     "sha256:abc123",
				BinaryURL:    "https://example.com/twin-stripe-darwin-arm64",
			},
		},
	}

	if err := Save(dir, lf); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Verify file exists
	path := filepath.Join(dir, Filename)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("lock file not created: %v", err)
	}

	// Verify it's valid JSON
	data, _ := os.ReadFile(path)
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	// Load it back
	loaded, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if len(loaded.Twins) != 1 {
		t.Fatalf("expected 1 twin, got %d", len(loaded.Twins))
	}

	stripe := loaded.Twins["stripe"]
	if stripe.Version != "0.1.0" {
		t.Errorf("version = %q, want %q", stripe.Version, "0.1.0")
	}
	if stripe.ResolvedFrom != "latest" {
		t.Errorf("resolved_from = %q, want %q", stripe.ResolvedFrom, "latest")
	}
	if stripe.SDKPackage != "github.com/stripe/stripe-go" {
		t.Errorf("sdk_package = %q, want %q", stripe.SDKPackage, "github.com/stripe/stripe-go")
	}
	if stripe.Checksum != "sha256:abc123" {
		t.Errorf("checksum = %q, want %q", stripe.Checksum, "sha256:abc123")
	}
	if stripe.BinaryURL != "https://example.com/twin-stripe-darwin-arm64" {
		t.Errorf("binary_url = %q, want %q", stripe.BinaryURL, "https://example.com/twin-stripe-darwin-arm64")
	}
}

func TestLoadMissing(t *testing.T) {
	dir := t.TempDir()
	_, err := Load(dir)
	if err == nil {
		t.Fatal("expected error loading missing lock file")
	}
}

func TestExists(t *testing.T) {
	dir := t.TempDir()

	if Exists(dir) {
		t.Fatal("Exists should return false for empty dir")
	}

	lf := &LockFile{
		GeneratedAt: time.Now().UTC(),
		Twins:       map[string]LockedTwin{},
	}
	if err := Save(dir, lf); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if !Exists(dir) {
		t.Fatal("Exists should return true after Save")
	}
}

func TestSaveOverwrite(t *testing.T) {
	dir := t.TempDir()
	now := time.Now().UTC()

	lf1 := &LockFile{
		GeneratedAt: now,
		Twins: map[string]LockedTwin{
			"stripe": {Version: "0.1.0", ResolvedFrom: "latest"},
		},
	}
	if err := Save(dir, lf1); err != nil {
		t.Fatalf("Save lf1: %v", err)
	}

	lf2 := &LockFile{
		GeneratedAt: now,
		Twins: map[string]LockedTwin{
			"stripe": {Version: "0.2.0", ResolvedFrom: "latest"},
			"twilio": {Version: "0.1.0", ResolvedFrom: "latest"},
		},
	}
	if err := Save(dir, lf2); err != nil {
		t.Fatalf("Save lf2: %v", err)
	}

	loaded, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if len(loaded.Twins) != 2 {
		t.Fatalf("expected 2 twins, got %d", len(loaded.Twins))
	}
	if loaded.Twins["stripe"].Version != "0.2.0" {
		t.Errorf("stripe version = %q, want %q", loaded.Twins["stripe"].Version, "0.2.0")
	}
}

func TestOmitEmptyFields(t *testing.T) {
	dir := t.TempDir()

	lf := &LockFile{
		GeneratedAt: time.Now().UTC(),
		Twins: map[string]LockedTwin{
			"stripe": {Version: "0.1.0", ResolvedFrom: "latest"},
		},
	}
	if err := Save(dir, lf); err != nil {
		t.Fatalf("Save: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(dir, Filename))
	var raw map[string]interface{}
	json.Unmarshal(data, &raw)

	twins := raw["twins"].(map[string]interface{})
	stripe := twins["stripe"].(map[string]interface{})

	// sdk_package, sdk_version, checksum, binary_url should be omitted when empty
	if _, ok := stripe["sdk_package"]; ok {
		t.Error("expected sdk_package to be omitted when empty")
	}
	if _, ok := stripe["checksum"]; ok {
		t.Error("expected checksum to be omitted when empty")
	}
}
