# Active Inference for Twin Telemetry

WonderTwin includes a native active inference system for detecting and investigating twin health issues through telemetry analysis. The system maintains probabilistic beliefs about twin states, detects anomalies, and autonomously spawns investigative agents to explore and remediate issues.

## Overview

Active inference is a computational framework where agents maintain beliefs about hidden causes of observations, update those beliefs from sensory data, and select actions that minimize expected uncertainty (exploration) or achieve goals (exploitation).

For WonderTwin, this means:
- **Hidden states**: Twin health status (Healthy, Degrading, Fragile, Failing)
- **Observations**: Telemetry signals (error rates, latency, throughput, failures)
- **Actions**: Investigation policies (InspectLogs, ProfileMetrics, RestartTwin, AlertOncall)

## Architecture

### Perception-Action Loop

```
Telemetry Observation
    ↓
Belief Update (Bayesian inference)
    ↓
Signal Analysis (surprise, entropy)
    ↓
Policy Selection (Expected Free Energy)
    ↓
Agent Spawning (if anomalous)
    ↓
Investigation & Findings
```

### Core Components

#### 1. Distributions (`internal/inference/dist/`)

Probabilistic distributions with zero-allocation conjugate updates:

**Categorical** — Discrete states (Healthy, Degrading, Fragile, Failing)
```go
prior := dist.NewCategorical([]float64{0.60, 0.25, 0.10, 0.05})
likelihood := dist.NewCategoricalFromLogProbs([]float64{2.0, 0, 0, 0})
posterior := prior.Add(likelihood) // 28 ns, zero alloc
```

**Beta** — Binary reliability tracking (detection system trustworthiness)
```go
reliability := dist.NewBetaUniform()
reliability = reliability.UpdateSuccess(5, 1.0)  // 5 true positives
reliability = reliability.UpdateFailure(2, 1.0)  // 2 false positives
mean := reliability.Mean() // expected reliability
```

**Gaussian** — Continuous metrics (future use)
```go
prior := dist.NewGaussian(0, 1.0)
likelihood := dist.LikelihoodFromObservation(0.8, 2.0)
posterior := prior.Add(likelihood) // 2 ns, zero alloc
```

All updates use **natural parameter space** (precision-weighted means, log-probabilities) so conjugate updates are pure addition — the key to nanosecond performance.

#### 2. Engine (`internal/inference/engine/`)

The `TelemetryEngine` maintains beliefs about twin health and recommends actions.

**Hidden States:**
- `TwinHealth`: Categorical(4) — {Healthy, Degrading, Fragile, Failing}
- `IssueReliability`: Beta(α, β) — trustworthiness of anomaly detection

**Observation Types:**

| Type | Effect on Belief |
|------|-----------------|
| `error_rate_spike` | Strongly → Failing |
| `latency_increase` | Moderately → Degrading/Fragile |
| `throughput_drop` | Moderately → Degrading |
| `silent_failure` | → Fragile/Failing, decreases reliability |
| `false_positive` | Decreases reliability |
| `recovery_without_intervention` | → Healthy |
| `recovery_with_intervention` | → Healthy, increases reliability |

**Usage:**
```go
engine := engine.NewTelemetryEngine()

// Process observation
engine.Observe(contract.Observation{
    Type:       "error_rate_spike",
    Confidence: 1.0,
    Payload:    map[string]any{"error_rate": 0.15},
})

// Get current belief
belief := engine.Belief()
summary := belief.Summary()
// → {health_state: "Failing", health_probability: 0.82, ...}

// Get recommended action
policy := engine.Policy()
// → {Action: "AlertOncall", EFE: 4.5, ...}

// Check for model inadequacy
signal := engine.Signal()
if signal.IsAnomalous() {
    // High surprise or stuck states
}
```

#### 3. Policy Selection

Policies are scored by **Expected Free Energy (EFE)**:

```
EFE(π) = epistemic + pragmatic - cost
```

- **Epistemic**: Information gain (exploration) — how much will this reduce uncertainty?
- **Pragmatic**: Goal achievement (exploitation) — how likely to fix the issue?
- **Cost**: Action expense (token budget for LM calls)

**Policies:**

