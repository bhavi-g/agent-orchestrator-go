package orchestrator

import (
	"context"
	"errors"
	"fmt"
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
	runs      RunRepository
	steps     StepRepository
}

func NewEngine(
	planner planner.Planner,
	agents *agent.Registry,
	tools tools.Executor,
	validator Validator,
	runs RunRepository,
	steps StepRepository,
) *Engine {
	return &Engine{
		planner:   planner,
		agents:    agents,
		tools:     tools,
		validator: validator,
		runs:      runs,
		steps:     steps,
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

	// --- Persistent AgentRun record ---
	run := &agent.AgentRun{
		RunID:            req.RunID,
		Goal:             req.TaskID,
		Status:           agent.AgentRunCreated,
		CurrentStepIndex: 0,
		PromptVersion:    "v1",
		ModelVersion:     "deterministic",
		MaxSteps:         0, // set after plan is created
		CreatedAt:        time.Now(),
		CompletedAt:      nil,
	}

	if e.runs != nil {
		if err := e.runs.Create(run); err != nil {
			return nil, err
		}
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

		run.Status = agent.AgentRunFailed
		run.CompletedAt = &now
		if e.runs != nil {
			_ = e.runs.Update(run)
		}

		return &ExecutionResult{
			RunID:  req.RunID,
			Status: StatusFailed,
			Err:    err,
		}, nil
	}

	// Update persistent run after plan creation
	run.MaxSteps = len(plan.Steps)
	run.Status = agent.AgentRunRunning
	if e.runs != nil {
		if err := e.runs.Update(run); err != nil {
			return nil, err
		}
	}

	var finalOutput map[string]any

	// 3. Execute plan steps sequentially
	for i, step := range plan.Steps {
		run.CurrentStepIndex = i
		if e.runs != nil {
			if err := e.runs.Update(run); err != nil {
				return nil, err
			}
		}

		stepStart := time.Now()

		agt, err := e.agents.Get(step.AgentID)
		if err != nil {
			now := time.Now()
			state.Status = StatusFailed
			state.EndedAt = &now
			state.Error = err.Error()

			stepRecord := &agent.AgentStep{
				StepID:     fmt.Sprintf("%s-step-%d", req.RunID, i),
				RunID:      req.RunID,
				Type:       agent.StepPlan,
				Status:     agent.StepFailed,
				Input:      fmt.Sprintf("%v", req.Input),
				Output:     err.Error(),
				StartedAt:  stepStart,
				FinishedAt: &now,
			}

			if e.steps != nil {
				_ = e.steps.Create(stepRecord)
			}

			run.Status = agent.AgentRunFailed
			run.CompletedAt = &now
			if e.runs != nil {
				_ = e.runs.Update(run)
			}

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

			stepRecord := &agent.AgentStep{
				StepID:     fmt.Sprintf("%s-step-%d", req.RunID, i),
				RunID:      req.RunID,
				Type:       agent.StepPlan,
				Status:     agent.StepFailed,
				Input:      fmt.Sprintf("%v", req.Input),
				Output:     err.Error(),
				StartedAt:  stepStart,
				FinishedAt: &now,
			}

			if e.steps != nil {
				_ = e.steps.Create(stepRecord)
			}

			run.Status = agent.AgentRunFailed
			run.CompletedAt = &now
			if e.runs != nil {
				_ = e.runs.Update(run)
			}

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

				stepRecord := &agent.AgentStep{
					StepID:     fmt.Sprintf("%s-step-%d", req.RunID, i),
					RunID:      req.RunID,
					Type:       agent.StepValidation,
					Status:     agent.StepFailed,
					Input:      fmt.Sprintf("%v", req.Input),
					Output:     err.Error(),
					StartedAt:  stepStart,
					FinishedAt: &now,
				}

				if e.steps != nil {
					_ = e.steps.Create(stepRecord)
				}

				run.Status = agent.AgentRunFailed
				run.CompletedAt = &now
				if e.runs != nil {
					_ = e.runs.Update(run)
				}

				return &ExecutionResult{
					RunID:  req.RunID,
					Status: StatusFailed,
					Err:    err,
				}, nil
			}
		}

		// Step succeeded → persist it
		finished := time.Now()
		stepRecord := &agent.AgentStep{
			StepID:     fmt.Sprintf("%s-step-%d", req.RunID, i),
			RunID:      req.RunID,
			Type:       agent.StepPlan,
			Status:     agent.StepSucceeded,
			Input:      fmt.Sprintf("%v", req.Input),
			Output:     fmt.Sprintf("%v", result.Output),
			StartedAt:  stepStart,
			FinishedAt: &finished,
		}

		if e.steps != nil {
			if err := e.steps.Create(stepRecord); err != nil {
				return nil, err
			}
		}
	}

	// Success transition
	now = time.Now()
	state.Status = StatusSucceeded
	state.EndedAt = &now

	run.Status = agent.AgentRunCompleted
	run.CompletedAt = &now
	if e.runs != nil {
		if err := e.runs.Update(run); err != nil {
			return nil, err
		}
	}

	return &ExecutionResult{
		RunID:  req.RunID,
		Status: StatusSucceeded,
		Output: finalOutput,
		Err:    nil,
	}, nil
}
