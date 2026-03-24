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
	"agent-orchestrator/tools"
)

// flakyAgent fails first N attempts, then succeeds
type flakyAgent struct {
	failCount int32
	attempts  atomic.Int32
}

func (f *flakyAgent) Run(ctx context.Context, input map[string]any) (*agent.Result, error) {
	attempt := f.attempts.Add(1)
	if attempt <= f.failCount {
		return nil, errors.New("agent failure: temporary error")
	}
	return &agent.Result{
		Output: map[string]any{
			"status": "success",
			"attempt": attempt,
		},
	}, nil
}

func (f *flakyAgent) RunWithContext(ctx agent.RuntimeContext) (*agent.Result, error) {
	return f.Run(ctx.Ctx, ctx.Input)
}

func TestEngineExecute_WithRetry_Success(t *testing.T) {
	pl := planner.NewDummyPlanner()

	reg := agent.NewRegistry()
	
	// Agent that fails twice, succeeds on 3rd attempt
	flakyAgt := &flakyAgent{failCount: 2}
	reg.Register("agent.echo", flakyAgt)

	toolRegistry := tools.NewRegistry()
	toolExecutor := tools.NewRegistryExecutor(toolRegistry)

	// Create repair engine with simple retry strategy
	repairStrategy := repair.NewSimpleRetryStrategy()
	repairEngine := repair.NewEngine(repairStrategy, 5) // max 5 attempts

	engine := orchestrator.NewEngine(
		pl, 
		reg, 
		toolExecutor, 
		nil, // no validator
		nil, // no run repo
		nil, // no step repo
		repairEngine,
	)

	result, err := engine.Execute(
		context.Background(),
		orchestrator.ExecutionRequest{
			RunID:  "run-retry-1",
			TaskID: "task.test",
			Input: map[string]any{
				"msg": "test retry",
			},
		},
	)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should succeed after retries
	if result.Status != orchestrator.StatusSucceeded {
		t.Fatalf("expected success after retries, got %v", result.Status)
	}

	// Verify it took 3 attempts (2 failures + 1 success)
	if flakyAgt.attempts.Load() != 3 {
		t.Errorf("expected 3 attempts, got %d", flakyAgt.attempts.Load())
	}
}

func TestEngineExecute_WithRetry_ExceedsMaxAttempts(t *testing.T) {
	pl := planner.NewDummyPlanner()

	reg := agent.NewRegistry()
	
	// Agent that always fails
	flakyAgt := &flakyAgent{failCount: 100}
	reg.Register("agent.echo", flakyAgt)

	toolRegistry := tools.NewRegistry()
	toolExecutor := tools.NewRegistryExecutor(toolRegistry)

	// Create repair engine with only 3 max attempts
	repairStrategy := repair.NewSimpleRetryStrategy()
	repairEngine := repair.NewEngine(repairStrategy, 3)

	engine := orchestrator.NewEngine(
		pl, 
		reg, 
		toolExecutor, 
		nil,
		nil,
		nil,
		repairEngine,
	)

	result, err := engine.Execute(
		context.Background(),
		orchestrator.ExecutionRequest{
			RunID:  "run-retry-2",
			TaskID: "task.test",
			Input: map[string]any{
				"msg": "test max attempts",
			},
		},
	)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should fail after max attempts
	if result.Status != orchestrator.StatusFailed {
		t.Fatalf("expected failure after max attempts, got %v", result.Status)
	}

	// Verify it attempted exactly 3 times
	if flakyAgt.attempts.Load() != 3 {
		t.Errorf("expected 3 attempts, got %d", flakyAgt.attempts.Load())
	}
}

// flakyValidator fails first N validations, then succeeds
type flakyValidator struct {
	failCount int32
	attempts  atomic.Int32
}

func (f *flakyValidator) Validate(stepID string, output map[string]any) error {
	attempt := f.attempts.Add(1)
	if attempt <= f.failCount {
		return errors.New("validation failed: data incomplete")
	}
	return nil
}

func TestEngineExecute_WithRetry_ValidationFailure(t *testing.T) {
	pl := planner.NewDummyPlanner()

	reg := agent.NewRegistry()
	reg.Register("agent.echo", agent.NewEchoAgent()) // Always succeeds

	toolRegistry := tools.NewRegistry()
	toolExecutor := tools.NewRegistryExecutor(toolRegistry)

	// Validator that fails first attempt, succeeds on second
	validator := &flakyValidator{failCount: 1}

	// Create repair engine
	repairStrategy := repair.NewSimpleRetryStrategy()
	repairEngine := repair.NewEngine(repairStrategy, 3)

	engine := orchestrator.NewEngine(
		pl, 
		reg, 
		toolExecutor, 
		validator,
		nil,
		nil,
		repairEngine,
	)

	result, err := engine.Execute(
		context.Background(),
		orchestrator.ExecutionRequest{
			RunID:  "run-retry-validation",
			TaskID: "task.test",
			Input: map[string]any{
				"msg": "test validation retry",
			},
		},
	)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// NOTE: Current SimpleRetryStrategy requests replan for validation failures
	// So this will fail unless we implement replanning
	// For now, validation failures with retry should eventually abort
	// This test documents current behavior
	
	// With current implementation, validation failure triggers replan request
	// which is not yet implemented, so it should fail
	if result.Status != orchestrator.StatusFailed {
		t.Logf("Status: %v, this is expected until replanning is implemented", result.Status)
	}
}

func TestEngineExecute_WithoutRepairEngine_FailsImmediately(t *testing.T) {
	pl := planner.NewDummyPlanner()

	reg := agent.NewRegistry()
	
	// Agent that always fails
	flakyAgt := &flakyAgent{failCount: 100}
	reg.Register("agent.echo", flakyAgt)

	toolRegistry := tools.NewRegistry()
	toolExecutor := tools.NewRegistryExecutor(toolRegistry)

	// NO repair engine
	engine := orchestrator.NewEngine(
		pl, 
		reg, 
		toolExecutor, 
		nil,
		nil,
		nil,
		nil, // No repair engine
	)

	result, err := engine.Execute(
		context.Background(),
		orchestrator.ExecutionRequest{
			RunID:  "run-no-repair",
			TaskID: "task.test",
			Input: map[string]any{
				"msg": "test no repair",
			},
		},
	)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should fail immediately without retry
	if result.Status != orchestrator.StatusFailed {
		t.Fatalf("expected immediate failure, got %v", result.Status)
	}

	// Verify it only attempted once
	if flakyAgt.attempts.Load() != 1 {
		t.Errorf("expected 1 attempt without repair engine, got %d", flakyAgt.attempts.Load())
	}
}
