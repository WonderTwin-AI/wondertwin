// Package quirks provides a runtime behavioral modification engine for
// WonderTwin twins. It loads structured rules from intelligence data and
// evaluates them against incoming requests to modify twin behavior —
// altering requests, responses, injecting errors, or adding delays.
//
// Quirks operate at the HTTP/JSON level, independent of domain engines.
// They sit in the request pipeline as middleware.
package quirks

import "time"

// Rule is a machine-executable behavioral observation.
// Loaded from the Content API intelligence payload at runtime.
type Rule struct {
	ID         string      `json:"id"`
	Enabled    bool        `json:"enabled"`
	Conditions []Condition `json:"conditions"`
	Action     Action      `json:"action"`
	Summary    string      `json:"summary,omitempty"`
}

// Condition defines a predicate that must be true for a rule to fire.
// All conditions in a rule are ANDed together.
type Condition struct {
	Field    string `json:"field"`  // e.g., "request.path", "request.body.amount"
	Operator string `json:"op"`    // eq, neq, gt, lt, gte, lte, matches, contains, exists, not_exists, in
	Value    any    `json:"value"` // the value to compare against
}

// Action defines what happens when a rule's conditions match.
type Action struct {
	Type          string         `json:"type"`                    // modify_request, modify_response, reject, delay
	Modifications []Modification `json:"modifications,omitempty"` // for modify_request and modify_response
	StatusCode    int            `json:"status_code,omitempty"`   // for reject
	Body          any            `json:"body,omitempty"`          // for reject (response body)
	Delay         time.Duration  `json:"delay,omitempty"`         // for delay
}

// Modification is a single change to apply to a request or response body.
type Modification struct {
	Op    string `json:"op"`              // set, remove, default (set if not present)
	Path  string `json:"path"`            // dot-notation path into the JSON body
	Value any    `json:"value,omitempty"` // for set and default
}

// RequestContext provides the data available for condition evaluation
// during the pre-processing phase (before the handler runs).
type RequestContext struct {
	Method  string
	Path    string
	Headers map[string]string
	Query   map[string]string
	Body    map[string]any
}

// ResponseContext provides the data available for condition evaluation
// during the post-processing phase (after the handler runs).
type ResponseContext struct {
	Request    *RequestContext
	StatusCode int
	Body       map[string]any
}

// QuirkStatus is the admin-visible status of a quirk rule.
type QuirkStatus struct {
	ID      string `json:"id"`
	Enabled bool   `json:"enabled"`
	Summary string `json:"summary,omitempty"`
	Fired   int    `json:"fired"` // number of times this quirk has been applied
}
