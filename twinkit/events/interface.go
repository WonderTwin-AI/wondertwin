// Package events provides an event stream engine for twins that simulate
// analytics, CDP, feature flag, and marketing automation platforms. It handles
// event ingestion, user profile accumulation, identity resolution, and
// audience segment matching.
package events

import "time"

// Event is a single tracked event in the stream.
type Event struct {
	ID         string         `json:"id"`
	EventName  string         `json:"event"`
	DistinctID string         `json:"distinct_id"`
	Properties map[string]any `json:"properties,omitempty"`
	Timestamp  time.Time      `json:"timestamp"`
}

// UserProfile represents accumulated properties for a user.
// Properties are built up from identify calls and enriched from events.
type UserProfile struct {
	DistinctID   string         `json:"distinct_id"`
	Properties   map[string]any `json:"properties,omitempty"`
	AnonymousIDs []string       `json:"anonymous_ids,omitempty"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
}

// IdentifyCall links properties to a user, and optionally merges an
// anonymous ID into a known user ID.
type IdentifyCall struct {
	DistinctID  string         `json:"distinct_id"`
	AnonymousID string         `json:"anonymous_id,omitempty"`
	Properties  map[string]any `json:"properties,omitempty"`
}

// SegmentFilter defines criteria for matching users into an audience segment.
type SegmentFilter struct {
	// PropertyFilters match against user profile properties.
	// All filters must match (AND logic).
	PropertyFilters []PropertyFilter `json:"property_filters,omitempty"`
	// EventFilter matches users who have performed a specific event.
	EventFilter *EventFilter `json:"event_filter,omitempty"`
}

// PropertyFilter matches a user property against a value.
type PropertyFilter struct {
	Key      string `json:"key"`
	Operator string `json:"operator"` // "eq", "neq", "gt", "lt", "gte", "lte", "contains", "exists"
	Value    any    `json:"value"`
}

// EventFilter matches users who have performed a specific event.
type EventFilter struct {
	EventName string `json:"event_name"`
	MinCount  int    `json:"min_count,omitempty"` // minimum number of times (default 1)
}
