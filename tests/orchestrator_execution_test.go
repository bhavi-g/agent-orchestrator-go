package tests

import (
	"context"
	"testing"

	"agent-orchestrator/agent"
	"agent-orchestrator/orchestrator"
	"agent-orchestrator/planner"
)

func TestEngineExecute(t *testing.T) {
	pl := planner.NewDummyPlanner()

	reg := agent.NewRegistry()
	reg.Register("agent.echo", agent.NewEchoAgent())

	engine := orchestrator.NewEngine(pl, reg)

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

	if result.Output["echo"] == nil {
		t.Fatalf("expected echo output, got %+v", result.Output)
	}
}
