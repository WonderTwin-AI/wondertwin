package registry

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestInstallFromURL(t *testing.T) {
	binaryContent := []byte("#!/bin/sh\necho hello\n")
	checksum := fmt.Sprintf("sha256:%x", sha256.Sum256(binaryContent))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(binaryContent)
	}))
	defer srv.Close()

	dir := t.TempDir()
	err := InstallFromURL("stripe", "0.1.0", srv.URL+"/twin-stripe", checksum, dir)
	if err != nil {
		t.Fatalf("InstallFromURL: %v", err)
	}

	// Verify binary exists and is executable
	binaryPath := filepath.Join(dir, "twin-stripe")
	info, err := os.Stat(binaryPath)
	if err != nil {
		t.Fatalf("binary not found: %v", err)
	}
	if info.Mode()&0o111 == 0 {
		t.Error("binary is not executable")
	}

	// Verify version sidecar
	versionData, err := os.ReadFile(binaryPath + ".version")
	if err != nil {
		t.Fatalf("version file not found: %v", err)
	}
	if string(versionData) != "0.1.0" {
		t.Errorf("version = %q, want %q", string(versionData), "0.1.0")
	}
}

func TestInstallFromURLChecksumMismatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("binary content"))
	}))
	defer srv.Close()

	dir := t.TempDir()
	err := InstallFromURL("stripe", "0.1.0", srv.URL+"/twin-stripe", "sha256:0000000000000000000000000000000000000000000000000000000000000000", dir)
	if err == nil {
		t.Fatal("expected checksum mismatch error")
	}

	// Binary should not exist after failed checksum
	binaryPath := filepath.Join(dir, "twin-stripe")
	if _, err := os.Stat(binaryPath); err == nil {
		t.Error("binary should not exist after checksum mismatch")
	}
}

// TestInstallFromURLNoChecksum asserts the Phase B strict-by-default
// behavior: with WT_STRICT_CHECKSUMS unset, a missing checksum hard-fails
// and no binary is written. The pre-Phase-B version of this test asserted
// the opposite (install succeeds) — flipped here per F-007 closure.
func TestInstallFromURLNoChecksum(t *testing.T) {
	t.Setenv("WT_STRICT_CHECKSUMS", "")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("binary content"))
	}))
	defer srv.Close()

	dir := t.TempDir()
	err := InstallFromURL("stripe", "0.1.0", srv.URL+"/twin-stripe", "", dir)
	if err == nil {
		t.Fatal("expected install to be rejected under strict-by-default mode")
	}

	binaryPath := filepath.Join(dir, "twin-stripe")
	if _, statErr := os.Stat(binaryPath); statErr == nil {
		t.Error("binary should not exist after strict-checksum rejection")
	}
}

func TestIsAlreadyInstalled(t *testing.T) {
	dir := t.TempDir()
	twinName := "stripe"

	// Not installed yet
	if IsAlreadyInstalled(twinName, "0.1.0", dir) {
		t.Error("should not be installed in empty dir")
	}

	// Write binary and version sidecar
	binaryPath := filepath.Join(dir, "twin-stripe")
	os.WriteFile(binaryPath, []byte("binary"), 0o755)
	os.WriteFile(binaryPath+".version", []byte("0.1.0"), 0o644)

	if !IsAlreadyInstalled(twinName, "0.1.0", dir) {
		t.Error("should be installed")
	}

	// Different version
	if IsAlreadyInstalled(twinName, "0.2.0", dir) {
		t.Error("should not match different version")
	}
}

func TestInstallFromURLHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	dir := t.TempDir()
	err := InstallFromURL("stripe", "0.1.0", srv.URL+"/twin-stripe", "", dir)
	if err == nil {
		t.Fatal("expected error for HTTP 404")
	}
}

// TestCheckTierAccessFreeTiers pins the contract that gen-registry writes
// against: every tier value a community twin can carry installs without a
// license. "community" is here because gen-registry published that value at
// the version level between the --tier flag landing and the versionTier fix,
// so registry entries in the wild still carry it.
func TestCheckTierAccessFreeTiers(t *testing.T) {
	for _, tier := range []string{"", "free", "community"} {
		t.Run("tier="+tier, func(t *testing.T) {
			// cfg is nil: no license configured at all.
			if err := CheckTierAccess("stripe", "0.2.0", Version{Tier: tier}, nil); err != nil {
				t.Errorf("tier %q must install without a license, got: %v", tier, err)
			}
		})
	}
}

// TestCheckTierAccessCommercialRequiresLicense is the other half of the
// contract: anything outside the free set still demands a license.
func TestCheckTierAccessCommercialRequiresLicense(t *testing.T) {
	err := CheckTierAccess("measure", "0.1.0", Version{Tier: "commercial"}, nil)
	if err == nil {
		t.Fatal("commercial tier must require a license, got nil")
	}
}

// TestCheckTierAccessUnknownTierIsGated documents the fail-closed default:
// an unrecognised tier is treated as license-required, not waved through.
func TestCheckTierAccessUnknownTierIsGated(t *testing.T) {
	if err := CheckTierAccess("x", "1.0.0", Version{Tier: "enterprise"}, nil); err == nil {
		t.Fatal("unknown tier must be gated, got nil")
	}
}
