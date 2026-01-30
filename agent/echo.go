package agent

import "context"

type EchoAgent struct{}

func NewEchoAgent() *EchoAgent {
	return &EchoAgent{}
}

/*
Run is the Phase 2 scaffolding entrypoint.
UNCHANGED.
*/
func (a *EchoAgent) Run(
	ctx context.Context,
	input map[string]any,
) (*Result, error) {

	return &Result{
		Output: map[string]any{
			"echo": input,
		},
	}, nil
}

/*
RunWithContext is the Phase 3 runtime-capable entrypoint.
It depends ONLY on agent.RuntimeContext.
*/
func (a *EchoAgent) RunWithContext(
	ctx context.Context,
	rtx RuntimeContext,
) (*Result, error) {

	return &Result{
		Output: map[string]any{
			"echo": rtx.Input,
		},
	}, nil
}
