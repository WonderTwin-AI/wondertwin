package v2

import (
	"fmt"
	"regexp"
	"strings"
)

// EvaluateBodyAssertions evaluates JSONPath-based body assertions against a response body.
func EvaluateBodyAssertions(body []byte, assertions map[string]any) error {
	parsed, err := parseJSONDoc(body)
	if err != nil {
		return err
	}

	for path, expected := range assertions {
		if err := evaluateOne(parsed, path, expected); err != nil {
			return err
		}
	}
	return nil
}

// evaluateOne evaluates a single JSONPath assertion.
func evaluateOne(doc any, path string, expected any) error {
	results, err := jsonPathGet(doc, path)
	if err != nil {
		return fmt.Errorf("invalid JSONPath %q: %w", path, err)
	}

	// Handle operator-based assertions (map form)
	if opMap, ok := expected.(map[string]any); ok {
		return evaluateOperators(path, results, opMap)
	}

	// Simple equality: exact value match
	if len(results) == 0 {
		return fmt.Errorf("JSONPath %q: no match found", path)
	}

	actual := results[0]
	if !valuesEqual(actual, expected) {
		return fmt.Errorf("JSONPath %q: expected %v (%T), got %v (%T)", path, expected, expected, actual, actual)
	}

	return nil
}

// evaluateOperators processes operator-based assertions like {"eq": v}, {"gte": n}, etc.
func evaluateOperators(path string, results []any, ops map[string]any) error {
	for op, expected := range ops {
		switch op {
		case "exists":
			wantExists, ok := expected.(bool)
			if !ok {
				return fmt.Errorf("JSONPath %q: 'exists' operator requires a boolean value", path)
			}
			hasResults := len(results) > 0
			if wantExists && !hasResults {
				return fmt.Errorf("JSONPath %q: expected to exist but no match found", path)
			}
			if !wantExists && hasResults {
				return fmt.Errorf("JSONPath %q: expected not to exist but found %v", path, results[0])
			}

		case "eq":
			if len(results) == 0 {
				return fmt.Errorf("JSONPath %q: no match found for 'eq' check", path)
			}
			if !valuesEqual(results[0], expected) {
				return fmt.Errorf("JSONPath %q: expected eq %v, got %v", path, expected, results[0])
			}

		case "gte":
			if len(results) == 0 {
				return fmt.Errorf("JSONPath %q: no match found for 'gte' check", path)
			}
			actualNum, err := toFloat64(results[0])
			if err != nil {
				return fmt.Errorf("JSONPath %q: 'gte' requires numeric actual value: %w", path, err)
			}
			expectedNum, err := toFloat64(expected)
			if err != nil {
				return fmt.Errorf("JSONPath %q: 'gte' requires numeric expected value: %w", path, err)
			}
			if actualNum < expectedNum {
				return fmt.Errorf("JSONPath %q: expected >= %v, got %v", path, expectedNum, actualNum)
			}

		case "lte":
			if len(results) == 0 {
				return fmt.Errorf("JSONPath %q: no match found for 'lte' check", path)
			}
			actualNum, err := toFloat64(results[0])
			if err != nil {
				return fmt.Errorf("JSONPath %q: 'lte' requires numeric actual value: %w", path, err)
			}
			expectedNum, err := toFloat64(expected)
			if err != nil {
				return fmt.Errorf("JSONPath %q: 'lte' requires numeric expected value: %w", path, err)
			}
			if actualNum > expectedNum {
				return fmt.Errorf("JSONPath %q: expected <= %v, got %v", path, expectedNum, actualNum)
			}

		case "contains":
			if len(results) == 0 {
				return fmt.Errorf("JSONPath %q: no match found for 'contains' check", path)
			}
			actualStr := fmt.Sprintf("%v", results[0])
			expectedStr := fmt.Sprintf("%v", expected)
			if !strings.Contains(actualStr, expectedStr) {
				return fmt.Errorf("JSONPath %q: expected to contain %q, got %q", path, expectedStr, actualStr)
			}

		case "regex":
			if len(results) == 0 {
				return fmt.Errorf("JSONPath %q: no match found for 'regex' check", path)
			}
			pattern, ok := expected.(string)
			if !ok {
				return fmt.Errorf("JSONPath %q: 'regex' operator requires a string pattern", path)
			}
			re, err := regexp.Compile(pattern)
			if err != nil {
				return fmt.Errorf("JSONPath %q: invalid regex pattern %q: %w", path, pattern, err)
			}
			actualStr := fmt.Sprintf("%v", results[0])
			if !re.MatchString(actualStr) {
				return fmt.Errorf("JSONPath %q: value %q does not match regex %q", path, actualStr, pattern)
			}

		default:
			return fmt.Errorf("JSONPath %q: unknown operator %q", path, op)
		}
	}
	return nil
}

// valuesEqual compares two values for equality, handling numeric type coercion.
// Values must be the same kind (both numeric or both string) to be equal.
func valuesEqual(actual, expected any) bool {
	actualNum, aErr := toFloat64(actual)
	expectedNum, eErr := toFloat64(expected)

	// Both numeric: compare as numbers
	if aErr == nil && eErr == nil {
		return actualNum == expectedNum
	}

	// One numeric, one not: different types are not equal
	if (aErr == nil) != (eErr == nil) {
		return false
	}

	// Both non-numeric: compare as strings
	return fmt.Sprintf("%v", actual) == fmt.Sprintf("%v", expected)
}

// toFloat64 converts a value to float64.
func toFloat64(v any) (float64, error) {
	switch n := v.(type) {
	case float64:
		return n, nil
	case float32:
		return float64(n), nil
	case int:
		return float64(n), nil
	case int64:
		return float64(n), nil
	case int32:
		return float64(n), nil
	default:
		return 0, fmt.Errorf("value %v (%T) is not numeric", v, v)
	}
}

// ExtractJSONPath extracts a value from parsed JSON using a JSONPath expression.
func ExtractJSONPath(body []byte, path string) (any, error) {
	parsed, err := parseJSONDoc(body)
	if err != nil {
		return nil, err
	}

	results, err := jsonPathGet(parsed, path)
	if err != nil {
		return nil, fmt.Errorf("invalid JSONPath %q: %w", path, err)
	}

	if len(results) == 0 {
		return nil, fmt.Errorf("JSONPath %q: no match found", path)
	}

	return results[0], nil
}
