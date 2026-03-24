package planner

import "context"

// PlanStep represents a single step in an execution plan.
type PlanStep struct {
	AgentID  string
	Input    map[string]any // Optional: per-step input overrides
	Metadata map[string]any // Optional: metadata for planning context
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
