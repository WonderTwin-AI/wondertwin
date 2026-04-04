// Package domainevents defines the interface for emitting domain-level
// behavioral events from twinkit domain engines. This package decouples
// domain engines from concrete telemetry implementations.
package domainevents

// DomainEvent captures a behavioral inflection point in a domain engine.
type DomainEvent struct {
	Engine     string         `json:"engine"`
	Hook       string         `json:"hook"`
	Resource   string         `json:"resource,omitempty"`
	Action     string         `json:"action,omitempty"`
	EntityType string         `json:"entity_type,omitempty"`
	EntityID   string         `json:"entity_id,omitempty"`
	StateFrom  string         `json:"state_from,omitempty"`
	StateTo    string         `json:"state_to,omitempty"`
	Context    map[string]any `json:"context,omitempty"`
}

// Emitter is the interface that domain engines use to emit behavioral events.
// The concrete implementation lives in the telemetry package.
type Emitter interface {
	EmitDomainEvent(correlationID string, event DomainEvent)
}
