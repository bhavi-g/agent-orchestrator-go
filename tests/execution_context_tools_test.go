package tests

import (
	"context"
	"testing"

	"agent-orchestrator/orchestrator"
	"agent-orchestrator/tools"
)

type echoTool struct{}

func (e echoTool) Name() string { return "echo" }

func (e echoTool) Spec() tools.Spec {
	return tools.Spec{
		Name: "echo",
		Input: map[string]tools.Field{
			"msg": {Type: "string", Required: true},
		},
		Output: map[string]tools.Field{
			"echo": {Type: "string", Required: true},
		},
	}
}

func (e echoTool) Execute(ctx context.Context, call tools.Call) (tools.Result, error) {
	return tools.Result{
		ToolName: "echo",
		Data:     map[string]any{"echo": call.Args["msg"]},
	}, nil
}

func TestExecutionContext_ToolExecution(t *testing.T) {
	reg := tools.NewRegistry()
	if err := reg.Register(echoTool{}); err != nil {
		t.Fatalf("register tool: %v", err)
	}

	execCtx := orchestrator.ExecutionContext{
		Ctx:   context.Background(),
		Tools: tools.NewRegistryExecutor(reg),
		Vars:  map[string]any{},
	}

	res, err := execCtx.Tools.Execute(execCtx.Ctx, tools.Call{
		ToolName: "echo",
		Args:     map[string]any{"msg": "hello"},
	})
	if err != nil {
		t.Fatalf("execute tool: %v", err)
	}

	if res.Data["echo"] != "hello" {
		t.Fatalf("unexpected result: %#v", res.Data)
	}
}
