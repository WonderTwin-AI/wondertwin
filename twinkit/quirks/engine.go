package quirks

import (
	"encoding/json"
	"sync"
)

// Engine loads and evaluates quirk rules at runtime.
type Engine struct {
	mu       sync.RWMutex
	rules    map[string]*Rule   // id -> rule
	byPath   map[string][]*Rule // path prefix -> rules (for fast lookup)
	allRules []*Rule            // all rules in load order
	stats    map[string]int     // id -> fire count
}

// NewEngine creates a new quirks engine with no rules loaded.
func NewEngine() *Engine {
	return &Engine{
		rules:  make(map[string]*Rule),
		byPath: make(map[string][]*Rule),
		stats:  make(map[string]int),
	}
}

// LoadRules loads a set of rules from intelligence data.
// Replaces any previously loaded rules.
func (e *Engine) LoadRules(rules []Rule) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.rules = make(map[string]*Rule, len(rules))
	e.byPath = make(map[string][]*Rule)
	e.allRules = make([]*Rule, 0, len(rules))
	e.stats = make(map[string]int)

	for i := range rules {
		r := &rules[i]
		if r.ID == "" {
			continue
		}
		e.rules[r.ID] = r
		e.allRules = append(e.allRules, r)

		// Index by path condition for fast lookup.
		for _, cond := range r.Conditions {
			if cond.Field == "request.path" && (cond.Operator == "eq" || cond.Operator == "matches") {
				pathKey, _ := cond.Value.(string)
				if pathKey != "" {
					e.byPath[pathKey] = append(e.byPath[pathKey], r)
				}
			}
		}
	}
}

// LoadRulesJSON loads rules from a JSON byte slice.
func (e *Engine) LoadRulesJSON(data []byte) error {
	var rules []Rule
	if err := json.Unmarshal(data, &rules); err != nil {
		return err
	}
	e.LoadRules(rules)
	return nil
}

// RuleCount returns the number of loaded rules.
func (e *Engine) RuleCount() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return len(e.rules)
}

// EnableQuirk enables a rule by ID.
func (e *Engine) EnableQuirk(id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	r, ok := e.rules[id]
	if !ok {
		return &QuirkNotFoundError{ID: id}
	}
	r.Enabled = true
	return nil
}

// DisableQuirk disables a rule by ID.
func (e *Engine) DisableQuirk(id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	r, ok := e.rules[id]
	if !ok {
		return &QuirkNotFoundError{ID: id}
	}
	r.Enabled = false
	return nil
}

// IsEnabled returns whether a quirk is enabled.
func (e *Engine) IsEnabled(id string) bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	r, ok := e.rules[id]
	return ok && r.Enabled
}

// ListQuirks returns the status of all loaded rules.
func (e *Engine) ListQuirks() []QuirkStatus {
	e.mu.RLock()
	defer e.mu.RUnlock()
	result := make([]QuirkStatus, 0, len(e.allRules))
	for _, r := range e.allRules {
		result = append(result, QuirkStatus{
			ID:      r.ID,
			Enabled: r.Enabled,
			Summary: r.Summary,
			Fired:   e.stats[r.ID],
		})
	}
	return result
}

// EvaluatePre evaluates all enabled rules against a request context (pre-handler).
// Returns actions to apply. Only returns actions of type: modify_request, reject, delay.
func (e *Engine) EvaluatePre(ctx *RequestContext) []Action {
	e.mu.RLock()
	defer e.mu.RUnlock()

	var actions []Action
	for _, r := range e.allRules {
		if !r.Enabled {
			continue
		}
		if !evaluateConditions(r.Conditions, ctx, nil) {
			continue
		}
		switch r.Action.Type {
		case "modify_request", "reject", "delay":
			actions = append(actions, r.Action)
			e.stats[r.ID]++
		}
	}
	return actions
}

// EvaluatePost evaluates all enabled rules against a response context (post-handler).
// Returns actions to apply. Only returns actions of type: modify_response.
func (e *Engine) EvaluatePost(ctx *ResponseContext) []Action {
	e.mu.RLock()
	defer e.mu.RUnlock()

	var actions []Action
	for _, r := range e.allRules {
		if !r.Enabled {
			continue
		}
		if !evaluateConditions(r.Conditions, ctx.Request, ctx) {
			continue
		}
		switch r.Action.Type {
		case "modify_response":
			actions = append(actions, r.Action)
			e.stats[r.ID]++
		}
	}
	return actions
}

// QuirkNotFoundError is returned when a quirk ID is not found.
type QuirkNotFoundError struct {
	ID string
}

func (e *QuirkNotFoundError) Error() string {
	return "quirk " + e.ID + " not found"
}
