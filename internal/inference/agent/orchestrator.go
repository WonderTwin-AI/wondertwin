package agent

import (
	"context"
	"fmt"
	"sync"

	"github.com/wondertwin-ai/wondertwin/internal/inference/contract"
)

// Agent represents an LM-based investigative agent spawned by the inference system.
type Agent struct {
	ID         string
	Hypothesis string             // what this agent is investigating
	State      AgentState         // current execution state
	Findings   []Finding          // accumulated findings
	Actions    []Action           // actions taken
	mu         sync.RWMutex
}

// AgentState represents the agent's execution state.
type AgentState string

const (
	StateInitialized AgentState = "initialized"
	StateExploring   AgentState = "exploring"
	StateExploiting  AgentState = "exploiting"
	StateCompleted   AgentState = "completed"
	StateFailed      AgentState = "failed"
)

// Finding represents a discovered piece of information.
type Finding struct {
	Type        string         // "evidence", "hypothesis", "root_cause", "remediation"
	Description string
	Confidence  float64        // 0-1
	Evidence    []string       // supporting evidence
	Metadata    map[string]any
}

// Action represents an investigative action taken by the agent.
type Action struct {
	Type        string         // "inspect_logs", "query_metrics", "analyze_traces", etc.
	Target      string         // what was investigated
	Result      string         // outcome
	CostTokens  int            // LM token cost
	Metadata    map[string]any
}

// Executor defines how agents interact with the system.
type Executor interface {
	// InspectLogs retrieves and analyzes log data.
	InspectLogs(ctx context.Context, target, query string) (string, error)

	// QueryMetrics retrieves time-series metrics.
	QueryMetrics(ctx context.Context, target, query string) (string, error)

	// AnalyzeTraces retrieves and analyzes distributed traces.
	AnalyzeTraces(ctx context.Context, target, query string) (string, error)

	// GenerateHypothesis uses LM to generate hypothesis from observations.
	GenerateHypothesis(ctx context.Context, observations []string) (string, error)

	// ProposeRemediation uses LM to suggest fixes.
	ProposeRemediation(ctx context.Context, rootCause string, context map[string]any) (string, error)
}

// Orchestrator manages a population of investigative agents.
type Orchestrator struct {
	agents   map[string]*Agent
	executor Executor
	budget   Budget
	mu       sync.RWMutex
}

// Budget tracks token/cost limits for agent actions.
type Budget struct {
	TotalTokens     int
	RemainingTokens int
	CostPerAction   map[string]int
	mu              sync.Mutex
}

// NewOrchestrator creates an agent orchestrator.
func NewOrchestrator(executor Executor, totalTokens int) *Orchestrator {
	return &Orchestrator{
		agents:   make(map[string]*Agent),
		executor: executor,
		budget: Budget{
			TotalTokens:     totalTokens,
			RemainingTokens: totalTokens,
			CostPerAction: map[string]int{
				"inspect_logs":        1000,
				"query_metrics":       500,
				"analyze_traces":      2000,
				"generate_hypothesis": 1500,
				"propose_remediation": 2000,
			},
		},
	}
}

// SpawnAgent creates a new agent to investigate a hypothesis.
func (o *Orchestrator) SpawnAgent(policy contract.Policy, signal contract.EngineSignal) (*Agent, error) {
	o.mu.Lock()
	defer o.mu.Unlock()

	agentID := fmt.Sprintf("agent-%d", len(o.agents)+1)

	agent := &Agent{
		ID:         agentID,
		Hypothesis: policy.Action,
		State:      StateInitialized,
		Findings:   []Finding{},
		Actions:    []Action{},
	}

	o.agents[agentID] = agent
	return agent, nil
}

// ExecutePolicy runs an agent to investigate based on the policy.
func (o *Orchestrator) ExecutePolicy(ctx context.Context, agent *Agent, policy contract.Policy) error {
	agent.mu.Lock()
	agent.State = StateExploring
	agent.mu.Unlock()

	// Map policy action to executor methods
	switch policy.Action {
	case "InspectLogs":
		return o.executeLogs(ctx, agent)
	case "ProfileMetrics":
		return o.executeMetrics(ctx, agent)
	case "RestartTwin":
		return o.executeRestart(ctx, agent)
	case "AlertOncall":
		return o.executeAlert(ctx, agent)
	default:
		return fmt.Errorf("unknown policy action: %s", policy.Action)
	}
}

