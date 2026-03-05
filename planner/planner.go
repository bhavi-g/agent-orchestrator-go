package planner

import "context"

type PlanStep struct {
    AgentID string
}

type Plan struct {
    TaskID string
    Steps  []PlanStep
}

type Planner interface {
    CreatePlan(ctx context.Context, taskID string) (*Plan, error)
}
