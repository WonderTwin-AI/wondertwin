package dist

import "math"

// Gaussian represents a univariate Gaussian in natural parameter space.
// Natural parameters: precision-weighted mean (η = τμ) and precision (τ = 1/σ²).
// This representation makes conjugate updates additive (zero allocation).
type Gaussian struct {
	PrecisionMean float64 // η = τμ
	Prec          float64 // τ = 1/σ²
}

// NewGaussian creates a Gaussian from mean and variance.
func NewGaussian(mean, variance float64) Gaussian {
	if variance <= 0 {
		panic("gaussian variance must be positive")
	}
	prec := 1.0 / variance
	return Gaussian{
		PrecisionMean: prec * mean,
		Prec:          prec,
	}
}

// NewGaussianPrecision creates a Gaussian from mean and precision directly.
func NewGaussianPrecision(mean, precision float64) Gaussian {
	if precision <= 0 {
		panic("gaussian precision must be positive")
	}
	return Gaussian{
		PrecisionMean: precision * mean,
		Prec:          precision,
	}
}

// Add performs conjugate update by adding natural parameters.
// This is the core operation that enables zero-allocation belief propagation.
func (g Gaussian) Add(likelihood Gaussian) Gaussian {
	return Gaussian{
		PrecisionMean: g.PrecisionMean + likelihood.PrecisionMean,
		Prec:          g.Prec + likelihood.Prec,
	}
}

// Mean returns the posterior mean μ = η/τ.
func (g Gaussian) Mean() float64 {
	if g.Prec == 0 {
		return 0
	}
	return g.PrecisionMean / g.Prec
}

// Variance returns the posterior variance σ² = 1/τ.
func (g Gaussian) Variance() float64 {
	if g.Prec == 0 {
		return math.Inf(1)
	}
	return 1.0 / g.Prec
}

// StdDev returns the posterior standard deviation σ = 1/√τ.
func (g Gaussian) StdDev() float64 {
	return math.Sqrt(g.Variance())
}

// FreeEnergy computes the KL divergence KL(g || preferred).
// Used for measuring surprise and policy scoring.
func (g Gaussian) FreeEnergy(preferred Gaussian) float64 {
	if g.Prec == 0 || preferred.Prec == 0 {
		return math.Inf(1)
	}

	mu := g.Mean()
	muPref := preferred.Mean()
	varPref := preferred.Variance()

	// KL(N(μ,σ²) || N(μ',σ'²)) = log(σ'/σ) + (σ² + (μ-μ')²)/(2σ'²) - 1/2
	variance := g.Variance()
	meanDiff := mu - muPref

	kl := 0.5 * (math.Log(varPref/variance) + (variance+meanDiff*meanDiff)/varPref - 1.0)
	return kl
}

// Entropy returns the differential entropy in nats.
func (g Gaussian) Entropy() float64 {
	// H = 0.5 * log(2πeσ²) = 0.5 * log(2πe/τ)
	if g.Prec <= 0 {
		return math.Inf(1)
	}
	return 0.5 * math.Log(2*math.Pi*math.E/g.Prec)
}

// LikelihoodFromObservation creates a likelihood term from an observation.
func LikelihoodFromObservation(observation, precision float64) Gaussian {
	return Gaussian{
		PrecisionMean: precision * observation,
		Prec:          precision,
	}
}
