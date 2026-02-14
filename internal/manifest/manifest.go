// Package manifest parses wondertwin.yaml project manifests.
package manifest

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Twin defines the configuration for a single twin in the manifest.
type Twin struct {
	Binary    string            `yaml:"binary"`
	Version   string            `yaml:"version"`
	Registry  string            `yaml:"registry"`
	Port      int               `yaml:"port"`
	AdminPort int               `yaml:"admin_port"`
	Seed      string            `yaml:"seed"`
	Env       map[string]string `yaml:"env"`
}

// Settings holds global CLI settings from the manifest.
type Settings struct {
	BinaryDir string `yaml:"binary_dir"`
	LogDir    string `yaml:"log_dir"`
	Verbose   bool   `yaml:"verbose"`
}

// Manifest represents a parsed wondertwin.yaml file.
type Manifest struct {
	Twins    map[string]Twin `yaml:"twins"`
	Settings Settings        `yaml:"settings"`

	// dir is the directory containing the manifest file, used for resolving relative paths.
	dir string
}

// Load reads and parses a wondertwin.yaml file.
func Load(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading manifest %s: %w", path, err)
	}

	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parsing manifest: %w", err)
	}

	if m.Twins == nil {
		return nil, fmt.Errorf("manifest has no twins defined")
	}

	// Store the manifest directory for relative path resolution
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolving manifest path: %w", err)
	}
	m.dir = filepath.Dir(absPath)

	// Apply defaults
	if m.Settings.LogDir == "" {
		m.Settings.LogDir = ".wt/logs"
	}
	if m.Settings.BinaryDir == "" {
		m.Settings.BinaryDir = "~/.wondertwin/bin"
	}

	for name, t := range m.Twins {
		if t.Binary == "" && t.Version == "" {
			return nil, fmt.Errorf("twin %q: binary path or version is required", name)
		}
		// Default registry to "public"
		if t.Registry == "" {
			t.Registry = "public"
		}
		// Resolve binary path
		if t.Binary != "" {
			t.Binary = m.resolvePath(t.Binary)
		} else if t.Version != "" {
			// When version is set but binary is not, resolve binary from BinaryDir
			binDir := expandPath(m.Settings.BinaryDir)
			t.Binary = filepath.Join(binDir, "twin-"+name)
		}
		if t.Port == 0 {
			return nil, fmt.Errorf("twin %q: port is required", name)
		}
		// Default admin_port to same as port (twins serve admin on the same router)
		if t.AdminPort == 0 {
			t.AdminPort = t.Port
		}
		m.Twins[name] = t
	}

	return &m, nil
}

// resolvePath resolves a binary path. Absolute paths and ~ paths are returned as-is.
// Relative paths (starting with ./ or ../) are resolved against the manifest directory.
func (m *Manifest) resolvePath(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	if strings.HasPrefix(path, "~/") {
		return expandPath(path)
	}
	// Relative path: resolve against manifest directory
	return filepath.Join(m.dir, path)
}

// Twin returns a named twin's config, or an error if not found.
func (m *Manifest) Twin(name string) (Twin, error) {
	t, ok := m.Twins[name]
	if !ok {
		return Twin{}, fmt.Errorf("twin %q not found in manifest", name)
	}
	return t, nil
}

// TwinNames returns all twin names in deterministic sorted order.
func (m *Manifest) TwinNames() []string {
	names := make([]string, 0, len(m.Twins))
	for name := range m.Twins {
		names = append(names, name)
	}
	sortStrings(names)
	return names
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

// expandPath expands a leading ~ to the user's home directory.
func expandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[2:])
	}
	return path
}
