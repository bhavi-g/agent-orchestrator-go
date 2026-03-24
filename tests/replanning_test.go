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

// ===================================================================
// Test Helpers — agents used by replanning tests
// ===================================================================

// alwaysFailAgent always returns an error.
type alwaysFailAgent struct{}

func (a *alwaysFailAgent) Run(ctx context.Context, input map[string]any) (*agent.Result, error) {
	return nil, errors.New("agent failure: primary agent is broken")
}

// successAgent always succeeds with a marker.
type successAgent struct {
	tag string
}

func (a *successAgent) Run(ctx context.Context, input map[string]any) (*agent.Result, error) {
	return &agent.Result{
		Output: map[string]any{
			"status":  "success",
			"handler": a.tag,
		},
	}, nil
}

// countingAgent tracks invocations and fails for the first N calls.
type countingAgent struct {
	failUntil int32
	calls     atomic.Int32
}

func (a *countingAgent) Run(ctx context.Context, input map[string]any) (*agent.Result, error) {
	n := a.calls.Add(1)
	if n <= a.failUntil {
		return nil, errors.New("agent failure: not ready yet")
	}
	return &agent.Result{
		Output: map[string]any{
			"status":  "success",
			"attempt": n,
		},
	}, nil
}

// ===================================================================
// A. Planner unit tests
// ===================================================================

func TestContextAwarePlanner_CreatePlan_Registered(t *testing.T) {
	p := planner.NewContextAwarePlanner()
	p.RegisterTask("task.greet", []planner.PlanStep{
		{AgentID: "agent.hello"},
		{AgentID: "agent.bye"},
	})

	plan, err := p.CreatePlan(context.Background(), "task.greet")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(plan.Steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(plan.Steps))
	}
	if plan.Steps[0].AgentID != "agent.hello" {
		t.Errorf("expected agent.hello, got %s", plan.Steps[0].AgentID)
	}
}

func TestContextAwarePlanner_CreatePlan_Fallback(t *testing.T) {
	p := planner.NewContextAwarePlanner()

	plan, err := p.CreatePlan(context.Background(), "task.unknown")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(plan.Steps) != 1 || plan.Steps[0].AgentID != "agent.echo" {
		t.Fatalf("expected single agent.echo step, got %+v", plan.Steps)
	}
}

