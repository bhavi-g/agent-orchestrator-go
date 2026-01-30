package planner

import "context"

type Plan struct {
	AgentID string
	TaskID  string
}

type Planner interface {
	CreatePlan(ctx context.Context, taskID string) (*Plan, error)
}
