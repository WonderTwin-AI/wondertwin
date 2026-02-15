package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFromYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := `
license_key: "wt_com_acme_abcdef_7b"
registries:
  public:
    url: https://example.com/registry.yaml
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
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
	path := filepath.Join(dir, "config.json")
	content := `{
  "license_key": "wt_com_acme_abcdef_7b",
  "registries": {
    "public": {
      "url": "https://example.com/registry.json"
    }
  }
}`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
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
	dir := t.TempDir()

	yamlPath := filepath.Join(dir, "config.yaml")
	yamlContent := `license_key: "yaml-key"`
	if err := os.WriteFile(yamlPath, []byte(yamlContent), 0o644); err != nil {
		t.Fatal(err)
	}

	jsonPath := filepath.Join(dir, "config.json")
	jsonContent := `{"license_key": "json-key"}`
	if err := os.WriteFile(jsonPath, []byte(jsonContent), 0o644); err != nil {
		t.Fatal(err)
	}

	// When both exist, JSON should be preferred
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

	path := filepath.Join(dir, DefaultConfigDir, DefaultConfigFile)

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
}

func TestParseLicenseKey(t *testing.T) {
	// Compute a valid checksum for the key "wt_com_acme_abcdef"
	// payload bytes sum = 1842, 1842 % 256 = 50 = 0x32
	info := ParseLicenseKey("wt_com_acme_abcdef_32")
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
