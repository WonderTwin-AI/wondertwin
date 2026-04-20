package agent

import (
	"context"
	"testing"

	"github.com/wondertwin-ai/wondertwin/internal/inference/contract"
)

// mockExecutor implements Executor for testing.
type mockExecutor struct {
	logsResult       string
	metricsResult    string
	tracesResult     string
	hypothesisResult string
	remediationResult string
}

func (m *mockExecutor) InspectLogs(ctx context.Context, target, query string) (string, error) {
	return m.logsResult, nil
}

func (m *mockExecutor) QueryMetrics(ctx context.Context, target, query string) (string, error) {
	return m.metricsResult, nil
}

func (m *mockExecutor) AnalyzeTraces(ctx context.Context, target, query string) (string, error) {
	return m.tracesResult, nil
}

func (m *mockExecutor) GenerateHypothesis(ctx context.Context, observations []string) (string, error) {
	return m.hypothesisResult, nil
}

func (m *mockExecutor) ProposeRemediation(ctx context.Context, rootCause string, context map[string]any) (string, error) {
	return m.remediationResult, nil
}

func TestOrchestrator_SpawnAgent(t *testing.T) {
	executor := &mockExecutor{}
	orch := NewOrchestrator(executor, 10000)

	policy := contract.Policy{
		Action: "InspectLogs",
		EFE:    1.5,
	}

	signal := contract.EngineSignal{
		Engine:    "telemetry",
		Iteration: 5,
		Surprise:  2.5,
	}

	agent, err := orch.SpawnAgent(policy, signal)
	if err != nil {
		t.Fatalf("SpawnAgent failed: %v", err)
	}

	if agent.State != StateInitialized {
		t.Errorf("expected state Initialized, got %s", agent.State)
	}

	if agent.Hypothesis != "InspectLogs" {
		t.Errorf("expected hypothesis InspectLogs, got %s", agent.Hypothesis)
	}
}

func TestOrchestrator_ExecutePolicy_InspectLogs(t *testing.T) {
	executor := &mockExecutor{
		logsResult:       "ERROR: connection timeout",
		hypothesisResult: "Network connectivity issue",
	}
	orch := NewOrchestrator(executor, 10000)

	policy := contract.Policy{Action: "InspectLogs"}
	signal := contract.EngineSignal{}
	agent, _ := orch.SpawnAgent(policy, signal)

	ctx := context.Background()
	if err := orch.ExecutePolicy(ctx, agent, policy); err != nil {
		t.Fatalf("ExecutePolicy failed: %v", err)
	}

	if agent.State != StateCompleted {
		t.Errorf("expected state Completed, got %s", agent.State)
	}

	if len(agent.Actions) != 1 {
		t.Fatalf("expected 1 action, got %d", len(agent.Actions))
	}

	if agent.Actions[0].Type != "inspect_logs" {
		t.Errorf("expected inspect_logs action, got %s", agent.Actions[0].Type)
	}

	if len(agent.Findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(agent.Findings))
	}

	if agent.Findings[0].Type != "hypothesis" {
		t.Errorf("expected hypothesis finding, got %s", agent.Findings[0].Type)
	}
}

func TestOrchestrator_ExecutePolicy_RestartTwin(t *testing.T) {
	executor := &mockExecutor{}
	orch := NewOrchestrator(executor, 10000)

	policy := contract.Policy{Action: "RestartTwin"}
	signal := contract.EngineSignal{}
	agent, _ := orch.SpawnAgent(policy, signal)

	ctx := context.Background()
	if err := orch.ExecutePolicy(ctx, agent, policy); err != nil {
		t.Fatalf("ExecutePolicy failed: %v", err)
	}

	if agent.State != StateCompleted {
		t.Errorf("expected state Completed, got %s", agent.State)
	}

	if len(agent.Findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(agent.Findings))
	}

	if agent.Findings[0].Type != "remediation" {
		t.Errorf("expected remediation finding, got %s", agent.Findings[0].Type)
	}
}

func TestBudget_Reserve(t *testing.T) {
	budget := Budget{
		TotalTokens:     5000,
		RemainingTokens: 5000,
		CostPerAction: map[string]int{
			"test_action": 2000,
		},
	}

	// First reservation should succeed
	if !budget.Reserve("test_action") {
		t.Error("first reserve should succeed")
	}

	if budget.RemainingBudget() != 3000 {
		t.Errorf("expected 3000 remaining, got %d", budget.RemainingBudget())
	}

	// Second reservation should succeed
	if !budget.Reserve("test_action") {
		t.Error("second reserve should succeed")
	}

	// Third reservation should fail (insufficient budget)
	if budget.Reserve("test_action") {
		t.Error("third reserve should fail due to insufficient budget")
	}

	if budget.RemainingBudget() != 1000 {
		t.Errorf("expected 1000 remaining, got %d", budget.RemainingBudget())
	}
}

func TestOrchestrator_BudgetExhaustion(t *testing.T) {
	executor := &mockExecutor{
		logsResult: "logs",
	}
	orch := NewOrchestrator(executor, 500) // Very low budget

	policy := contract.Policy{Action: "InspectLogs"}
	signal := contract.EngineSignal{}
	agent, _ := orch.SpawnAgent(policy, signal)

	ctx := context.Background()
	err := orch.ExecutePolicy(ctx, agent, policy)

	// Should fail due to insufficient budget
	if err == nil {
		t.Error("expected error due to insufficient budget")
	}
}
