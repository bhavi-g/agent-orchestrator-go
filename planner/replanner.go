package planner

import (
	"context"
)

// ReplanContext provides the replanner with execution context to make
// informed decisions about how to create a new plan.
type ReplanContext struct {
	// TaskID is the original task being executed
	TaskID string
	// OriginalPlan is the plan that was being executed
	OriginalPlan *Plan
	// FailedStepIndex is the index of the step that failed (0-based)
	FailedStepIndex int
	// FailedAgentID is the agent that failed
	FailedAgentID string
	// FailureError describes the error that occurred
	FailureError string
	// FailureType classifies the failure (e.g. "validation_failure")
	FailureType string
	// CompletedSteps are the steps that completed successfully before failure
	CompletedSteps []CompletedStep
	// Vars contains the execution context variables accumulated so far
	Vars map[string]any
	// Attempt is the replan attempt number (1-based)
	Attempt int
	// MaxReplans limits how many times replanning can occur
	MaxReplans int
}

// CompletedStep captures what happened during an already-executed step.
type CompletedStep struct {
	StepIndex int
	AgentID   string
	Output    map[string]any
}

// Replanner creates a new plan in response to a failure during execution.
// Unlike Planner (which produces an initial plan), Replanner has access to
// the full failure and execution context so it can make smarter decisions.
type Replanner interface {
	// Replan creates a new plan of remaining steps given failure context.
	// It should return only the steps that still need to be executed,
	// NOT the steps that already completed successfully.
	Replan(ctx context.Context, rctx ReplanContext) (*Plan, error)
}
