package tools

import "context"

// Executor is the minimal runtime interface exposed to agents.
type Executor interface {
	Execute(ctx context.Context, call Call) (Result, error)
}
