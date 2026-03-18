package api

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// FilterItems applies a WHERE clause from a ParsedQuery to a slice of items.
// It uses JSON marshaling to extract field values generically.
func FilterItems[T any](items []T, pq *ParsedQuery) []T {
	if pq.WhereField == "" {
		return items
	}

	var result []T
	for _, item := range items {
		if matchesWhere(item, pq.WhereField, pq.WhereOp, pq.WhereValue) {
			result = append(result, item)
		}
	}
	return result
}

// matchesWhere checks if a single item matches the WHERE condition.
func matchesWhere(item any, field, op, value string) bool {
	fieldVal, ok := extractField(item, field)
	if !ok {
		return false
	}

	switch op {
	case "=":
		return fieldVal == value
	case "!=":
		return fieldVal != value
	case "<":
		return compareNumeric(fieldVal, value) < 0
	case ">":
		return compareNumeric(fieldVal, value) > 0
	case "<=":
		return compareNumeric(fieldVal, value) <= 0
	case ">=":
		return compareNumeric(fieldVal, value) >= 0
	case "LIKE":
		return matchLike(fieldVal, value)
	case "IN":
		// IN values are comma-separated in the parsed value.
		for _, v := range strings.Split(value, ",") {
			if strings.TrimSpace(v) == fieldVal {
				return true
			}
		}
		return false
	default:
		return false
	}
}

// extractField gets a field value from an entity by JSON field name.
// Handles both PascalCase JSON tags (QBO style) and common field aliases.
func extractField(item any, field string) (string, bool) {
	data, err := json.Marshal(item)
	if err != nil {
		return "", false
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return "", false
	}

	// Direct field lookup.
	if v, ok := m[field]; ok {
		return fmt.Sprintf("%v", v), true
	}

	// Try common QBO field aliases.
	// QBO queries use field names like "Id", "DisplayName", "Active", "Balance",
	// "TotalAmt", "CustomerRef", "VendorRef", "TxnDate", etc.
	// Ref fields need special handling — "CustomerRef" in a query means CustomerRef.value.
	if strings.HasSuffix(field, "Ref") {
		if ref, ok := m[field]; ok {
			if refMap, ok := ref.(map[string]any); ok {
				if v, ok := refMap["value"]; ok {
					return fmt.Sprintf("%v", v), true
				}
			}
		}
	}

	return "", false
}

func compareNumeric(a, b string) int {
	fa, errA := strconv.ParseFloat(a, 64)
	fb, errB := strconv.ParseFloat(b, 64)
	if errA != nil || errB != nil {
		return strings.Compare(a, b)
	}
	if fa < fb {
		return -1
	}
	if fa > fb {
		return 1
	}
	return 0
}

func matchLike(value, pattern string) bool {
	// QBO LIKE uses % as wildcard.
	pattern = strings.ToLower(pattern)
	value = strings.ToLower(value)

	if strings.HasPrefix(pattern, "%") && strings.HasSuffix(pattern, "%") {
		return strings.Contains(value, strings.Trim(pattern, "%"))
	}
	if strings.HasPrefix(pattern, "%") {
		return strings.HasSuffix(value, strings.TrimPrefix(pattern, "%"))
	}
	if strings.HasSuffix(pattern, "%") {
		return strings.HasPrefix(value, strings.TrimSuffix(pattern, "%"))
	}
	return value == pattern
}
