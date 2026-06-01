// Package config loads and manages the WonderTwin CLI configuration file
// stored at ~/.wondertwin/config.json (preferred) or ~/.wondertwin/config.yaml.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"gopkg.in/yaml.v3"
)

func runtimeIsWindows() bool { return runtime.GOOS == "windows" }

// DefaultConfigDir is the directory under the user's home for CLI state.
const DefaultConfigDir = ".wondertwin"

// DefaultConfigFile is the preferred config file name (JSON).
const DefaultConfigFile = "config.json"

// legacyConfigFile is the legacy config file name (YAML).
const legacyConfigFile = "config.yaml"

// RegistryEntry describes a named registry endpoint.
type RegistryEntry struct {
	URL   string `yaml:"url" json:"url"`
	Token string `yaml:"token,omitempty" json:"token,omitempty"` // Bearer token for private registries
}

// Config represents the contents of ~/.wondertwin/config.json or config.yaml.
type Config struct {
	LicenseKey string                   `yaml:"license_key" json:"license_key"`
	Registries map[string]RegistryEntry `yaml:"registries" json:"registries"`

	// Org context for authenticated API access.
	OrgSlug string `yaml:"org_slug,omitempty" json:"org_slug,omitempty"`
	OrgID   string `yaml:"org_id,omitempty" json:"org_id,omitempty"`
	APIKey  string `yaml:"api_key,omitempty" json:"api_key,omitempty"`
}

// LicenseInfo holds parsed fields from a license key.
type LicenseInfo struct {
	Tier string // "com" or "ent"
	Org  string // org slug
	Raw  string // the full key
}

// configDir returns the path to the config directory.
func configDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("determining home directory: %w", err)
	}
	return filepath.Join(home, DefaultConfigDir), nil
}

// resolveConfigPath returns the path to the config file to load.
// It prefers config.json; if that does not exist, it falls back to config.yaml.
// The returned boolean indicates whether the file is JSON (true) or YAML (false).
func resolveConfigPath() (string, bool, error) {
	dir, err := configDir()
	if err != nil {
		return "", false, err
	}

	jsonPath := filepath.Join(dir, DefaultConfigFile)
	if _, err := os.Stat(jsonPath); err == nil {
		return jsonPath, true, nil
	}

	yamlPath := filepath.Join(dir, legacyConfigFile)
	return yamlPath, false, nil
}

// Load reads the config from ~/.wondertwin/config.json (preferred) or
// ~/.wondertwin/config.yaml (legacy fallback).
// Returns a default config if neither file exists.
func Load() (*Config, error) {
	path, isJSON, err := resolveConfigPath()
	if err != nil {
		return nil, err
	}

	return LoadFrom(path, isJSON)
}

// LoadFrom reads and parses a config from the given path.
// If isJSON is true, the file is parsed as JSON; otherwise as YAML.
//
// LoadFrom refuses to read a config file whose Unix permissions are
// world- or group-readable (anything beyond 0600). The file holds an
// API key and the license; a misconfigured umask or a recursive chmod
// can leak credentials to any other user on the box. The CLI fails
// loud with a chmod hint rather than silently loading.
//
// Per-platform note: Windows does not honor Unix mode bits in any
// meaningful way; the check is skipped there.
func LoadFrom(path string, isJSON bool) (*Config, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return defaultConfig(), nil
		}
		return nil, fmt.Errorf("stat config %s: %w", path, err)
	}
	if err := checkConfigPerms(path, info); err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return defaultConfig(), nil
		}
		return nil, fmt.Errorf("reading config %s: %w", path, err)
	}

	var cfg Config
	if isJSON {
		if err := json.Unmarshal(data, &cfg); err != nil {
			return nil, fmt.Errorf("parsing config JSON: %w", err)
		}
	} else {
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return nil, fmt.Errorf("parsing config YAML: %w", err)
		}
	}

	// Ensure registries always has the public entry
	if cfg.Registries == nil {
		cfg.Registries = make(map[string]RegistryEntry)
	}
	if _, ok := cfg.Registries["public"]; !ok {
		cfg.Registries["public"] = RegistryEntry{
			URL: "https://raw.githubusercontent.com/wondertwin-ai/registry/main/registry.yaml",
		}
	}

	return &cfg, nil
}

