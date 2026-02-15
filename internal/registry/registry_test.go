package registry

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"gopkg.in/yaml.v3"
)

func sampleRegistry() *Registry {
	return &Registry{
		SchemaVersion: 1,
		Twins: map[string]TwinEntry{
			"stripe": {
				Description: "Stripe behavioral twin",
				Latest:      "0.4.0",
				Versions: map[string]Version{
					"0.4.0": {
						Released: "2025-01-15",
						Tier:     "free",
					},
				},
			},
		},
	}
}

func TestFetchRegistryYAML(t *testing.T) {
	reg := sampleRegistry()
	data, _ := yaml.Marshal(reg)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(data)
	}))
	defer srv.Close()

	got, err := FetchRegistry(srv.URL+"/registry.yaml", "")
	if err != nil {
		t.Fatalf("FetchRegistry() error: %v", err)
	}
	if got.Twins["stripe"].Latest != "0.4.0" {
		t.Errorf("expected latest 0.4.0, got %q", got.Twins["stripe"].Latest)
	}
}

func TestFetchRegistryJSON(t *testing.T) {
	reg := sampleRegistry()
	data, _ := json.Marshal(reg)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(data)
	}))
	defer srv.Close()

	got, err := FetchRegistry(srv.URL+"/registry.json", "")
	if err != nil {
		t.Fatalf("FetchRegistry() error: %v", err)
	}
	if got.Twins["stripe"].Latest != "0.4.0" {
		t.Errorf("expected latest 0.4.0, got %q", got.Twins["stripe"].Latest)
	}
}

func TestFetchRegistryJSONFallbackToYAML(t *testing.T) {
	reg := sampleRegistry()
	yamlData, _ := yaml.Marshal(reg)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/registry.json" {
			http.NotFound(w, r)
			return
		}
		w.Write(yamlData)
	}))
	defer srv.Close()

	// Request .yaml URL -- it should try .json first (404), then fall back to .yaml
	got, err := FetchRegistry(srv.URL+"/registry.yaml", "")
	if err != nil {
		t.Fatalf("FetchRegistry() fallback error: %v", err)
	}
	if got.Twins["stripe"].Latest != "0.4.0" {
		t.Errorf("expected latest 0.4.0, got %q", got.Twins["stripe"].Latest)
	}
}

func TestToJSONURL(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"https://example.com/registry.yaml", "https://example.com/registry.json"},
		{"https://example.com/registry.yml", "https://example.com/registry.json"},
		{"https://example.com/registry.json", ""},
		{"https://example.com/registry", ""},
	}

	for _, tt := range tests {
		got := toJSONURL(tt.input)
		if got != tt.want {
			t.Errorf("toJSONURL(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestResolveVersion(t *testing.T) {
	reg := sampleRegistry()

	ver, v, err := reg.ResolveVersion("stripe", "latest")
	if err != nil {
		t.Fatalf("ResolveVersion error: %v", err)
	}
	if ver != "0.4.0" {
		t.Errorf("expected version 0.4.0, got %q", ver)
	}
	if v.Tier != "free" {
		t.Errorf("expected tier free, got %q", v.Tier)
	}
}
