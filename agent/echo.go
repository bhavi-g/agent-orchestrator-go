package agent

import "context"

type EchoAgent struct{}

func NewEchoAgent() *EchoAgent {
	return &EchoAgent{}
}

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
