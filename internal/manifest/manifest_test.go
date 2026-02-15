package manifest

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wondertwin.yaml")
	content := `
twins:
  stripe:
    binary: ./bin/twin-stripe
    port: 4111
settings:
  verbose: true
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	m, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if len(m.Twins) != 1 {
		t.Fatalf("expected 1 twin, got %d", len(m.Twins))
	}
	tw := m.Twins["stripe"]
	if tw.Port != 4111 {
		t.Errorf("expected port 4111, got %d", tw.Port)
	}
	if !m.Settings.Verbose {
		t.Error("expected verbose to be true")
	}
}

func TestLoadJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wondertwin.json")
	content := `{
  "twins": {
    "stripe": {
      "binary": "./bin/twin-stripe",
      "port": 4111,
      "sdk": "github.com/stripe/stripe-go/v76",
      "build": "latest"
    }
  },
  "settings": {
    "verbose": true
  }
}`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	m, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if len(m.Twins) != 1 {
		t.Fatalf("expected 1 twin, got %d", len(m.Twins))
	}
	tw := m.Twins["stripe"]
	if tw.Port != 4111 {
		t.Errorf("expected port 4111, got %d", tw.Port)
	}
	if tw.SDK != "github.com/stripe/stripe-go/v76" {
		t.Errorf("expected SDK field, got %q", tw.SDK)
	}
	if tw.Build != "latest" {
		t.Errorf("expected Build=latest, got %q", tw.Build)
	}
	if !m.Settings.Verbose {
		t.Error("expected verbose to be true")
	}
}

func TestLoadUnsupportedExtension(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wondertwin.toml")
	if err := os.WriteFile(path, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for unsupported extension")
	}
}

func TestLoadJSONAndYAMLProduceSameResult(t *testing.T) {
	dir := t.TempDir()

	yamlPath := filepath.Join(dir, "wondertwin.yaml")
	yamlContent := `
twins:
  stripe:
    binary: ./bin/twin-stripe
    port: 4111
    env:
      API_KEY: test123
settings:
  binary_dir: ~/.wondertwin/bin
  log_dir: .wt/logs
  verbose: true
`

	jsonPath := filepath.Join(dir, "wondertwin.json")
	jsonContent := `{
  "twins": {
    "stripe": {
      "binary": "./bin/twin-stripe",
      "port": 4111,
      "env": {
        "API_KEY": "test123"
      }
    }
  },
  "settings": {
    "binary_dir": "~/.wondertwin/bin",
    "log_dir": ".wt/logs",
    "verbose": true
  }
}`

	if err := os.WriteFile(yamlPath, []byte(yamlContent), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(jsonPath, []byte(jsonContent), 0o644); err != nil {
		t.Fatal(err)
	}

	mYAML, err := Load(yamlPath)
	if err != nil {
		t.Fatalf("Load YAML error: %v", err)
	}
	mJSON, err := Load(jsonPath)
	if err != nil {
		t.Fatalf("Load JSON error: %v", err)
	}

	if mYAML.Twins["stripe"].Port != mJSON.Twins["stripe"].Port {
		t.Error("port mismatch between YAML and JSON")
	}
	if mYAML.Twins["stripe"].Env["API_KEY"] != mJSON.Twins["stripe"].Env["API_KEY"] {
		t.Error("env mismatch between YAML and JSON")
	}
	if mYAML.Settings.Verbose != mJSON.Settings.Verbose {
		t.Error("verbose mismatch between YAML and JSON")
	}
}
