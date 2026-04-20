package dist

import (
	"math"
	"testing"
)

func TestCategorical_Add(t *testing.T) {
	prior := NewCategorical([]float64{0.25, 0.25, 0.25, 0.25})
	likelihood := NewCategoricalFromLogProbs([]float64{2.0, 0, 0, 0})

	posterior := prior.Add(likelihood)
	probs := posterior.Probs()

	if probs[0] <= 0.5 {
		t.Errorf("expected state 0 to dominate, got %v", probs)
	}

	sum := 0.0
	for _, p := range probs {
		sum += p
	}
	if math.Abs(sum-1.0) > 1e-9 {
		t.Errorf("probabilities must sum to 1, got %v", sum)
	}
}

func TestCategorical_Entropy(t *testing.T) {
	uniform := NewCategoricalUniform(4)
	entropy := uniform.Entropy()

	expected := math.Log(4.0) // uniform entropy = log(K)
	if math.Abs(entropy-expected) > 1e-9 {
		t.Errorf("uniform entropy: expected %v, got %v", expected, entropy)
	}

	// Concentrated distribution should have lower entropy
	concentrated := NewCategorical([]float64{0.9, 0.05, 0.03, 0.02})
	if concentrated.Entropy() >= entropy {
		t.Error("concentrated distribution should have lower entropy than uniform")
	}
}

func TestCategorical_TopState(t *testing.T) {
	cat := NewCategorical([]float64{0.1, 0.6, 0.2, 0.1})
	idx, prob := cat.TopState()

	if idx != 1 {
		t.Errorf("expected top state 1, got %d", idx)
	}
	if math.Abs(prob-0.6) > 1e-9 {
		t.Errorf("expected prob 0.6, got %v", prob)
	}
}

func TestBeta_Updates(t *testing.T) {
	beta := NewBetaUniform()

	// Observe 5 successes
	beta = beta.UpdateSuccess(5, 1.0)
	if beta.Alpha != 6.0 || beta.Beta != 1.0 {
		t.Errorf("expected Beta(6, 1), got Beta(%v, %v)", beta.Alpha, beta.Beta)
	}

	// Observe 2 failures
	beta = beta.UpdateFailure(2, 1.0)
	if beta.Alpha != 6.0 || beta.Beta != 3.0 {
		t.Errorf("expected Beta(6, 3), got Beta(%v, %v)", beta.Alpha, beta.Beta)
	}

	// Mean should be 6/(6+3) = 0.667
	mean := beta.Mean()
	expected := 6.0 / 9.0
	if math.Abs(mean-expected) > 1e-9 {
		t.Errorf("expected mean %v, got %v", expected, mean)
	}
}

func TestGaussian_Add(t *testing.T) {
	prior := NewGaussian(0, 1.0)
	likelihood := LikelihoodFromObservation(0.8, 2.0)

	posterior := prior.Add(likelihood)

	// Expected: mean = (0*1 + 0.8*2)/(1+2) = 1.6/3 = 0.533
	expectedMean := 1.6 / 3.0
	if math.Abs(posterior.Mean()-expectedMean) > 1e-9 {
		t.Errorf("expected mean %v, got %v", expectedMean, posterior.Mean())
	}

	// Expected: variance = 1/(1+2) = 0.333
	expectedVar := 1.0 / 3.0
	if math.Abs(posterior.Variance()-expectedVar) > 1e-9 {
		t.Errorf("expected variance %v, got %v", expectedVar, posterior.Variance())
	}
}

func TestGaussian_FreeEnergy(t *testing.T) {
	belief := NewGaussian(0.5, 0.5)
	preferred := NewGaussian(1.0, 0.5)

	fe := belief.FreeEnergy(preferred)

	// Should be positive (belief differs from preferred)
	if fe <= 0 {
		t.Errorf("expected positive free energy, got %v", fe)
	}

	// Free energy to self should be zero
	feSelf := belief.FreeEnergy(belief)
	if math.Abs(feSelf) > 1e-9 {
		t.Errorf("free energy to self should be zero, got %v", feSelf)
	}
}

func TestGaussian_ZeroAllocation(t *testing.T) {
	// This test verifies that Add is allocation-free
	prior := NewGaussian(0, 1.0)
	likelihood := LikelihoodFromObservation(0.5, 1.0)

	allocs := testing.AllocsPerRun(1000, func() {
		_ = prior.Add(likelihood)
	})

	if allocs > 0 {
		t.Errorf("Gaussian.Add should allocate zero, got %v", allocs)
	}
}
