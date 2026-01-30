package orchestrator

import "context"

type ExecutionContext struct {
	Ctx     context.Context
	Request ExecutionRequest

	// Mutable runtime state
	Vars map[string]any
	Logs []string
}

