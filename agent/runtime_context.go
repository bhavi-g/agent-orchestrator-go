package agent

import (
	"context"

	"agent-orchestrator/tools"
)

// RuntimeContext is the minimal execution surface exposed to agents.
// It deliberately avoids any orchestrator dependency to prevent cycles.
type RuntimeContext struct {
	Ctx   context.Context
	Input map[string]any
	Tools tools.Executor
}
