package twincore

import "strings"

// ResourceID is a standardized identifier for an API resource.
// Format: lowercase, underscore-separated, matching the SDK's resource naming.
// Examples: "invoices", "payment_intents", "webhook_endpoints".
type ResourceID string

// String returns the resource ID as a string.
func (r ResourceID) String() string { return string(r) }

// NormalizeResourceID converts a resource name to the standard format:
// lowercase, trimmed, slashes and hyphens replaced with underscores,
// leading/trailing underscores stripped.
func NormalizeResourceID(name string) ResourceID {
	s := strings.ToLower(strings.TrimSpace(name))
	s = strings.ReplaceAll(s, "-", "_")
	s = strings.ReplaceAll(s, "/", "_")
	s = strings.Trim(s, "_")
	return ResourceID(s)
}

// ResourceRoute returns the URL path segment for this resource.
// Example: ResourceID("payment_intents") → "payment_intents"
func (r ResourceID) ResourceRoute() string {
	return string(r)
}
