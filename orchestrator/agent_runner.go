package orchestrator

import (
	"context"

	"agent-orchestrator/agent"
	"agent-orchestrator/planner"
)

// runtimeCapableAgent is an OPTIONAL capability.
// Any agent can implement it without importing orchestrator.
type runtimeCapableAgent interface {
	RunWithContext(ctx context.Context, rt agent.RuntimeContext) (*agent.Result, error)
}

func runAgent(ctx context.Context, a agent.Agent, execCtx ExecutionContext, step planner.PlanStep) (*agent.Result, error) {
	// Build effective input: start with Vars (outputs of prior steps),
	// overlay the original request input, then overlay per-step input.
	effectiveInput := make(map[string]any)
	for k, v := range execCtx.Vars {
		effectiveInput[k] = v
	}
	for k, v := range execCtx.Request.Input {
		effectiveInput[k] = v
	}
	for k, v := range step.Input {
		effectiveInput[k] = v
	}

	// Prefer runtime path when available.
	if ra, ok := any(a).(runtimeCapableAgent); ok {
		rt := agent.RuntimeContext{
			Ctx:   execCtx.Ctx,
			Input: effectiveInput,
			Tools: execCtx.Tools,
		}
		return ra.RunWithContext(ctx, rt)
	}

	// Phase-2 fallback.
	return a.Run(ctx, effectiveInput)
}
