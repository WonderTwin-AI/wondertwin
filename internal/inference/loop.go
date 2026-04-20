package inference

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/wondertwin-ai/wondertwin/internal/inference/agent"
	"github.com/wondertwin-ai/wondertwin/internal/inference/contract"
	"github.com/wondertwin-ai/wondertwin/internal/inference/engine"
)

// Loop orchestrates the perception-action cycle for telemetry inference.
// It maintains beliefs, spawns agents to investigate anomalies, and tracks findings.
type Loop struct {
	engine       *engine.TelemetryEngine
	orchestrator *agent.Orchestrator
	observations chan contract.Observation
	findings     []agent.Finding
	mu           sync.RWMutex
	ctx          context.Context
	cancel       context.CancelFunc
}

// Config configures the inference loop.
type Config struct {
	TokenBudget int                // total token budget for agent actions
	Executor    agent.Executor     // executor for agent actions
	BufferSize  int                // observation buffer size
}

// NewLoop creates a new inference loop.
func NewLoop(cfg Config) *Loop {
	if cfg.BufferSize == 0 {
		cfg.BufferSize = 100
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &Loop{
		engine:       engine.NewTelemetryEngine(),
		orchestrator: agent.NewOrchestrator(cfg.Executor, cfg.TokenBudget),
		observations: make(chan contract.Observation, cfg.BufferSize),
		findings:     []agent.Finding{},
		ctx:          ctx,
		cancel:       cancel,
	}
}

// Start begins the inference loop.
func (l *Loop) Start() {
	go l.run()
}

// Stop halts the inference loop.
func (l *Loop) Stop() {
	l.cancel()
}

// Observe sends an observation to the inference loop.
func (l *Loop) Observe(obs contract.Observation) error {
	select {
	case l.observations <- obs:
		return nil
	case <-l.ctx.Done():
		return fmt.Errorf("inference loop stopped")
	default:
		return fmt.Errorf("observation buffer full")
	}
}

// run is the main perception-action loop.
func (l *Loop) run() {
	for {
		select {
		case <-l.ctx.Done():
			return

		case obs := <-l.observations:
			l.processObservation(obs)
		}
	}
}

// processObservation handles a single observation through the inference cycle.
func (l *Loop) processObservation(obs contract.Observation) {
	// 1. Update beliefs
	if err := l.engine.Observe(obs); err != nil {
		// Log error but continue
		return
	}

	// 2. Get diagnostic signal
	signal := l.engine.Signal()

	// 3. If signal indicates anomaly, evaluate policy and spawn agent
	if signal.IsAnomalous() {
		policy := l.engine.Policy()

		// Spawn agent to investigate
		ag, err := l.orchestrator.SpawnAgent(policy, signal)
		if err != nil {
			return
		}

		// Execute policy asynchronously
		go func() {
			ctx, cancel := context.WithTimeout(l.ctx, 30*time.Second)
			defer cancel()

			if err := l.orchestrator.ExecutePolicy(ctx, ag, policy); err != nil {
				// Log error
				return
			}

			// Collect findings
			l.mu.Lock()
			l.findings = append(l.findings, ag.Findings...)
			l.mu.Unlock()
		}()
	}
}

// Belief returns current belief state.
func (l *Loop) Belief() contract.Belief {
	return l.engine.Belief()
}

// Signal returns current diagnostic signal.
func (l *Loop) Signal() contract.EngineSignal {
	return l.engine.Signal()
}

// Findings returns all accumulated findings.
func (l *Loop) Findings() []agent.Finding {
	l.mu.RLock()
	defer l.mu.RUnlock()

	findings := make([]agent.Finding, len(l.findings))
	copy(findings, l.findings)
	return findings
}

// Agents returns all active/completed agents.
func (l *Loop) Agents() []*agent.Agent {
	return l.orchestrator.AllAgents()
}

// Reset clears the engine state and findings.
func (l *Loop) Reset() {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.engine.Reset()
	l.findings = []agent.Finding{}
}

// Status returns a summary of the current inference state.
func (l *Loop) Status() Status {
	belief := l.engine.Belief()
	signal := l.engine.Signal()
	agents := l.orchestrator.AllAgents()

	activeCount := 0
	for _, a := range agents {
		if a.State == agent.StateExploring || a.State == agent.StateExploiting {
			activeCount++
		}
	}

	return Status{
		Belief:       belief.Summary(),
		Signal:       signal,
		ActiveAgents: activeCount,
		TotalAgents:  len(agents),
		Findings:     len(l.findings),
	}
}

// Status represents the current state of the inference system.
type Status struct {
	Belief       map[string]any
	Signal       contract.EngineSignal
	ActiveAgents int
	TotalAgents  int
	Findings     int
}
