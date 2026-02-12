// Package manifest parses wondertwin.yaml project manifests.
package manifest

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Twin defines the configuration for a single twin in the manifest.
type Twin struct {
	Binary    string            `yaml:"binary"`
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

	// Apply defaults
	if m.Settings.LogDir == "" {
		m.Settings.LogDir = ".wt/logs"
	}

	for name, t := range m.Twins {
		if t.Binary == "" {
			return nil, fmt.Errorf("twin %q: binary path is required", name)
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
