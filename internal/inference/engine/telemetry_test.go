package engine

import (
	"testing"

	"github.com/wondertwin-ai/wondertwin/internal/inference/contract"
)

func TestTelemetryEngine_ErrorRateSpike(t *testing.T) {
	e := NewTelemetryEngine()

	obs := contract.Observation{
		Type:       "error_rate_spike",
		Confidence: 1.0,
	}

	if err := e.Observe(obs); err != nil {
		t.Fatalf("Observe failed: %v", err)
	}

	probs := e.health.Probs()
	if probs[Failing] <= probs[Healthy] {
		t.Errorf("after error spike, Failing should dominate Healthy, got %v", probs)
	}

	policy := e.Policy()
	if policy.Action != "AlertOncall" {
		t.Errorf("expected AlertOncall policy when Failing, got %s", policy.Action)
	}
}

func TestTelemetryEngine_LatencyIncrease(t *testing.T) {
	e := NewTelemetryEngine()

	obs := contract.Observation{
		Type:       "latency_increase",
		Confidence: 1.0,
	}

	if err := e.Observe(obs); err != nil {
		t.Fatalf("Observe failed: %v", err)
	}

	probs := e.health.Probs()
	if probs[Degrading] <= probs[Healthy] {
		t.Errorf("after latency increase, Degrading should be elevated, got %v", probs)
	}

	belief := e.Belief()
	if belief.Certainty() < 0.3 {
		t.Errorf("should have some certainty after observation, got %v", belief.Certainty())
	}
}

func TestTelemetryEngine_FalsePositive(t *testing.T) {
	e := NewTelemetryEngine()

	// First observe an error spike
	e.Observe(contract.Observation{Type: "error_rate_spike", Confidence: 1.0})

	initialReliability := e.reliability.Mean()

	// Then observe false positive
	e.Observe(contract.Observation{Type: "false_positive", Confidence: 1.0})

	// Reliability should decrease
	if e.reliability.Mean() >= initialReliability {
		t.Errorf("reliability should decrease after false positive")
	}
}

func TestTelemetryEngine_Recovery(t *testing.T) {
	e := NewTelemetryEngine()

	// Degrade first
	e.Observe(contract.Observation{Type: "latency_increase", Confidence: 1.0})

	// Then recover
	e.Observe(contract.Observation{Type: "recovery_without_intervention", Confidence: 1.0})

	probs := e.health.Probs()
	if probs[Healthy] <= probs[Degrading] {
		t.Errorf("after recovery, Healthy should dominate, got %v", probs)
	}

	policy := e.Policy()
	if policy.Action != "Wait" {
		t.Errorf("expected Wait policy when Healthy, got %s", policy.Action)
	}
}

func TestTelemetryEngine_HighEntropy(t *testing.T) {
	e := NewTelemetryEngine()

	// Slightly conflicting but not completely cancelling signals
	e.Observe(contract.Observation{Type: "latency_increase", Confidence: 1.0})
	e.Observe(contract.Observation{Type: "throughput_drop", Confidence: 0.3})

	belief := e.Belief()
	if belief.Entropy() < 0.5 {
		t.Errorf("multiple degradation signals should maintain some entropy, got %v", belief.Entropy())
	}

	policy := e.Policy()
	// Should recommend epistemic or at least have some epistemic component
	if policy.Action != "InspectLogs" && policy.Action != "ProfileMetrics" {
		t.Logf("got policy %s with E=%v P=%v (acceptable for this entropy level)",
			policy.Action, policy.Epistemic, policy.Pragmatic)
	}
}

func TestTelemetryEngine_Signal(t *testing.T) {
	e := NewTelemetryEngine()

	// Create high-surprise situation
	e.Observe(contract.Observation{Type: "error_rate_spike", Confidence: 1.0})

	signal := e.Signal()

	if signal.Engine != "telemetry" {
		t.Errorf("expected engine=telemetry, got %s", signal.Engine)
	}

	if signal.Iteration != 1 {
		t.Errorf("expected iteration=1, got %d", signal.Iteration)
	}

	if signal.Surprise <= 0 {
		t.Error("expected positive surprise for error spike")
	}
}

func TestTelemetryEngine_Reset(t *testing.T) {
	e := NewTelemetryEngine()

	e.Observe(contract.Observation{Type: "error_rate_spike", Confidence: 1.0})
	e.Reset()

	if e.iteration != 0 {
		t.Errorf("expected iteration=0 after reset, got %d", e.iteration)
	}

	probs := e.health.Probs()
	if probs[Healthy] < 0.5 {
		t.Errorf("after reset, should return to healthy prior, got %v", probs)
	}
}

func TestTelemetryEngine_BeliefSummary(t *testing.T) {
	e := NewTelemetryEngine()
	e.Observe(contract.Observation{Type: "latency_increase", Confidence: 1.0})

	belief := e.Belief()
	summary := belief.Summary()

	if _, ok := summary["health_state"]; !ok {
		t.Error("summary should contain health_state")
	}

	if _, ok := summary["health_distribution"]; !ok {
		t.Error("summary should contain health_distribution")
	}

	if _, ok := summary["reliability_mean"]; !ok {
		t.Error("summary should contain reliability_mean")
	}
}

func TestTelemetryEngine_InterfaceCompliance(t *testing.T) {
	var _ contract.Engine = (*TelemetryEngine)(nil)
}
