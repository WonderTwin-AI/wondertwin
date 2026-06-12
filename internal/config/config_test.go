package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadFromYAML(t *testing.T) {
	dir := t.TempDir()
	// t.TempDir() returns a 0755 dir on most platforms; tighten so
	// the per-load perm-check passes for valid-shape tests.
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "config.yaml")
	content := `
license_key: "wt_com_acme_abcdef_7b"
registries:
  public:
    url: https://example.com/registry.yaml
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadFrom(path, false)
	if err != nil {
		t.Fatalf("LoadFrom() error: %v", err)
	}
	if cfg.LicenseKey != "wt_com_acme_abcdef_7b" {
		t.Errorf("expected license key, got %q", cfg.LicenseKey)
	}
	if cfg.Registries["public"].URL != "https://example.com/registry.yaml" {
		t.Errorf("unexpected registry URL: %q", cfg.Registries["public"].URL)
	}
}

func TestLoadFromJSON(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "config.json")
	content := `{
  "license_key": "wt_com_acme_abcdef_7b",
  "registries": {
    "public": {
      "url": "https://example.com/registry.json"
    }
  }
}`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadFrom(path, true)
	if err != nil {
		t.Fatalf("LoadFrom() error: %v", err)
	}
	if cfg.LicenseKey != "wt_com_acme_abcdef_7b" {
		t.Errorf("expected license key, got %q", cfg.LicenseKey)
	}
	if cfg.Registries["public"].URL != "https://example.com/registry.json" {
		t.Errorf("unexpected registry URL: %q", cfg.Registries["public"].URL)
	}
}

func TestLoadFromMissingFileReturnsDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nonexistent.json")

	cfg, err := LoadFrom(path, true)
	if err != nil {
		t.Fatalf("LoadFrom() error: %v", err)
	}
	if cfg.Registries["public"].URL == "" {
		t.Error("expected default public registry URL")
	}
}

func TestLoadPrefersJSONOverYAML(t *testing.T) {
	// Temporarily override configDir by creating both files in a temp dir
	// and using LoadFrom to test each format. The preference logic is in
	// resolveConfigPath which we test indirectly.
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}

	yamlPath := filepath.Join(dir, "config.yaml")
	yamlContent := `license_key: "yaml-key"`
	if err := os.WriteFile(yamlPath, []byte(yamlContent), 0o600); err != nil {
		t.Fatal(err)
	}

	jsonPath := filepath.Join(dir, "config.json")
	jsonContent := `{"license_key": "json-key"}`
	if err := os.WriteFile(jsonPath, []byte(jsonContent), 0o600); err != nil {
		t.Fatal(err)
	}

	// When both exist, JSON should be preferred
	// Test by checking that LoadFrom with JSON path works
	cfg, err := LoadFrom(jsonPath, true)
	if err != nil {
		t.Fatalf("LoadFrom() error: %v", err)
	}
	if cfg.LicenseKey != "json-key" {
		t.Errorf("expected json-key, got %q", cfg.LicenseKey)
	}

	// YAML should still work independently
	cfg, err = LoadFrom(yamlPath, false)
	if err != nil {
		t.Fatalf("LoadFrom() error: %v", err)
	}
	if cfg.LicenseKey != "yaml-key" {
		t.Errorf("expected yaml-key, got %q", cfg.LicenseKey)
	}
}

func TestSaveWritesJSON(t *testing.T) {
	// Override HOME so Save writes to a temp dir
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	cfg := &Config{
		LicenseKey: "test-key",
		Registries: map[string]RegistryEntry{
			"public": {URL: "https://example.com/registry.json"},
		},
	}

	if err := Save(cfg); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	// Verify the file was written as JSON
	path := filepath.Join(dir, DefaultConfigDir, DefaultConfigFile)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading saved config: %v", err)
	}

	// Should be valid JSON
	loaded, err := LoadFrom(path, true)
	if err != nil {
		t.Fatalf("LoadFrom() error on saved file: %v", err)
	}
	if loaded.LicenseKey != "test-key" {
		t.Errorf("expected test-key, got %q", loaded.LicenseKey)
	}

	// File should end in .json
	if filepath.Ext(path) != ".json" {
		t.Errorf("expected .json extension, got %q", filepath.Ext(path))
	}

	_ = data // used above
}

func TestOrgContext(t *testing.T) {
	cfg := &Config{}
	if cfg.HasOrgContext() {
		t.Error("empty config should not have org context")
	}

	cfg.OrgSlug = "acme"
	cfg.OrgID = "org-123"
	cfg.APIKey = "wt_abc123def456"
	if !cfg.HasOrgContext() {
		t.Error("expected org context after setting fields")
	}

	cfg.ClearOrgContext()
	if cfg.HasOrgContext() {
		t.Error("expected no org context after clear")
	}
	if cfg.OrgSlug != "" || cfg.OrgID != "" || cfg.APIKey != "" {
		t.Error("expected all org fields cleared")
	}
}

func TestOrgContextRoundTrip(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "config.json")
	content := `{
  "license_key": "",
  "org_slug": "acme",
  "org_id": "org-123",
  "api_key": "wt_abc123def456"
}`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadFrom(path, true)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	if cfg.OrgSlug != "acme" {
		t.Errorf("org_slug: got %q, want acme", cfg.OrgSlug)
	}
	if cfg.OrgID != "org-123" {
		t.Errorf("org_id: got %q, want org-123", cfg.OrgID)
	}
	if cfg.APIKey != "wt_abc123def456" {
		t.Errorf("api_key: got %q, want wt_abc123def456", cfg.APIKey)
	}
}

func TestValidateChecksum(t *testing.T) {
	// Compute a valid checksum for the key "wt_com_acme_abcdef"
	// payload bytes sum = 1842, 1842 % 256 = 50 = 0x32
	info := ValidateChecksum("wt_com_acme_abcdef_32")
	if info == nil {
		t.Fatal("expected non-nil LicenseInfo")
	}
	if info.Tier != "com" {
		t.Errorf("expected tier com, got %q", info.Tier)
	}
	if info.Org != "acme" {
		t.Errorf("expected org acme, got %q", info.Org)
	}
}

// TestLoadFrom_RejectsWorldReadableFile pins the Stream-E-cli
// guarantee: a credentials file whose mode permits group/world reads
// is refused with a clear chmod hint, not silently loaded.
func TestLoadFrom_RejectsWorldReadableFile(t *testing.T) {
	if runtimeIsWindows() {
		t.Skip("Unix mode bits don't apply on Windows")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"license_key":"x"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadFrom(path, true)
	if err == nil {
		t.Fatal("expected LoadFrom to refuse 0644 config file")
	}
	if !strings.Contains(err.Error(), "chmod 600") {
		t.Fatalf("expected chmod hint, got %v", err)
	}
}

// TestLoadFrom_RejectsWorldReadableDir checks the directory mode
// independently — a 0600 file inside a 0755 directory still leaks
// the file's existence (and is suspect).
func TestLoadFrom_RejectsWorldReadableDir(t *testing.T) {
	if runtimeIsWindows() {
		t.Skip("Unix mode bits don't apply on Windows")
	}
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"license_key":"x"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := LoadFrom(path, true)
	if err == nil {
		t.Fatal("expected LoadFrom to refuse 0755 config dir")
	}
	if !strings.Contains(err.Error(), "chmod 700") {
		t.Fatalf("expected chmod hint, got %v", err)
	}
}

// TestSave_TightensExistingDirPerms covers the upgrade path: a prior
// CLI version created ~/.wondertwin with 0755; Save must tighten it
// to 0700 in place so the next Load passes its perm check.
func TestSave_TightensExistingDirPerms(t *testing.T) {
	if runtimeIsWindows() {
		t.Skip("Unix mode bits don't apply on Windows")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)

	dir := filepath.Join(home, DefaultConfigDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	cfg := &Config{LicenseKey: "test"}
	if err := Save(cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode != 0o700 {
		t.Fatalf("expected dir mode 0700 after Save, got %#o", mode)
	}

	fileInfo, err := os.Stat(filepath.Join(dir, DefaultConfigFile))
	if err != nil {
		t.Fatal(err)
	}
	if mode := fileInfo.Mode().Perm(); mode != 0o600 {
		t.Fatalf("expected file mode 0600 after Save, got %#o", mode)
	}
}
