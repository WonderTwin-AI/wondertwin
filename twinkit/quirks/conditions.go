package quirks

import (
	"fmt"
	"regexp"
	"strings"
)

// evaluateConditions checks if all conditions in a rule match the given context.
// All conditions are ANDed — if any fails, the rule doesn't fire.
func evaluateConditions(conditions []Condition, reqCtx *RequestContext, respCtx *ResponseContext) bool {
	for _, cond := range conditions {
		if !evaluateCondition(cond, reqCtx, respCtx) {
			return false
		}
	}
	return true
}

// evaluateCondition checks a single condition against the available context.
func evaluateCondition(cond Condition, reqCtx *RequestContext, respCtx *ResponseContext) bool {
	value := resolveField(cond.Field, reqCtx, respCtx)
	return applyOperator(cond.Operator, value, cond.Value)
}

// resolveField extracts a value from the context using dot-notation field paths.
// Supported namespaces: request.path, request.method, request.header.*,
// request.body.*, request.query.*, response.status, response.body.*
func resolveField(field string, reqCtx *RequestContext, respCtx *ResponseContext) any {
	parts := strings.SplitN(field, ".", 2)
	if len(parts) < 2 {
		return nil
	}

	namespace := parts[0]
	rest := parts[1]

	switch namespace {
	case "request":
		if reqCtx == nil {
			return nil
		}
		return resolveRequestField(rest, reqCtx)
	case "response":
		if respCtx == nil {
			return nil
		}
		return resolveResponseField(rest, respCtx)
	}
	return nil
}

func resolveRequestField(field string, ctx *RequestContext) any {
	switch {
	case field == "path":
		return ctx.Path
	case field == "method":
		return ctx.Method
	case strings.HasPrefix(field, "header."):
		key := field[7:]
		return ctx.Headers[key]
	case strings.HasPrefix(field, "query."):
		key := field[6:]
		return ctx.Query[key]
	case strings.HasPrefix(field, "body."):
		path := field[5:]
		return resolveJSONPath(ctx.Body, path)
	case field == "body":
		return ctx.Body
	}
	return nil
}

func resolveResponseField(field string, ctx *ResponseContext) any {
	switch {
	case field == "status":
		return ctx.StatusCode
	case strings.HasPrefix(field, "body."):
		path := field[5:]
		return resolveJSONPath(ctx.Body, path)
	case field == "body":
		return ctx.Body
	}
	return nil
}

// resolveJSONPath traverses a nested map using dot-notation.
// "payment_method_data.type" resolves m["payment_method_data"]["type"].
func resolveJSONPath(m map[string]any, path string) any {
	parts := strings.Split(path, ".")
	current := any(m)
	for _, part := range parts {
		obj, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current, ok = obj[part]
		if !ok {
			return nil
		}
	}
	return current
}

// applyOperator evaluates an operator against actual and expected values.
func applyOperator(op string, actual, expected any) bool {
	switch op {
	case "eq":
		return fmt.Sprintf("%v", actual) == fmt.Sprintf("%v", expected)
	case "neq":
		return fmt.Sprintf("%v", actual) != fmt.Sprintf("%v", expected)
	case "gt":
		return compareNumeric(actual, expected) > 0
	case "lt":
		return compareNumeric(actual, expected) < 0
	case "gte":
		return compareNumeric(actual, expected) >= 0
	case "lte":
		return compareNumeric(actual, expected) <= 0
	case "contains":
		return strings.Contains(fmt.Sprintf("%v", actual), fmt.Sprintf("%v", expected))
	case "matches":
		pattern, ok := expected.(string)
		if !ok {
			return false
		}
		re, err := regexp.Compile(pattern)
		if err != nil {
			return false
		}
		return re.MatchString(fmt.Sprintf("%v", actual))
	case "exists":
		return actual != nil
	case "not_exists":
		return actual == nil
	case "in":
		return inSlice(actual, expected)
	default:
		return false
	}
}

func compareNumeric(a, b any) int {
	af := toFloat(a)
	bf := toFloat(b)
	if af < bf {
		return -1
	}
	if af > bf {
		return 1
	}
	return 0
}

func toFloat(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case string:
		var f float64
		fmt.Sscanf(n, "%f", &f)
		return f
	default:
		return 0
	}
}

func inSlice(actual, expected any) bool {
	actualStr := fmt.Sprintf("%v", actual)
	switch v := expected.(type) {
	case []any:
		for _, item := range v {
			if fmt.Sprintf("%v", item) == actualStr {
				return true
			}
		}
	case []string:
		for _, item := range v {
			if item == actualStr {
				return true
			}
		}
	}
	return false
}
