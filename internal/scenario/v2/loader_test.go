package v2

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadScenario_Valid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.json")
	content := `{
  "name": "Health check",
  "description": "Check twin health",
  "setup": {
    "reset": ["stripe"]
  },
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
        "status": 200,
        "body": {
          "$.status": "ok"
        }
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
	if s.Description != "Check twin health" {
		t.Errorf("expected description 'Check twin health', got %q", s.Description)
	}
	if len(s.Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(s.Steps))
	}
	if s.Steps[0].Request.Method != "GET" {
		t.Errorf("expected GET, got %q", s.Steps[0].Request.Method)
	}
	if s.Variables["stripe_port"] != "4111" {
		t.Errorf("expected variable stripe_port=4111, got %q", s.Variables["stripe_port"])
	}
	if s.Steps[0].Capture["status_text"] != "$.status" {
		t.Errorf("expected capture status_text=$.status, got %v", s.Steps[0].Capture)
	}
	if s.Setup == nil {
		t.Fatal("expected setup to be non-nil")
	}
	if len(s.Setup.Reset) != 1 || s.Setup.Reset[0] != "stripe" {
		t.Errorf("expected setup reset [stripe], got %v", s.Setup.Reset)
	}
	if s.Steps[0].Assert == nil {
		t.Fatal("expected assert to be non-nil")
	}
	if s.Steps[0].Assert.Status != 200 {
		t.Errorf("expected assert status 200, got %d", s.Steps[0].Assert.Status)
	}
}

func TestLoadScenario_MissingName(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.json")
	content := `{
  "steps": [
    {
      "name": "step1",
      "request": {"method": "GET", "url": "http://localhost/health"}
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

func TestLoadScenario_MissingSteps(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.json")
	content := `{"name": "Test"}`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadScenario(path)
	if err == nil {
		t.Fatal("expected error for missing steps")
	}
}

func TestLoadScenario_NonJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.yaml")
	content := `name: test`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadScenario(path)
	if err == nil {
		t.Fatal("expected error for non-JSON file")
	}
}

func TestLoadScenario_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.json")
	if err := os.WriteFile(path, []byte(`{invalid`), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadScenario(path)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestLoadScenario_FileNotFound(t *testing.T) {
	_, err := LoadScenario("/nonexistent/path/test.json")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoadDir(t *testing.T) {
	dir := t.TempDir()

	// Write two JSON scenario files
	s1 := `{"name": "Scenario 1", "steps": [{"name": "s1", "request": {"method": "GET", "url": "http://localhost/a"}}]}`
	s2 := `{"name": "Scenario 2", "steps": [{"name": "s2", "request": {"method": "GET", "url": "http://localhost/b"}}]}`

	if err := os.WriteFile(filepath.Join(dir, "a.json"), []byte(s1), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.json"), []byte(s2), 0o644); err != nil {
		t.Fatal(err)
	}
	// Write a YAML file that should be ignored by v2 loader
	if err := os.WriteFile(filepath.Join(dir, "c.yaml"), []byte("name: yaml test"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Write a non-scenario file
	if err := os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("ignore"), 0o644); err != nil {
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
	if !names["Scenario 1"] {
		t.Error("missing Scenario 1")
	}
	if !names["Scenario 2"] {
		t.Error("missing Scenario 2")
	}
}

func TestLoadDir_Empty(t *testing.T) {
	dir := t.TempDir()

	scenarios, err := LoadDir(dir)
	if err != nil {
		t.Fatalf("LoadDir() error: %v", err)
	}
	if scenarios != nil {
		t.Errorf("expected nil scenarios for empty dir, got %v", scenarios)
	}
}

func TestLoadScenario_WithBody(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.json")
	content := `{
  "name": "Body test",
  "steps": [
    {
      "name": "Create resource",
      "request": {
        "method": "POST",
        "url": "http://localhost:4111/v1/resource",
        "headers": {"Authorization": "Bearer tok_123"},
        "body": {"key": "value", "count": 42}
      },
      "assert": {
        "status": 201,
        "body_contains": "value"
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

	if s.Steps[0].Request.Body == nil {
		t.Fatal("expected body to be non-nil")
	}
	if s.Steps[0].Request.Headers["Authorization"] != "Bearer tok_123" {
		t.Errorf("expected Authorization header, got %v", s.Steps[0].Request.Headers)
	}
	if s.Steps[0].Assert.BodyContains != "value" {
		t.Errorf("expected body_contains 'value', got %q", s.Steps[0].Assert.BodyContains)
	}
}
