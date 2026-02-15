// Package v2 implements a JSON-based scenario runner with rich assertions,
// variable capture, and JSONPath support.
package v2

// Scenario is a complete test scenario loaded from a JSON file.
type Scenario struct {
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Setup       *Setup            `json:"setup,omitempty"`
	Workflow    string            `json:"workflow,omitempty"`
	Variables   map[string]string `json:"variables,omitempty"`
	Steps       []Step            `json:"steps"`
}

// Setup defines pre-test actions: resetting twins and seeding data.
type Setup struct {
	Reset     []string          `json:"reset,omitempty"`
	SeedFiles map[string]string `json:"seed_files,omitempty"`
}

// Step is a single request/assert pair within a scenario.
type Step struct {
	Name    string            `json:"name"`
	Request Request           `json:"request"`
	Capture map[string]string `json:"capture,omitempty"`
	Assert  *Assert           `json:"assert,omitempty"`
}

// Request defines the HTTP request to make during a step.
type Request struct {
	Method  string            `json:"method"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers,omitempty"`
	Body    any               `json:"body,omitempty"`
}

// Assert defines the expected results of a step.
type Assert struct {
	Status       int               `json:"status,omitempty"`
	BodyContains string            `json:"body_contains,omitempty"`
	Headers      map[string]string `json:"headers,omitempty"`
	Body         map[string]any    `json:"body,omitempty"`
}
