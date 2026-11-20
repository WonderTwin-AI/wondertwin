// Package store defines the PostHog twin's state types and in-memory store.
package store

// StoredEvent is an analytics event the emulator holds after capture. Token is
// the project key the event arrived under; ReceivedAt is read from the
// emulator clock, so it is deterministic when the clock is seeded.
type StoredEvent struct {
	UUID       string         `json:"uuid"`
	Event      string         `json:"event"`
	DistinctID string         `json:"distinct_id"`
	Properties map[string]any `json:"properties,omitempty"`
	Timestamp  string         `json:"timestamp"`
	Token      string         `json:"token"`
	ReceivedAt string         `json:"received_at"`
}

// FeatureFlag represents a PostHog feature flag configuration.
type FeatureFlag struct {
	Key     string `json:"key"`
	Enabled bool   `json:"enabled"`
	Variant string `json:"variant,omitempty"` // optional multivariate variant
	Payload string `json:"payload,omitempty"` // optional JSON payload
}

// Person represents an identified user with properties.
type Person struct {
	DistinctID string         `json:"distinct_id"`
	Properties map[string]any `json:"properties,omitempty"`
	CreatedAt  string         `json:"created_at"`
}

// AliasMapping links two distinct IDs.
type AliasMapping struct {
	Alias      string `json:"alias"`
	DistinctID string `json:"distinct_id"`
	CreatedAt  string `json:"created_at"`
}

// Group represents a group identity.
type Group struct {
	Type       string         `json:"type"`
	Key        string         `json:"key"`
	Properties map[string]any `json:"properties,omitempty"`
	CreatedAt  string         `json:"created_at"`
}
