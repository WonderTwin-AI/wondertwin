package v2

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// LoadScenario parses a JSON scenario file.
func LoadScenario(path string) (*Scenario, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading scenario %s: %w", path, err)
	}

	ext := strings.ToLower(filepath.Ext(path))
	if ext != ".json" {
		return nil, fmt.Errorf("v2 runner only supports .json scenarios, got %q", ext)
	}

	var s Scenario
	if err := json.Unmarshal(data, &s); err != nil {
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

// LoadDir loads all .json scenario files from a directory.
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
		if ext != ".json" {
			continue
		}
		s, err := LoadScenario(filepath.Join(dir, name))
		if err != nil {
			return nil, err
		}
		scenarios = append(scenarios, s)
	}

	return scenarios, nil
}
