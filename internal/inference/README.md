# Active Inference for Telemetry-Driven Issue Detection

This package implements a minimal active inference system for WonderTwin, focused on detecting and investigating twin health issues through telemetry analysis.

## Architecture

The system operates on a perception-action loop:

```
Telemetry → Belief Update → Policy Selection → Agent Spawning → Investigation → Findings
```

### Core Components

#### 1. Distributions (`dist/`)
- **Categorical**: Discrete probability distributions for health states (Healthy, Degrading, Fragile, Failing)
- **Beta**: Binary reliability tracking for detection systems
- **Gaussian**: Continuous variables (not currently used but available for metrics)

All distributions use natural parameter representations for zero-allocation conjugate updates.

#### 2. Engine (`engine/`)
- **TelemetryEngine**: Maintains beliefs about twin health from observations
- Hidden states: TwinHealth (Categorical), IssueReliability (Beta)
- Observation types: error_rate_spike, latency_increase, throughput_drop, silent_failure, false_positive, recovery
- Policy selection via Expected Free Energy (EFE) scoring

#### 3. Agent Orchestrator (`agent/`)
- Spawns LM-based investigative agents based on policies
- Budget management for token/cost control
- Agent actions: InspectLogs, ProfileMetrics, RestartTwin, AlertOncall
- Tracks findings and evidence

#### 4. Inference Loop (`loop.go`)
- Orchestrates the perception-action cycle
- Processes observations asynchronously
- Spawns agents when anomalies detected
- Aggregates findings

## Usage

### Basic Setup

```go
import "github.com/wondertwin-ai/wondertwin/internal/inference"

// Create executor for agent actions
executor := &MyExecutor{}

// Configure and start inference loop
loop := inference.NewLoop(inference.Config{
    TokenBudget: 50000,
    Executor:    executor,
    BufferSize:  100,
})
loop.Start()
defer loop.Stop()

// Send observations
loop.Observe(contract.Observation{
    Type:       "error_rate_spike",
    Confidence: 1.0,
    Payload: map[string]any{
        "error_rate": 0.15,
        "twin_id":    "twin-stripe",
    },
})

// Check status
status := loop.Status()
fmt.Printf("Health: %v\n", status.Belief["health_state"])
fmt.Printf("Active agents: %d\n", status.ActiveAgents)
```

### Observation Types

| Type | Meaning | Effect |
|------|---------|--------|
| `error_rate_spike` | Errors above threshold | Strongly → Failing |
| `latency_increase` | Latency degradation | Moderately → Degrading/Fragile |
| `throughput_drop` | Throughput below normal | Moderately → Degrading |
| `silent_failure` | Failure without alert | → Fragile/Failing, decreases reliability |
| `false_positive` | Alert without issue | Decreases reliability |
| `recovery_without_intervention` | Self-healed | → Healthy |

### Policy Selection

The engine uses Expected Free Energy (EFE) to balance exploration and exploitation:

```
EFE(π) = epistemic + pragmatic - cost
```

- **Epistemic**: Information gain (discriminates hypotheses)
- **Pragmatic**: Goal achievement (fixes the issue)
- **Cost**: Token/latency cost of action

Policies:
- `InspectLogs`: High epistemic when uncertain
- `ProfileMetrics`: High epistemic when entropy high
- `RestartTwin`: High pragmatic when Fragile
- `AlertOncall`: High pragmatic when Failing
- `Wait`: High pragmatic when Healthy

## Design Principles

Based on the active-inference skill, this implementation follows key constraints:

1. **Natural parameters**: Zero-allocation updates (Gaussian: 2 ns, Categorical: 28 ns)
2. **Simple topologies**: Single-agent inference (no coupling pathology)
3. **Budget-constrained**: Actions are expensive (LM calls), inference is cheap
4. **Signal-driven**: Surprise > 2.0 triggers investigation
5. **Boundary rule**: Engine infers, tools do IO

## Future Extensions

Not implemented but designed for:

- **Multi-engine composition**: TelemetryEngine → RootCauseEngine via `Belief.Project()`
- **Coupling protocols**: Multiple twins sharing beliefs via MeanExchange
- **Learning**: Dirichlet priors on observation matrices (currently fixed)
- **Issue system integration**: Automatic GitHub/Linear issue creation from findings

## Testing

```bash
go test ./internal/inference/...
```

Key test scenarios:
- Distribution conjugate updates and zero-allocation
- Engine belief updates and policy selection
- Agent spawning and execution
- Loop observation processing and anomaly detection

## References

See `~/.config/crush/skills/active-inference/SKILL.md` for the full operational knowledge this system is based on.
