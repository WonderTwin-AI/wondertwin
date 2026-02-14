// Package config loads and manages the WonderTwin CLI configuration file
// stored at ~/.wondertwin/config.yaml.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// DefaultConfigDir is the directory under the user's home for CLI state.
const DefaultConfigDir = ".wondertwin"

// DefaultConfigFile is the config file name within the config directory.
const DefaultConfigFile = "config.yaml"

// RegistryEntry describes a named registry endpoint.
type RegistryEntry struct {
	URL   string `yaml:"url"`
	Token string `yaml:"token,omitempty"` // Phase 2: auth token for private registries
}

// Config represents the contents of ~/.wondertwin/config.yaml.
type Config struct {
	LicenseKey string                   `yaml:"license_key"`
	Registries map[string]RegistryEntry `yaml:"registries"`
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

// configPath returns the full path to the config file.
func configPath() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, DefaultConfigFile), nil
}

// Load reads the config from ~/.wondertwin/config.yaml.
// Returns a default config if the file doesn't exist.
func Load() (*Config, error) {
	path, err := configPath()
	if err != nil {
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
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
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

// Save writes the config to ~/.wondertwin/config.yaml.
func Save(cfg *Config) error {
	path, err := configPath()
	if err != nil {
		return err
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}

	return os.WriteFile(path, data, 0o644)
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

func defaultConfig() *Config {
	return &Config{
		Registries: map[string]RegistryEntry{
			"public": {
				URL: "https://raw.githubusercontent.com/wondertwin-ai/registry/main/registry.yaml",
			},
		},
	}
}
