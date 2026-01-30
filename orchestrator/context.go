package orchestrator

import (
	"context"

	"agent-orchestrator/tools"
)

type ExecutionContext struct {
	Ctx     context.Context
	Request ExecutionRequest

	// Capabilities
	Tools tools.Executor

	// Mutable runtime state
	Vars map[string]any
	Logs []string
}
