// Package workspace provides a generic engine for stateful collaboration
// platform twins. It handles entity CRUD with automatic event emission,
// hierarchical containment, configurable workflow state machines, and
// threaded conversations.
package workspace

import "time"

// Entity is the core data type. Every workspace object (channel, issue,
// contact, PR, deal) is an Entity with platform-specific fields stored
// in the Properties map.
type Entity struct {
	ID         string         `json:"id"`
	Type       string         `json:"type"`
	ParentID   string         `json:"parent_id,omitempty"`
	ParentType string         `json:"parent_type,omitempty"`
	Status     string         `json:"status,omitempty"`
	Properties map[string]any `json:"properties,omitempty"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
	CreatedBy  string         `json:"created_by,omitempty"`
}

// MutationEvent is emitted on every entity mutation. This is the engine's
// primary output — it feeds into the webhook dispatcher.
type MutationEvent struct {
	Action     string         `json:"action"`
	EntityType string         `json:"entity_type"`
	EntityID   string         `json:"entity_id"`
	Changes    map[string]any `json:"changes,omitempty"`
	OldStatus  string         `json:"old_status,omitempty"`
	NewStatus  string         `json:"new_status,omitempty"`
	Actor      string         `json:"actor,omitempty"`
	Timestamp  time.Time      `json:"timestamp"`
}

// Comment is a threaded reply attached to an entity.
type Comment struct {
	ID        string    `json:"id"`
	EntityID  string    `json:"entity_id"`
	AuthorID  string    `json:"author_id"`
	Body      any       `json:"body"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Member represents membership in a container.
type Member struct {
	UserID      string    `json:"user_id"`
	ContainerID string    `json:"container_id"`
	Role        string    `json:"role,omitempty"`
	JoinedAt    time.Time `json:"joined_at"`
}

// WorkflowConfig defines the state machine for an entity type.
type WorkflowConfig struct {
	EntityType     string              `json:"entity_type"`
	InitialStatus  string              `json:"initial_status"`
	Transitions    map[string][]string `json:"transitions"`
	TerminalStates []string            `json:"terminal_states,omitempty"`
}