| Action | Type | When Selected |
|--------|------|--------------|
| `InspectLogs` | Epistemic | High uncertainty between Degrading/Fragile |
| `ProfileMetrics` | Epistemic | High entropy overall |
| `RestartTwin` | Pragmatic | Fragile state likely |
| `AlertOncall` | Pragmatic | Failing state likely |
| `Wait` | Pragmatic | Healthy state likely |

The system naturally balances exploration (learning what's wrong) and exploitation (fixing it) through EFE scoring.

#### 4. Agent Orchestrator (`internal/inference/agent/`)

When the engine detects an anomaly (high surprise, high entropy), the orchestrator spawns an investigative agent to execute the recommended policy.

**Executor Interface:**
```go
type Executor interface {
    InspectLogs(ctx context.Context, target, query string) (string, error)
    QueryMetrics(ctx context.Context, target, query string) (string, error)
    AnalyzeTraces(ctx context.Context, target, query string) (string, error)
    GenerateHypothesis(ctx context.Context, observations []string) (string, error)
    ProposeRemediation(ctx context.Context, rootCause string, context map[string]any) (string, error)
}
```

**Budget Management:**

LM calls are expensive. The orchestrator tracks token usage and refuses actions when budget exhausted:

```go
orchestrator := agent.NewOrchestrator(executor, 50000) // 50k token budget

agent, _ := orchestrator.SpawnAgent(policy, signal)
orchestrator.ExecutePolicy(ctx, agent, policy) // deducts from budget

remaining := orchestrator.budget.RemainingBudget()
```

**Findings:**

Agents accumulate structured findings:
```go
type Finding struct {
    Type        string  // "evidence", "hypothesis", "root_cause", "remediation"
    Description string
    Confidence  float64
    Evidence    []string
}
```

#### 5. Inference Loop (`internal/inference/loop.go`)

The loop orchestrates the full perception-action cycle:

```go
loop := inference.NewLoop(inference.Config{
    TokenBudget: 50000,
    Executor:    myExecutor,
    BufferSize:  100,
})
loop.Start()
defer loop.Stop()

// Send observations
loop.Observe(contract.Observation{
    Type:       "error_rate_spike",
    Confidence: 1.0,
})

// Check status
status := loop.Status()
fmt.Printf("Health: %v\n", status.Belief["health_state"])
fmt.Printf("Agents: %d active, %d total\n", status.ActiveAgents, status.TotalAgents)
fmt.Printf("Findings: %d\n", status.Findings)

// Retrieve findings
for _, finding := range loop.Findings() {
    fmt.Printf("[%s] %s (conf: %.2f)\n", 
        finding.Type, finding.Description, finding.Confidence)
}
```

**Asynchronous Agent Execution:**

Agents run in goroutines with timeouts. The loop continues processing new observations while agents investigate in parallel.

## Design Principles

### 1. Zero-Allocation Inference

Belief updates are pure addition in natural parameter space:
- Gaussian: 2 ns per update
- Categorical: 28 ns per update
- Beta: <10 ns per update

This enables 100K+ agents at nanosecond per-agent cost (though WonderTwin uses single-agent topology).

### 2. Budget-Constrained Actions

Inference is cheap (nanoseconds). Actions are expensive (LM calls, API queries). The system concentrates spend on high-uncertainty situations:

- When nothing is wrong (Healthy, low entropy): minimal cost
- When anomaly detected (high surprise): spend budget investigating
- When budget exhausted: stop spawning agents, continue inference

### 3. Signal-Driven Investigation

The engine emits signals indicating model inadequacy:

```go
type EngineSignal struct {
    Surprise        float64       // -log P(obs | model)
    ResidualEntropy float64       // uncertainty that won't resolve
    EFEFloor        float64       // best available policy score
    StuckStates     []string      // high-entropy variables
    UnresolvedObs   []Observation // high-surprise observations
}
```

**Surprise > 2.0**: Model predicted confidently and was wrong → spawn agent  
**Stuck states**: Variable entropy doesn't decrease after observations → missing hidden variable  
**Low EFE floor**: No good actions available → policy space incomplete

### 4. Boundary Rule

**Engines infer. Tools do IO.**

- Engines live in `internal/inference/engine/`
- They receive `contract.Observation`, emit `contract.Policy`
- They never import `net/http`, `os/exec`, or do IO
- The orchestrator (outside the engine) executes policies and feeds results back

This separation keeps inference pure and testable.

## Performance

### Inference

| Operation | Latency | Allocations |
|-----------|---------|-------------|
| Gaussian update | 2 ns | 0 |
| Categorical update | 28 ns | 0 |
| Full belief update + policy selection | <1 µs | 0 (hot path) |

### Agent Actions

| Action | Typical Cost |
|--------|-------------|
| InspectLogs | ~1000 tokens |
| QueryMetrics | ~500 tokens |
| AnalyzeTraces | ~2000 tokens |
| GenerateHypothesis | ~1500 tokens |
| ProposeRemediation | ~2000 tokens |

At 50K token budget:
- ~50 inspect actions, or
- ~25 hypothesis generations, or
- Mixed: 10 investigations + 10 hypotheses + 10 remediations

## Testing

```bash
go test ./internal/inference/...
```

**Test Coverage:**
- Distribution conjugate updates and zero-allocation verification
- Engine belief updates under various observation sequences
- Policy selection for different belief states
- Agent spawning, execution, and budget exhaustion
- Loop observation processing and anomaly detection
- Signal emission for high-surprise events

## Future Extensions

### Multi-Engine Composition

Engines can be chained or composed:

```go
// TelemetryEngine → RootCauseEngine
telemetryBelief := telemetryEngine.Belief()
rootCauseObs := telemetryBelief.Project(rootCauseEngine.Schema())
rootCauseEngine.Observe(rootCauseObs)
```

This enables hierarchical reasoning: telemetry → health state → root cause → remediation.

### Coupling Protocols

Multiple twins can share beliefs via **MeanExchange** protocol (shares normalized beliefs, not raw evidence) to enable population-level inference without pathological accumulation.

### Learning

Observation matrices can be learned from data using Dirichlet priors:

```go
// Instead of fixed likelihoods
likelihoodTable["error_rate_spike"] = [2.0, 0.5, 1.0, 2.0]

// Use learnable factors
priors := []dist.Dirichlet{
    dist.NewDirichlet([]float64{9, 1, 1, 1}), // Strong Healthy prior
    dist.NewDirichlet([]float64{1, 9, 1, 1}), // Strong Degrading prior
    // ...
}
engine.AddLearnableObservationFactor("error_spike", priors)
engine.LearnObservation("error_spike", observedState)
```

### Issue System Integration

Findings can automatically create issues:

```go
for _, finding := range loop.Findings() {
    if finding.Type == "root_cause" && finding.Confidence > 0.7 {
        issue := createGitHubIssue(finding)
        // or Linear, Jira, etc.
    }
}
```

## References

- Internal skill: `~/.config/crush/skills/active-inference/SKILL.md`
- Implementation: `internal/inference/`
- Tests: `internal/inference/*_test.go`

## Example: End-to-End Flow

```go
// 1. Setup
executor := &TwinExecutor{twinID: "twin-stripe"}
loop := inference.NewLoop(inference.Config{
    TokenBudget: 50000,
    Executor:    executor,
})
loop.Start()

// 2. Observe telemetry
loop.Observe(contract.Observation{
    Type:       "error_rate_spike",
    Confidence: 1.0,
    Payload:    map[string]any{"error_rate": 0.15, "twin": "stripe"},
})

// 3. Engine updates beliefs
// P(Failing) increases significantly

// 4. Engine detects anomaly
// Surprise > 2.0 (unexpected error spike)

// 5. Engine selects policy
// EFE scoring → "AlertOncall" (high pragmatic value)

// 6. Orchestrator spawns agent
// Agent executes AlertOncall policy

// 7. Agent collects findings
// Finding{Type: "escalation", Description: "Oncall alerted"}

// 8. Retrieve status
status := loop.Status()
// Health: Failing, 1 agent, 1 finding

// 9. Observe recovery
loop.Observe(contract.Observation{
    Type:       "recovery_with_intervention",
    Confidence: 1.0,
})

// 10. Belief updates
// P(Healthy) increases, reliability increases (intervention worked)
```

---

This system enables WonderTwin to autonomously detect, investigate, and remediate twin health issues through principled Bayesian inference and budget-constrained LM-based exploration.
