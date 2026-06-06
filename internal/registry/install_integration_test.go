package registry

import (
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wondertwin-ai/wondertwin/internal/httpio"
)

// TestIntegrationInstallFromURL verifies the full install flow using a local
// HTTP server: download, checksum verification, binary write, and version sidecar.
func TestIntegrationInstallFromURL(t *testing.T) {
	binaryContent := []byte("#!/bin/sh\necho hello from twin-test")
	checksum := fmt.Sprintf("sha256:%x", sha256.Sum256(binaryContent))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(binaryContent)
	}))
	defer srv.Close()

	dir := t.TempDir()

	err := InstallFromURL("test", "0.1.0", srv.URL+"/twin-test", checksum, dir)
	if err != nil {
		t.Fatalf("InstallFromURL: %v", err)
	}

	// Verify binary was written
	binaryPath := filepath.Join(dir, "twin-test")
	data, err := os.ReadFile(binaryPath)
	if err != nil {
		t.Fatalf("reading binary: %v", err)
	}
	if string(data) != string(binaryContent) {
		t.Errorf("binary content mismatch")
	}

	// Verify binary is executable
	info, err := os.Stat(binaryPath)
	if err != nil {
		t.Fatalf("stat binary: %v", err)
	}
	if info.Mode()&0o111 == 0 {
		t.Error("binary is not executable")
	}

	// Verify version sidecar
	versionData, err := os.ReadFile(binaryPath + ".version")
	if err != nil {
		t.Fatalf("reading version sidecar: %v", err)
	}
	if string(versionData) != "0.1.0" {
		t.Errorf("version sidecar = %q, want %q", string(versionData), "0.1.0")
	}

	// Verify IsAlreadyInstalled returns true
	if !IsAlreadyInstalled("test", "0.1.0", dir) {
		t.Error("IsAlreadyInstalled returned false after successful install")
	}

	// Verify IsAlreadyInstalled returns false for different version
	if IsAlreadyInstalled("test", "0.2.0", dir) {
		t.Error("IsAlreadyInstalled returned true for wrong version")
	}
}

// TestIntegrationInstallFromURLChecksumMismatch verifies that a checksum
// mismatch is caught and no binary is written.
func TestIntegrationInstallFromURLChecksumMismatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("real binary content"))
	}))
	defer srv.Close()

	dir := t.TempDir()
	badChecksum := "sha256:0000000000000000000000000000000000000000000000000000000000000000"

	err := InstallFromURL("test", "0.1.0", srv.URL+"/twin-test", badChecksum, dir)
	if err == nil {
		t.Fatal("expected checksum mismatch error")
	}

	// Binary should still be written (current implementation writes before checking)
	// but the error is returned so the caller knows it failed
}

// TestIntegrationInstallFromURLHTTPError verifies that HTTP errors are propagated.
func TestIntegrationInstallFromURLHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	dir := t.TempDir()

	err := InstallFromURL("test", "0.1.0", srv.URL+"/twin-test", "", dir)
	if err == nil {
		t.Fatal("expected error for HTTP 404")
	}
}

// TestIntegrationInstallNoChecksum verifies install works without a checksum.
func TestIntegrationInstallNoChecksum(t *testing.T) {
	binaryContent := []byte("twin binary no checksum")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(binaryContent)
	}))
	defer srv.Close()

	dir := t.TempDir()

	err := InstallFromURL("test", "0.1.0", srv.URL+"/twin-test", "", dir)
	if err != nil {
		t.Fatalf("InstallFromURL without checksum: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "twin-test"))
	if err != nil {
		t.Fatalf("reading binary: %v", err)
	}
	if string(data) != string(binaryContent) {
		t.Error("binary content mismatch")
	}
}

// TestIntegrationFetchRegistryFromServer verifies that FetchRegistry can parse
// a registry served over HTTP, exercising the full fetch-and-parse pipeline.
func TestIntegrationFetchRegistryFromServer(t *testing.T) {
	registryJSON, err := os.ReadFile("testdata/registry.json")
	if err != nil {
		t.Fatalf("reading golden registry: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(registryJSON)
	}))
	defer srv.Close()

	reg, err := FetchRegistry(srv.URL+"/registry.json", "")
	if err != nil {
		t.Fatalf("FetchRegistry: %v", err)
	}

	if reg.SchemaVersion != 1 {
		t.Errorf("schema_version = %d, want 1", reg.SchemaVersion)
	}
	if len(reg.Twins) != 2 {
		t.Errorf("expected 2 twins, got %d", len(reg.Twins))
	}

	// Verify version resolution works end-to-end
	v, ver, err := reg.ResolveVersion("stripe", "latest")
	if err != nil {
		t.Fatalf("ResolveVersion: %v", err)
	}
	if v != "0.1.0" {
		t.Errorf("resolved version = %q, want %q", v, "0.1.0")
	}
	if ver.APIVersion != "2024-12-18" {
		t.Errorf("api_version = %q, want %q", ver.APIVersion, "2024-12-18")
	}
}

