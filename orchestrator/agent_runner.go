package orchestrator

import (
	"context"

	"agent-orchestrator/agent"
)

// runtimeCapableAgent is an OPTIONAL capability.
// Any agent can implement it without importing orchestrator.
type runtimeCapableAgent interface {
	RunWithContext(ctx context.Context, rt agent.RuntimeContext) (*agent.Result, error)
}

func runAgent(ctx context.Context, a agent.Agent, execCtx ExecutionContext) (*agent.Result, error) {
	// Prefer runtime path when available.
	if ra, ok := any(a).(runtimeCapableAgent); ok {
		rt := agent.RuntimeContext{
			Ctx:   execCtx.Ctx,
			Input: execCtx.Request.Input,
			Tools: execCtx.Tools,
		}
		return ra.RunWithContext(ctx, rt)
	}

	// Phase-2 fallback.
	return a.Run(ctx, execCtx.Request.Input)
}
