package repair

import (
	"agent-orchestrator/failure"
	"context"
	"fmt"
)

// AdvancedStrategy implements sophisticated repair logic with multiple tactics
type AdvancedStrategy struct {
	corrector         *InputCorrector
	enableInputCorrection bool
	enableStepReplacement bool
}

// NewAdvancedStrategy creates an advanced repair strategy
func NewAdvancedStrategy() *AdvancedStrategy {
	return &AdvancedStrategy{
		corrector:             NewInputCorrector(),
		enableInputCorrection: true,
		enableStepReplacement: true,
	}
}

// CreateRepairPlan creates a sophisticated repair plan
func (s *AdvancedStrategy) CreateRepairPlan(ctx context.Context, f *failure.FailureEvent) (*RepairPlan, error) {
	plan := NewRepairPlan(f)

	switch f.Type {
	case failure.ValidationFailure:
		return s.handleValidationFailure(ctx, f, plan)
	
	case failure.ToolFailure:
		return s.handleToolFailure(ctx, f, plan)
	
	case failure.AgentFailure:
		return s.handleAgentFailure(ctx, f, plan)
	
	default:
		plan.AddAction(RepairAction{
			Type:      Abort,
			StepIndex: f.StepIndex,
		})
		plan.WithReasoning("unknown failure type")
		return plan, nil
	}
}

// handleValidationFailure creates repair plan for validation failures
func (s *AdvancedStrategy) handleValidationFailure(ctx context.Context, f *failure.FailureEvent, plan *RepairPlan) (*RepairPlan, error) {
	// Attempt 1: Try input correction if applicable
	if f.Attempt == 1 && s.enableInputCorrection && s.corrector.CanCorrect(f) {
		// Note: We don't have original input here, so we signal need for correction
		plan.AddAction(RepairAction{
			Type:      ModifyInput,
			StepIndex: f.StepIndex,
			AgentID:   f.AgentID,
			Metadata: map[string]any{
				"correction_strategy": "validation_based",
			},
		})
		plan.WithReasoning("attempting input correction based on validation error")
		return plan, nil
	}

	// Attempt 2+: Request replan
	if f.Attempt >= 2 {
		plan.ShouldReplan = true
		plan.AddAction(RepairAction{
			Type:      Replan,
			StepIndex: f.StepIndex,
			Metadata: map[string]any{
				"reason": "validation_failed_after_correction",
			},
		})
		plan.WithReasoning("validation failed repeatedly, requesting replan")
		return plan, nil
	}

	// Default: simple retry
	plan.AddAction(RepairAction{
		Type:      RetryStep,
		StepIndex: f.StepIndex,
	})
	plan.WithReasoning("retrying after validation failure")
	return plan, nil
}

// handleToolFailure creates repair plan for tool failures
func (s *AdvancedStrategy) handleToolFailure(ctx context.Context, f *failure.FailureEvent, plan *RepairPlan) (*RepairPlan, error) {
	// Tool failures are usually transient (timeouts, rate limits, etc.)
	// Simple retry is often sufficient
	
	if f.Attempt <= 2 {
		// First two attempts: retry with backoff
		plan.AddAction(RepairAction{
			Type:      RetryStep,
			StepIndex: f.StepIndex,
			Metadata: map[string]any{
				"tool_failure": true,
			},
		})
		plan.WithReasoning(fmt.Sprintf("tool failure (attempt %d), retrying with backoff", f.Attempt))
		return plan, nil
	}

	// After 2 attempts, consider step replacement
	if s.enableStepReplacement {
		plan.AddAction(RepairAction{
			Type:      ReplaceStep,
			StepIndex: f.StepIndex,
			AgentID:   fmt.Sprintf("%s.alternative", f.AgentID), // Signal for alternative agent
			Metadata: map[string]any{
				"replacement_reason": "tool_failure_persistent",
			},
		})
		plan.WithReasoning("tool failure persists, attempting step replacement")
		return plan, nil
	}

	// No replacement available, abort
	plan.AddAction(RepairAction{
		Type:      Abort,
		StepIndex: f.StepIndex,
	})
	plan.WithReasoning("tool failure persists, no alternatives available")
	return plan, nil
}

// handleAgentFailure creates repair plan for agent failures
func (s *AdvancedStrategy) handleAgentFailure(ctx context.Context, f *failure.FailureEvent, plan *RepairPlan) (*RepairPlan, error) {
	// Agent failures might be due to internal errors, crashes, etc.
	
	if f.Attempt == 1 {
		// First attempt: simple retry (might be transient)
		plan.AddAction(RepairAction{
			Type:      RetryStep,
			StepIndex: f.StepIndex,
		})
		plan.WithReasoning("agent failure, attempting retry")
		return plan, nil
	}

	// After first retry, try step replacement
	if f.Attempt == 2 && s.enableStepReplacement {
		plan.AddAction(RepairAction{
			Type:      ReplaceStep,
			StepIndex: f.StepIndex,
			AgentID:   "", // Signal that engine should find replacement
			Metadata: map[string]any{
				"replacement_reason": "agent_failure_persistent",
				"original_agent":     f.AgentID,
			},
		})
		plan.WithReasoning("agent failure persists, attempting agent replacement")
		return plan, nil
	}

	// Last resort: replan
	if f.Attempt >= 3 {
		plan.ShouldReplan = true
		plan.AddAction(RepairAction{
			Type:      Replan,
			StepIndex: f.StepIndex,
		})
		plan.WithReasoning("agent failure persists, requesting replan")
		return plan, nil
	}

	// Fallback
	plan.AddAction(RepairAction{
		Type:      Abort,
		StepIndex: f.StepIndex,
	})
	plan.WithReasoning("agent failure cannot be repaired")
	return plan, nil
}
