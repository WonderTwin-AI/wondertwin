package v2

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/wondertwin-ai/wondertwin/internal/manifest"
)

// ExpandTemplates replaces template placeholders in a string:
//   - {{twins.<name>.port}} and {{twins.<name>.admin_port}} from the manifest
//   - {{env.VARIABLE}} from environment variables
//   - {{variable_name}} from captured variables
func ExpandTemplates(s string, m *manifest.Manifest, vars map[string]string) (string, error) {
	result := s
	for {
		start := strings.Index(result, "{{")
		if start == -1 {
			break
		}
		end := strings.Index(result[start:], "}}")
		if end == -1 {
			return "", fmt.Errorf("unterminated template expression at position %d", start)
		}
		end += start + 2 // move past "}}"

		expr := result[start+2 : end-2] // strip {{ and }}

		value, err := resolveExpr(expr, m, vars)
		if err != nil {
			return "", err
		}

		result = result[:start] + value + result[end:]
	}
	return result, nil
}

// resolveExpr resolves a template expression.
func resolveExpr(expr string, m *manifest.Manifest, vars map[string]string) (string, error) {
	// twins.<name>.<field>
	if strings.HasPrefix(expr, "twins.") {
		return resolveTwinExpr(expr, m)
	}

	// env.VARIABLE
	if strings.HasPrefix(expr, "env.") {
		envKey := expr[4:]
		val := os.Getenv(envKey)
		return val, nil
	}

	// Captured variable
	if vars != nil {
		if val, ok := vars[expr]; ok {
			return val, nil
		}
	}

	return "", fmt.Errorf("unresolved template expression: %q", expr)
}

// resolveTwinExpr resolves twins.<name>.<field> expressions.
func resolveTwinExpr(expr string, m *manifest.Manifest) (string, error) {
	parts := strings.SplitN(expr, ".", 3)
	if len(parts) != 3 || parts[0] != "twins" {
		return "", fmt.Errorf("invalid twin template expression: %q (expected twins.<name>.<field>)", expr)
	}

	twinName := parts[1]
	field := parts[2]

	twin, err := m.Twin(twinName)
	if err != nil {
		return "", fmt.Errorf("template %q: %w", expr, err)
	}

	switch field {
	case "port":
		return strconv.Itoa(twin.Port), nil
	case "admin_port":
		return strconv.Itoa(twin.AdminPort), nil
	default:
		return "", fmt.Errorf("template %q: unknown field %q (expected port or admin_port)", expr, field)
	}
}
