// Package scenario loads and runs YAML and JSON test scenarios against running twins.
package scenario

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Scenario is a complete test scenario loaded from a YAML or JSON file.
// JSON scenarios use the new schema (with variables, workflow, capture, etc.);
// YAML scenarios use the legacy format.
type Scenario struct {
	Name        string `yaml:"name" json:"name"`
	Description string `yaml:"description" json:"description"`
	Setup       Setup  `yaml:"setup" json:"setup"`
	Steps       []Step `yaml:"steps" json:"steps"`

	// JSON-schema fields (only populated from JSON scenarios)
	Workflow  string            `yaml:"-" json:"workflow,omitempty"`
	Variables map[string]string `yaml:"-" json:"variables,omitempty"`
}

// Setup defines pre-test actions: resetting twins and seeding data.
type Setup struct {
	Reset     []string          `yaml:"reset" json:"reset"`
	Seed      map[string]string `yaml:"seed" json:"seed,omitempty"`
	SeedFiles map[string]string `yaml:"-" json:"seed_files,omitempty"`
}

// Step is a single request/assert pair within a scenario.
type Step struct {
	Name    string  `yaml:"name" json:"name"`
	Request Request `yaml:"request" json:"request"`
	Assert  Assert  `yaml:"assert" json:"assert"`

	// Capture is only populated from JSON scenarios.
	Capture map[string]string `yaml:"-" json:"capture,omitempty"`
}

// Request defines the HTTP request to make during a step.
type Request struct {
	Method  string            `yaml:"method" json:"method"`
	URL     string            `yaml:"url" json:"url"`
	Headers map[string]string `yaml:"headers" json:"headers"`
	Body    string            `yaml:"body" json:"body"`
}

// Assert defines the expected results of a step.
type Assert struct {
	Status       int               `yaml:"status" json:"status"`
	BodyContains string            `yaml:"body_contains" json:"body_contains"`
	BodyJSON     map[string]string `yaml:"body_json" json:"body_json,omitempty"`
	Headers      map[string]string `yaml:"-" json:"headers,omitempty"`
	Body2        map[string]string `yaml:"-" json:"body,omitempty"`
}

// LoadScenario parses a single YAML or JSON scenario file.
// The format is detected by file extension.
func LoadScenario(path string) (*Scenario, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading scenario %s: %w", path, err)
	}

	var s Scenario
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".json":
		if err := json.Unmarshal(data, &s); err != nil {
			return nil, fmt.Errorf("parsing scenario %s: %w", path, err)
		}
	case ".yaml", ".yml":
		if err := yaml.Unmarshal(data, &s); err != nil {
			return nil, fmt.Errorf("parsing scenario %s: %w", path, err)
		}
	default:
		return nil, fmt.Errorf("unsupported scenario format %q (expected .json, .yaml, or .yml)", ext)
	}

	if s.Name == "" {
		return nil, fmt.Errorf("scenario %s: name is required", path)
	}
	if len(s.Steps) == 0 {
		return nil, fmt.Errorf("scenario %s: at least one step is required", path)
	}

	return &s, nil
}

// LoadDir loads all .yaml, .yml, and .json scenario files from a directory.
func LoadDir(dir string) ([]*Scenario, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("reading scenario directory %s: %w", dir, err)
	}

	var scenarios []*Scenario
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		ext := strings.ToLower(filepath.Ext(name))
		if ext != ".yaml" && ext != ".yml" && ext != ".json" {
			continue
		}
		s, err := LoadScenario(filepath.Join(dir, name))
		if err != nil {
			return nil, err
		}
		scenarios = append(scenarios, s)
	}

	if len(scenarios) == 0 {
		return nil, fmt.Errorf("no scenario files found in %s", dir)
	}

	return scenarios, nil
}
