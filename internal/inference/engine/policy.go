package engine

import (
	"math"

	"github.com/wondertwin-ai/wondertwin/internal/inference/contract"
)

// policySpec defines a policy with EFE scoring functions.
type policySpec struct {
	action      string
	cost        float64
	epistemicFn func(*TelemetryEngine) float64
	pragmaticFn func(*TelemetryEngine) float64
}

// EFE weights (can be made adaptive in future versions)
const (
	WEpistemic = 1.0
	WPragmatic = 1.0
	WCost      = 1.0
)

var allPolicies = []policySpec{
	{
		action: "InspectLogs",
		cost:   0.3,
		epistemicFn: func(e *TelemetryEngine) float64 {
			// High value when uncertain between Degrading and Fragile
			probs := e.health.Probs()
			return probs[Degrading]*probs[Fragile] * 2.0
		},
		pragmaticFn: func(e *TelemetryEngine) float64 {
			// Low pragmatic value — primarily for exploration
			return 0.1
		},
	},
	{
		action: "ProfileMetrics",
		cost:   0.5,
		epistemicFn: func(e *TelemetryEngine) float64 {
			// High value when entropy is high
			return e.health.Entropy() * 0.5
		},
		pragmaticFn: func(e *TelemetryEngine) float64 {
			return 0.1
		},
	},
	{
		action: "RestartTwin",
		cost:   1.0,
		epistemicFn: func(e *TelemetryEngine) float64 {
			return 0.0 // no information gain
		},
		pragmaticFn: func(e *TelemetryEngine) float64 {
			// High value when Fragile is likely
			probs := e.health.Probs()
			return probs[Fragile] * 2.0
		},
	},
	{
		action: "AlertOncall",
		cost:   0.5, // Lower cost so it wins when Failing is dominant
		epistemicFn: func(e *TelemetryEngine) float64 {
			return 0.0
		},
		pragmaticFn: func(e *TelemetryEngine) float64 {
			// High value when Failing is likely
			probs := e.health.Probs()
			return probs[Failing] * 5.0
		},
	},
	{
		action: "Wait",
		cost:   0.0,
		epistemicFn: func(e *TelemetryEngine) float64 {
			return 0.0
		},
		pragmaticFn: func(e *TelemetryEngine) float64 {
			// High value when Healthy is likely
			probs := e.health.Probs()
			return probs[Healthy] * 1.0
		},
	},
}

// selectPolicy chooses the action with highest Expected Free Energy.
func selectPolicy(e *TelemetryEngine) contract.Policy {
	var best contract.Policy
	bestScore := math.Inf(-1)

	for _, spec := range allPolicies {
		epistemic := spec.epistemicFn(e)
		pragmatic := spec.pragmaticFn(e)
		score := epistemic*WEpistemic + pragmatic*WPragmatic - spec.cost*WCost

		if score > bestScore {
			bestScore = score
			best = contract.Policy{
				Action:    spec.action,
				EFE:       score,
				Epistemic: epistemic,
				Pragmatic: pragmatic,
				Cost:      spec.cost,
			}
		}
	}

	return best
}
