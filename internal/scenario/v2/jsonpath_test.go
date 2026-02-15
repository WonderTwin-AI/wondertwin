package v2

import (
	"fmt"
	"testing"
)

func TestJsonPathGet_SimpleFields(t *testing.T) {
	doc := map[string]any{
		"name":   "Alice",
		"age":    float64(30),
		"active": true,
	}

	tests := []struct {
		path    string
		want    string
		wantNil bool
	}{
		{"$.name", "Alice", false},
		{"$.age", "30", false},
		{"$.active", "true", false},
		{"$.missing", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			results, err := jsonPathGet(doc, tt.path)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantNil {
				if len(results) > 0 {
					t.Errorf("expected no results, got %v", results)
				}
				return
			}
			if len(results) == 0 {
				t.Fatal("expected a result, got none")
			}
			got := fmt.Sprintf("%v", results[0])
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestJsonPathGet_NestedFields(t *testing.T) {
	doc := map[string]any{
		"data": map[string]any{
			"user": map[string]any{
				"name": "Bob",
				"id":   float64(42),
			},
		},
	}

	tests := []struct {
		path    string
		want    string
		wantNil bool
	}{
		{"$.data.user.name", "Bob", false},
		{"$.data.user.id", "42", false},
		{"$.data.missing", "", true},
		{"$.data.user.missing", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			results, err := jsonPathGet(doc, tt.path)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantNil {
				if len(results) > 0 {
					t.Errorf("expected no results, got %v", results)
				}
				return
			}
			if len(results) == 0 {
				t.Fatal("expected a result, got none")
			}
			got := fmt.Sprintf("%v", results[0])
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestJsonPathGet_ArrayIndex(t *testing.T) {
	doc := map[string]any{
		"items": []any{
			map[string]any{"name": "first"},
			map[string]any{"name": "second"},
		},
	}

	tests := []struct {
		path    string
		want    string
		wantNil bool
	}{
		{"$.items[0].name", "first", false},
		{"$.items[1].name", "second", false},
		{"$.items[5].name", "", true}, // out of bounds
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			results, err := jsonPathGet(doc, tt.path)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantNil {
				if len(results) > 0 {
					t.Errorf("expected no results, got %v", results)
				}
				return
			}
			if len(results) == 0 {
				t.Fatal("expected a result, got none")
			}
			got := fmt.Sprintf("%v", results[0])
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestJsonPathGet_RootDoc(t *testing.T) {
	doc := map[string]any{"key": "value"}

	results, err := jsonPathGet(doc, "$")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected a result for $")
	}
}

func TestJsonPathGet_InvalidPath(t *testing.T) {
	doc := map[string]any{"key": "value"}

	_, err := jsonPathGet(doc, "no_dollar")
	if err == nil {
		t.Error("expected error for path without $")
	}
}
