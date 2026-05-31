package registry

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// serveBinary returns an httptest server that returns the given bytes
// at every request — useful for the bundle commit tests.
func serveBinary(t *testing.T, body []byte) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestCommitInstallBundle_HappyPathWritesBinaryAndLicense(t *testing.T) {
	t.Parallel()
	binaryBytes := []byte("fake twin binary v1.0.0")
	checksum := hex.EncodeToString(sha256.New().Sum(binaryBytes)[:0]) // placeholder
	sum := sha256.Sum256(binaryBytes)
	checksum = hex.EncodeToString(sum[:])

	srv := serveBinary(t, binaryBytes)
	dir := t.TempDir()
	binaryDir := filepath.Join(dir, "bin")
	licensePath := filepath.Join(dir, "license.json")
	licenseContent := []byte(`{"format_version":"2.0","account_id":"org_t"}`)

	err := CommitInstallBundle("stripe", InstallBundle{
		BinaryURL:      srv.URL,
		BinarySHA256:   checksum,
		BinaryVersion:  "1.0.0",
		LicenseFileB64: base64.StdEncoding.EncodeToString(licenseContent),
	}, binaryDir, licensePath)
	if err != nil {
		t.Fatalf("CommitInstallBundle: %v", err)
	}

	binaryPath := filepath.Join(binaryDir, "twin-stripe")
	got, err := os.ReadFile(binaryPath)
	if err != nil {
		t.Fatalf("read binary: %v", err)
	}
	if string(got) != string(binaryBytes) {
		t.Errorf("binary content mismatch")
	}

	gotLicense, err := os.ReadFile(licensePath)
	if err != nil {
		t.Fatalf("read license: %v", err)
	}
	if string(gotLicense) != string(licenseContent) {
		t.Errorf("license content mismatch: got %q", string(gotLicense))
	}

	// Permission check on the license file: 0600.
	info, err := os.Stat(licensePath)
	if err != nil {
		t.Fatalf("stat license: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("license perm: want 0600, got %#o", perm)
	}
}

func TestCommitInstallBundle_BadBase64LicenseFails(t *testing.T) {
	t.Parallel()
	srv := serveBinary(t, []byte("anything"))
	dir := t.TempDir()
	err := CommitInstallBundle("stripe", InstallBundle{
		BinaryURL:      srv.URL,
		BinarySHA256:   "deadbeef",
		BinaryVersion:  "1.0.0",
		LicenseFileB64: "not!valid!base64!",
	}, filepath.Join(dir, "bin"), filepath.Join(dir, "license.json"))
	if err == nil || !strings.Contains(err.Error(), "decode license") {
		t.Errorf("want decode license error, got %v", err)
	}
}

func TestCommitInstallBundle_BadChecksumDoesNotWriteLicense(t *testing.T) {
	t.Parallel()
	srv := serveBinary(t, []byte("real binary"))
	dir := t.TempDir()
	licensePath := filepath.Join(dir, "license.json")
	err := CommitInstallBundle("stripe", InstallBundle{
		BinaryURL:      srv.URL,
		BinarySHA256:   "0000000000000000000000000000000000000000000000000000000000000000",
		BinaryVersion:  "1.0.0",
		LicenseFileB64: base64.StdEncoding.EncodeToString([]byte("license")),
	}, filepath.Join(dir, "bin"), licensePath)
	if err == nil || !strings.Contains(err.Error(), "binary") {
		t.Errorf("want binary error on checksum mismatch, got %v", err)
	}
	// License must NOT have been written: prior state untouched.
	if _, err := os.Stat(licensePath); !os.IsNotExist(err) {
		t.Errorf("license written despite binary failure: %v", err)
	}
}

func TestCommitInstallBundle_PreservesPriorLicenseOnBinaryFailure(t *testing.T) {
	t.Parallel()
	srv := serveBinary(t, []byte("real binary"))
	dir := t.TempDir()
	licensePath := filepath.Join(dir, "license.json")
	priorLicense := []byte(`{"prior":"license"}`)
	if err := os.WriteFile(licensePath, priorLicense, 0o600); err != nil {
		t.Fatalf("seed prior license: %v", err)
	}

	// Bad checksum → binary install fails.
	err := CommitInstallBundle("stripe", InstallBundle{
		BinaryURL:      srv.URL,
		BinarySHA256:   "0000000000000000000000000000000000000000000000000000000000000000",
		BinaryVersion:  "1.0.0",
		LicenseFileB64: base64.StdEncoding.EncodeToString([]byte("new license")),
	}, filepath.Join(dir, "bin"), licensePath)
	if err == nil {
		t.Fatal("want error, got nil")
	}

	got, err := os.ReadFile(licensePath)
	if err != nil {
		t.Fatalf("read license: %v", err)
	}
	if string(got) != string(priorLicense) {
		t.Errorf("prior license was clobbered: got %q", string(got))
	}
}

func TestCommitInstallBundle_OverwritesExistingLicenseAtomically(t *testing.T) {
	t.Parallel()
	binaryBytes := []byte("binary content")
	sum := sha256.Sum256(binaryBytes)

	srv := serveBinary(t, binaryBytes)
	dir := t.TempDir()
	licensePath := filepath.Join(dir, "license.json")
	if err := os.WriteFile(licensePath, []byte(`{"prior":true}`), 0o600); err != nil {
		t.Fatalf("seed prior license: %v", err)
	}
	newContent := []byte(`{"new":true,"twins":["stripe","hubspot"]}`)

	err := CommitInstallBundle("stripe", InstallBundle{
		BinaryURL:      srv.URL,
		BinarySHA256:   hex.EncodeToString(sum[:]),
		BinaryVersion:  "1.0.0",
		LicenseFileB64: base64.StdEncoding.EncodeToString(newContent),
	}, filepath.Join(dir, "bin"), licensePath)
	if err != nil {
		t.Fatalf("CommitInstallBundle: %v", err)
	}

	got, _ := os.ReadFile(licensePath)
	if string(got) != string(newContent) {
		t.Errorf("license: want new content, got %q", string(got))
	}
}

func TestCommitInstallBundle_RejectsMissingFields(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		bundle InstallBundle
	}{
		{"no binary URL", InstallBundle{BinarySHA256: "x", BinaryVersion: "1", LicenseFileB64: "x"}},
		{"no checksum", InstallBundle{BinaryURL: "u", BinaryVersion: "1", LicenseFileB64: "x"}},
		{"no version", InstallBundle{BinaryURL: "u", BinarySHA256: "x", LicenseFileB64: "x"}},
		{"no license", InstallBundle{BinaryURL: "u", BinarySHA256: "x", BinaryVersion: "1"}},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			err := CommitInstallBundle("stripe", tc.bundle,
				filepath.Join(dir, "bin"), filepath.Join(dir, "license.json"))
			if err == nil {
				t.Errorf("want error for %s, got nil", tc.name)
			}
		})
	}
}

func TestNormalizeChecksumForInstaller(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in, want string
	}{
		{"", ""},
		{"deadbeef", "sha256:deadbeef"},
		{"sha256:deadbeef", "sha256:deadbeef"},
	}
	for _, tc := range cases {
		if got := normalizeChecksumForInstaller(tc.in); got != tc.want {
			t.Errorf("normalize(%q): want %q, got %q", tc.in, tc.want, got)
		}
	}
}
