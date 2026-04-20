package engine

import (
	"fmt"

	"github.com/wondertwin-ai/wondertwin/internal/inference/contract"
	"github.com/wondertwin-ai/wondertwin/internal/inference/dist"
)

// TelemetryEngine infers hidden states about twin health from telemetry observations.
//
// Hidden States:
// - TwinHealth: Categorical(4) — Healthy, Degrading, Fragile, Failing
// - IssueReliability: Beta(α,β) — whether anomaly detection is trustworthy
//
// Observations:
// - error_rate_spike: Failing ↑
// - latency_increase: Degrading/Fragile ↑
// - throughput_drop: Degrading ↑
// - silent_failure: Fragile/Failing ↑, Reliability β ↑
// - false_positive: Reliability β ↑
// - recovery: Healthy ↑
//
// Policies:
// - InspectLogs: epistemic — discriminates Degrading vs Fragile
// - ProfileMetrics: epistemic — confirms hypothesis
// - RestartTwin: pragmatic — effective if Fragile
// - AlertOncall: fallback — when Failing
// - Wait: low-cost fallback
type TelemetryEngine struct {
	health      dist.Categorical // TwinHealth distribution
	reliability dist.Beta        // IssueReliability

	iteration   int
	surprise    float64
	unresolved  []contract.Observation
	stuckStates []string
}

// Health state indices
const (
	Healthy = iota
	Degrading
	Fragile
	Failing
)

var healthNames = []string{"Healthy", "Degrading", "Fragile", "Failing"}

// NewTelemetryEngine creates a telemetry inference engine with default priors.
func NewTelemetryEngine() *TelemetryEngine {
	// Prior: most twins healthy at any given time
	healthPrior := dist.NewCategorical([]float64{0.60, 0.25, 0.10, 0.05})

	return &TelemetryEngine{
		health:      healthPrior,
		reliability: dist.NewBetaUniform(), // no prior knowledge
	}
}

// likelihoodUpdate defines how observations shift beliefs.
type likelihoodUpdate struct {
	healthDeltas     [4]float64 // log-likelihood deltas for each health state
	reliabilityAlpha float64    // success evidence
	reliabilityBeta  float64    // failure evidence
}

// likelihoodTable maps observation types to belief updates.
var likelihoodTable = map[string]likelihoodUpdate{
	"error_rate_spike": {
		healthDeltas: [4]float64{-2.0, 0.5, 1.0, 2.0}, // strongly Failing
	},
	"latency_increase": {
		healthDeltas: [4]float64{-1.0, 1.0, 1.0, 0.5}, // moderate Degrading/Fragile
	},
	"throughput_drop": {
		healthDeltas: [4]float64{-1.0, 1.0, 0.5, 0.3}, // moderate Degrading
	},
	"silent_failure": {
		healthDeltas:     [4]float64{-2.0, 0.0, 1.5, 1.5}, // Fragile/Failing
		reliabilityBeta:  1.0,                             // detection missed
	},
	"false_positive": {
		healthDeltas:    [4]float64{0.5, -0.5, -0.5, -1.0}, // not actually degraded
		reliabilityBeta: 1.0,                                // wrong alert
	},
	"recovery_without_intervention": {
		healthDeltas: [4]float64{2.0, -1.0, -1.5, -2.0}, // self-healed
	},
	"recovery_with_intervention": {
		healthDeltas:     [4]float64{1.5, -0.5, -1.0, -1.5},
		reliabilityAlpha: 1.0, // detection was useful
	},
}

// Observe updates beliefs given a telemetry observation.
func (e *TelemetryEngine) Observe(obs contract.Observation) error {
	update, ok := likelihoodTable[obs.Type]
	if !ok {
		return fmt.Errorf("unknown observation type: %s", obs.Type)
	}

	confidence := obs.Confidence
	if confidence <= 0 {
		confidence = 1.0
	}

	// Compute surprise before update
	e.surprise = e.computeSurprise(obs, update)

	// Apply health belief update
	healthLikelihood := dist.NewCategoricalFromLogProbs([]float64{
		update.healthDeltas[Healthy] * confidence,
		update.healthDeltas[Degrading] * confidence,
		update.healthDeltas[Fragile] * confidence,
		update.healthDeltas[Failing] * confidence,
	})
	e.health = e.health.Add(healthLikelihood)

	// Apply reliability updates
	if update.reliabilityAlpha > 0 {
		e.reliability = e.reliability.UpdateSuccess(update.reliabilityAlpha, confidence)
	}
	if update.reliabilityBeta > 0 {
		e.reliability = e.reliability.UpdateFailure(update.reliabilityBeta, confidence)
	}

	// Track high-surprise observations
	if e.surprise > 2.0 {
		e.unresolved = append(e.unresolved, obs)
	}

	e.iteration++
	e.updateStuckStates()

	return nil
}

// Policy returns the recommended action based on EFE scoring.
func (e *TelemetryEngine) Policy() contract.Policy {
	return selectPolicy(e)
}

// Belief returns current belief accessor.
func (e *TelemetryEngine) Belief() contract.Belief {
	return &beliefAccessor{engine: e}
}

// Signal returns diagnostic information.
func (e *TelemetryEngine) Signal() contract.EngineSignal {
	return contract.EngineSignal{
		Engine:          "telemetry",
		Iteration:       e.iteration,
		ResidualEntropy: e.health.Entropy(),
		EFEFloor:        e.Policy().EFE,
		Surprise:        e.surprise,
		UnresolvedObs:   e.unresolved,
		StuckStates:     e.stuckStates,
	}
}

// Reset clears state and returns to priors.
func (e *TelemetryEngine) Reset() {
	*e = *NewTelemetryEngine()
}

// computeSurprise calculates -log P(obs | current belief).
func (e *TelemetryEngine) computeSurprise(obs contract.Observation, update likelihoodUpdate) float64 {
	// Compute expected log-likelihood under current belief
	probs := e.health.Probs()
	expectedLL := 0.0
	for i, p := range probs {
		expectedLL += p * update.healthDeltas[i]
	}

	// Surprise is negative expected log-likelihood
	return -expectedLL
}

// updateStuckStates identifies variables with persistent high entropy.
func (e *TelemetryEngine) updateStuckStates() {
	e.stuckStates = nil
	if e.iteration > 5 && e.health.Entropy() > 1.0 {
		e.stuckStates = append(e.stuckStates, "TwinHealth")
	}
}

// beliefAccessor implements contract.Belief.
type beliefAccessor struct {
	engine *TelemetryEngine
}

func (b *beliefAccessor) Entropy() float64 {
	return b.engine.health.Entropy()
}

func (b *beliefAccessor) Certainty() float64 {
	_, prob := b.engine.health.TopState()
	return prob
}

func (b *beliefAccessor) Summary() map[string]any {
	topIdx, topProb := b.engine.health.TopState()
	probs := b.engine.health.Probs()

	return map[string]any{
		"health_state":        healthNames[topIdx],
		"health_probability":  topProb,
		"health_entropy":      b.engine.health.Entropy(),
		"health_distribution": map[string]float64{
			"Healthy":   probs[Healthy],
			"Degrading": probs[Degrading],
			"Fragile":   probs[Fragile],
			"Failing":   probs[Failing],
		},
		"reliability_mean": b.engine.reliability.Mean(),
		"iteration":        b.engine.iteration,
	}
}

// Verify interface compliance at compile time.
var _ contract.Engine = (*TelemetryEngine)(nil)