func (o *Orchestrator) executeLogs(ctx context.Context, agent *Agent) error {
	// Check budget
	if !o.budget.Reserve("inspect_logs") {
		return fmt.Errorf("insufficient budget")
	}

	result, err := o.executor.InspectLogs(ctx, "twin", "error OR warning")
	if err != nil {
		agent.mu.Lock()
		agent.State = StateFailed
		agent.mu.Unlock()
		return err
	}

	agent.mu.Lock()
	defer agent.mu.Unlock()

	agent.Actions = append(agent.Actions, Action{
		Type:       "inspect_logs",
		Target:     "twin",
		Result:     result,
		CostTokens: o.budget.CostPerAction["inspect_logs"],
	})

	// Generate hypothesis from logs
	if !o.budget.Reserve("generate_hypothesis") {
		agent.State = StateCompleted
		return nil
	}

	hypothesis, err := o.executor.GenerateHypothesis(ctx, []string{result})
	if err == nil {
		agent.Findings = append(agent.Findings, Finding{
			Type:        "hypothesis",
			Description: hypothesis,
			Confidence:  0.7,
			Evidence:    []string{result},
		})
	}

	agent.State = StateCompleted
	return nil
}

func (o *Orchestrator) executeMetrics(ctx context.Context, agent *Agent) error {
	if !o.budget.Reserve("query_metrics") {
		return fmt.Errorf("insufficient budget")
	}

	result, err := o.executor.QueryMetrics(ctx, "twin", "latency,throughput,errors")
	if err != nil {
		agent.mu.Lock()
		agent.State = StateFailed
		agent.mu.Unlock()
		return err
	}

	agent.mu.Lock()
	defer agent.mu.Unlock()

	agent.Actions = append(agent.Actions, Action{
		Type:       "query_metrics",
		Target:     "twin",
		Result:     result,
		CostTokens: o.budget.CostPerAction["query_metrics"],
	})

	agent.State = StateCompleted
	return nil
}

func (o *Orchestrator) executeRestart(ctx context.Context, agent *Agent) error {
	agent.mu.Lock()
	defer agent.mu.Unlock()

	agent.Actions = append(agent.Actions, Action{
		Type:   "restart_twin",
		Target: "twin",
		Result: "restart initiated",
	})

	agent.Findings = append(agent.Findings, Finding{
		Type:        "remediation",
		Description: "Twin restart initiated",
		Confidence:  1.0,
	})

	agent.State = StateCompleted
	return nil
}

func (o *Orchestrator) executeAlert(ctx context.Context, agent *Agent) error {
	agent.mu.Lock()
	defer agent.mu.Unlock()

	agent.Actions = append(agent.Actions, Action{
		Type:   "alert_oncall",
		Target: "oncall",
		Result: "alert sent",
	})

	agent.Findings = append(agent.Findings, Finding{
		Type:        "escalation",
		Description: "Oncall alerted for failing twin",
		Confidence:  1.0,
	})

	agent.State = StateCompleted
	return nil
}

// GetAgent retrieves an agent by ID.
func (o *Orchestrator) GetAgent(id string) (*Agent, bool) {
	o.mu.RLock()
	defer o.mu.RUnlock()
	agent, ok := o.agents[id]
	return agent, ok
}

// AllAgents returns all agents.
func (o *Orchestrator) AllAgents() []*Agent {
	o.mu.RLock()
	defer o.mu.RUnlock()

	agents := make([]*Agent, 0, len(o.agents))
	for _, a := range o.agents {
		agents = append(agents, a)
	}
	return agents
}

// Reserve attempts to reserve tokens for an action.
func (b *Budget) Reserve(actionType string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	cost, ok := b.CostPerAction[actionType]
	if !ok {
		return false
	}

	if b.RemainingTokens < cost {
		return false
	}

	b.RemainingTokens -= cost
	return true
}

// RemainingBudget returns the remaining token budget.
func (b *Budget) RemainingBudget() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.RemainingTokens
}
