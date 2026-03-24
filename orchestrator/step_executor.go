package orchestrator

import (
	"context"
	"fmt"
	"time"

	"agent-orchestrator/agent"
	"agent-orchestrator/failure"
	"agent-orchestrator/planner"
	"agent-orchestrator/repair"
)

// stepAttemptTracker tracks attempts for each step
type stepAttemptTracker struct {
	attempts map[int]int // stepIndex -> attempt count
}

func newStepAttemptTracker() *stepAttemptTracker {
	return &stepAttemptTracker{
		attempts: make(map[int]int),
	}
}

func (t *stepAttemptTracker) incrementAttempt(stepIndex int) int {
	t.attempts[stepIndex]++
	return t.attempts[stepIndex]
}

func (t *stepAttemptTracker) getAttempt(stepIndex int) int {
	return t.attempts[stepIndex]
}

// executeStepWithRetry executes a single step with retry logic
func (e *Engine) executeStepWithRetry(
	ctx context.Context,
	execCtx *ExecutionContext,
	step planner.PlanStep,
	stepIndex int,
	run *agent.AgentRun,
	tracker *stepAttemptTracker,
) (map[string]any, error) {
	
	// Current input for this step (may be modified by repair)
	currentInput := execCtx.Request.Input
	currentAgentID := step.AgentID
	
	for {
		attemptNum := tracker.incrementAttempt(stepIndex)
		
		// Apply retry delay if configured
		if attemptNum > 1 && e.repairEng != nil {
			delay := e.repairEng.GetRetryDelay(attemptNum)
			if delay > 0 {
				time.Sleep(delay)
			}
		}
		
		stepStart := time.Now()

		// Get agent (use current agent ID which may have been replaced)
		agt, err := e.agents.Get(currentAgentID)
		if err != nil {
			if e.repairEng == nil {
				// No repair engine, fail immediately
				return nil, err
			}

			// Create failure event
			failureEvent := failure.NewFailureEvent(
				run.RunID,
				stepIndex,
				currentAgentID,
				failure.AgentFailure,
				err,
			).WithAttempt(attemptNum)

			// Attempt repair
			repairPlan, repairErr := e.repairEng.Repair(ctx, failureEvent)
			if repairErr != nil {
				return nil, fmt.Errorf("repair failed: %w", repairErr)
			}

			if repairPlan.HasAbort() {
				return nil, err
			}

			// Record failed attempt
			e.recordFailedStep(run.RunID, stepIndex, stepStart, err, attemptNum)

			// For now, simple retry on agent not found is pointless
			// In future, repair might suggest alternative agent
			return nil, err
		}

		// Execute agent
		result, err := runAgent(ctx, agt, *execCtx)
		if err != nil {
			if e.repairEng == nil {
				return nil, err
			}

			// Classify and create failure event
			failureType := e.classifier.Classify(err)
			failureEvent := failure.NewFailureEvent(
				run.RunID,
				stepIndex,
				currentAgentID,
				failureType,
				err,
			).WithAttempt(attemptNum)

			// Attempt repair
			repairPlan, repairErr := e.repairEng.Repair(ctx, failureEvent)
			if repairErr != nil {
				return nil, fmt.Errorf("repair failed: %w", repairErr)
			}

			if repairPlan.HasAbort() {
				e.recordFailedStep(run.RunID, stepIndex, stepStart, err, attemptNum)
				return nil, err
			}

			// Record failed attempt
			e.recordFailedStep(run.RunID, stepIndex, stepStart, err, attemptNum)

			// Handle repair actions
			actionApplied := e.applyRepairActions(repairPlan, &currentInput, &currentAgentID)
			if actionApplied {
				continue // Retry with modifications
			}

			return nil, err
		}

		// Validate if validator exists
		if e.validator != nil {
			err := e.validator.Validate(step.AgentID, result.Output)
			if err != nil {
				if e.repairEng == nil {
					return nil, err
				}

				// Create validation failure event
				failureEvent := failure.NewFailureEvent(
					run.RunID,
					stepIndex,
					currentAgentID,
					failure.ValidationFailure,
					err,
				).WithOutput(result.Output).WithAttempt(attemptNum)

				// Attempt repair
				repairPlan, repairErr := e.repairEng.Repair(ctx, failureEvent)
				if repairErr != nil {
					return nil, fmt.Errorf("repair failed: %w", repairErr)
				}

				if repairPlan.HasAbort() {
					e.recordFailedStep(run.RunID, stepIndex, stepStart, err, attemptNum)
					return nil, err
				}

				// Record failed attempt
				e.recordFailedStep(run.RunID, stepIndex, stepStart, err, attemptNum)

				// Handle repair actions
				actionApplied := e.applyRepairActions(repairPlan, &currentInput, &currentAgentID)
				if actionApplied {
					continue // Retry with modifications
				}

				return nil, err
			}
		}

		// Success! Record the step
		e.recordSuccessfulStep(run.RunID, stepIndex, stepStart, execCtx.Request.Input, result.Output, attemptNum)

		return result.Output, nil
	}
}

// applyRepairActions applies repair actions and returns true if retry should happen
func (e *Engine) applyRepairActions(plan *repair.RepairPlan, currentInput *map[string]any, currentAgentID *string) bool {
	for _, action := range plan.Actions {
		switch action.Type {
		case repair.RetryStep:
			// Simple retry with current settings
			return true
		
		case repair.ModifyInput:
			// Modify input and retry
			if action.ModifiedInput != nil {
				*currentInput = action.ModifiedInput
			}
			return true
		
		case repair.ReplaceStep:
			// Replace agent and retry
			if action.AgentID != "" && action.AgentID != *currentAgentID {
				// Check if alternative agent exists
				if _, err := e.agents.Get(action.AgentID); err == nil {
					*currentAgentID = action.AgentID
					return true
				}
			}
			// If replacement not available, continue to next action
		
		case repair.Replan:
			// Signal the engine-level loop to replan
			return false
		
		case repair.Abort:
			return false
		}
	}
	return false
}

// recordFailedStep persists a failed step attempt
func (e *Engine) recordFailedStep(runID string, stepIndex int, startedAt time.Time, err error, attempt int) {
	if e.steps == nil {
		return
	}

	now := time.Now()
	stepRecord := &agent.AgentStep{
		StepID:     fmt.Sprintf("%s-step-%d-attempt-%d", runID, stepIndex, attempt),
		RunID:      runID,
		Type:       agent.StepPlan,
		Status:     agent.StepFailed,
		Input:      fmt.Sprintf("step-%d", stepIndex),
		Output:     err.Error(),
		StartedAt:  startedAt,
		FinishedAt: &now,
	}

	_ = e.steps.Create(stepRecord)
}

// recordSuccessfulStep persists a successful step
func (e *Engine) recordSuccessfulStep(runID string, stepIndex int, startedAt time.Time, input, output map[string]any, attempt int) {
	if e.steps == nil {
		return
	}

	now := time.Now()
	stepRecord := &agent.AgentStep{
		StepID:     fmt.Sprintf("%s-step-%d-attempt-%d", runID, stepIndex, attempt),
		RunID:      runID,
		Type:       agent.StepPlan,
		Status:     agent.StepSucceeded,
		Input:      fmt.Sprintf("%v", input),
		Output:     fmt.Sprintf("%v", output),
		StartedAt:  startedAt,
		FinishedAt: &now,
	}

	_ = e.steps.Create(stepRecord)
}
