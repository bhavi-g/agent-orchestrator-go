package agent

import "context"

// Agent is the Phase 2 scaffolding interface.
// It must not depend on orchestrator to avoid import cycles.
type Agent interface {
	Run(ctx context.Context, input map[string]any) (*Result, error)
}