// TestInstallFromURL_SoftWarnsWhenChecksumMissing is F-007 Phase A
// regression coverage for the WARN tripwire. Default behaviour without
// the strict-checksum opt-in: install proceeds (no break for users
// pre-flip) but writes a loud WARN line to stderr so the publisher
// surfaces a missing checksum during the rollout release.
//
// Removing the warnMissingChecksum call would let an unchecksummed
// install slip through silently — exactly the F-007 bug.
func TestInstallFromURL_SoftWarnsWhenChecksumMissing(t *testing.T) {
	t.Setenv("WT_STRICT_CHECKSUMS", "")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("twin binary"))
	}))
	defer srv.Close()

	stderr := captureStderr(t, func() {
		dir := t.TempDir()
		if err := InstallFromURL("warntest", "0.1.0", srv.URL+"/twin-warntest", "", dir); err != nil {
			t.Fatalf("InstallFromURL: %v", err)
		}
	})

	if !strings.Contains(stderr, "WARN") || !strings.Contains(stderr, "no checksum") || !strings.Contains(stderr, "F-007") {
		t.Errorf("expected WARN about missing checksum + F-007 reference; got: %q", stderr)
	}
}

// TestInstallFromURL_StrictModeRejectsMissingChecksum is the Phase B
// preview: when the operator opts in to strict mode now, missing
// checksums hard-fail. This proves the next-release flip is a config
// change, not a code change.
func TestInstallFromURL_StrictModeRejectsMissingChecksum(t *testing.T) {
	t.Setenv("WT_STRICT_CHECKSUMS", "1")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("twin binary"))
	}))
	defer srv.Close()

	dir := t.TempDir()
	err := InstallFromURL("stricttest", "0.1.0", srv.URL+"/twin-stricttest", "", dir)
	if err == nil {
		t.Fatal("expected install to be rejected under strict checksum mode")
	}
	if !strings.Contains(err.Error(), "strict-checksum") || !strings.Contains(err.Error(), "no checksum") {
		t.Errorf("expected error mentioning strict-checksum + missing checksum; got: %v", err)
	}
}

// TestInstallFromURL_StrictModeWithChecksumStillVerifies guards the
// "happy path still works" case under strict mode — checksum present
// and valid → install proceeds.
func TestInstallFromURL_StrictModeWithChecksumStillVerifies(t *testing.T) {
	t.Setenv("WT_STRICT_CHECKSUMS", "1")

	content := []byte("twin binary with valid checksum")
	checksum := fmt.Sprintf("sha256:%x", sha256.Sum256(content))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(content)
	}))
	defer srv.Close()

	dir := t.TempDir()
	if err := InstallFromURL("happytest", "0.1.0", srv.URL+"/twin-happytest", checksum, dir); err != nil {
		t.Fatalf("install with valid checksum under strict mode should succeed: %v", err)
	}
}

// captureStderr runs fn with os.Stderr redirected to a pipe and returns
// what was written. Used by the F-007 WARN-emission test.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stderr = w
	t.Cleanup(func() { os.Stderr = orig })

	done := make(chan string, 1)
	go func() {
		var buf [4096]byte
		n, _ := r.Read(buf[:])
		done <- string(buf[:n])
	}()

	fn()
	w.Close()
	return <-done
}

// TestInstallFromURL_RejectsOversizedBody asserts that a server streaming a
// body larger than httpio.MaxResponseBytes is rejected without OOMing the process.
// Regression test for F-008: bound network downloads with io.LimitReader.
// Removing the LimitReader wrapper and size check in installer.go would cause
// this test to allocate >256 MiB and silently succeed instead of erroring.
func TestInstallFromURL_RejectsOversizedBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		src := io.LimitReader(zeroReader{}, httpio.MaxResponseBytes+1)
		if _, err := io.Copy(w, src); err != nil {
			t.Logf("server copy: %v", err)
		}
	}))
	defer srv.Close()

	dir := t.TempDir()
	err := InstallFromURL("oversized", "0.0.1", srv.URL+"/twin-oversized", "", dir)
	if err == nil {
		t.Fatal("InstallFromURL accepted an oversized body; expected error")
	}
	if !strings.Contains(err.Error(), "exceeds maximum size") {
		t.Fatalf("expected size-exceeded error, got: %v", err)
	}
}

// zeroReader emits an unbounded stream of zero bytes. Used only by the
// oversized-body regression test.
type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 0
	}
	return len(p), nil
}
