// Package scenario loads and runs YAML-based test scenarios against running twins.
package scenario

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Scenario is a complete test scenario loaded from a YAML file.
type Scenario struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Setup       Setup  `yaml:"setup"`
	Steps       []Step `yaml:"steps"`
}

// Setup defines pre-test actions: resetting twins and seeding data.
type Setup struct {
	Reset []string          `yaml:"reset"`
	Seed  map[string]string `yaml:"seed"`
}

// Step is a single request/assert pair within a scenario.
type Step struct {
	Name    string  `yaml:"name"`
	Request Request `yaml:"request"`
	Assert  Assert  `yaml:"assert"`
}

// Request defines the HTTP request to make during a step.
type Request struct {
	Method  string            `yaml:"method"`
	URL     string            `yaml:"url"`
	Headers map[string]string `yaml:"headers"`
	Body    string            `yaml:"body"`
}

// Assert defines the expected results of a step.
type Assert struct {
	Status       int               `yaml:"status"`
	BodyContains string            `yaml:"body_contains"`
	BodyJSON     map[string]string `yaml:"body_json"`
}

// LoadScenario parses a single YAML scenario file.
func LoadScenario(path string) (*Scenario, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading scenario %s: %w", path, err)
	}

	var s Scenario
	if err := yaml.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parsing scenario %s: %w", path, err)
	}

	if s.Name == "" {
		return nil, fmt.Errorf("scenario %s: name is required", path)
	}
	if len(s.Steps) == 0 {
		return nil, fmt.Errorf("scenario %s: at least one step is required", path)
	}

	return &s, nil
}

// LoadDir loads all .yaml and .yml scenario files from a directory.
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
		if ext != ".yaml" && ext != ".yml" {
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
