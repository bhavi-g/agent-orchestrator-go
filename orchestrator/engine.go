package orchestrator

import (
	"context"
	"errors"
	"time"

	"agent-orchestrator/agent"
	"agent-orchestrator/planner"
)

type Engine struct {
	planner planner.Planner
	agents  *agent.Registry
}

func NewEngine(
	planner planner.Planner,
	agents *agent.Registry,
) *Engine {
	return &Engine{
		planner: planner,
		agents:  agents,
	}
}

func (e *Engine) Execute(
	ctx context.Context,
	req ExecutionRequest,
) (*ExecutionResult, error) {

	if req.TaskID == "" {
		return nil, errors.New("taskID is required")
	}

	// 1. Create execution context
	execCtx := &ExecutionContext{
		Ctx:     ctx,
		Request: req,
		Vars:    make(map[string]any),
	}

	_ = execCtx // will be used in Phase 3


	start := time.Now()
	_ = start

	// 2. Ask planner for plan
	plan, err := e.planner.CreatePlan(ctx, req.TaskID)
	if err != nil {
		return nil, err
	}

	// 3. Resolve agent
	agt, err := e.agents.Get(plan.AgentID)
	if err != nil {
		return nil, err
	}

	// 4. Execute agent (still stub logic)
	result, err := agt.Run(ctx, req.Input)
	if err != nil {
		return &ExecutionResult{
			RunID: req.RunID,
			Err:   err,
		}, nil
	}

	return &ExecutionResult{
		RunID:  req.RunID,
		Output: result.Output,
		Err:    nil,
	}, nil
}
