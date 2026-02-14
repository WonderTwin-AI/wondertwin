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
)

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

	// Read entire binary into memory for checksum verification
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading binary data: %w", err)
	}

	// Verify checksum
	if hasChecksum {
		fmt.Printf("  Verifying checksum...\n")
		actual := fmt.Sprintf("sha256:%x", sha256.Sum256(data))
		if actual != expectedChecksum {
			return fmt.Errorf("checksum mismatch: expected %s, got %s", expectedChecksum, actual)
		}
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
			"twin-%s v%s requires a %s license.\nRun `wt auth login` to activate, or use `wt install %s@latest` (free).",
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