// Save writes the config to ~/.wondertwin/config.json with 0600
// permissions inside a 0700 directory. If the directory already
// exists with looser permissions (e.g., a prior CLI version wrote it
// 0755), Save tightens it. Symmetric with LoadFrom's perm-check —
// the two together close the credential-leak window.
func Save(cfg *Config) error {
	dir, err := configDir()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}
	// MkdirAll respects existing permissions; explicitly drop them
	// to 0700 in case a prior version of the CLI created the dir
	// with 0755. Skipped on Windows where Unix mode bits are inert.
	if !runtimeIsWindows() {
		if err := os.Chmod(dir, 0o700); err != nil {
			return fmt.Errorf("tightening config dir perms: %w", err)
		}
	}

	path := filepath.Join(dir, DefaultConfigFile)

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}
	data = append(data, '\n')

	if err := os.WriteFile(path, data, 0o600); err != nil {
		return err
	}
	// Likewise, tighten the file in case it pre-existed with a
	// looser mode that WriteFile preserved.
	if !runtimeIsWindows() {
		if err := os.Chmod(path, 0o600); err != nil {
			return fmt.Errorf("tightening config file perms: %w", err)
		}
	}
	return nil
}

// ParseLicenseKey parses a license key of the format wt_{tier}_{org}_{random}_{check}.
// Returns nil if the key is empty or invalid.
func ParseLicenseKey(key string) *LicenseInfo {
	if key == "" {
		return nil
	}

	parts := strings.Split(key, "_")
	if len(parts) < 5 {
		return nil
	}

	if parts[0] != "wt" {
		return nil
	}

	tier := parts[1]
	if tier != "com" && tier != "ent" {
		return nil
	}

	org := parts[2]
	if org == "" {
		return nil
	}

	// Validate checksum digit (last part)
	check := parts[len(parts)-1]
	if len(check) != 2 {
		return nil
	}

	// Reconstruct the random portion (everything between org and check)
	random := strings.Join(parts[3:len(parts)-1], "_")
	if len(random) < 6 {
		return nil
	}

	// Verify checksum: sum of bytes in "wt_{tier}_{org}_{random}" mod 256, as 2-char hex
	payload := strings.Join(parts[:len(parts)-1], "_")
	var sum byte
	for _, b := range []byte(payload) {
		sum += b
	}
	expected := fmt.Sprintf("%02x", sum)
	if check != expected {
		return nil
	}

	return &LicenseInfo{
		Tier: tier,
		Org:  org,
		Raw:  key,
	}
}

// HasValidLicense returns true if the config has a parseable license key.
func (c *Config) HasValidLicense() bool {
	return ParseLicenseKey(c.LicenseKey) != nil
}

// HasOrgContext returns true if the config has stored org credentials.
func (c *Config) HasOrgContext() bool {
	return c.APIKey != "" && c.OrgID != ""
}

// ClearOrgContext removes all org-scoped credentials.
func (c *Config) ClearOrgContext() {
	c.OrgSlug = ""
	c.OrgID = ""
	c.APIKey = ""
}

// TierName returns a human-readable tier name from a license key tier code.
func TierName(tier string) string {
	switch tier {
	case "com":
		return "commercial"
	case "ent":
		return "enterprise"
	default:
		return "free"
	}
}

// checkConfigPerms returns an error if the config file or its parent
// directory permit access outside the owner. The directory check
// catches the "world-readable ~/.wondertwin" footgun even when the
// file itself is 0600 (since a path-traversal-via-readable-dir is
// the same disclosure).
//
// On Windows we skip — `runtime.GOOS == "windows"` doesn't expose
// meaningful Unix mode bits and the check is noise there.
func checkConfigPerms(path string, info os.FileInfo) error {
	if runtimeIsWindows() {
		return nil
	}
	mode := info.Mode().Perm()
	if mode&0o077 != 0 {
		return fmt.Errorf(
			"config file %s has insecure permissions %#o; run `chmod 600 %s` and retry",
			path, mode, path,
		)
	}

	dir := filepath.Dir(path)
	dirInfo, err := os.Stat(dir)
	if err != nil {
		// If the directory is unreadable we can't continue anyway —
		// the file read would already have failed before this point.
		return nil
	}
	dirMode := dirInfo.Mode().Perm()
	if dirMode&0o077 != 0 {
		return fmt.Errorf(
			"config directory %s has insecure permissions %#o; run `chmod 700 %s` and retry",
			dir, dirMode, dir,
		)
	}
	return nil
}

func defaultConfig() *Config {
	return &Config{
		Registries: map[string]RegistryEntry{
			"public": {
				URL: "https://raw.githubusercontent.com/wondertwin-ai/registry/main/registry.yaml",
			},
		},
	}
}
