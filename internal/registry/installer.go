package registry

import (
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/wondertwin-ai/wondertwin/internal/config"
	"github.com/wondertwin-ai/wondertwin/internal/httpio"
)

// strictChecksumsEnvVar is the F-007 Phase B escape hatch: setting it to
// "0" opts a CLI OUT of the new strict default (which rejects installs
// when the registry entry has no expected_sha256). Any other value, or
// unset, keeps the strict default. The opt-out sunsets in v0.3.
// F-007 in wondertwin-docs/security/known-findings.yaml; design in
// wondertwin-docs/adr/adr-twin-binary-trust-model.md.
const strictChecksumsEnvVar = "WT_STRICT_CHECKSUMS"

// requireChecksum returns true when missing checksums must hard-fail.
// Phase B default is strict; only the literal "0" opts out into legacy
// soft-WARN behavior. The opt-out sunsets in v0.3.
func requireChecksum() bool {
	return os.Getenv(strictChecksumsEnvVar) != "0"
}

// warnMissingChecksum writes the loud-WARN line for the opt-out path
// (WT_STRICT_CHECKSUMS=0). Centralised so the message stays consistent
// across Install() and InstallFromURL() and so a single test can assert
// the tripwire fired.
func warnMissingChecksum(twinName, version, source string) {
	fmt.Fprintf(os.Stderr,
		"  WARN: no checksum for twin-%s v%s from %s; WT_STRICT_CHECKSUMS=0 opt-out is deprecated and sunsets in v0.3 (F-007). "+
			"Re-publish the registry/lockfile with a sha256 entry for this platform.\n",
		twinName, version, source,
	)
}

// Install downloads a twin binary for the current platform, verifies its
// checksum, and saves it to binaryDir. It also writes a .version sidecar file.
func Install(twinName string, resolvedVersion string, ver Version, binaryDir string) error {
	platform := runtime.GOOS + "-" + runtime.GOARCH

	binaryURL, ok := ver.BinaryURLs[platform]
	if !ok {
		return fmt.Errorf("no binary available for platform %s", platform)
	}

	expectedChecksum, hasChecksum := ver.Checksums[platform]

	// Ensure binary directory exists
	if err := os.MkdirAll(binaryDir, 0o755); err != nil {
		return fmt.Errorf("creating binary dir %s: %w", binaryDir, err)
	}

	// Download binary
	fmt.Printf("  Downloading twin-%s v%s (%s)...\n", twinName, resolvedVersion, platform)

	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Get(binaryURL)
	if err != nil {
		return fmt.Errorf("downloading binary: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download returned HTTP %d", resp.StatusCode)
	}

	// Read entire binary into memory for checksum verification, bounded
	// by httpio.MaxResponseBytes. Reading cap+1 and rejecting on overflow
	// fails loud — a plain truncate would write a corrupt binary when no
	// checksum is present, which the regression test in
	// install_integration_test.go exercises.
	data, err := io.ReadAll(io.LimitReader(resp.Body, httpio.MaxResponseBytes+1))
	if err != nil {
		return fmt.Errorf("reading binary data: %w", err)
	}
	if int64(len(data)) > httpio.MaxResponseBytes {
		return fmt.Errorf("binary exceeds maximum size of %d bytes", httpio.MaxResponseBytes)
	}

	// Verify checksum. Phase B of F-007 closure: strict-by-default. When
	// no checksum is provided we hard-fail unless the operator has opted
	// out via WT_STRICT_CHECKSUMS=0 (sunsets in v0.3). The registry-side
	// validator (cmd/verify-registry) already enforces that every
	// published version carries checksums for every platform; this is
	// the CLI tripwire against a malformed registry slipping through.
	if hasChecksum {
		fmt.Printf("  Verifying checksum...\n")
		actual := fmt.Sprintf("sha256:%x", sha256.Sum256(data))
		if actual != expectedChecksum {
			return fmt.Errorf("checksum mismatch: expected %s, got %s", expectedChecksum, actual)
		}
	} else if requireChecksum() {
		return fmt.Errorf("install rejected: no expected_sha256 in registry entry for twin-%s v%s (%s); "+
			"set %s=0 to bypass for now (escape hatch sunsets in v0.3 — re-publish the registry entry with a SHA256 before then)",
			twinName, resolvedVersion, platform, strictChecksumsEnvVar)
	} else {
		warnMissingChecksum(twinName, resolvedVersion, "registry")
	}

	// Write binary to disk
	binaryPath := filepath.Join(binaryDir, "twin-"+twinName)
	if err := os.WriteFile(binaryPath, data, 0o755); err != nil {
		return fmt.Errorf("writing binary to %s: %w", binaryPath, err)
	}

	// Write version sidecar file
	versionPath := binaryPath + ".version"
	if err := os.WriteFile(versionPath, []byte(resolvedVersion), 0o644); err != nil {
		return fmt.Errorf("writing version file: %w", err)
	}

	fmt.Printf("  Installed twin-%s v%s -> %s\n", twinName, resolvedVersion, binaryPath)
	return nil
}

