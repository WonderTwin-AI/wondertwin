package v2

import (
	"os"
	"testing"

	"github.com/wondertwin-ai/wondertwin/internal/manifest"
)

func testManifest() *manifest.Manifest {
	return &manifest.Manifest{
		Twins: map[string]manifest.Twin{
			"stripe": {
				Port:      4111,
				AdminPort: 4112,
			},
			"github": {
				Port:      4113,
				AdminPort: 4114,
			},
		},
	}
}

func TestExpandTemplates_TwinPort(t *testing.T) {
	m := testManifest()

	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{
			name:  "twin port",
			input: "http://localhost:{{twins.stripe.port}}/health",
			want:  "http://localhost:4111/health",
		},
		{
			name:  "twin admin_port",
			input: "http://localhost:{{twins.stripe.admin_port}}/admin/health",
			want:  "http://localhost:4112/admin/health",
		},
		{
			name:  "multiple twins",
			input: "{{twins.stripe.port}} and {{twins.github.port}}",
			want:  "4111 and 4113",
		},
		{
			name:    "unknown twin",
			input:   "{{twins.unknown.port}}",
			wantErr: true,
		},
		{
			name:    "unknown field",
			input:   "{{twins.stripe.binary}}",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ExpandTemplates(tt.input, m, nil)
			if (err != nil) != tt.wantErr {
				t.Errorf("ExpandTemplates() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("ExpandTemplates() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExpandTemplates_CapturedVariables(t *testing.T) {
	m := testManifest()
	vars := map[string]string{
		"customer_id": "cus_abc123",
		"token":       "tok_xyz",
	}

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "single variable",
			input: "/v1/customers/{{customer_id}}",
			want:  "/v1/customers/cus_abc123",
		},
		{
			name:  "multiple variables",
			input: "{{customer_id}} {{token}}",
			want:  "cus_abc123 tok_xyz",
		},
		{
			name:  "mixed twin and variable",
			input: "http://localhost:{{twins.stripe.port}}/v1/customers/{{customer_id}}",
			want:  "http://localhost:4111/v1/customers/cus_abc123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ExpandTemplates(tt.input, m, vars)
			if err != nil {
				t.Errorf("ExpandTemplates() error = %v", err)
				return
			}
			if got != tt.want {
				t.Errorf("ExpandTemplates() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExpandTemplates_EnvVariables(t *testing.T) {
	m := testManifest()

	os.Setenv("WT_TEST_VAR", "test_value")
	defer os.Unsetenv("WT_TEST_VAR")

	got, err := ExpandTemplates("{{env.WT_TEST_VAR}}", m, nil)
	if err != nil {
		t.Errorf("ExpandTemplates() error = %v", err)
		return
	}
	if got != "test_value" {
		t.Errorf("ExpandTemplates() = %q, want %q", got, "test_value")
	}
}

func TestExpandTemplates_NoTemplates(t *testing.T) {
	m := testManifest()

	got, err := ExpandTemplates("plain string", m, nil)
	if err != nil {
		t.Errorf("ExpandTemplates() error = %v", err)
		return
	}
	if got != "plain string" {
		t.Errorf("ExpandTemplates() = %q, want %q", got, "plain string")
	}
}

func TestExpandTemplates_UnterminatedTemplate(t *testing.T) {
	m := testManifest()

	_, err := ExpandTemplates("{{unterminated", m, nil)
	if err == nil {
		t.Error("expected error for unterminated template")
	}
}