func TestContextAwarePlanner_Replan_ValidationFailure(t *testing.T) {
	p := planner.NewContextAwarePlanner()
	original := &planner.Plan{
		TaskID: "task.test",
		Steps: []planner.PlanStep{
			{AgentID: "agent.a"},
			{AgentID: "agent.b"},
			{AgentID: "agent.c"},
		},
	}

	rctx := planner.ReplanContext{
		TaskID:          "task.test",
		OriginalPlan:    original,
		FailedStepIndex: 1, // agent.b failed
		FailedAgentID:   "agent.b",
		FailureError:    "validation error",
		FailureType:     "validation_failure",
		CompletedSteps: []planner.CompletedStep{
			{StepIndex: 0, AgentID: "agent.a", Output: map[string]any{"ok": true}},
		},
		Vars:       map[string]any{"ok": true},
		Attempt:    1,
		MaxReplans: 3,
	}

	newPlan, err := p.Replan(context.Background(), rctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should return steps from failed index onward (b, c)
	if len(newPlan.Steps) != 2 {
		t.Fatalf("expected 2 remaining steps, got %d", len(newPlan.Steps))
	}

	// Should have replan metadata
	if newPlan.Steps[0].Metadata["__replan__"] != true {
		t.Error("expected replan metadata on first new step")
	}
	if newPlan.Steps[0].Metadata["__replan_type__"] != "validation_failure" {
		t.Errorf("expected validation_failure type, got %v", newPlan.Steps[0].Metadata["__replan_type__"])
	}
}

func TestContextAwarePlanner_Replan_AgentReplacedWithFallback(t *testing.T) {
	p := planner.NewContextAwarePlanner()
	p.SetFallbackAgent("agent.backup")

	original := &planner.Plan{
		TaskID: "task.test",
		Steps: []planner.PlanStep{
			{AgentID: "agent.primary"},
		},
	}

	rctx := planner.ReplanContext{
		TaskID:          "task.test",
		OriginalPlan:    original,
		FailedStepIndex: 0,
		FailedAgentID:   "agent.primary",
		FailureError:    "agent crashed",
		FailureType:     "agent_failure",
		Attempt:         1,
		MaxReplans:      3,
	}

	newPlan, err := p.Replan(context.Background(), rctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Failed agent should be replaced with fallback
	if newPlan.Steps[0].AgentID != "agent.backup" {
		t.Errorf("expected agent.backup, got %s", newPlan.Steps[0].AgentID)
	}
}

func TestContextAwarePlanner_Replan_ExceedsMaxReplans(t *testing.T) {
	p := planner.NewContextAwarePlanner()

	rctx := planner.ReplanContext{
		TaskID:     "task.test",
		Attempt:    4, // Exceeds max
		MaxReplans: 3,
	}

	_, err := p.Replan(context.Background(), rctx)
	if err == nil {
		t.Fatal("expected error for exceeding max replans")
	}
}

func TestContextAwarePlanner_Replan_ToolFailureFallback(t *testing.T) {
	p := planner.NewContextAwarePlanner()
	p.SetFallbackAgent("agent.fallback")

	original := &planner.Plan{
		TaskID: "task.test",
		Steps: []planner.PlanStep{
			{AgentID: "agent.fetch"},
			{AgentID: "agent.process"},
		},
	}

	rctx := planner.ReplanContext{
		TaskID:          "task.test",
		OriginalPlan:    original,
		FailedStepIndex: 0,
		FailedAgentID:   "agent.fetch",
		FailureError:    "tool timeout",
		FailureType:     "tool_failure",
		Attempt:         1,
		MaxReplans:      3,
	}

	newPlan, err := p.Replan(context.Background(), rctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Step 0 should use fallback; step 1 should remain unchanged
	if newPlan.Steps[0].AgentID != "agent.fallback" {
		t.Errorf("expected agent.fallback for failed step, got %s", newPlan.Steps[0].AgentID)
	}
	if newPlan.Steps[1].AgentID != "agent.process" {
		t.Errorf("expected agent.process for remaining step, got %s", newPlan.Steps[1].AgentID)
	}
}

// ===================================================================
// B. End-to-end engine replanning tests
// ===================================================================

func TestEngine_ReplanOnAgentFailure(t *testing.T) {
	// Create a context-aware planner that replans with a fallback agent
	cap := planner.NewContextAwarePlanner()
	cap.RegisterTask("task.test", []planner.PlanStep{
		{AgentID: "agent.primary"},
	})
	cap.SetFallbackAgent("agent.backup")

	reg := agent.NewRegistry()
	reg.Register("agent.primary", &alwaysFailAgent{})
	reg.Register("agent.backup", &successAgent{tag: "backup"})

	toolRegistry := tools.NewRegistry()
	toolExecutor := tools.NewRegistryExecutor(toolRegistry)

	// Repair engine with simple retry (will exhaust retries and bubble up)
	repairStrategy := repair.NewSimpleRetryStrategy()
	repairEngine := repair.NewEngine(repairStrategy, 2)

	engine := orchestrator.NewEngine(cap, reg, toolExecutor, nil, nil, nil, repairEngine)

	result, err := engine.Execute(
		context.Background(),
		orchestrator.ExecutionRequest{
			RunID:  "run-replan-1",
			TaskID: "task.test",
			Input:  map[string]any{"msg": "test"},
		},
	)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// After primary fails and retries exhaust, engine should replan
	// with fallback agent and succeed
	if result.Status != orchestrator.StatusSucceeded {
		t.Fatalf("expected success after replan, got %v (err: %v)", result.Status, result.Err)
	}

	if result.Output["handler"] != "backup" {
		t.Errorf("expected backup handler, got %v", result.Output["handler"])
	}
}

func TestEngine_ReplanMaxLimitEnforced(t *testing.T) {
	// Planner that always sends to a broken agent even on replan
	cap := planner.NewContextAwarePlanner()
	cap.RegisterTask("task.test", []planner.PlanStep{
		{AgentID: "agent.broken"},
	})
	// No fallback → replan will keep using agent.broken

	reg := agent.NewRegistry()
	reg.Register("agent.broken", &alwaysFailAgent{})

	toolRegistry := tools.NewRegistry()
	toolExecutor := tools.NewRegistryExecutor(toolRegistry)

	repairStrategy := repair.NewSimpleRetryStrategy()
	repairEngine := repair.NewEngine(repairStrategy, 1) // Only 1 retry attempt

	engine := orchestrator.NewEngine(cap, reg, toolExecutor, nil, nil, nil, repairEngine)
	engine.SetMaxReplans(2) // Allow max 2 replans

	result, err := engine.Execute(
		context.Background(),
		orchestrator.ExecutionRequest{
			RunID:  "run-replan-limit",
			TaskID: "task.test",
			Input:  map[string]any{"msg": "test"},
		},
	)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should eventually fail because replans are exhausted
	if result.Status != orchestrator.StatusFailed {
		t.Fatalf("expected failure after max replans, got %v", result.Status)
	}
}

func TestEngine_ReplanPreservesCompletedStepVars(t *testing.T) {
	// Two-step plan: step 0 succeeds, step 1 fails.
	// Replan should continue from step 1 with fallback,
	// and the vars from step 0 should still be available.
	cap := planner.NewContextAwarePlanner()
	cap.RegisterTask("task.two", []planner.PlanStep{
		{AgentID: "agent.first"},
		{AgentID: "agent.broken"},
	})
	cap.SetFallbackAgent("agent.backup")

	// agent.first produces output
	firstAgent := &successAgent{tag: "first"}
	brokenAgent := &alwaysFailAgent{}
	backupAgent := &successAgent{tag: "backup"}

	reg := agent.NewRegistry()
	reg.Register("agent.first", firstAgent)
	reg.Register("agent.broken", brokenAgent)
	reg.Register("agent.backup", backupAgent)

	toolRegistry := tools.NewRegistry()
	toolExecutor := tools.NewRegistryExecutor(toolRegistry)

	repairStrategy := repair.NewSimpleRetryStrategy()
	repairEngine := repair.NewEngine(repairStrategy, 1) // 1 attempt per step

	engine := orchestrator.NewEngine(cap, reg, toolExecutor, nil, nil, nil, repairEngine)

	result, err := engine.Execute(
		context.Background(),
		orchestrator.ExecutionRequest{
			RunID:  "run-preserve-vars",
			TaskID: "task.two",
			Input:  map[string]any{"msg": "test"},
		},
	)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Status != orchestrator.StatusSucceeded {
		t.Fatalf("expected success, got %v (err: %v)", result.Status, result.Err)
	}

	// Final output should come from the backup agent
	if result.Output["handler"] != "backup" {
		t.Errorf("expected backup handler in final output, got %v", result.Output["handler"])
	}
}

func TestEngine_NoReplannerFallsBackToFailure(t *testing.T) {
	// DummyPlanner doesn't implement Replanner, so engine should
	// fall back to normal failure behavior.
	pl := planner.NewDummyPlanner()

	reg := agent.NewRegistry()
	reg.Register("agent.echo", &alwaysFailAgent{})

	toolRegistry := tools.NewRegistry()
	toolExecutor := tools.NewRegistryExecutor(toolRegistry)

	// No repair engine either
	engine := orchestrator.NewEngine(pl, reg, toolExecutor, nil, nil, nil, nil)

	result, err := engine.Execute(
		context.Background(),
		orchestrator.ExecutionRequest{
			RunID:  "run-no-replanner",
			TaskID: "task.test",
			Input:  map[string]any{"msg": "test"},
		},
	)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Status != orchestrator.StatusFailed {
		t.Fatalf("expected failure without replanner, got %v", result.Status)
	}
}

func TestEngine_SetReplannerExplicitly(t *testing.T) {
	// Use DummyPlanner for initial plan, but set a separate replanner
	pl := planner.NewDummyPlanner()

	cap := planner.NewContextAwarePlanner()
	cap.SetFallbackAgent("agent.backup")

	reg := agent.NewRegistry()
	reg.Register("agent.echo", &alwaysFailAgent{})
	reg.Register("agent.backup", &successAgent{tag: "backup"})

	toolRegistry := tools.NewRegistry()
	toolExecutor := tools.NewRegistryExecutor(toolRegistry)

	repairStrategy := repair.NewSimpleRetryStrategy()
	repairEngine := repair.NewEngine(repairStrategy, 1)

	engine := orchestrator.NewEngine(pl, reg, toolExecutor, nil, nil, nil, repairEngine)
	engine.SetReplanner(cap) // Explicitly set replanner

	result, err := engine.Execute(
		context.Background(),
		orchestrator.ExecutionRequest{
			RunID:  "run-explicit-replanner",
			TaskID: "task.test",
			Input:  map[string]any{"msg": "test"},
		},
	)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Status != orchestrator.StatusSucceeded {
		t.Fatalf("expected success with explicit replanner, got %v (err: %v)", result.Status, result.Err)
	}
}

func TestEngine_ReplanMultiStep_PartialRecovery(t *testing.T) {
	// 3-step plan: step 0 OK, step 1 OK, step 2 fails.
	// After replan the new plan has 1 step (the fallback for step 2).
	cap := planner.NewContextAwarePlanner()
	cap.RegisterTask("task.three", []planner.PlanStep{
		{AgentID: "agent.a"},
		{AgentID: "agent.b"},
		{AgentID: "agent.broken"},
	})
	cap.SetFallbackAgent("agent.backup")

	reg := agent.NewRegistry()
	reg.Register("agent.a", &successAgent{tag: "a"})
	reg.Register("agent.b", &successAgent{tag: "b"})
	reg.Register("agent.broken", &alwaysFailAgent{})
	reg.Register("agent.backup", &successAgent{tag: "backup"})

	toolRegistry := tools.NewRegistry()
	toolExecutor := tools.NewRegistryExecutor(toolRegistry)

	repairStrategy := repair.NewSimpleRetryStrategy()
	repairEngine := repair.NewEngine(repairStrategy, 1)

	engine := orchestrator.NewEngine(cap, reg, toolExecutor, nil, nil, nil, repairEngine)

	result, err := engine.Execute(
		context.Background(),
		orchestrator.ExecutionRequest{
			RunID:  "run-partial-recovery",
			TaskID: "task.three",
			Input:  map[string]any{},
		},
	)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Status != orchestrator.StatusSucceeded {
		t.Fatalf("expected success after partial recovery, got %v", result.Status)
	}

	if result.Output["handler"] != "backup" {
		t.Errorf("expected backup handler, got %v", result.Output)
	}
}
