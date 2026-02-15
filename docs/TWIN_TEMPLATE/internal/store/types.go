// Package store defines the twin's state types and in-memory store.
// Replace these types with the domain models for your target service.
package store

// Resource represents a single resource from the target service.
// Use the exact same JSON field names as the real API's responses.
type Resource struct {
	ID          string            `json:"id"`
	Object      string            `json:"object"`
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	CreatedAt   int64             `json:"created_at"`
}
