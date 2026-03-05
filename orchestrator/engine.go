package orchestrator

import (
	"context"
	"errors"
	"time"

	"agent-orchestrator/agent"
	"agent-orchestrator/planner"
	"agent-orchestrator/tools"
)

type Engine struct {
	planner   planner.Planner
	agents    *agent.Registry
	tools     tools.Executor
	validator Validator
}

func NewEngine(
	planner planner.Planner,
	agents *agent.Registry,
	tools tools.Executor,
	validator Validator,
) *Engine {
	return &Engine{
		planner:   planner,
		agents:    agents,
		tools:     tools,
		validator: validator,
	}
}

func (e *Engine) Execute(
	ctx context.Context,
	req ExecutionRequest,
) (*ExecutionResult, error) {

	if req.TaskID == "" {
		return nil, errors.New("taskID is required")
	}

	// --- Execution State Initialization ---
	state := &ExecutionState{
		RunID:  req.RunID,
		Status: StatusPending,
	}

	// 1. Create execution context
	execCtx := &ExecutionContext{
		Ctx:     ctx,
		Request: req,
		Tools:   e.tools,
		Vars:    make(map[string]any),
	}

	// Move to running
	now := time.Now()
	state.StartedAt = now
	state.Status = StatusRunning

	// 2. Ask planner for plan
	plan, err := e.planner.CreatePlan(ctx, req.TaskID)
	if err != nil {
		now := time.Now()
		state.Status = StatusFailed
		state.EndedAt = &now
		state.Error = err.Error()

		return &ExecutionResult{
			RunID:  req.RunID,
			Status: StatusFailed,
			Err:    err,
		}, nil
	}

	var finalOutput map[string]any

	// 3. Execute plan steps sequentially
	for _, step := range plan.Steps {

		agt, err := e.agents.Get(step.AgentID)
		if err != nil {
			now := time.Now()
			state.Status = StatusFailed
			state.EndedAt = &now
			state.Error = err.Error()

			return &ExecutionResult{
				RunID:  req.RunID,
				Status: StatusFailed,
				Err:    err,
			}, nil
		}

		result, err := runAgent(ctx, agt, *execCtx)
		if err != nil {
			now := time.Now()
			state.Status = StatusFailed
			state.EndedAt = &now
			state.Error = err.Error()

			return &ExecutionResult{
				RunID:  req.RunID,
				Status: StatusFailed,
				Err:    err,
			}, nil
		}

		finalOutput = result.Output

		// --- Persist step output into shared execution context ---
		for k, v := range result.Output {
			execCtx.Vars[k] = v
		}
		
		if e.validator != nil {
			if err := e.validator.Validate(step.AgentID, result.Output); err != nil {
				now := time.Now()
				state.Status = StatusFailed
				state.EndedAt = &now
				state.Error = err.Error()

				return &ExecutionResult{
					RunID:  req.RunID,
					Status: StatusFailed,
					Err:    err,
				}, nil
			}
		}
	}

	// Success transition
	now = time.Now()
	state.Status = StatusSucceeded
	state.EndedAt = &now

	return &ExecutionResult{
		RunID:  req.RunID,
		Status: StatusSucceeded,
		Output: finalOutput,
		Err:    nil,
	}, nil
}
