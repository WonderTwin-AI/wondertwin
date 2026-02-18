package lockfile

import (
	"encoding/json"
	"os"
	"testing"
)

// TestContractGoldenLockFileParseable verifies that the golden lock file can be
// round-tripped through the lockfile package. If anyone changes struct tags or
// field names, this breaks.
func TestContractGoldenLockFileParseable(t *testing.T) {
	data, err := os.ReadFile("testdata/wondertwin-lock.json")
	if err != nil {
		t.Fatalf("reading golden lock file: %v", err)
	}

	var lf LockFile
	if err := json.Unmarshal(data, &lf); err != nil {
		t.Fatalf("parsing golden lock file: %v", err)
	}

	if lf.GeneratedAt.IsZero() {
		t.Error("generated_at is zero")
	}
	if lf.RegistryFetchedAt.IsZero() {
		t.Error("registry_fetched_at is zero")
	}
	if len(lf.Twins) != 2 {
		t.Fatalf("expected 2 twins, got %d", len(lf.Twins))
	}

	// Verify stripe entry
	stripe, ok := lf.Twins["stripe"]
	if !ok {
		t.Fatal("missing twin 'stripe'")
	}
	if stripe.Version != "0.1.0" {
		t.Errorf("stripe version = %q, want %q", stripe.Version, "0.1.0")
	}
	if stripe.ResolvedFrom != "latest" {
		t.Errorf("stripe resolved_from = %q, want %q", stripe.ResolvedFrom, "latest")
	}
	if stripe.SDKPackage != "github.com/stripe/stripe-go" {
		t.Errorf("stripe sdk_package = %q", stripe.SDKPackage)
	}
	if stripe.SDKVersion != "v81" {
		t.Errorf("stripe sdk_version = %q", stripe.SDKVersion)
	}
	if stripe.Checksum == "" {
		t.Error("stripe checksum is empty")
	}
	if stripe.BinaryURL == "" {
		t.Error("stripe binary_url is empty")
	}

	// Verify twilio entry
	twilio, ok := lf.Twins["twilio"]
	if !ok {
		t.Fatal("missing twin 'twilio'")
	}
	if twilio.Version == "" {
		t.Error("twilio version is empty")
	}
}

// TestContractLockFileRoundTrip verifies that Save followed by Load preserves
// all fields from the golden file.
func TestContractLockFileRoundTrip(t *testing.T) {
	data, err := os.ReadFile("testdata/wondertwin-lock.json")
	if err != nil {
		t.Fatalf("reading golden lock file: %v", err)
	}

	var original LockFile
	if err := json.Unmarshal(data, &original); err != nil {
		t.Fatalf("parsing golden lock file: %v", err)
	}

	dir := t.TempDir()
	if err := Save(dir, &original); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// Verify key fields survived the round trip
	if !loaded.GeneratedAt.Equal(original.GeneratedAt) {
		t.Errorf("generated_at changed: %v -> %v", original.GeneratedAt, loaded.GeneratedAt)
	}
	if !loaded.RegistryFetchedAt.Equal(original.RegistryFetchedAt) {
		t.Errorf("registry_fetched_at changed: %v -> %v", original.RegistryFetchedAt, loaded.RegistryFetchedAt)
	}
	if len(loaded.Twins) != len(original.Twins) {
		t.Fatalf("twin count changed: %d -> %d", len(original.Twins), len(loaded.Twins))
	}

	for name, orig := range original.Twins {
		got, ok := loaded.Twins[name]
		if !ok {
			t.Errorf("twin %q missing after round trip", name)
			continue
		}
		if got.Version != orig.Version {
			t.Errorf("%s version: %q -> %q", name, orig.Version, got.Version)
		}
		if got.ResolvedFrom != orig.ResolvedFrom {
			t.Errorf("%s resolved_from: %q -> %q", name, orig.ResolvedFrom, got.ResolvedFrom)
		}
		if got.SDKPackage != orig.SDKPackage {
			t.Errorf("%s sdk_package: %q -> %q", name, orig.SDKPackage, got.SDKPackage)
		}
		if got.SDKVersion != orig.SDKVersion {
			t.Errorf("%s sdk_version: %q -> %q", name, orig.SDKVersion, got.SDKVersion)
		}
		if got.Checksum != orig.Checksum {
			t.Errorf("%s checksum: %q -> %q", name, orig.Checksum, got.Checksum)
		}
		if got.BinaryURL != orig.BinaryURL {
			t.Errorf("%s binary_url: %q -> %q", name, orig.BinaryURL, got.BinaryURL)
		}
	}
}

// TestContractLockFileExists verifies Exists returns correct results.
func TestContractLockFileExists(t *testing.T) {
	dir := t.TempDir()

	if Exists(dir) {
		t.Error("Exists returned true for empty directory")
	}

	data, err := os.ReadFile("testdata/wondertwin-lock.json")
	if err != nil {
		t.Fatalf("reading golden lock file: %v", err)
	}

	var lf LockFile
	if err := json.Unmarshal(data, &lf); err != nil {
		t.Fatalf("parsing golden lock file: %v", err)
	}

	if err := Save(dir, &lf); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if !Exists(dir) {
		t.Error("Exists returned false after Save")
	}
}
