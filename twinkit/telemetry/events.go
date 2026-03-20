// Package telemetry provides anonymized behavioral telemetry collection for
// WonderTwin twins. All data is sanitized locally before emission — only
// shapes (field names + types), never values.
package telemetry

// EventType distinguishes telemetry event categories.
type EventType string

const (
	EventHTTPObservation EventType = "http_observation"
	EventDomainEvent     EventType = "domain_event"
	EventScenarioResult  EventType = "scenario_result"
)

// Event is the base telemetry event envelope.
type Event struct {
	EventID       string    `json:"event_id,omitempty"`
	EventType     EventType `json:"event_type"`
	Twin          string    `json:"twin"`
	TwinVersion   string    `json:"twin_version"`
	OrgID         string    `json:"org_id"`
	Timestamp     int64     `json:"timestamp"`
	CorrelationID string    `json:"correlation_id,omitempty"`
	Payload       any       `json:"payload"`
}

// HTTPObservation captures the shape of an HTTP request/response pair.
type HTTPObservation struct {
	Method            string            `json:"method"`
	PathTemplate      string            `json:"path_template"`
	Status            int               `json:"status"`
	DurationMS        int64             `json:"duration_ms"`
	RequestBodyShape  map[string]string `json:"request_body_shape,omitempty"`
	ResponseBodyShape map[string]string `json:"response_body_shape,omitempty"`
	ErrorCode         string            `json:"error_code,omitempty"`
	Resource          string            `json:"resource,omitempty"`
}

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

// ScenarioResult captures the outcome of a scenario run.
type ScenarioResult struct {
	ScenarioName string       `json:"scenario_name"`
	Passed       bool         `json:"passed"`
	StepCount    int          `json:"step_count"`
	Steps        []StepResult `json:"steps,omitempty"`
	SeenInProd   *bool        `json:"seen_in_production"` // nullable
}

// StepResult captures one step in a scenario.
type StepResult struct {
	Name       string `json:"name"`
	Passed     bool   `json:"passed"`
	DurationMS int64  `json:"duration_ms"`
}
