package dist

import (
	"math"
)

// Categorical represents a discrete probability distribution over K states in log-probability space.
// Updates are element-wise addition (equivalent to multiplying likelihoods and renormalizing).
// Natural parameter representation enables zero-allocation conjugate updates.
type Categorical struct {
	logProbs []float64 // log-probabilities for each state
}

// NewCategorical creates a categorical distribution from normalized probabilities.
func NewCategorical(probs []float64) Categorical {
	logProbs := make([]float64, len(probs))
	for i, p := range probs {
		if p <= 0 {
			logProbs[i] = math.Inf(-1)
		} else {
			logProbs[i] = math.Log(p)
		}
	}
	return Categorical{logProbs: logProbs}
}

// NewCategoricalUniform creates a uniform distribution over K states.
func NewCategoricalUniform(k int) Categorical {
	logProb := -math.Log(float64(k))
	logProbs := make([]float64, k)
	for i := range logProbs {
		logProbs[i] = logProb
	}
	return Categorical{logProbs: logProbs}
}

// NewCategoricalFromLogProbs creates a categorical from log-probabilities directly.
func NewCategoricalFromLogProbs(logProbs []float64) Categorical {
	cp := make([]float64, len(logProbs))
	copy(cp, logProbs)
	return Categorical{logProbs: cp}
}

// Add performs conjugate update by adding log-likelihoods element-wise.
func (c Categorical) Add(likelihood Categorical) Categorical {
	if len(c.logProbs) != len(likelihood.logProbs) {
		panic("categorical dimensions must match")
	}
	result := make([]float64, len(c.logProbs))
	for i := range c.logProbs {
		result[i] = c.logProbs[i] + likelihood.logProbs[i]
	}
	return Categorical{logProbs: result}
}

// Probs returns normalized probabilities.
func (c Categorical) Probs() []float64 {
	// Log-sum-exp trick for numerical stability
	maxLog := c.logProbs[0]
	for _, lp := range c.logProbs[1:] {
		if lp > maxLog {
			maxLog = lp
		}
	}

	probs := make([]float64, len(c.logProbs))
	sum := 0.0
	for i, lp := range c.logProbs {
		probs[i] = math.Exp(lp - maxLog)
		sum += probs[i]
	}

	for i := range probs {
		probs[i] /= sum
	}
	return probs
}

// LogProbs returns the raw log-probabilities.
func (c Categorical) LogProbs() []float64 {
	return c.logProbs
}

// Entropy computes the Shannon entropy in nats.
func (c Categorical) Entropy() float64 {
	probs := c.Probs()
	h := 0.0
	for _, p := range probs {
		if p > 0 {
			h -= p * math.Log(p)
		}
	}
	return h
}

// TopState returns the index and probability of the most probable state.
func (c Categorical) TopState() (int, float64) {
	probs := c.Probs()
	maxIdx := 0
	maxProb := probs[0]
	for i, p := range probs[1:] {
		if p > maxProb {
			maxProb = p
			maxIdx = i + 1
		}
	}
	return maxIdx, maxProb
}

// Mean returns the expected probability vector (same as Probs).
func (c Categorical) Mean() []float64 {
	return c.Probs()
}
