// Package expand implements generic Stripe-style response expansion: given a
// JSON-shaped response body and a set of dotted field paths (the contents of
// a Stripe `expand[]` request parameter), it replaces ID-reference fields
// with the full resolved object, recursively for nested paths.
//
// This package implements the mechanical primitive only: given a path, turn
// the ID at that path into an object. It does not enforce which fields a
// given endpoint allows expanding — real Stripe restricts expansion to a
// documented allowlist per endpoint and errors on anything else. Callers
// that need that exact upstream contract validate paths against an
// allowlist before calling Apply; this package does not know about
// allowlists at all.
package expand

import "strings"

// Resolver looks up the full object for a Stripe-style object ID (e.g.
// "cus_000001"). It returns ok=false if the ID is unknown, in which case
// the caller leaves the original ID string in place.
type Resolver interface {
	Resolve(id string) (obj map[string]any, ok bool)
}

// maxDepth defensively bounds recursion. It is not a claim about Stripe's
// own documented expansion depth limit — it exists only to keep a
// pathological path list (or a resolver cycle) from recursing unbounded.
const maxDepth = 8

// Apply walks body in place and replaces any field named by paths whose
// current value is a string ID with the resolver's full object for that ID,
// recursing into nested paths (e.g. "latest_invoice.payment_intent" expands
// latest_invoice, then expands payment_intent within the resulting object).
// A field whose value is already an embedded object rather than an ID
// string (e.g. invoice.lines, subscription.items) is left as-is, but any
// subpaths are still walked into it.
//
// List envelopes ({"object": "list", "data": [...]}) use the "data.<field>"
// convention: a path of "data.customer" expands the customer field of every
// item in the data array.
//
// Fields that are absent or unresolved by the Resolver are left unchanged.
// body is mutated and returned for convenience; a nil body, nil resolver,
// or empty paths list is a no-op.
func Apply(body map[string]any, paths []string, resolver Resolver) map[string]any {
	if body == nil || resolver == nil || len(paths) == 0 {
		return body
	}
	applyGroups(body, groupPaths(paths), resolver, 0)
	return body
}

// groupPaths turns a flat path list into a map from the first path segment
// to the list of remaining (possibly empty) subpaths. For example,
// ["customer", "latest_invoice.payment_intent", "latest_invoice.customer"]
// becomes {"customer": [], "latest_invoice": ["payment_intent", "customer"]}.
func groupPaths(paths []string) map[string][]string {
	groups := make(map[string][]string, len(paths))
	for _, p := range paths {
		if p == "" {
			continue
		}
		head, rest, hasRest := strings.Cut(p, ".")
		if _, ok := groups[head]; !ok {
			groups[head] = nil
		}
		if hasRest {
			groups[head] = append(groups[head], rest)
		}
	}
	return groups
}

func applyGroups(body map[string]any, groups map[string][]string, resolver Resolver, depth int) {
	if depth >= maxDepth {
		return
	}

	if dataSubpaths, ok := groups["data"]; ok && len(dataSubpaths) > 0 {
		if data, ok := body["data"].([]any); ok {
			dataGroups := groupPaths(dataSubpaths)
			for _, item := range data {
				if m, ok := item.(map[string]any); ok {
					applyGroups(m, dataGroups, resolver, depth+1)
				}
			}
		}
	}

	for field, subpaths := range groups {
		if field == "data" {
			continue // handled above as list expansion, not a plain field
		}
		raw, ok := body[field]
		if !ok {
			continue
		}

		switch v := raw.(type) {
		case string:
			if v == "" {
				continue
			}
			resolved, ok := resolver.Resolve(v)
			if !ok {
				continue
			}
			body[field] = resolved
			if len(subpaths) > 0 {
				applyGroups(resolved, groupPaths(subpaths), resolver, depth+1)
			}
		case map[string]any:
			// Already an embedded object rather than a lazily-expandable ID
			// reference (e.g. Stripe's always-embedded list sub-resources
			// like invoice.lines or subscription.items). There is nothing
			// to substitute, but a deeper path still needs to walk into it
			// (e.g. "lines.data.price" reaches the list-envelope handling
			// above on the next recursion).
			if len(subpaths) > 0 {
				applyGroups(v, groupPaths(subpaths), resolver, depth+1)
			}
		}
	}
}
