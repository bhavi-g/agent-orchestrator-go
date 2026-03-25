package planner

import "context"

// LogAnalysisPlanner creates a two-step plan for the Golden Path:
//
//  1. agent.log_reader  — scan directory, find error/warning lines
//  2. agent.log_analyzer — analyse findings and produce structured report
//
// The steps are sequential (step 2 depends on step 1's output via Vars).
type LogAnalysisPlanner struct{}

func NewLogAnalysisPlanner() *LogAnalysisPlanner {
	return &LogAnalysisPlanner{}
}

func (p *LogAnalysisPlanner) CreatePlan(_ context.Context, taskID string) (*Plan, error) {
	return &Plan{
		TaskID: taskID,
		Steps: []PlanStep{
			{
				AgentID: "agent.log_reader",
				StepID:  "scan",
			},
			{
				AgentID:   "agent.log_analyzer",
				StepID:    "analyze",
				DependsOn: []string{"scan"},
			},
		},
	}, nil
}
