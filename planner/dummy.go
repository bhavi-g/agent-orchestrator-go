package planner

import "context"

type DummyPlanner struct{}

func NewDummyPlanner() *DummyPlanner {
    return &DummyPlanner{}
}

func (p *DummyPlanner) CreatePlan(
    ctx context.Context,
    taskID string,
) (*Plan, error) {
    return &Plan{
        TaskID: taskID,
        Steps: []PlanStep{
            {AgentID: "agent.echo"},
        },
    }, nil
}
