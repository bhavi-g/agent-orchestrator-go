package agent

import (
	"context"

	"agent-orchestrator/tools"
)


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

	output := map[string]any{
		"echo": rtx.Input,
	}

	// Optional tool usage (Phase 4 behavior)
	useTool, _ := rtx.Input["use_tool"].(bool)
	msg, _ := rtx.Input["msg"].(string)

	if useTool && rtx.Tools != nil {
		res, err := rtx.Tools.Execute(ctx, tools.Call{
			ToolName: "uppercase",
			Args: map[string]any{
				"text": msg,
			},
		})
		if err != nil {
			return nil, err
		}

		output["upper"] = res.Data["text"]
	}

	return &Result{Output: output}, nil
}
