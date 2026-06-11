package registry

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
)

// allowedBinaryURLHosts is the closed set of hosts CommitInstallBundle
// will fetch a twin binary from. BinaryURL is server-supplied (the MCP
// registry returns it in the install envelope), so a compromised or
// misconfigured registry could otherwise hand the installer a
// file:///etc/passwd or http://evil.com URL that http.Client.Get would
// happily honor. The 2026-06-04 audit (F-007-adjacent) flagged this.
// Extending the list is a deliberate follow-up PR, not a config knob.
var allowedBinaryURLHosts = map[string]struct{}{
	"releases.wondertwin.ai":    {},
	"github.com":                {},
	"raw.githubusercontent.com": {},
}

// testSkipBinaryURLValidation, when true, bypasses validateBinaryURL.
// Set ONLY by TestMain in this package because the integration tests
// drive the installer via httptest.Server (http loopback), which by
// design fails the production allowlist + https check. Never reference
// this from non-test code; there is no production escape hatch.
var testSkipBinaryURLValidation bool

// validateBinaryURL parses rawURL, requires the https scheme, and
// confirms the host is in allowedBinaryURLHosts. The returned *url.URL
// is currently unused by callers — a later refactor could thread it
// into InstallFromURL to avoid the re-parse, but that's out of scope.
func validateBinaryURL(rawURL string) (*url.URL, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("parse binary URL: %w", err)
	}
	if u.Scheme != "https" {
		return nil, fmt.Errorf("binary URL scheme must be https, got %q", u.Scheme)
	}
	host := u.Hostname()
	if host == "" {
		return nil, fmt.Errorf("binary URL must have a host")
	}
	if _, ok := allowedBinaryURLHosts[host]; !ok {
		return nil, fmt.Errorf("binary URL host %q not in allowlist (releases.wondertwin.ai, github.com, raw.githubusercontent.com)", host)
	}
	return u, nil
}

// InstallBundle is the local view of the wire-shape install bundle
// the MCP server returns. The exact wire shape lives in
// internal/platform/install_envelope.go; the registry package takes
// the runtime-facing slice so it doesn't import platform.
type InstallBundle struct {
	BinaryURL      string
	BinarySHA256   string
	BinaryVersion  string
	LicenseFileB64 string
}

// CommitInstallBundle delivers both halves of an MCP install bundle
// to disk. Binary lands at <binaryDir>/twin-<twinName> via the same
// InstallFromURL path the CLI uses; the license file lands at
// licensePath via a temp-then-rename so a partial write never bricks
// the runtime.
//
// Atomicity contract per adr-license-delivery-via-mcp §1:
//   - If the binary install fails, the license file is NOT touched.
//     The prior license (if any) stays in place.
//   - If the binary install succeeds but the license write fails,
//     the binary is updated and the prior license remains. The
//     runtime is in a usable state (last-known-good license) and
//     the next MCP call's freshness check will retry the license
//     update via wt_license_refresh (separate PR).
//   - If both succeed, the runtime has the new binary and the
//     freshly-issued org license.
//
// We deliberately do NOT roll back the binary on license failure.
// Binary rollback would require keeping the prior binary bytes in
// memory (or a temp copy) for the full duration of the license
// write, and the failure mode "new binary, old license" is strictly
// safer than "no binary at all" — the user can re-run the install
// or wait for the refresh path to catch up.
//
// The license bundle's BinarySHA256 format is plain hex per the
// JSON schema; we normalize to the "sha256:<hex>" form that
// InstallFromURL expects.
func CommitInstallBundle(twinName string, bundle InstallBundle, binaryDir, licensePath string) error {
	if bundle.BinaryURL == "" || bundle.BinarySHA256 == "" || bundle.BinaryVersion == "" {
		return fmt.Errorf("install bundle: binary fields are required")
	}
	if bundle.LicenseFileB64 == "" {
		return fmt.Errorf("install bundle: license_file is required")
	}
	if !testSkipBinaryURLValidation {
		if _, err := validateBinaryURL(bundle.BinaryURL); err != nil {
			return fmt.Errorf("install bundle binary URL invalid: %w", err)
		}
	}

	licenseBytes, err := base64.StdEncoding.DecodeString(bundle.LicenseFileB64)
	if err != nil {
		return fmt.Errorf("install bundle: decode license: %w", err)
	}

	if err := InstallFromURL(
		twinName, bundle.BinaryVersion, bundle.BinaryURL,
		normalizeChecksumForInstaller(bundle.BinarySHA256),
		binaryDir,
	); err != nil {
		return fmt.Errorf("install bundle: binary: %w", err)
	}

	if err := writeLicenseAtomic(licensePath, licenseBytes); err != nil {
		// Binary is installed; license write failed. Return the
		// error so the caller can surface it, but the partial state
		// is recoverable per the contract above.
		return fmt.Errorf("install bundle: license write (binary installed): %w", err)
	}
	return nil
}

// normalizeChecksumForInstaller converts the JSON-schema plain-hex
// SHA-256 ("0000...0000") to the "sha256:0000...0000" prefix-tagged
// form that InstallFromURL compares against. Empty input passes
// through to allow tests to skip checksum verification.
func normalizeChecksumForInstaller(s string) string {
	if s == "" {
		return ""
	}
	const prefix = "sha256:"
	if len(s) >= len(prefix) && s[:len(prefix)] == prefix {
		return s
	}
	return prefix + s
}

// writeLicenseAtomic writes the license bytes to a temp file in the
// same directory, fsyncs, then renames into place. Atomic on POSIX:
// the runtime never reads a half-written license file. Permissions
// match the doc.go convention: 0700 on the parent dir, 0600 on the
// file itself.
func writeLicenseAtomic(path string, content []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create license dir %s: %w", dir, err)
	}

	tmp, err := os.CreateTemp(dir, ".license-")
	if err != nil {
		return fmt.Errorf("create temp license file: %w", err)
	}
	tmpPath := tmp.Name()
	// Best-effort cleanup if the rename never happens.
	defer func() {
		if _, statErr := os.Stat(tmpPath); statErr == nil {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := tmp.Write(content); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp license: %w", err)
	}
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("chmod temp license: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("fsync temp license: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp license: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("rename license into place: %w", err)
	}
	return nil
}
