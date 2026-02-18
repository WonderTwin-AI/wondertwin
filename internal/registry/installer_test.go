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

func TestInstallFromURLNoChecksum(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("binary content"))
	}))
	defer srv.Close()

	dir := t.TempDir()
	err := InstallFromURL("stripe", "0.1.0", srv.URL+"/twin-stripe", "", dir)
	if err != nil {
		t.Fatalf("InstallFromURL without checksum: %v", err)
	}

	binaryPath := filepath.Join(dir, "twin-stripe")
	if _, err := os.Stat(binaryPath); err != nil {
		t.Fatalf("binary not found: %v", err)
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
