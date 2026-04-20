package contract

// Engine is the core interface for domain-specific active inference engines.
// Engines maintain beliefs about hidden states, update from observations,
// and recommend policies that balance exploration and exploitation.
type Engine interface {
	// Observe updates internal beliefs given an observation.
	Observe(obs Observation) error

	// Policy returns the currently recommended action based on Expected Free Energy.
	Policy() Policy

	// Belief returns the current belief state.
	Belief() Belief

	// Signal returns diagnostic information about model adequacy.
	Signal() EngineSignal

	// Reset clears all state and returns to priors.
	Reset()
}

// Observation represents sensory input to an engine.
type Observation struct {
	Type       string         // observation type identifier
	Confidence float64        // 0-1 confidence in this observation
	Payload    map[string]any // domain-specific data
	Timestamp  int64          // unix timestamp (optional)
}

// Policy represents a recommended action with scoring breakdown.
type Policy struct {
	Action    string  // action identifier
	EFE       float64 // total expected free energy score
	Epistemic float64 // information gain component
	Pragmatic float64 // goal achievement component
	Cost      float64 // action cost component
}

// Belief provides access to the engine's internal state.
type Belief interface {
	// Entropy returns the total uncertainty in nats.
	Entropy() float64

	// Certainty returns a concentration measure (inverse entropy).
	Certainty() float64

	// Summary returns a human-readable belief summary.
	Summary() map[string]any
}

// EngineSignal indicates model inadequacy or diagnostic information.
type EngineSignal struct {
	Engine            string        // engine identifier
	Iteration         int           // observation count since last reset
	ResidualEntropy   float64       // entropy that won't resolve with current model
	EFEFloor          float64       // best available policy score
	Surprise          float64       // -log P(obs | model) — primary signal
	UnresolvedObs     []Observation // high-surprise observations
	StuckStates       []string      // persistently high-entropy variables
	MissingPolicyHint string        // suggested action type
}

// IsAnomalous returns true if this signal indicates model inadequacy.
func (s EngineSignal) IsAnomalous() bool {
	return s.Surprise > 2.0 || s.ResidualEntropy > 1.0 || len(s.StuckStates) > 0
}
