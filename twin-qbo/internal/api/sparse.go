package api

import "encoding/json"

// SparseUpdate detects if the raw JSON has "sparse": true and if so,
// merges the request fields into the existing entity. Returns the merged
// entity. If not sparse, returns the request entity unchanged.
//
// The merge works by: marshal existing → overlay request fields → unmarshal.
// This preserves existing fields not present in the request.
func SparseUpdate[T any](existing T, requestJSON []byte) (T, error) {
	// Marshal existing to a map.
	existingBytes, err := json.Marshal(existing)
	if err != nil {
		return existing, err
	}
	var base map[string]any
	if err := json.Unmarshal(existingBytes, &base); err != nil {
		return existing, err
	}

	// Unmarshal request to a map (only has the fields the client sent).
	var overlay map[string]any
	if err := json.Unmarshal(requestJSON, &overlay); err != nil {
		return existing, err
	}

	// Overlay request fields onto base.
	for k, v := range overlay {
		base[k] = v
	}

	// Marshal merged map back to the entity type.
	merged, err := json.Marshal(base)
	if err != nil {
		return existing, err
	}
	var result T
	if err := json.Unmarshal(merged, &result); err != nil {
		return existing, err
	}
	return result, nil
}

// IsSparse checks if the raw JSON contains "sparse": true.
func IsSparse(raw []byte) bool {
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return false
	}
	v, ok := m["sparse"]
	if !ok {
		return false
	}
	b, ok := v.(bool)
	return ok && b
}
