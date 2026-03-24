package planner

import (
	"context"
	"fmt"
	"strings"
)

// ContextAwarePlanner creates plans that adapt based on context.
// It implements both Planner and Replanner.
type ContextAwarePlanner struct {
	// registry maps taskID patterns to plan templates
	registry map[string][]PlanStep
	// fallbackAgentID is used when the original agent fails
	fallbackAgentID string
}

// NewContextAwarePlanner creates a new context-aware planner.
func NewContextAwarePlanner() *ContextAwarePlanner {
	return &ContextAwarePlanner{
		registry: make(map[string][]PlanStep),
	}
}

// RegisterTask registers a plan template for a given task ID.
func (p *ContextAwarePlanner) RegisterTask(taskID string, steps []PlanStep) {
	p.registry[taskID] = steps
}

// SetFallbackAgent sets the agent to use when the primary agent fails.
func (p *ContextAwarePlanner) SetFallbackAgent(agentID string) {
	p.fallbackAgentID = agentID
}

// CreatePlan implements Planner. It looks up a registered template or
// returns a default single-step plan.
func (p *ContextAwarePlanner) CreatePlan(ctx context.Context, taskID string) (*Plan, error) {
	// Exact match first
	if steps, ok := p.registry[taskID]; ok {
		return &Plan{TaskID: taskID, Steps: copySteps(steps)}, nil
	}

	// Prefix match
	for pattern, steps := range p.registry {
		if strings.HasPrefix(taskID, pattern) {
			return &Plan{TaskID: taskID, Steps: copySteps(steps)}, nil
		}
	}

	// Fallback: single step with echo agent
	return &Plan{
		TaskID: taskID,
		Steps:  []PlanStep{{AgentID: "agent.echo"}},
	}, nil
}

// Replan implements Replanner. It analyzes the failure context and produces
// a new plan of remaining steps.
func (p *ContextAwarePlanner) Replan(ctx context.Context, rctx ReplanContext) (*Plan, error) {
	if rctx.Attempt > rctx.MaxReplans {
		return nil, fmt.Errorf("exceeded max replans (%d)", rctx.MaxReplans)
	}

	remaining := p.buildRemainingSteps(rctx)

	return &Plan{
		TaskID: rctx.TaskID,
		Steps:  remaining,
	}, nil
}

// buildRemainingSteps decides what steps to produce based on the failure type.
func (p *ContextAwarePlanner) buildRemainingSteps(rctx ReplanContext) []PlanStep {
	switch rctx.FailureType {
	case "validation_failure":
		return p.handleValidationReplan(rctx)
	case "tool_failure":
		return p.handleToolReplan(rctx)
	case "agent_failure":
		return p.handleAgentReplan(rctx)
	default:
		return p.handleDefaultReplan(rctx)
	}
}

// handleValidationReplan tries the same remaining steps with adjusted metadata
// so the agent knows its previous output was rejected.
func (p *ContextAwarePlanner) handleValidationReplan(rctx ReplanContext) []PlanStep {
	original := rctx.OriginalPlan
	if original == nil {
		return []PlanStep{{AgentID: "agent.echo"}}
	}

	var remaining []PlanStep
	for i := rctx.FailedStepIndex; i < len(original.Steps); i++ {
		step := original.Steps[i]
		step.Metadata = mergeMetadata(step.Metadata, map[string]any{
			"__replan__":     true,
			"__replan_type__": "validation_failure",
			"__prev_error__": rctx.FailureError,
		})
		remaining = append(remaining, step)
	}

	if len(remaining) == 0 {
		remaining = []PlanStep{{AgentID: rctx.FailedAgentID}}
	}

	return remaining
}

// handleToolReplan keeps the same steps but tries an alternative agent
// for the failed step if a fallback is available.
func (p *ContextAwarePlanner) handleToolReplan(rctx ReplanContext) []PlanStep {
	original := rctx.OriginalPlan
	if original == nil {
		return []PlanStep{{AgentID: "agent.echo"}}
	}

	var remaining []PlanStep
	for i := rctx.FailedStepIndex; i < len(original.Steps); i++ {
		step := original.Steps[i]
		// For the failed step, try the fallback agent
		if i == rctx.FailedStepIndex && p.fallbackAgentID != "" {
			step.AgentID = p.fallbackAgentID
		}
		step.Metadata = mergeMetadata(step.Metadata, map[string]any{
			"__replan__":     true,
			"__replan_type__": "tool_failure",
		})
		remaining = append(remaining, step)
	}

	if len(remaining) == 0 {
		remaining = []PlanStep{{AgentID: rctx.FailedAgentID}}
	}

	return remaining
}

// handleAgentReplan replaces the failed agent with a fallback for the
// failed step and keeps remaining steps unchanged.
func (p *ContextAwarePlanner) handleAgentReplan(rctx ReplanContext) []PlanStep {
	original := rctx.OriginalPlan
	if original == nil {
		if p.fallbackAgentID != "" {
			return []PlanStep{{AgentID: p.fallbackAgentID}}
		}
		return []PlanStep{{AgentID: "agent.echo"}}
	}

	var remaining []PlanStep
	for i := rctx.FailedStepIndex; i < len(original.Steps); i++ {
		step := original.Steps[i]
		if i == rctx.FailedStepIndex && p.fallbackAgentID != "" {
			step.AgentID = p.fallbackAgentID
		}
		step.Metadata = mergeMetadata(step.Metadata, map[string]any{
			"__replan__":     true,
			"__replan_type__": "agent_failure",
		})
		remaining = append(remaining, step)
	}

	if len(remaining) == 0 {
		if p.fallbackAgentID != "" {
			remaining = []PlanStep{{AgentID: p.fallbackAgentID}}
		} else {
			remaining = []PlanStep{{AgentID: rctx.FailedAgentID}}
		}
	}

	return remaining
}

// handleDefaultReplan retries the remaining steps unchanged.
func (p *ContextAwarePlanner) handleDefaultReplan(rctx ReplanContext) []PlanStep {
	original := rctx.OriginalPlan
	if original == nil {
		return []PlanStep{{AgentID: "agent.echo"}}
	}

	var remaining []PlanStep
	for i := rctx.FailedStepIndex; i < len(original.Steps); i++ {
		remaining = append(remaining, original.Steps[i])
	}

	if len(remaining) == 0 {
		remaining = []PlanStep{{AgentID: rctx.FailedAgentID}}
	}

	return remaining
}

// mergeMetadata merges additional metadata into an existing map (non-destructive).
func mergeMetadata(existing, additional map[string]any) map[string]any {
	merged := make(map[string]any)
	for k, v := range existing {
		merged[k] = v
	}
	for k, v := range additional {
		merged[k] = v
	}
	return merged
}

// copySteps creates a deep copy of a step slice.
func copySteps(steps []PlanStep) []PlanStep {
	cp := make([]PlanStep, len(steps))
	for i, s := range steps {
		cp[i] = PlanStep{
			AgentID:  s.AgentID,
			Input:    copyMap(s.Input),
			Metadata: copyMap(s.Metadata),
		}
	}
	return cp
}

// copyMap shallow-copies a map.
func copyMap(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	cp := make(map[string]any, len(m))
	for k, v := range m {
		cp[k] = v
	}
	return cp
}
