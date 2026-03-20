package quirks

import (
	"strings"
)

// ApplyRequestModifications applies modify_request actions to a request body.
func ApplyRequestModifications(body map[string]any, modifications []Modification) map[string]any {
	if body == nil {
		body = make(map[string]any)
	}
	for _, mod := range modifications {
		applyModification(body, mod)
	}
	return body
}

// ApplyResponseModifications applies modify_response actions to a response body.
func ApplyResponseModifications(body map[string]any, modifications []Modification) map[string]any {
	if body == nil {
		body = make(map[string]any)
	}
	for _, mod := range modifications {
		applyModification(body, mod)
	}
	return body
}

// applyModification applies a single modification to a map using dot-notation path.
func applyModification(m map[string]any, mod Modification) {
	// Strip namespace prefix if present (e.g., "request.body.currency" -> "currency")
	path := mod.Path
	for _, prefix := range []string{"request.body.", "response.body."} {
		if strings.HasPrefix(path, prefix) {
			path = path[len(prefix):]
			break
		}
	}

	parts := strings.Split(path, ".")
	if len(parts) == 0 {
		return
	}

	// Navigate to the parent of the target field.
	current := m
	for i := 0; i < len(parts)-1; i++ {
		next, ok := current[parts[i]]
		if !ok {
			if mod.Op == "remove" {
				return // nothing to remove
			}
			// Create intermediate objects for set/default.
			next = make(map[string]any)
			current[parts[i]] = next
		}
		nextMap, ok := next.(map[string]any)
		if !ok {
			return // can't traverse non-object
		}
		current = nextMap
	}

	key := parts[len(parts)-1]

	switch mod.Op {
	case "set":
		current[key] = mod.Value
	case "remove":
		delete(current, key)
	case "default":
		if _, exists := current[key]; !exists {
			current[key] = mod.Value
		}
	}
}
