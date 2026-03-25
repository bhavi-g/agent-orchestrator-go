package orchestrator

import (
	"context"
	"errors"
	"time"

	"agent-orchestrator/agent"
	"agent-orchestrator/failure"
	"agent-orchestrator/planner"
	"agent-orchestrator/repair"
	"agent-orchestrator/retry"
	"agent-orchestrator/tools"
)

type Engine struct {
	planner      planner.Planner
	replanner    planner.Replanner
	agents       *agent.Registry
	tools        tools.Executor
	validator    Validator
	runs         RunRepository
	steps        StepRepository
	toolRecorder tools.ToolCallRecorder
	repairEng    *repair.Engine
	classifier   *failure.Classifier
	maxReplans   int
	retryPolicy  retry.Policy
}

func NewEngine(
	p planner.Planner,
	agents *agent.Registry,
	tools tools.Executor,
	validator Validator,
	runs RunRepository,
	steps StepRepository,
	repairEng *repair.Engine,
) *Engine {
	// If planner also implements Replanner, use it
	var rp planner.Replanner
	if r, ok := p.(planner.Replanner); ok {
		rp = r
	}

	return &Engine{
		planner:     p,
		replanner:   rp,
		agents:      agents,
		tools:       tools,
		validator:   validator,
		runs:        runs,
		steps:       steps,
		repairEng:   repairEng,
		classifier:  failure.NewClassifier(),
		maxReplans:  3,
		retryPolicy: retry.DefaultPolicy(),
	}
}

// SetReplanner explicitly sets the replanner (if different from planner).
func (e *Engine) SetReplanner(rp planner.Replanner) {
	e.replanner = rp
}

// SetMaxReplans configures the maximum number of replans per execution.
func (e *Engine) SetMaxReplans(n int) {
	e.maxReplans = n
}

// SetRetryPolicy sets the global default retry policy for all steps.
func (e *Engine) SetRetryPolicy(p retry.Policy) {
	e.retryPolicy = p
}

// SetToolCallRepository enables tool call persistence.
func (e *Engine) SetToolCallRepository(repo tools.ToolCallRecorder) {
	e.toolRecorder = repo
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

	// Track completed steps (for replan context)
	var completedSteps []planner.CompletedStep
	replanCount := 0

	// 3. Execute plan steps sequentially with retry + replan
	i := 0
	for i < len(plan.Steps) {
		step := plan.Steps[i]

		run.CurrentStepIndex = i + len(completedSteps)
		if e.runs != nil {
			if err := e.runs.Update(run); err != nil {
				return nil, err
			}
		}

		// Execute step with automatic retry on failure
		output, stepErr := e.executeStepWithRetry(ctx, execCtx, step, i, run, tracker)
		if stepErr != nil {
			// Check if we should replan instead of failing
			if e.replanner != nil && replanCount < e.maxReplans {
				failureType := string(e.classifier.Classify(stepErr))

				rctx := planner.ReplanContext{
					TaskID:          req.TaskID,
					OriginalPlan:    plan,
					FailedStepIndex: i,
					FailedAgentID:   step.AgentID,
					FailureError:    stepErr.Error(),
					FailureType:     failureType,
					CompletedSteps:  completedSteps,
					Vars:            execCtx.Vars,
					Attempt:         replanCount + 1,
					MaxReplans:      e.maxReplans,
				}

				newPlan, replanErr := e.replanner.Replan(ctx, rctx)
				if replanErr == nil && newPlan != nil && len(newPlan.Steps) > 0 {
					replanCount++
					plan = newPlan
					tracker = newStepAttemptTracker()
					i = 0

					// Update run with new plan info
					run.MaxSteps = len(completedSteps) + len(newPlan.Steps)
					if e.runs != nil {
						_ = e.runs.Update(run)
					}

					continue
				}
			}

			// No replan available — fail
			now := time.Now()
			state.Status = StatusFailed
			state.EndedAt = &now
			state.Error = stepErr.Error()

			run.Status = agent.AgentRunFailed
			run.CompletedAt = &now
			if e.runs != nil {
				_ = e.runs.Update(run)
			}

			return &ExecutionResult{
				RunID:  req.RunID,
				Status: StatusFailed,
				Err:    stepErr,
			}, nil
		}

		finalOutput = output

		// Track completed step
		completedSteps = append(completedSteps, planner.CompletedStep{
			StepIndex: i,
			AgentID:   step.AgentID,
			Output:    output,
		})

		// Store step output in execution context
		for k, v := range output {
			execCtx.Vars[k] = v
		}

		i++
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
