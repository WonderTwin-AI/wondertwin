package v2

import (
	"fmt"
	"testing"
)

func TestEvaluateBodyAssertions_ExactValue(t *testing.T) {
	body := []byte(`{"status": "ok", "count": 42, "name": "test"}`)

	tests := []struct {
		name       string
		assertions map[string]any
		wantErr    bool
	}{
		{
			name:       "string match",
			assertions: map[string]any{"$.status": "ok"},
			wantErr:    false,
		},
		{
			name:       "numeric match",
			assertions: map[string]any{"$.count": float64(42)},
			wantErr:    false,
		},
		{
			name:       "string mismatch",
			assertions: map[string]any{"$.status": "error"},
			wantErr:    true,
		},
		{
			name:       "missing field",
			assertions: map[string]any{"$.missing": "value"},
			wantErr:    true,
		},
		{
			name:       "multiple assertions pass",
			assertions: map[string]any{"$.status": "ok", "$.name": "test"},
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := EvaluateBodyAssertions(body, tt.assertions)
			if (err != nil) != tt.wantErr {
				t.Errorf("EvaluateBodyAssertions() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestEvaluateBodyAssertions_EqOperator(t *testing.T) {
	body := []byte(`{"status": "ok", "count": 42}`)

	tests := []struct {
		name       string
		assertions map[string]any
		wantErr    bool
	}{
		{
			name:       "eq string match",
			assertions: map[string]any{"$.status": map[string]any{"eq": "ok"}},
			wantErr:    false,
		},
		{
			name:       "eq numeric match",
			assertions: map[string]any{"$.count": map[string]any{"eq": float64(42)}},
			wantErr:    false,
		},
		{
			name:       "eq mismatch",
			assertions: map[string]any{"$.status": map[string]any{"eq": "error"}},
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := EvaluateBodyAssertions(body, tt.assertions)
			if (err != nil) != tt.wantErr {
				t.Errorf("EvaluateBodyAssertions() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestEvaluateBodyAssertions_GteOperator(t *testing.T) {
	body := []byte(`{"count": 42}`)

	tests := []struct {
		name       string
		assertions map[string]any
		wantErr    bool
	}{
		{
			name:       "gte equal",
			assertions: map[string]any{"$.count": map[string]any{"gte": float64(42)}},
			wantErr:    false,
		},
		{
			name:       "gte less",
			assertions: map[string]any{"$.count": map[string]any{"gte": float64(10)}},
			wantErr:    false,
		},
		{
			name:       "gte greater fails",
			assertions: map[string]any{"$.count": map[string]any{"gte": float64(100)}},
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := EvaluateBodyAssertions(body, tt.assertions)
			if (err != nil) != tt.wantErr {
				t.Errorf("EvaluateBodyAssertions() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestEvaluateBodyAssertions_LteOperator(t *testing.T) {
	body := []byte(`{"count": 42}`)

	tests := []struct {
		name       string
		assertions map[string]any
		wantErr    bool
	}{
		{
			name:       "lte equal",
			assertions: map[string]any{"$.count": map[string]any{"lte": float64(42)}},
			wantErr:    false,
		},
		{
			name:       "lte greater",
			assertions: map[string]any{"$.count": map[string]any{"lte": float64(100)}},
			wantErr:    false,
		},
		{
			name:       "lte less fails",
			assertions: map[string]any{"$.count": map[string]any{"lte": float64(10)}},
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := EvaluateBodyAssertions(body, tt.assertions)
			if (err != nil) != tt.wantErr {
				t.Errorf("EvaluateBodyAssertions() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestEvaluateBodyAssertions_ContainsOperator(t *testing.T) {
	body := []byte(`{"message": "hello world", "code": "ERR_NOT_FOUND"}`)

	tests := []struct {
		name       string
		assertions map[string]any
		wantErr    bool
	}{
		{
			name:       "contains match",
			assertions: map[string]any{"$.message": map[string]any{"contains": "world"}},
			wantErr:    false,
		},
		{
			name:       "contains no match",
			assertions: map[string]any{"$.message": map[string]any{"contains": "foo"}},
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := EvaluateBodyAssertions(body, tt.assertions)
			if (err != nil) != tt.wantErr {
				t.Errorf("EvaluateBodyAssertions() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestEvaluateBodyAssertions_RegexOperator(t *testing.T) {
	body := []byte(`{"id": "cus_abc123", "email": "test@example.com"}`)

	tests := []struct {
		name       string
		assertions map[string]any
		wantErr    bool
	}{
		{
			name:       "regex match",
			assertions: map[string]any{"$.id": map[string]any{"regex": "^cus_[a-z0-9]+$"}},
			wantErr:    false,
		},
		{
			name:       "regex no match",
			assertions: map[string]any{"$.id": map[string]any{"regex": "^prod_"}},
			wantErr:    true,
		},
		{
			name:       "regex email",
			assertions: map[string]any{"$.email": map[string]any{"regex": `.+@.+\..+`}},
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := EvaluateBodyAssertions(body, tt.assertions)
			if (err != nil) != tt.wantErr {
				t.Errorf("EvaluateBodyAssertions() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestEvaluateBodyAssertions_ExistsOperator(t *testing.T) {
	body := []byte(`{"status": "ok", "count": 42}`)

	tests := []struct {
		name       string
		assertions map[string]any
		wantErr    bool
	}{
		{
			name:       "exists true and field exists",
			assertions: map[string]any{"$.status": map[string]any{"exists": true}},
			wantErr:    false,
		},
		{
			name:       "exists true but field missing",
			assertions: map[string]any{"$.missing": map[string]any{"exists": true}},
			wantErr:    true,
		},
		{
			name:       "exists false and field missing",
			assertions: map[string]any{"$.missing": map[string]any{"exists": false}},
			wantErr:    false,
		},
		{
			name:       "exists false but field present",
			assertions: map[string]any{"$.status": map[string]any{"exists": false}},
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := EvaluateBodyAssertions(body, tt.assertions)
			if (err != nil) != tt.wantErr {
				t.Errorf("EvaluateBodyAssertions() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestEvaluateBodyAssertions_NestedPath(t *testing.T) {
	body := []byte(`{"data": {"user": {"name": "Alice", "age": 30}}}`)

	err := EvaluateBodyAssertions(body, map[string]any{
		"$.data.user.name": "Alice",
		"$.data.user.age":  float64(30),
	})
	if err != nil {
		t.Errorf("expected no error for nested path assertions, got: %v", err)
	}
}

func TestEvaluateBodyAssertions_InvalidJSON(t *testing.T) {
	body := []byte(`not json`)

	err := EvaluateBodyAssertions(body, map[string]any{"$.status": "ok"})
	if err == nil {
		t.Error("expected error for invalid JSON body")
	}
}

func TestEvaluateBodyAssertions_UnknownOperator(t *testing.T) {
	body := []byte(`{"status": "ok"}`)

	err := EvaluateBodyAssertions(body, map[string]any{
		"$.status": map[string]any{"unknown_op": "value"},
	})
	if err == nil {
		t.Error("expected error for unknown operator")
	}
}

func TestExtractJSONPath(t *testing.T) {
	body := []byte(`{"id": "cus_123", "nested": {"value": 42}}`)

	tests := []struct {
		name    string
		path    string
		want    string
		wantErr bool
	}{
		{
			name: "simple field",
			path: "$.id",
			want: "cus_123",
		},
		{
			name: "nested field",
			path: "$.nested.value",
			want: "42",
		},
		{
			name:    "missing field",
			path:    "$.missing",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ExtractJSONPath(body, tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("ExtractJSONPath() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				gotStr := fmt.Sprintf("%v", got)
				if gotStr != tt.want {
					t.Errorf("ExtractJSONPath() = %v, want %v", gotStr, tt.want)
				}
			}
		})
	}
}

func TestValuesEqual(t *testing.T) {
	tests := []struct {
		name     string
		actual   any
		expected any
		want     bool
	}{
		{"same strings", "hello", "hello", true},
		{"different strings", "hello", "world", false},
		{"same numbers", float64(42), float64(42), true},
		{"int and float equal", float64(42), float64(42), true},
		{"different numbers", float64(42), float64(43), false},
		{"string vs number same repr", "42", float64(42), true},
		{"string vs number different repr", "hello", float64(42), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := valuesEqual(tt.actual, tt.expected)
			if got != tt.want {
				t.Errorf("valuesEqual(%v, %v) = %v, want %v", tt.actual, tt.expected, got, tt.want)
			}
		})
	}
}
