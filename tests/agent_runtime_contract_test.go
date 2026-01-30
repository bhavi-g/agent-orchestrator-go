package tests

import (
	"context"
	"testing"

	"agent-orchestrator/agent"
)

func TestEchoAgent_RuntimeContract(t *testing.T) {
	a := agent.NewEchoAgent()

	rtx := agent.RuntimeContext{
		Ctx:   context.Background(),
		Input: map[string]any{"k": "v"},
	}

	out, err := a.RunWithContext(context.Background(), rtx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if out.Output["echo"].(map[string]any)["k"] != "v" {
		t.Fatalf("unexpected output: %#v", out.Output)
	}
}
