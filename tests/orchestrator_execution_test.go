package tests

import (
	"context"
	"errors"
	"strings"
	"testing"

	"agent-orchestrator/agent"
	"agent-orchestrator/orchestrator"
	"agent-orchestrator/planner"
	"agent-orchestrator/tools"
)

func TestEngineExecute(t *testing.T) {
	pl := planner.NewDummyPlanner()

	reg := agent.NewRegistry()
	reg.Register("agent.echo", agent.NewEchoAgent())

	toolRegistry := tools.NewRegistry()
	toolExecutor := tools.NewRegistryExecutor(toolRegistry)

	engine := orchestrator.NewEngine(pl, reg, toolExecutor, nil)

	result, err := engine.Execute(
		context.Background(),
		orchestrator.ExecutionRequest{
			RunID:  "run-1",
			TaskID: "task.test",
			Input: map[string]any{
				"msg": "hello",
			},
		},
	)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Status != orchestrator.StatusSucceeded {
		t.Fatalf("expected success, got %v", result.Status)
	}

	if result.Output["echo"] == nil {
		t.Fatalf("expected echo output, got %+v", result.Output)
	}
}

type uppercaseTool struct{}

func (u uppercaseTool) Name() string { return "uppercase" }

func (u uppercaseTool) Spec() tools.Spec {
	return tools.Spec{
		Name:        "uppercase",
		Description: "Converts text to uppercase",
		Input: map[string]tools.Field{
			"text": {Type: "string", Required: true},
		},
		Output: map[string]tools.Field{
			"text": {Type: "string", Required: true},
		},
	}
}

func (u uppercaseTool) Execute(ctx context.Context, call tools.Call) (tools.Result, error) {
	text, ok := call.Args["text"].(string)
	if !ok {
		return tools.Result{}, tools.ErrInvalidArgs
	}

	return tools.Result{
		ToolName: "uppercase",
		Data: map[string]any{
			"text": strings.ToUpper(text),
		},
	}, nil
}

func TestEngineExecute_WithTool(t *testing.T) {
	pl := planner.NewDummyPlanner()

	reg := agent.NewRegistry()
	reg.Register("agent.echo", agent.NewEchoAgent())

	toolRegistry := tools.NewRegistry()
	toolRegistry.Register(uppercaseTool{})
	toolExecutor := tools.NewRegistryExecutor(toolRegistry)

	engine := orchestrator.NewEngine(pl, reg, toolExecutor, nil)

	result, err := engine.Execute(
		context.Background(),
		orchestrator.ExecutionRequest{
			RunID:  "run-2",
			TaskID: "task.test",
			Input: map[string]any{
				"msg":      "hello",
				"use_tool": true,
			},
		},
	)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Status != orchestrator.StatusSucceeded {
		t.Fatalf("expected success, got %v", result.Status)
	}

	if result.Output["upper"] != "HELLO" {
		t.Fatalf("expected HELLO, got %+v", result.Output)
	}
}

// --- Validator used only for testing the validation hook ---
type failingValidator struct{}

func (f failingValidator) Validate(stepID string, output map[string]any) error {
	// EchoAgent always returns "echo", so this should deterministically fail.
	if output["echo"] != nil {
		return errors.New("validation failed: echo not allowed")
	}
	return nil
}

func TestEngineExecute_WithValidationFailure(t *testing.T) {
	pl := planner.NewDummyPlanner()

	reg := agent.NewRegistry()
	reg.Register("agent.echo", agent.NewEchoAgent())

	toolRegistry := tools.NewRegistry()
	toolExecutor := tools.NewRegistryExecutor(toolRegistry)

	engine := orchestrator.NewEngine(pl, reg, toolExecutor, failingValidator{})

	result, err := engine.Execute(
		context.Background(),
		orchestrator.ExecutionRequest{
			RunID:  "run-3",
			TaskID: "task.test",
			Input: map[string]any{
				"msg": "hello",
			},
		},
	)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Status != orchestrator.StatusFailed {
		t.Fatalf("expected StatusFailed, got %v", result.Status)
	}
}
