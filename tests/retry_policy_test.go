package tests

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"agent-orchestrator/agent"
	"agent-orchestrator/orchestrator"
	"agent-orchestrator/planner"
	"agent-orchestrator/repair"
	"agent-orchestrator/retry"
	"agent-orchestrator/tools"
)

// countingFlakyAgent fails first N calls then succeeds, tracking attempts per call
type countingFlakyAgent struct {
	failCount int32
	attempts  atomic.Int32
}

func (a *countingFlakyAgent) Run(ctx context.Context, input map[string]any) (*agent.Result, error) {
	n := a.attempts.Add(1)
	if n <= a.failCount {
		return nil, errors.New("agent failure: temporary error")
	}
	return &agent.Result{Output: map[string]any{"attempt": n}}, nil
}

func (a *countingFlakyAgent) RunWithContext(ctx agent.RuntimeContext) (*agent.Result, error) {
	return a.Run(ctx.Ctx, ctx.Input)
}

// --- Per-step retry policy tests ---

func TestRetryPolicy_PerStepOverride(t *testing.T) {
	// Agent fails 4 times then succeeds. Global policy allows 3 attempts.
	// Per-step policy allows 6 → should succeed.
	agt := &countingFlakyAgent{failCount: 4}
	reg := agent.NewRegistry()
	reg.Register("agent.flaky", agt)

	perStep := retry.Policy{MaxAttempts: 6, Backoff: retry.BackoffNone}
	steps := []planner.PlanStep{
		{AgentID: "agent.flaky", RetryPolicy: &perStep},
	}

	pl := &staticPlanner{steps: steps}
	toolExec := tools.NewRegistryExecutor(tools.NewRegistry())
	repairEngine := repair.NewEngine(repair.NewSimpleRetryStrategy(), 10)

	engine := orchestrator.NewEngine(pl, reg, toolExec, nil, nil, nil, repairEngine)

	result, err := engine.Execute(context.Background(), orchestrator.ExecutionRequest{
		RunID: "run-per-step-1", TaskID: "task.test", Input: map[string]any{"k": "v"},
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != orchestrator.StatusSucceeded {
		t.Fatalf("expected succeeded, got %v", result.Status)
	}
	if agt.attempts.Load() != 5 {
		t.Errorf("expected 5 attempts, got %d", agt.attempts.Load())
	}
}

func TestRetryPolicy_PerStepLimitsRetries(t *testing.T) {
	// Agent always fails. Global default would allow 3, but per-step limits to 1 (no retry).
	agt := &countingFlakyAgent{failCount: 100}
	reg := agent.NewRegistry()
	reg.Register("agent.flaky", agt)

	noRetry := retry.NoRetryPolicy()
	steps := []planner.PlanStep{
		{AgentID: "agent.flaky", RetryPolicy: &noRetry},
	}

	pl := &staticPlanner{steps: steps}
	toolExec := tools.NewRegistryExecutor(tools.NewRegistry())
	repairEngine := repair.NewEngine(repair.NewSimpleRetryStrategy(), 10)

	engine := orchestrator.NewEngine(pl, reg, toolExec, nil, nil, nil, repairEngine)

	result, err := engine.Execute(context.Background(), orchestrator.ExecutionRequest{
		RunID: "run-no-retry-1", TaskID: "task.test", Input: map[string]any{"k": "v"},
	})

	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.Status != orchestrator.StatusFailed {
		t.Fatalf("expected failed status, got %v", result.Status)
	}
	// With NoRetryPolicy (MaxAttempts=1), only 1 attempt should be made
	if agt.attempts.Load() != 1 {
		t.Errorf("expected 1 attempt with NoRetryPolicy, got %d", agt.attempts.Load())
	}
}

func TestRetryPolicy_GlobalDefault(t *testing.T) {
	// Agent fails 2 times then succeeds. Set engine global policy to MaxAttempts=5.
	// No per-step override → global applies.
	agt := &countingFlakyAgent{failCount: 2}
	reg := agent.NewRegistry()
	reg.Register("agent.flaky", agt)

	steps := []planner.PlanStep{
		{AgentID: "agent.flaky"}, // no per-step retry policy
	}

	pl := &staticPlanner{steps: steps}
	toolExec := tools.NewRegistryExecutor(tools.NewRegistry())
	repairEngine := repair.NewEngine(repair.NewSimpleRetryStrategy(), 10)

	engine := orchestrator.NewEngine(pl, reg, toolExec, nil, nil, nil, repairEngine)
	engine.SetRetryPolicy(retry.Policy{
		MaxAttempts: 5,
		Backoff:     retry.BackoffNone,
	})

	result, err := engine.Execute(context.Background(), orchestrator.ExecutionRequest{
		RunID: "run-global-1", TaskID: "task.test", Input: map[string]any{"k": "v"},
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != orchestrator.StatusSucceeded {
		t.Fatalf("expected succeeded, got %v", result.Status)
	}
	if agt.attempts.Load() != 3 {
		t.Errorf("expected 3 attempts, got %d", agt.attempts.Load())
	}
}

func TestRetryPolicy_MergeInheritsFromGlobal(t *testing.T) {
	// Per-step overrides only MaxAttempts; BackoffNone should be inherited from global.
	agt := &countingFlakyAgent{failCount: 1}
	reg := agent.NewRegistry()
	reg.Register("agent.flaky", agt)

	perStep := retry.Policy{MaxAttempts: 5} // only MaxAttempts set
	steps := []planner.PlanStep{
		{AgentID: "agent.flaky", RetryPolicy: &perStep},
	}

	pl := &staticPlanner{steps: steps}
	toolExec := tools.NewRegistryExecutor(tools.NewRegistry())
	repairEngine := repair.NewEngine(repair.NewSimpleRetryStrategy(), 10)

	engine := orchestrator.NewEngine(pl, reg, toolExec, nil, nil, nil, repairEngine)
	engine.SetRetryPolicy(retry.Policy{
		MaxAttempts:  3,
		Backoff:      retry.BackoffNone,
		InitialDelay: 0,
	})

	result, err := engine.Execute(context.Background(), orchestrator.ExecutionRequest{
		RunID: "run-merge-1", TaskID: "task.test", Input: map[string]any{"k": "v"},
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != orchestrator.StatusSucceeded {
		t.Fatalf("expected succeeded, got %v", result.Status)
	}
	// Agent fails once, succeeds on attempt 2
	if agt.attempts.Load() != 2 {
		t.Errorf("expected 2 attempts, got %d", agt.attempts.Load())
	}
}

func TestRetryPolicy_MaxAttemptsExceeded_ReturnsError(t *testing.T) {
	// Agent fails 5 times. Policy allows 3 → should fail.
	agt := &countingFlakyAgent{failCount: 100}
	reg := agent.NewRegistry()
	reg.Register("agent.flaky", agt)

	steps := []planner.PlanStep{{AgentID: "agent.flaky"}}
	pl := &staticPlanner{steps: steps}
	toolExec := tools.NewRegistryExecutor(tools.NewRegistry())
	repairEngine := repair.NewEngine(repair.NewSimpleRetryStrategy(), 10)

	engine := orchestrator.NewEngine(pl, reg, toolExec, nil, nil, nil, repairEngine)
	engine.SetRetryPolicy(retry.Policy{
		MaxAttempts: 3,
		Backoff:     retry.BackoffNone,
	})

	result, err := engine.Execute(context.Background(), orchestrator.ExecutionRequest{
		RunID: "run-exceed-1", TaskID: "task.test", Input: map[string]any{"k": "v"},
	})

	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.Status != orchestrator.StatusFailed {
		t.Fatalf("expected failed status, got %v", result.Status)
	}
	// Should have attempted exactly MaxAttempts times
	if agt.attempts.Load() != 3 {
		t.Errorf("expected 3 attempts, got %d", agt.attempts.Load())
	}
}

// staticPlanner returns a fixed list of steps
type staticPlanner struct {
	steps []planner.PlanStep
}

func (p *staticPlanner) CreatePlan(ctx context.Context, taskID string) (*planner.Plan, error) {
	return &planner.Plan{TaskID: taskID, Steps: p.steps}, nil
}
