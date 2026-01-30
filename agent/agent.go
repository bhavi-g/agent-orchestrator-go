package agent

import "context"

type Result struct {
	Output map[string]any
}

type Agent interface {
	Run(ctx context.Context, input map[string]any) (*Result, error)
}
