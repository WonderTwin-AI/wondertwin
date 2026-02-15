package scenario

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadScenarioYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.yaml")
	content := `
name: "Health check"
description: "Check twin health"
steps:
  - name: "Check stripe"
    request:
      method: GET
      url: "http://localhost:4111/admin/health"
    assert:
      status: 200
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	s, err := LoadScenario(path)
	if err != nil {
		t.Fatalf("LoadScenario() error: %v", err)
	}
	if s.Name != "Health check" {
		t.Errorf("expected name 'Health check', got %q", s.Name)
	}
	if len(s.Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(s.Steps))
	}
	if s.Steps[0].Request.Method != "GET" {
		t.Errorf("expected GET, got %q", s.Steps[0].Request.Method)
	}
}

func TestLoadScenarioJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.json")
	content := `{
  "name": "Health check",
  "description": "Check twin health",
  "variables": {
    "stripe_port": "4111"
  },
  "steps": [
    {
      "name": "Check stripe",
      "request": {
        "method": "GET",
        "url": "http://localhost:4111/admin/health"
      },
      "capture": {
        "status_text": "$.status"
      },
      "assert": {
        "status": 200
      }
    }
  ]
}`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	s, err := LoadScenario(path)
	if err != nil {
		t.Fatalf("LoadScenario() error: %v", err)
	}
	if s.Name != "Health check" {
		t.Errorf("expected name 'Health check', got %q", s.Name)
	}
	if len(s.Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(s.Steps))
	}
	// JSON-only fields
	if s.Variables["stripe_port"] != "4111" {
		t.Errorf("expected variable stripe_port=4111, got %q", s.Variables["stripe_port"])
	}
	if s.Steps[0].Capture["status_text"] != "$.status" {
		t.Errorf("expected capture status_text, got %v", s.Steps[0].Capture)
	}
}

func TestLoadScenarioUnsupportedFormat(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.toml")
	if err := os.WriteFile(path, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadScenario(path)
	if err == nil {
		t.Fatal("expected error for unsupported format")
	}
}

func TestLoadScenarioMissingName(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.json")
	content := `{
  "steps": [
    {
      "name": "step1",
      "request": {"method": "GET", "url": "http://localhost/health"},
      "assert": {"status": 200}
    }
  ]
}`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadScenario(path)
	if err == nil {
		t.Fatal("expected error for missing name")
	}
}

func TestLoadDirMixed(t *testing.T) {
	dir := t.TempDir()

	yamlContent := `
name: "YAML scenario"
steps:
  - name: "step1"
    request:
      method: GET
      url: "http://localhost:4111/health"
    assert:
      status: 200
`
	jsonContent := `{
  "name": "JSON scenario",
  "steps": [
    {
      "name": "step1",
      "request": {"method": "GET", "url": "http://localhost:4111/health"},
      "assert": {"status": 200}
    }
  ]
}`

	if err := os.WriteFile(filepath.Join(dir, "a.yaml"), []byte(yamlContent), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.json"), []byte(jsonContent), 0o644); err != nil {
		t.Fatal(err)
	}
	// Write an unrelated file that should be ignored
	if err := os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("ignore me"), 0o644); err != nil {
		t.Fatal(err)
	}

	scenarios, err := LoadDir(dir)
	if err != nil {
		t.Fatalf("LoadDir() error: %v", err)
	}
	if len(scenarios) != 2 {
		t.Fatalf("expected 2 scenarios, got %d", len(scenarios))
	}

	names := map[string]bool{}
	for _, s := range scenarios {
		names[s.Name] = true
	}
	if !names["YAML scenario"] {
		t.Error("missing YAML scenario")
	}
	if !names["JSON scenario"] {
		t.Error("missing JSON scenario")
	}
}
