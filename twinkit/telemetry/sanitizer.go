package telemetry

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// idPattern matches path segments that look like prefixed IDs (e.g., cus_abc123,
// pi_1A2b3C). The suffix must contain at least one digit or uppercase letter to
// distinguish IDs from resource names like "payment_intents".
var idPattern = regexp.MustCompile(`^[a-z]+_[a-zA-Z0-9]*[A-Z0-9][a-zA-Z0-9]*$`)

// numericIDPattern matches purely numeric path segments.
var numericIDPattern = regexp.MustCompile(`^\d+$`)

// TemplatePath replaces ID-like path segments with {id}.
func TemplatePath(path string) string {
	parts := strings.Split(path, "/")
	for i, part := range parts {
		if part == "" {
			continue
		}
		if idPattern.MatchString(part) || numericIDPattern.MatchString(part) {
			parts[i] = "{id}"
		}
	}
	return strings.Join(parts, "/")
}

// ExtractBodyShape extracts the field name -> type shape from a JSON body.
// Returns nil if body is empty or not valid JSON.
func ExtractBodyShape(body []byte) map[string]string {
	if len(body) == 0 {
		return nil
	}
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil
	}
	return extractShape(raw)
}

func extractShape(m map[string]any) map[string]string {
	shape := make(map[string]string, len(m))
	for k, v := range m {
		shape[k] = typeOf(v)
	}
	return shape
}

func typeOf(v any) string {
	if v == nil {
		return "null"
	}
	switch v.(type) {
	case bool:
		return "boolean"
	case float64:
		return "number"
	case string:
		return "string"
	case []any:
		return "array"
	case map[string]any:
		return "object"
	default:
		return fmt.Sprintf("%T", v)
	}
}
