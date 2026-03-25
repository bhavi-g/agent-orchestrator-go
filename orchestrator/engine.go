package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"sync"
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
	toolCalls    ToolCallRepository
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

// SetToolCallReader enables reading back persisted tool calls for replay.
func (e *Engine) SetToolCallReader(repo ToolCallRepository) {
	e.toolCalls = repo
}

// groundingValidator returns the GroundingValidator if it's part of the
// current validator chain.
func (e *Engine) groundingValidator() (*GroundingValidator, bool) {
	if e.validator == nil {
		return nil, false
	}
	if gv, ok := e.validator.(*GroundingValidator); ok {
		return gv, true
	}
	if cv, ok := e.validator.(*CompositeValidator); ok {
		for _, v := range cv.validators {
			if gv, ok := v.(*GroundingValidator); ok {
				return gv, true
			}
		}
	}
	return nil, false
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
		// If the record was pre-created by the HTTP handler, this will fail
		// (duplicate key). That's fine — just look it up instead.
		if err := e.runs.Create(run); err != nil {
			if existing, lookupErr := e.runs.GetByID(req.RunID); lookupErr == nil {
				run = existing
			} else {
				return nil, err
			}
		}
	}

	// 1. Create execution context
	execCtx := &ExecutionContext{
		Ctx:     ctx,
		Request: req,
		Tools:   e.tools,
		Vars:    make(map[string]any),
	}

	// Wrap the tool executor with a run-level collector so that grounding
	// validation can cross-reference claims against ALL tool calls in the run.
	if execCtx.Tools != nil {
		runCollector := tools.NewToolCallCollector(execCtx.Tools)
		execCtx.Tools = runCollector

		if gv, ok := e.groundingValidator(); ok {
			gv.SetCollector(runCollector)
		}
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

	// 3. Build DAG scheduler and execute steps (parallel when possible)
	dag, dagErr := newDAGScheduler(plan)
	if dagErr != nil {
		now := time.Now()
		run.Status = agent.AgentRunFailed
		run.CompletedAt = &now
		if e.runs != nil {
			_ = e.runs.Update(run)
		}
		return &ExecutionResult{
			RunID:  req.RunID,
			Status: StatusFailed,
			Err:    dagErr,
		}, nil
	}

	completed := 0
	var mu sync.Mutex // protects execCtx.Vars, completedSteps, finalOutput

	for completed < dag.Len() {
		ready := dag.ReadySteps()
		if len(ready) == 0 {
			// Should never happen if cycle detection is correct
			break
		}

		// Parallel batch: launch all ready steps concurrently.
		type stepResult struct {
			idx    int
			output map[string]any
			err    error
		}

		results := make([]stepResult, len(ready))
		var wg sync.WaitGroup

		for ri, stepIdx := range ready {
			wg.Add(1)

			go func(ri, stepIdx int) {
				defer wg.Done()

				step := plan.Steps[stepIdx]

				mu.Lock()
				run.CurrentStepIndex = stepIdx + len(completedSteps)
				if e.runs != nil {
					_ = e.runs.Update(run)
				}
				mu.Unlock()

				output, stepErr := e.executeStepWithRetry(ctx, execCtx, step, stepIdx, run, tracker)
				results[ri] = stepResult{idx: stepIdx, output: output, err: stepErr}
			}(ri, stepIdx)
		}

		wg.Wait()

		// Process results: if any step failed, try replan or fail.
		var firstFailure *stepResult
		for i := range results {
			if results[i].err != nil {
				firstFailure = &results[i]
				break
			}
		}

		if firstFailure != nil {
			failedStep := plan.Steps[firstFailure.idx]

			if e.replanner != nil && replanCount < e.maxReplans {
				failureType := string(e.classifier.Classify(firstFailure.err))

				// Collect completed steps from successful results in this batch
				mu.Lock()
				for _, r := range results {
					if r.err == nil && r.output != nil {
						completedSteps = append(completedSteps, planner.CompletedStep{
							StepIndex: r.idx,
							AgentID:   plan.Steps[r.idx].AgentID,
							Output:    r.output,
						})
						for k, v := range r.output {
							execCtx.Vars[k] = v
						}
					}
				}
				mu.Unlock()

				rctx := planner.ReplanContext{
					TaskID:          req.TaskID,
					OriginalPlan:    plan,
					FailedStepIndex: firstFailure.idx,
					FailedAgentID:   failedStep.AgentID,
					FailureError:    firstFailure.err.Error(),
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

					// Rebuild DAG for the new plan
					newDAG, dErr := newDAGScheduler(plan)
					if dErr != nil {
						return &ExecutionResult{
							RunID:  req.RunID,
							Status: StatusFailed,
							Err:    fmt.Errorf("replan DAG error: %w", dErr),
						}, nil
					}
					dag = newDAG
					completed = 0

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
			state.Error = firstFailure.err.Error()

			run.Status = agent.AgentRunFailed
			run.CompletedAt = &now
			if e.runs != nil {
				_ = e.runs.Update(run)
			}

			return &ExecutionResult{
				RunID:  req.RunID,
				Status: StatusFailed,
				Err:    firstFailure.err,
			}, nil
		}

		// All steps in batch succeeded — merge outputs and advance DAG.
		mu.Lock()
		for _, r := range results {
			finalOutput = r.output

			completedSteps = append(completedSteps, planner.CompletedStep{
				StepIndex: r.idx,
				AgentID:   plan.Steps[r.idx].AgentID,
				Output:    r.output,
			})

			for k, v := range r.output {
				execCtx.Vars[k] = v
			}

			dag.MarkDone(r.idx)
			completed++
		}
		mu.Unlock()
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

// Replay re-runs a previously persisted agent run. It loads the stored tool
// call records and substitutes a ReplayExecutor that returns those outputs
// instead of invoking real tools. The agents, planner, and validators run
// exactly as in a normal execution — only the tool layer is replaced.
func (e *Engine) Replay(
	ctx context.Context,
	originalRunID string,
) (*ExecutionResult, error) {
	if e.runs == nil {
		return nil, errors.New("replay: run repository is required")
	}
	if e.toolCalls == nil {
		return nil, errors.New("replay: tool call repository is required")
	}

	// 1. Load the original run to recover the goal (task ID).
	origRun, err := e.runs.GetByID(originalRunID)
	if err != nil {
		return nil, fmt.Errorf("replay: run %s not found: %w", originalRunID, err)
	}

	// 2. Load persisted tool call records.
	agentCalls, err := e.toolCalls.GetByRunID(originalRunID)
	if err != nil {
		return nil, fmt.Errorf("replay: failed to load tool calls: %w", err)
	}

	records := make([]tools.ToolCallRecord, len(agentCalls))
	for i, tc := range agentCalls {
		records[i] = tools.ToolCallRecord{
			ToolCallID: tc.ToolCallID,
			RunID:      tc.RunID,
			StepID:     tc.StepID,
			ToolName:   tc.ToolName,
			Input:      tc.Input,
			Output:     tc.Output,
			Succeeded:  tc.Status == agent.ToolCallSucceeded,
			StartedAt:  tc.StartedAt,
		}
		if tc.FinishedAt != nil {
			records[i].FinishedAt = *tc.FinishedAt
		}
	}

	replayExec, err := tools.NewReplayExecutor(records)
	if err != nil {
		return nil, fmt.Errorf("replay: %w", err)
	}

	// 3. Temporarily swap the engine's tool executor for the replay executor
	//    and disable tool-call recording (we don't persist replayed calls).
	origTools := e.tools
	origRecorder := e.toolRecorder
	e.tools = replayExec
	e.toolRecorder = nil
	defer func() {
		e.tools = origTools
		e.toolRecorder = origRecorder
	}()

	// 4. Execute through the normal pipeline with a new run ID.
	newRunID := fmt.Sprintf("replay-%s", originalRunID)

	result, err := e.Execute(ctx, ExecutionRequest{
		RunID:  newRunID,
		TaskID: origRun.Goal,
		Input:  nil, // the original input is embedded in the stored tool call outputs
	})

	// Attach replay metadata.
	if result != nil {
		if result.Output == nil {
			result.Output = make(map[string]any)
		}
		result.Output["_replay_source"] = originalRunID
		result.Output["_replay_unconsumed_calls"] = replayExec.Unconsumed()
	}

	return result, err
}
