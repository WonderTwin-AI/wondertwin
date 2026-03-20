package sim

import (
	"math"
	"time"
)

// WeightedOutcome represents one possible outcome with a relative weight.
type WeightedOutcome struct {
	Value  string  `json:"value"`
	Weight float64 `json:"weight"`
}

// PickWeighted selects an outcome based on weights.
// Weights are relative (don't need to sum to 100).
func (r *Rand) PickWeighted(outcomes []WeightedOutcome) string {
	if len(outcomes) == 0 {
		return ""
	}
	if len(outcomes) == 1 {
		return outcomes[0].Value
	}

	var total float64
	for _, o := range outcomes {
		total += o.Weight
	}

	roll := r.Float64() * total
	var cumulative float64
	for _, o := range outcomes {
		cumulative += o.Weight
		if roll < cumulative {
			return o.Value
		}
	}
	return outcomes[len(outcomes)-1].Value
}

// Jitter adds random variance to a duration.
// Returns a duration in the range [d * (1-factor), d * (1+factor)].
// factor=0.2 means ±20%.
func (r *Rand) Jitter(d time.Duration, factor float64) time.Duration {
	if factor <= 0 {
		return d
	}
	// Random value in [-factor, +factor].
	variance := (r.Float64()*2 - 1) * factor
	return time.Duration(float64(d) * (1 + variance))
}

// NormalDuration returns a duration drawn from a normal (Gaussian) distribution.
// mean is the center, stddev controls spread. Result is clamped to [0, 10*mean].
func (r *Rand) NormalDuration(mean, stddev time.Duration) time.Duration {
	// Box-Muller transform for normal distribution.
	u1 := r.Float64()
	u2 := r.Float64()
	if u1 == 0 {
		u1 = 0.0001
	}
	z := math.Sqrt(-2*math.Log(u1)) * math.Cos(2*math.Pi*u2)

	result := float64(mean) + z*float64(stddev)
	if result < 0 {
		result = 0
	}
	maxVal := float64(mean) * 10
	if result > maxVal {
		result = maxVal
	}
	return time.Duration(result)
}

// ExponentialDuration returns a duration drawn from an exponential distribution.
// mean is the average duration. Useful for retry backoff simulation.
func (r *Rand) ExponentialDuration(mean time.Duration) time.Duration {
	u := r.Float64()
	if u == 0 {
		u = 0.0001
	}
	return time.Duration(-float64(mean) * math.Log(u))
}

// BoolWithProbability returns true with the given probability (0.0 to 1.0).
func (r *Rand) BoolWithProbability(p float64) bool {
	return r.Float64() < p
}
