package schemas_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

func compileSchema(t *testing.T, schemaFile string) *jsonschema.Schema {
	t.Helper()
	c := jsonschema.NewCompiler()
	abs, err := filepath.Abs(schemaFile)
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	f, err := os.Open(abs)
	if err != nil {
		t.Fatalf("open schema %s: %v", schemaFile, err)
	}
	defer f.Close()
	doc, err := jsonschema.UnmarshalJSON(f)
	if err != nil {
		t.Fatalf("decode schema: %v", err)
	}
	if err := c.AddResource(schemaFile, doc); err != nil {
		t.Fatalf("add resource: %v", err)
	}
	sch, err := c.Compile(schemaFile)
	if err != nil {
		t.Fatalf("compile schema: %v", err)
	}
	return sch
}

func loadJSON(t *testing.T, path string) any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatalf("unmarshal %s: %v", path, err)
	}
	return v
}

func TestTwinManifestSchema(t *testing.T) {
	sch := compileSchema(t, "twin-manifest.schema.json")
	tests := []struct {
		file    string
		wantErr bool
	}{
		{"testdata/manifest_valid_minimal.json", false},
		{"testdata/manifest_valid_certified.json", false},
		{"testdata/manifest_invalid_missing_publisher.json", true},
		{"testdata/manifest_invalid_certified_missing_fields.json", true},
	}
	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			err := sch.Validate(loadJSON(t, tt.file))
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate(%s) err=%v, wantErr=%v", tt.file, err, tt.wantErr)
			}
		})
	}
}

func TestScenarioSchema(t *testing.T) {
	sch := compileSchema(t, "scenario.schema.json")
	tests := []struct {
		file    string
		wantErr bool
	}{
		{"testdata/scenario_valid.json", false},
		{"testdata/scenario_invalid_bare.json", true},
	}
	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			err := sch.Validate(loadJSON(t, tt.file))
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate(%s) err=%v, wantErr=%v", tt.file, err, tt.wantErr)
			}
		})
	}
}

func TestWondertwinConfigSchema(t *testing.T) {
	sch := compileSchema(t, "wondertwin.schema.json")
	tests := []struct {
		file    string
		wantErr bool
	}{
		{"testdata/wondertwin_valid.json", false},
		{"testdata/wondertwin_invalid_bare.json", true},
	}
	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			err := sch.Validate(loadJSON(t, tt.file))
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate(%s) err=%v, wantErr=%v", tt.file, err, tt.wantErr)
			}
		})
	}
}
