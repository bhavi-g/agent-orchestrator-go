package planner

import (
	"context"

	"agent-orchestrator/retry"
)

// PlanStep represents a single step in an execution plan.
type PlanStep struct {
	AgentID     string
	Input       map[string]any // Optional: per-step input overrides
	Metadata    map[string]any // Optional: metadata for planning context
	RetryPolicy *retry.Policy  // Optional: per-step retry override
	StepID      string         // Optional: unique ID for DAG dependency references
	DependsOn   []string       // Optional: StepIDs this step depends on
}

// Plan represents a sequence of steps to execute.
type Plan struct {
	TaskID string
	Steps  []PlanStep
}

// Planner creates an initial plan for a given task.
type Planner interface {
	CreatePlan(ctx context.Context, taskID string) (*Plan, error)
}
