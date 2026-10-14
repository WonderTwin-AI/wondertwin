package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wondertwin-ai/wondertwin/internal/releaseid"
)

func runCommunity(t *testing.T, version, checksumsPath, registryPath string) error {
	t.Helper()
	return run([]string{
		"--twin", "stripe", "--version", version,
		"--checksums-file", checksumsPath, "--registry-file", registryPath,
	})
}

func readRegistry(t *testing.T, path string) Registry {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var reg Registry
	if err := json.Unmarshal(data, &reg); err != nil {
		t.Fatal(err)
	}
	return reg
}

func inManifestDir(t *testing.T) string {
	t.Helper()
	dir := setupManifest(t)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	t.Cleanup(func() { os.Chdir(orig) })
	nowFunc = fixedTime
	t.Cleanup(func() { nowFunc = time.Now })
	return dir
}

func TestCommunityReleaseRecordsDigest(t *testing.T) {
	dir := inManifestDir(t)
	registryPath := writeEmptyRegistry(t, dir)
	if err := runCommunity(t, stripeID, writeChecksums(t, dir), registryPath); err != nil {
		t.Fatal(err)
	}

	entry := readRegistry(t, registryPath).Twins["stripe"]
	if entry.Latest != stripeID {
		t.Errorf("latest = %q, want %q", entry.Latest, stripeID)
	}
	digest := entry.Versions[stripeID].ReleaseDigest
	want, _ := releaseid.Digest("stripe", fixtureChecksums("a"))
	if digest != want {
		t.Errorf("release_digest = %q, want %q", digest, want)
	}
	if releaseid.ID(digest) != stripeID {
		t.Errorf("release_digest %q does not give key %q", digest, stripeID)
	}
}

func TestCommunityReleaseRejectsSemver(t *testing.T) {
	dir := inManifestDir(t)
	err := runCommunity(t, "0.4.0", writeChecksums(t, dir), writeEmptyRegistry(t, dir))
	if err == nil || !strings.Contains(err.Error(), "is not the release id") {
		t.Fatalf("expected a release id error, got %v", err)
	}
}

func TestCommunityReleaseRejectsMismatchedID(t *testing.T) {
	dir := inManifestDir(t)
	// stripeID2 belongs to the second checksums, not these.
	err := runCommunity(t, stripeID2, writeChecksums(t, dir), writeEmptyRegistry(t, dir))
	if err == nil || !strings.Contains(err.Error(), stripeID) {
		t.Fatalf("expected an error naming the computed id %s, got %v", stripeID, err)
	}
}

func TestCommunityReleaseNeedsEveryPlatform(t *testing.T) {
	dir := inManifestDir(t)
	path := filepath.Join(dir, "partial.txt")
	content := strings.Repeat("a", 64) + "  twin-stripe-linux-amd64\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := runCommunity(t, stripeID, path, writeEmptyRegistry(t, dir)); err == nil {
		t.Fatal("expected an error for a release missing platforms")
	}
}

func TestReleaseDigestSurvivesLaterReleases(t *testing.T) {
	dir := inManifestDir(t)
	registryPath := writeEmptyRegistry(t, dir)
	if err := runCommunity(t, stripeID, writeChecksums(t, dir), registryPath); err != nil {
		t.Fatal(err)
	}
	first := readRegistry(t, registryPath).Twins["stripe"].Versions[stripeID].ReleaseDigest

	if err := runCommunity(t, stripeID2, writeSecondChecksums(t, dir), registryPath); err != nil {
		t.Fatal(err)
	}
	entry := readRegistry(t, registryPath).Twins["stripe"]
	if got := entry.Versions[stripeID].ReleaseDigest; got != first || got == "" {
		t.Errorf("first release's release_digest = %q after a second release, want %q", got, first)
	}
	if entry.Latest != stripeID2 {
		t.Errorf("latest = %q, want %q", entry.Latest, stripeID2)
	}
}