// CheckTierAccess verifies that the user has the required license for a version's tier.
// Returns nil if access is allowed, or an error with instructions if not.
func CheckTierAccess(twinName, resolvedVersion string, ver Version, cfg *config.Config) error {
	if ver.Tier == "" || ver.Tier == "free" {
		return nil
	}

	if cfg == nil || !cfg.HasValidLicense() {
		return fmt.Errorf(
			"twin-%s v%s requires a %s license\nrun `wt auth login` to activate, or use `wt install %s@latest` (free)",
			twinName, resolvedVersion, ver.Tier, twinName,
		)
	}

	return nil
}

// IsAlreadyInstalled checks if a twin binary with the matching version is already present.
func IsAlreadyInstalled(twinName, resolvedVersion, binaryDir string) bool {
	binaryPath := filepath.Join(binaryDir, "twin-"+twinName)
	versionPath := binaryPath + ".version"

	// Check binary exists
	if _, err := os.Stat(binaryPath); err != nil {
		return false
	}

	// Check version sidecar matches
	data, err := os.ReadFile(versionPath)
	if err != nil {
		return false
	}

	return strings.TrimSpace(string(data)) == resolvedVersion
}

// InstallFromURL downloads a twin binary from a specific URL, verifies its
// checksum, and saves it to binaryDir. Used by lock file installs where the
// exact URL and checksum are known.
func InstallFromURL(twinName, version, binaryURL, expectedChecksum, binaryDir string) error {
	if err := os.MkdirAll(binaryDir, 0o755); err != nil {
		return fmt.Errorf("creating binary dir %s: %w", binaryDir, err)
	}

	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Get(binaryURL)
	if err != nil {
		return fmt.Errorf("downloading binary: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download returned HTTP %d", resp.StatusCode)
	}

	// Bounded read with overflow detection — see Install() for rationale.
	data, err := io.ReadAll(io.LimitReader(resp.Body, httpio.MaxResponseBytes+1))
	if err != nil {
		return fmt.Errorf("reading binary data: %w", err)
	}
	if int64(len(data)) > httpio.MaxResponseBytes {
		return fmt.Errorf("binary exceeds maximum size of %d bytes", httpio.MaxResponseBytes)
	}

	// Verify checksum. F-007 Phase B — see Install() for full rationale.
	// InstallFromURL is the lockfile path; missing checksums here mean
	// the lockfile was generated against an unchecksummed registry entry,
	// which the registry-side validator should not allow but the CLI
	// still defends against.
	if expectedChecksum != "" {
		actual := fmt.Sprintf("sha256:%x", sha256.Sum256(data))
		if actual != expectedChecksum {
			return fmt.Errorf("checksum mismatch: expected %s, got %s", expectedChecksum, actual)
		}
	} else if requireChecksum() {
		return fmt.Errorf("install rejected: no expected_sha256 in lockfile entry for twin-%s v%s; "+
			"set %s=0 to bypass for now (escape hatch sunsets in v0.3 — re-generate the lockfile against a registry with a SHA256 before then)",
			twinName, version, strictChecksumsEnvVar)
	} else {
		warnMissingChecksum(twinName, version, "lockfile")
	}

	binaryPath := filepath.Join(binaryDir, "twin-"+twinName)
	if err := os.WriteFile(binaryPath, data, 0o755); err != nil {
		return fmt.Errorf("writing binary to %s: %w", binaryPath, err)
	}

	versionPath := binaryPath + ".version"
	if err := os.WriteFile(versionPath, []byte(version), 0o644); err != nil {
		return fmt.Errorf("writing version file: %w", err)
	}

	fmt.Printf("  Installed twin-%s v%s -> %s\n", twinName, version, binaryPath)
	return nil
}

// ExpandPath expands a leading ~ to the user's home directory.
func ExpandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[2:])
	}
	return path
}
