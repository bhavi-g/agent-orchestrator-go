package repair

import (
	"agent-orchestrator/failure"
	"context"
	"errors"
	"testing"
)

func TestNewRepairPlan(t *testing.T) {
	f := failure.NewFailureEvent("run-1", 0, "agent-1", failure.ValidationFailure, errors.New("test"))
	plan := NewRepairPlan(f)

	if plan.Failure != f {
		t.Error("expected failure to be set")
	}
	if !plan.IsEmpty() {
		t.Error("expected plan to be empty initially")
	}
}

func TestRepairPlan_AddAction(t *testing.T) {
	f := failure.NewFailureEvent("run-1", 0, "agent-1", failure.ToolFailure, errors.New("test"))
	plan := NewRepairPlan(f)

	action := RepairAction{
		Type:      RetryStep,
		StepIndex: 0,
	}
	plan.AddAction(action)

	if plan.IsEmpty() {
		t.Error("expected plan to have actions")
	}
	if len(plan.Actions) != 1 {
		t.Errorf("expected 1 action, got %d", len(plan.Actions))
	}
	if plan.Actions[0].Type != RetryStep {
		t.Error("expected RetryStep action")
	}
}

func TestRepairPlan_HasReplan(t *testing.T) {
	f := failure.NewFailureEvent("run-1", 0, "agent-1", failure.ValidationFailure, errors.New("test"))
	plan := NewRepairPlan(f)

	if plan.HasReplan() {
		t.Error("expected no replan initially")
	}

	plan.AddAction(RepairAction{Type: Replan, StepIndex: 0})

	if !plan.HasReplan() {
		t.Error("expected replan after adding Replan action")
	}
}

func TestRepairPlan_HasAbort(t *testing.T) {
	f := failure.NewFailureEvent("run-1", 0, "agent-1", failure.UnknownFailure, errors.New("test"))
	plan := NewRepairPlan(f)

	if plan.HasAbort() {
		t.Error("expected no abort initially")
	}

	plan.AddAction(RepairAction{Type: Abort, StepIndex: 0})

	if !plan.HasAbort() {
		t.Error("expected abort after adding Abort action")
	}
}

func TestEngine_Repair_ExceedsMaxAttempts(t *testing.T) {
	strategy := NewSimpleRetryStrategy()
	engine := NewEngine(strategy, 3)

	f := failure.NewFailureEvent("run-1", 0, "agent-1", failure.ToolFailure, errors.New("test"))
	f.WithAttempt(3) // Already at max

	plan, err := engine.Repair(context.Background(), f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !plan.HasAbort() {
		t.Error("expected abort when max attempts exceeded")
	}
}

func TestEngine_Repair_NotRetryable(t *testing.T) {
	strategy := NewSimpleRetryStrategy()
	engine := NewEngine(strategy, 3)

	f := failure.NewFailureEvent("run-1", 0, "agent-1", failure.UnknownFailure, errors.New("test"))

	plan, err := engine.Repair(context.Background(), f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !plan.HasAbort() {
		t.Error("expected abort for non-retryable failure")
	}
}

func TestSimpleRetryStrategy_ValidationFailure(t *testing.T) {
	strategy := NewSimpleRetryStrategy()

	f := failure.NewFailureEvent("run-1", 2, "agent-1", failure.ValidationFailure, errors.New("validation failed"))

	plan, err := strategy.CreateRepairPlan(context.Background(), f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !plan.HasReplan() {
		t.Error("expected replan for validation failure")
	}
}

func TestSimpleRetryStrategy_ToolFailure(t *testing.T) {
	strategy := NewSimpleRetryStrategy()

	f := failure.NewFailureEvent("run-1", 1, "agent-1", failure.ToolFailure, errors.New("tool failed"))

	plan, err := strategy.CreateRepairPlan(context.Background(), f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if plan.IsEmpty() {
		t.Fatal("expected non-empty plan")
	}

	if plan.Actions[0].Type != RetryStep {
		t.Errorf("expected RetryStep, got %s", plan.Actions[0].Type)
	}
}

func TestSimpleRetryStrategy_AgentFailure(t *testing.T) {
	strategy := NewSimpleRetryStrategy()

	f := failure.NewFailureEvent("run-1", 0, "agent-1", failure.AgentFailure, errors.New("agent crashed"))

	plan, err := strategy.CreateRepairPlan(context.Background(), f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if plan.IsEmpty() {
		t.Fatal("expected non-empty plan")
	}

	if plan.Actions[0].Type != RetryStep {
		t.Errorf("expected RetryStep, got %s", plan.Actions[0].Type)
	}
}

func TestEngine_DefaultMaxAttempts(t *testing.T) {
	strategy := NewSimpleRetryStrategy()
	engine := NewEngine(strategy, 0) // Should default to 3

	// Create a failure at attempt 3
	f := failure.NewFailureEvent("run-1", 0, "agent-1", failure.ToolFailure, errors.New("test"))
	f.WithAttempt(3)

	plan, err := engine.Repair(context.Background(), f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should abort since we're at the default max attempts
	if !plan.HasAbort() {
		t.Error("expected abort when at default max attempts")
	}
}
