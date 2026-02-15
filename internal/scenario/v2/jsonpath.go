package v2

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// jsonPathGet evaluates a simple JSONPath expression against parsed JSON data.
// Supports dot-notation paths like $.field, $.field.nested, $.array[0], $.array[0].field.
// Returns a slice of matching values (empty if no match).
func jsonPathGet(doc any, path string) ([]any, error) {
	if !strings.HasPrefix(path, "$") {
		return nil, fmt.Errorf("JSONPath must start with $: %q", path)
	}

	// Strip the leading $
	rest := path[1:]
	if rest == "" {
		return []any{doc}, nil
	}

	// Strip leading dot
	if strings.HasPrefix(rest, ".") {
		rest = rest[1:]
	}

	current := doc
	segments := splitPathSegments(rest)

	for _, seg := range segments {
		if seg == "" {
			continue
		}

		// Check for array index notation: field[0]
		if idx := strings.Index(seg, "["); idx >= 0 {
			field := seg[:idx]
			indexStr := strings.TrimSuffix(seg[idx+1:], "]")

			// Navigate to the field first (if non-empty)
			if field != "" {
				val, err := getField(current, field)
				if err != nil {
					return nil, nil // no match
				}
				current = val
			}

			// Parse the array index
			arrIdx, err := strconv.Atoi(indexStr)
			if err != nil {
				return nil, fmt.Errorf("invalid array index in %q: %w", seg, err)
			}

			arr, ok := current.([]any)
			if !ok {
				return nil, nil // no match
			}
			if arrIdx < 0 || arrIdx >= len(arr) {
				return nil, nil // out of bounds
			}
			current = arr[arrIdx]
			continue
		}

		// Simple field access
		val, err := getField(current, seg)
		if err != nil {
			return nil, nil // no match
		}
		current = val
	}

	return []any{current}, nil
}

// splitPathSegments splits a path like "field.nested[0].name" into segments.
func splitPathSegments(path string) []string {
	var segments []string
	var current strings.Builder
	depth := 0

	for _, ch := range path {
		switch ch {
		case '[':
			depth++
			current.WriteRune(ch)
		case ']':
			depth--
			current.WriteRune(ch)
		case '.':
			if depth == 0 {
				segments = append(segments, current.String())
				current.Reset()
			} else {
				current.WriteRune(ch)
			}
		default:
			current.WriteRune(ch)
		}
	}

	if current.Len() > 0 {
		segments = append(segments, current.String())
	}

	return segments
}

// getField retrieves a field from a map.
func getField(doc any, field string) (any, error) {
	switch m := doc.(type) {
	case map[string]any:
		val, ok := m[field]
		if !ok {
			return nil, fmt.Errorf("field %q not found", field)
		}
		return val, nil
	default:
		return nil, fmt.Errorf("cannot access field %q on %T", field, doc)
	}
}

// parseJSONDoc parses a JSON byte slice into a generic structure.
func parseJSONDoc(body []byte) (any, error) {
	var doc any
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("response body is not valid JSON: %w", err)
	}
	return doc, nil
}
