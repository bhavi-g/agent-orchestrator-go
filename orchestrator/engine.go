package orchestrator

import (
	"context"
	"errors"
	"time"

	"agent-orchestrator/agent"
	"agent-orchestrator/failure"
	"agent-orchestrator/planner"
	"agent-orchestrator/repair"
	"agent-orchestrator/tools"
)

type Engine struct {
	planner    planner.Planner
	agents     *agent.Registry
	tools      tools.Executor
	validator  Validator
	runs       RunRepository
	steps      StepRepository
	repairEng  *repair.Engine
	classifier *failure.Classifier
}

func NewEngine(
	planner planner.Planner,
	agents *agent.Registry,
	tools tools.Executor,
	validator Validator,
	runs RunRepository,
	steps StepRepository,
	repairEng *repair.Engine,
) *Engine {
	return &Engine{
		planner:    planner,
		agents:     agents,
		tools:      tools,
		validator:  validator,
		runs:       runs,
		steps:      steps,
		repairEng:  repairEng,
		classifier: failure.NewClassifier(),
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

	// Initialize step attempt tracker
	tracker := newStepAttemptTracker()

	// 3. Execute plan steps sequentially with retry
	for i, step := range plan.Steps {
		run.CurrentStepIndex = i
		if e.runs != nil {
			if err := e.runs.Update(run); err != nil {
				return nil, err
			}
		}

		// Execute step with automatic retry on failure
		output, err := e.executeStepWithRetry(ctx, execCtx, step, i, run, tracker)
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

		finalOutput = output

		// Store step output in execution context
		for k, v := range output {
			execCtx.Vars[k] = v
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
