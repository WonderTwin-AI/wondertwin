package inference

import (
	"context"
	"testing"
	"time"

	"github.com/wondertwin-ai/wondertwin/internal/inference/contract"
)

type mockExecutor struct{}

func (m *mockExecutor) InspectLogs(ctx context.Context, target, query string) (string, error) {
	return "mock logs", nil
}

func (m *mockExecutor) QueryMetrics(ctx context.Context, target, query string) (string, error) {
	return "mock metrics", nil
}

func (m *mockExecutor) AnalyzeTraces(ctx context.Context, target, query string) (string, error) {
	return "mock traces", nil
}

func (m *mockExecutor) GenerateHypothesis(ctx context.Context, observations []string) (string, error) {
	return "mock hypothesis", nil
}

func (m *mockExecutor) ProposeRemediation(ctx context.Context, rootCause string, context map[string]any) (string, error) {
	return "mock remediation", nil
}

func TestLoop_ObserveAndInfer(t *testing.T) {
	cfg := Config{
		TokenBudget: 10000,
		Executor:    &mockExecutor{},
		BufferSize:  10,
	}

	loop := NewLoop(cfg)
	loop.Start()
	defer loop.Stop()

	// Send a high-surprise observation
	obs := contract.Observation{
		Type:       "error_rate_spike",
		Confidence: 1.0,
		Timestamp:  time.Now().Unix(),
	}

	if err := loop.Observe(obs); err != nil {
		t.Fatalf("Observe failed: %v", err)
	}

	// Give loop time to process
	time.Sleep(100 * time.Millisecond)

	// Check belief updated
	belief := loop.Belief()
	if belief.Entropy() >= 1.0 {
		t.Logf("entropy still high after error spike: %v (may be expected)", belief.Entropy())
	}

	// Check signal indicates anomaly
	signal := loop.Signal()
	if !signal.IsAnomalous() {
		t.Error("expected anomalous signal after error spike")
	}

	// Check agent spawned
	time.Sleep(200 * time.Millisecond) // Give agent time to execute
	agents := loop.Agents()
	if len(agents) == 0 {
		t.Error("expected at least one agent spawned")
	}
}

func TestLoop_MultipleObservations(t *testing.T) {
	cfg := Config{
		TokenBudget: 10000,
		Executor:    &mockExecutor{},
		BufferSize:  10,
	}

	loop := NewLoop(cfg)
	loop.Start()
	defer loop.Stop()

	observations := []contract.Observation{
		{Type: "latency_increase", Confidence: 1.0},
		{Type: "throughput_drop", Confidence: 0.8},
		{Type: "recovery_without_intervention", Confidence: 1.0},
	}

	for _, obs := range observations {
		if err := loop.Observe(obs); err != nil {
			t.Fatalf("Observe failed: %v", err)
		}
		time.Sleep(50 * time.Millisecond)
	}

	time.Sleep(200 * time.Millisecond)

	belief := loop.Belief()
	summary := belief.Summary()

	if summary["iteration"].(int) != len(observations) {
		t.Errorf("expected %d iterations, got %v", len(observations), summary["iteration"])
	}
}

func TestLoop_Status(t *testing.T) {
	cfg := Config{
		TokenBudget: 10000,
		Executor:    &mockExecutor{},
		BufferSize:  10,
	}

	loop := NewLoop(cfg)
	loop.Start()
	defer loop.Stop()

	// Trigger anomaly
	loop.Observe(contract.Observation{
		Type:       "error_rate_spike",
		Confidence: 1.0,
	})

	time.Sleep(200 * time.Millisecond)

	status := loop.Status()

	if status.TotalAgents == 0 {
		t.Error("expected agents to be spawned")
	}

	if status.Belief == nil {
		t.Error("status should include belief summary")
	}

	if !status.Signal.IsAnomalous() {
		t.Error("signal should be anomalous")
	}
}

func TestLoop_Reset(t *testing.T) {
	cfg := Config{
		TokenBudget: 10000,
		Executor:    &mockExecutor{},
		BufferSize:  10,
	}

	loop := NewLoop(cfg)
	loop.Start()
	defer loop.Stop()

	loop.Observe(contract.Observation{Type: "error_rate_spike", Confidence: 1.0})
	time.Sleep(100 * time.Millisecond)

	loop.Reset()

	belief := loop.Belief()
	summary := belief.Summary()

	if summary["iteration"].(int) != 0 {
		t.Errorf("expected iteration=0 after reset, got %v", summary["iteration"])
	}

	findings := loop.Findings()
	if len(findings) != 0 {
		t.Errorf("expected no findings after reset, got %d", len(findings))
	}
}

func TestLoop_BufferFull(t *testing.T) {
	cfg := Config{
		TokenBudget: 10000,
		Executor:    &mockExecutor{},
		BufferSize:  2, // Very small buffer
	}

	loop := NewLoop(cfg)
	// Don't start the loop so observations accumulate

	// Fill buffer
	loop.Observe(contract.Observation{Type: "latency_increase", Confidence: 1.0})
	loop.Observe(contract.Observation{Type: "throughput_drop", Confidence: 1.0})

	// Next should fail
	err := loop.Observe(contract.Observation{Type: "error_rate_spike", Confidence: 1.0})
	if err == nil {
		t.Error("expected error when buffer full")
	}
}
