package scenario

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/wondertwin-ai/wondertwin/internal/manifest"
)

// ExpandTemplates replaces {{twins.<name>.port}} and {{twins.<name>.admin_port}}
// placeholders in a string with actual port values from the manifest.
func ExpandTemplates(s string, m *manifest.Manifest) (string, error) {
	result := s
	for {
		start := strings.Index(result, "{{twins.")
		if start == -1 {
			break
		}
		end := strings.Index(result[start:], "}}")
		if end == -1 {
			return "", fmt.Errorf("unterminated template expression at position %d", start)
		}
		end += start + 2 // move past "}}"

		// Extract the inner expression: twins.<name>.<field>
		expr := result[start+2 : end-2] // strip {{ and }}
		value, err := resolveExpr(expr, m)
		if err != nil {
			return "", err
		}

		result = result[:start] + value + result[end:]
	}
	return result, nil
}

// resolveExpr resolves an expression like "twins.stripe.port" or "twins.stripe.admin_port".
func resolveExpr(expr string, m *manifest.Manifest) (string, error) {
	parts := strings.SplitN(expr, ".", 3)
	if len(parts) != 3 || parts[0] != "twins" {
		return "", fmt.Errorf("invalid template expression: %q (expected twins.<name>.<field>)", expr)
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
