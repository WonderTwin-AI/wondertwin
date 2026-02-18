package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// setupManifest creates a twin-manifest.json in a temp twin directory and
// changes to the parent so readManifest("stripe") can find it.
func setupManifest(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	twinDir := filepath.Join(dir, "twin-stripe")
	if err := os.MkdirAll(twinDir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `{
  "twin": "stripe",
  "display_name": "Stripe",
  "category": "payments",
  "description": "Stripe twin for testing",
  "sdk_target": {
    "primary": {
      "package": "github.com/stripe/stripe-go",
      "language": "go",
      "version": "v81"
    }
  }
}`
	if err := os.WriteFile(filepath.Join(twinDir, "twin-manifest.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func writeChecksums(t *testing.T, dir string) string {
	t.Helper()
	content := `abc123def456  twin-stripe-darwin-amd64
789ghi012jkl  twin-stripe-darwin-arm64
mno345pqr678  twin-stripe-linux-amd64
stu901vwx234  twin-stripe-linux-arm64
`
	path := filepath.Join(dir, "checksums.txt")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeEmptyRegistry(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "registry.json")
	if err := os.WriteFile(path, []byte(`{"schema_version": 1, "twins": {}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func fixedTime() time.Time {
	return time.Date(2026, 2, 17, 0, 0, 0, 0, time.UTC)
}

func TestCreateRegistryFromScratch(t *testing.T) {
	dir := setupManifest(t)
	// Change to the dir so readManifest finds twin-stripe/twin-manifest.json
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	nowFunc = fixedTime
	defer func() { nowFunc = time.Now }()

	checksumsPath := writeChecksums(t, dir)
	registryPath := writeEmptyRegistry(t, dir)

	err := run([]string{
		"--twin", "stripe",
		"--version", "0.1.0",
		"--checksums-file", checksumsPath,
		"--registry-file", registryPath,
	})
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}

	data, _ := os.ReadFile(registryPath)
	var reg Registry
	if err := json.Unmarshal(data, &reg); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if reg.SchemaVersion != 1 {
		t.Errorf("schema_version = %d, want 1", reg.SchemaVersion)
	}
	entry, ok := reg.Twins["stripe"]
	if !ok {
		t.Fatal("missing twin 'stripe'")
	}
	if entry.Latest != "0.1.0" {
		t.Errorf("latest = %q, want %q", entry.Latest, "0.1.0")
	}
	if entry.Description != "Stripe twin for testing" {
		t.Errorf("description = %q", entry.Description)
	}
	if entry.Category != "payments" {
		t.Errorf("category = %q", entry.Category)
	}
	if entry.Author != "WonderTwin" {
		t.Errorf("author = %q", entry.Author)
	}

	ver, ok := entry.Versions["0.1.0"]
	if !ok {
		t.Fatal("missing version 0.1.0")
	}
	if ver.Released != "2026-02-17" {
		t.Errorf("released = %q", ver.Released)
	}
	if ver.SDKPackage != "github.com/stripe/stripe-go" {
		t.Errorf("sdk_package = %q", ver.SDKPackage)
	}
	if ver.SDKVersion != "v81" {
		t.Errorf("sdk_version = %q", ver.SDKVersion)
	}
	if ver.Tier != "free" {
		t.Errorf("tier = %q", ver.Tier)
	}
}

func TestAddSecondVersion(t *testing.T) {
	dir := setupManifest(t)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	nowFunc = fixedTime
	defer func() { nowFunc = time.Now }()

	checksumsPath := writeChecksums(t, dir)
	registryPath := writeEmptyRegistry(t, dir)

	// First version
	if err := run([]string{
		"--twin", "stripe", "--version", "0.1.0",
		"--checksums-file", checksumsPath, "--registry-file", registryPath,
	}); err != nil {
		t.Fatal(err)
	}

	// Second version
	if err := run([]string{
		"--twin", "stripe", "--version", "0.2.0",
		"--checksums-file", checksumsPath, "--registry-file", registryPath,
	}); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(registryPath)
	var reg Registry
	json.Unmarshal(data, &reg)

	entry := reg.Twins["stripe"]
	if entry.Latest != "0.2.0" {
		t.Errorf("latest = %q, want %q", entry.Latest, "0.2.0")
	}
	if len(entry.Versions) != 2 {
		t.Errorf("versions count = %d, want 2", len(entry.Versions))
	}
	if _, ok := entry.Versions["0.1.0"]; !ok {
		t.Error("version 0.1.0 was removed")
	}
	if _, ok := entry.Versions["0.2.0"]; !ok {
		t.Error("version 0.2.0 not added")
	}
}

func TestAddSecondTwin(t *testing.T) {
	dir := setupManifest(t)
	// Also create a twilio manifest
	twilioDir := filepath.Join(dir, "twin-twilio")
	os.MkdirAll(twilioDir, 0o755)
	twilioManifest := `{
  "twin": "twilio",
  "description": "Twilio twin",
  "category": "messaging",
  "sdk_target": { "primary": { "package": "github.com/twilio/twilio-go", "version": "v1" } }
}`
	os.WriteFile(filepath.Join(twilioDir, "twin-manifest.json"), []byte(twilioManifest), 0o644)

	// Twilio checksums
	twilioChecksums := `aaa111  twin-twilio-darwin-amd64
bbb222  twin-twilio-darwin-arm64
ccc333  twin-twilio-linux-amd64
ddd444  twin-twilio-linux-arm64
`
	twilioChecksumsPath := filepath.Join(dir, "twilio-checksums.txt")
	os.WriteFile(twilioChecksumsPath, []byte(twilioChecksums), 0o644)

	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	nowFunc = fixedTime
	defer func() { nowFunc = time.Now }()

	checksumsPath := writeChecksums(t, dir)
	registryPath := writeEmptyRegistry(t, dir)

	// Add stripe
	run([]string{
		"--twin", "stripe", "--version", "0.1.0",
		"--checksums-file", checksumsPath, "--registry-file", registryPath,
	})

	// Add twilio
	if err := run([]string{
		"--twin", "twilio", "--version", "0.1.0",
		"--checksums-file", twilioChecksumsPath, "--registry-file", registryPath,
	}); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(registryPath)
	var reg Registry
	json.Unmarshal(data, &reg)

	if len(reg.Twins) != 2 {
		t.Fatalf("twins count = %d, want 2", len(reg.Twins))
	}
	if _, ok := reg.Twins["stripe"]; !ok {
		t.Error("stripe twin missing")
	}
	if _, ok := reg.Twins["twilio"]; !ok {
		t.Error("twilio twin missing")
	}
}

func TestChecksumParsing(t *testing.T) {
	dir := t.TempDir()
	content := `abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890  twin-stripe-darwin-arm64
1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef  twin-stripe-linux-amd64
`
	path := filepath.Join(dir, "checksums.txt")
	os.WriteFile(path, []byte(content), 0o644)

	checksums, err := parseChecksums(path, "stripe")
	if err != nil {
		t.Fatal(err)
	}

	if len(checksums) != 2 {
		t.Fatalf("got %d checksums, want 2", len(checksums))
	}

	expected := "sha256:abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890"
	if checksums["darwin-arm64"] != expected {
		t.Errorf("darwin-arm64 checksum = %q, want %q", checksums["darwin-arm64"], expected)
	}

	expected2 := "sha256:1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"
	if checksums["linux-amd64"] != expected2 {
		t.Errorf("linux-amd64 checksum = %q, want %q", checksums["linux-amd64"], expected2)
	}
}

func TestOutputMatchesSchema(t *testing.T) {
	dir := setupManifest(t)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	nowFunc = fixedTime
	defer func() { nowFunc = time.Now }()

	checksumsPath := writeChecksums(t, dir)
	registryPath := writeEmptyRegistry(t, dir)

	run([]string{
		"--twin", "stripe", "--version", "0.1.0",
		"--checksums-file", checksumsPath, "--registry-file", registryPath,
	})

	data, _ := os.ReadFile(registryPath)

	// Verify all expected JSON keys exist by unmarshalling to a generic map
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}

	// Check top-level keys
	if _, ok := raw["schema_version"]; !ok {
		t.Error("missing schema_version")
	}
	twins := raw["twins"].(map[string]interface{})
	stripe := twins["stripe"].(map[string]interface{})

	for _, key := range []string{"description", "repo", "category", "author", "latest", "versions"} {
		if _, ok := stripe[key]; !ok {
			t.Errorf("missing twin key %q", key)
		}
	}

	versions := stripe["versions"].(map[string]interface{})
	v010 := versions["0.1.0"].(map[string]interface{})

	for _, key := range []string{"released", "sdk_package", "sdk_version", "tier", "checksums", "binary_urls"} {
		if _, ok := v010[key]; !ok {
			t.Errorf("missing version key %q", key)
		}
	}

	// Verify binary URL format
	binaryURLs := v010["binary_urls"].(map[string]interface{})
	expectedURL := "https://github.com/wondertwin-ai/registry/releases/download/twin-stripe-v0.1.0/twin-stripe-darwin-arm64"
	if binaryURLs["darwin-arm64"] != expectedURL {
		t.Errorf("binary_url = %q, want %q", binaryURLs["darwin-arm64"], expectedURL)
	}

	// Verify checksum format starts with sha256:
	checksums := v010["checksums"].(map[string]interface{})
	for platform, cs := range checksums {
		s := cs.(string)
		if len(s) < 8 || s[:7] != "sha256:" {
			t.Errorf("checksum for %s doesn't start with sha256: got %q", platform, s)
		}
	}
}

func TestPrereleaseDoesNotUpdateLatest(t *testing.T) {
	dir := setupManifest(t)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	nowFunc = fixedTime
	defer func() { nowFunc = time.Now }()

	checksumsPath := writeChecksums(t, dir)
	registryPath := writeEmptyRegistry(t, dir)

	// First stable release
	if err := run([]string{
		"--twin", "stripe", "--version", "0.1.0",
		"--checksums-file", checksumsPath, "--registry-file", registryPath,
	}); err != nil {
		t.Fatal(err)
	}

	// Prerelease
	if err := run([]string{
		"--twin", "stripe", "--version", "0.2.0-beta.1",
		"--checksums-file", checksumsPath, "--registry-file", registryPath,
		"--prerelease",
	}); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(registryPath)
	var reg Registry
	json.Unmarshal(data, &reg)

	entry := reg.Twins["stripe"]
	if entry.Latest != "0.1.0" {
		t.Errorf("latest = %q, want %q (should not update for prerelease)", entry.Latest, "0.1.0")
	}
	if _, ok := entry.Versions["0.2.0-beta.1"]; !ok {
		t.Error("prerelease version 0.2.0-beta.1 not added")
	}
	if len(entry.Versions) != 2 {
		t.Errorf("versions count = %d, want 2", len(entry.Versions))
	}
}

func TestPrereleaseWithoutFlag(t *testing.T) {
	dir := setupManifest(t)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	nowFunc = fixedTime
	defer func() { nowFunc = time.Now }()

	checksumsPath := writeChecksums(t, dir)
	registryPath := writeEmptyRegistry(t, dir)

	// First version
	if err := run([]string{
		"--twin", "stripe", "--version", "0.1.0",
		"--checksums-file", checksumsPath, "--registry-file", registryPath,
	}); err != nil {
		t.Fatal(err)
	}

	// Second version without --prerelease
	if err := run([]string{
		"--twin", "stripe", "--version", "0.2.0",
		"--checksums-file", checksumsPath, "--registry-file", registryPath,
	}); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(registryPath)
	var reg Registry
	json.Unmarshal(data, &reg)

	if reg.Twins["stripe"].Latest != "0.2.0" {
		t.Errorf("latest = %q, want %q", reg.Twins["stripe"].Latest, "0.2.0")
	}
}

func TestPrereleaseFirstReleaseSetsLatest(t *testing.T) {
	dir := setupManifest(t)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	nowFunc = fixedTime
	defer func() { nowFunc = time.Now }()

	checksumsPath := writeChecksums(t, dir)
	registryPath := writeEmptyRegistry(t, dir)

	// First release is a prerelease on an empty twin
	if err := run([]string{
		"--twin", "stripe", "--version", "0.1.0-alpha.1",
		"--checksums-file", checksumsPath, "--registry-file", registryPath,
		"--prerelease",
	}); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(registryPath)
	var reg Registry
	json.Unmarshal(data, &reg)

	// Should still set latest since there's no previous latest
	if reg.Twins["stripe"].Latest != "0.1.0-alpha.1" {
		t.Errorf("latest = %q, want %q (first release should set latest even with --prerelease)",
			reg.Twins["stripe"].Latest, "0.1.0-alpha.1")
	}
}

func setupManifestWithAPIVersion(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	twinDir := filepath.Join(dir, "twin-stripe")
	if err := os.MkdirAll(twinDir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `{
  "twin": "stripe",
  "display_name": "Stripe",
  "category": "payments",
  "description": "Stripe twin for testing",
  "sdk_target": {
    "primary": {
      "package": "github.com/stripe/stripe-go",
      "language": "go",
      "version": "v81",
      "api_version": "2024-12-18"
    }
  }
}`
	if err := os.WriteFile(filepath.Join(twinDir, "twin-manifest.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestAPIVersionIncludedInRegistry(t *testing.T) {
	dir := setupManifestWithAPIVersion(t)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	nowFunc = fixedTime
	defer func() { nowFunc = time.Now }()

	checksumsPath := writeChecksums(t, dir)
	registryPath := writeEmptyRegistry(t, dir)

	if err := run([]string{
		"--twin", "stripe", "--version", "0.1.0",
		"--checksums-file", checksumsPath, "--registry-file", registryPath,
	}); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(registryPath)
	var reg Registry
	json.Unmarshal(data, &reg)

	ver := reg.Twins["stripe"].Versions["0.1.0"]
	if ver.APIVersion != "2024-12-18" {
		t.Errorf("api_version = %q, want %q", ver.APIVersion, "2024-12-18")
	}
}

func TestAPIVersionOmittedWhenAbsent(t *testing.T) {
	// setupManifest (no api_version)
	dir := setupManifest(t)
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	nowFunc = fixedTime
	defer func() { nowFunc = time.Now }()

	checksumsPath := writeChecksums(t, dir)
	registryPath := writeEmptyRegistry(t, dir)

	if err := run([]string{
		"--twin", "stripe", "--version", "0.1.0",
		"--checksums-file", checksumsPath, "--registry-file", registryPath,
	}); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(registryPath)

	// api_version should not appear in JSON when empty
	var raw map[string]interface{}
	json.Unmarshal(data, &raw)
	twins := raw["twins"].(map[string]interface{})
	stripe := twins["stripe"].(map[string]interface{})
	versions := stripe["versions"].(map[string]interface{})
	v010 := versions["0.1.0"].(map[string]interface{})

	if _, ok := v010["api_version"]; ok {
		t.Error("api_version should be omitted when not present in manifest")
	}
}

func TestBackwardCompatibilityWithoutAPIVersion(t *testing.T) {
	// Simulate a registry.json that was created before api_version existed
	dir := t.TempDir()
	registryJSON := `{
  "schema_version": 1,
  "twins": {
    "stripe": {
      "description": "Stripe twin",
      "repo": "https://github.com/wondertwin-ai/wondertwin",
      "category": "payments",
      "author": "WonderTwin",
      "latest": "0.1.0",
      "versions": {
        "0.1.0": {
          "released": "2026-02-17",
          "sdk_package": "github.com/stripe/stripe-go",
          "sdk_version": "v81",
          "tier": "free",
          "checksums": {},
          "binary_urls": {}
        }
      }
    }
  }
}`
	registryPath := filepath.Join(dir, "registry.json")
	os.WriteFile(registryPath, []byte(registryJSON), 0o644)

	var reg Registry
	data, _ := os.ReadFile(registryPath)
	if err := json.Unmarshal(data, &reg); err != nil {
		t.Fatalf("failed to parse registry without api_version: %v", err)
	}

	ver := reg.Twins["stripe"].Versions["0.1.0"]
	if ver.APIVersion != "" {
		t.Errorf("api_version should be empty for old registry, got %q", ver.APIVersion)
	}
	if ver.SDKPackage != "github.com/stripe/stripe-go" {
		t.Errorf("sdk_package = %q", ver.SDKPackage)
	}
}
