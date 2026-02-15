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

func TestLoadScenarioRejectsJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.json")
	content := `{"name": "test", "steps": [{"name": "s", "request": {"method": "GET", "url": "http://localhost/health"}, "assert": {"status": 200}}]}`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadScenario(path)
	if err == nil {
		t.Fatal("expected error for JSON file in v1 runner")
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
	path := filepath.Join(dir, "test.yaml")
	content := `
steps:
  - name: "step1"
    request:
      method: GET
      url: "http://localhost/health"
    assert:
      status: 200
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadScenario(path)
	if err == nil {
		t.Fatal("expected error for missing name")
	}
}

func TestLoadDirYAMLOnly(t *testing.T) {
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
	// JSON file should be ignored by v1 LoadDir
	if err := os.WriteFile(filepath.Join(dir, "b.json"), []byte(jsonContent), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("ignore me"), 0o644); err != nil {
		t.Fatal(err)
	}

	scenarios, err := LoadDir(dir)
	if err != nil {
		t.Fatalf("LoadDir() error: %v", err)
	}
	if len(scenarios) != 1 {
		t.Fatalf("expected 1 scenario (YAML only), got %d", len(scenarios))
	}
	if scenarios[0].Name != "YAML scenario" {
		t.Errorf("expected 'YAML scenario', got %q", scenarios[0].Name)
	}
}

func TestLoadDirEmpty(t *testing.T) {
	dir := t.TempDir()

	scenarios, err := LoadDir(dir)
	if err != nil {
		t.Fatalf("LoadDir() error: %v", err)
	}
	if len(scenarios) != 0 {
		t.Fatalf("expected 0 scenarios, got %d", len(scenarios))
	}
}
