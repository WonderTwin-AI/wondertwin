// Package store defines the Algolia twin's state types and in-memory store.
package store

// Synonym represents an Algolia synonym entry.
type Synonym struct {
	ObjectID     string   `json:"objectID"`
	Type         string   `json:"type"` // "synonym", "oneWaySynonym", "altCorrection1", "altCorrection2", "placeholder"
	Synonyms     []string `json:"synonyms,omitempty"`
	Input        string   `json:"input,omitempty"`        // for oneWaySynonym
	Word         string   `json:"word,omitempty"`          // for altCorrection
	Corrections  []string `json:"corrections,omitempty"`   // for altCorrection
	Placeholder  string   `json:"placeholder,omitempty"`   // for placeholder
	Replacements []string `json:"replacements,omitempty"`  // for placeholder
}

// Rule represents an Algolia query rule.
type Rule struct {
	ObjectID    string         `json:"objectID"`
	Conditions  []RuleCondition `json:"conditions,omitempty"`
	Consequence RuleConsequence `json:"consequence,omitempty"`
	Description string         `json:"description,omitempty"`
	Enabled     bool           `json:"enabled"`
}

// RuleCondition defines when a rule triggers.
type RuleCondition struct {
	Anchoring string `json:"anchoring,omitempty"` // "is", "contains", "startsWith", "endsWith"
	Pattern   string `json:"pattern,omitempty"`
	Context   string `json:"context,omitempty"`
}

// RuleConsequence defines what a rule does.
type RuleConsequence struct {
	Params   map[string]any `json:"params,omitempty"`
	Promote  []RulePromote  `json:"promote,omitempty"`
	Hide     []RuleHide     `json:"hide,omitempty"`
	UserData map[string]any `json:"userData,omitempty"`
}

// RulePromote pins a record to a position.
type RulePromote struct {
	ObjectID string `json:"objectID"`
	Position int    `json:"position"`
}

// RuleHide removes a record from results.
type RuleHide struct {
	ObjectID string `json:"objectID"`
}

// APIKey represents an Algolia API key.
type APIKey struct {
	Value                  string   `json:"value"`
	ACL                    []string `json:"acl"`
	Description            string   `json:"description,omitempty"`
	Indexes                []string `json:"indexes,omitempty"`
	MaxHitsPerQuery        int      `json:"maxHitsPerQuery,omitempty"`
	MaxQueriesPerIPPerHour int      `json:"maxQueriesPerIPPerHour,omitempty"`
	Validity               int      `json:"validity,omitempty"`
	CreatedAt              int64    `json:"createdAt"`
}

// TaskStatus represents an async task status.
type TaskStatus struct {
	TaskID    int64  `json:"taskID"`
	Status    string `json:"status"` // "published"
	UpdatedAt string `json:"updatedAt"`
}
