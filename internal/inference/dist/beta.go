package dist

import "math"

// Beta represents a conjugate prior for binary success/failure variables.
// Used for tracking reliability, integrity, and other binary evidence accumulation.
type Beta struct {
	Alpha float64 // success count (pseudo-observations)
	Beta  float64 // failure count (pseudo-observations)
}

// NewBeta creates a Beta distribution with given parameters.
func NewBeta(alpha, beta float64) Beta {
	if alpha <= 0 || beta <= 0 {
		panic("beta parameters must be positive")
	}
	return Beta{Alpha: alpha, Beta: beta}
}

// NewBetaUniform creates a uniform Beta(1, 1) prior.
func NewBetaUniform() Beta {
	return Beta{Alpha: 1.0, Beta: 1.0}
}

// UpdateSuccess performs conjugate update for observing a success.
func (b Beta) UpdateSuccess(count, confidence float64) Beta {
	return Beta{
		Alpha: b.Alpha + count*confidence,
		Beta:  b.Beta,
	}
}

// UpdateFailure performs conjugate update for observing a failure.
func (b Beta) UpdateFailure(count, confidence float64) Beta {
	return Beta{
		Alpha: b.Alpha,
		Beta:  b.Beta + count*confidence,
	}
}

// Mean returns the posterior mean (expected probability).
func (b Beta) Mean() float64 {
	return b.Alpha / (b.Alpha + b.Beta)
}

// Variance returns the posterior variance.
func (b Beta) Variance() float64 {
	total := b.Alpha + b.Beta
	return (b.Alpha * b.Beta) / (total * total * (total + 1))
}

// Mode returns the mode (most probable value).
// Only defined when Alpha > 1 and Beta > 1.
func (b Beta) Mode() (float64, bool) {
	if b.Alpha <= 1 || b.Beta <= 1 {
		return 0, false
	}
	return (b.Alpha - 1) / (b.Alpha + b.Beta - 2), true
}

// Entropy computes the differential entropy in nats.
func (b Beta) Entropy() float64 {
	// H = log(B(α,β)) - (α-1)ψ(α) - (β-1)ψ(β) + (α+β-2)ψ(α+β)
	// where B is the beta function and ψ is the digamma function
	// Simplified approximation for practical use
	return -math.Log(b.Alpha + b.Beta)
}

// Certainty returns a measure of concentration (inverse entropy).
func (b Beta) Certainty() float64 {
	// Total pseudo-count as proxy for certainty
	return b.Alpha + b.Beta
}
