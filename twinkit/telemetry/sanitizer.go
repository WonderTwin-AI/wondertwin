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

const maxShapeDepth = 4

// BodyShapeStats holds pre-computed metrics about a body shape.
type BodyShapeStats struct {
	Shape      map[string]any
	Depth      int
	FieldCount int
}

// ExtractBodyShapeWithStats extracts the recursive shape and computes depth/field metrics.
// Returns zero-value stats if body is empty or not valid JSON.
func ExtractBodyShapeWithStats(body []byte) BodyShapeStats {
	if len(body) == 0 {
		return BodyShapeStats{}
	}
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return BodyShapeStats{}
	}
	shape := extractShape(raw, 1)
	depth, count := shapeMetrics(shape)
	return BodyShapeStats{Shape: shape, Depth: depth, FieldCount: count}
}

// ExtractBodyShape extracts the recursive field name -> type shape from a JSON body.
// Returns nil if body is empty or not valid JSON.
func ExtractBodyShape(body []byte) map[string]any {
	if len(body) == 0 {
		return nil
	}
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil
	}
	return extractShape(raw, 1)
}

func extractShape(m map[string]any, depth int) map[string]any {
	shape := make(map[string]any, len(m))
	for k, v := range m {
		shape[k] = shapeOf(v, depth)
	}
	return shape
}

func shapeOf(v any, depth int) any {
	if v == nil {
		return "null"
	}
	switch val := v.(type) {
	case bool:
		return "boolean"
	case float64:
		return "number"
	case string:
		return "string"
	case []any:
		if depth >= maxShapeDepth || len(val) == 0 {
			return "array"
		}
		elemShape := shapeOf(val[0], depth+1)
		if nested, ok := elemShape.(map[string]any); ok {
			return map[string]any{"_type": "array", "_element": "object", "_shape": nested}
		}
		return map[string]any{"_type": "array", "_element": elemShape}
	case map[string]any:
		if depth >= maxShapeDepth {
			return "object"
		}
		return map[string]any{"_type": "object", "_shape": extractShape(val, depth+1)}
	default:
		return fmt.Sprintf("%T", v)
	}
}

// shapeMetrics computes max depth and total field count from a shape map.
func shapeMetrics(shape map[string]any) (depth, count int) {
	if shape == nil {
		return 0, 0
	}
	maxD := 1
	total := 0
	for _, v := range shape {
		total++
		switch val := v.(type) {
		case map[string]any:
			if nested, ok := val["_shape"]; ok {
				if nestedMap, ok := nested.(map[string]any); ok {
					d, c := shapeMetrics(nestedMap)
					total += c
					if d+1 > maxD {
						maxD = d + 1
					}
				}
			}
		}
	}
	return maxD, total
}
