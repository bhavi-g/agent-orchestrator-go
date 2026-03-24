package tests

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"agent-orchestrator/agent"
	"agent-orchestrator/orchestrator"
	"agent-orchestrator/planner"
	"agent-orchestrator/storage/sqlite"
	"agent-orchestrator/tools"
)

func TestEngineExecute(t *testing.T) {
	pl := planner.NewDummyPlanner()

	reg := agent.NewRegistry()
	reg.Register("agent.echo", agent.NewEchoAgent())

	toolRegistry := tools.NewRegistry()
	toolExecutor := tools.NewRegistryExecutor(toolRegistry)

	engine := orchestrator.NewEngine(pl, reg, toolExecutor, nil, nil, nil, nil)

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

	engine := orchestrator.NewEngine(pl, reg, toolExecutor, nil, nil, nil, nil)

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

	engine := orchestrator.NewEngine(pl, reg, toolExecutor, failingValidator{}, nil, nil, nil)

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

func TestEngineExecute_PersistsRunAndSteps_SQLite(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "orchestrator_test.db")

	sqliteRepo := sqlite.New(dbPath)
	if err := sqliteRepo.Open(); err != nil {
		t.Fatalf("failed to open sqlite repo: %v", err)
	}
	defer func() {
		_ = sqliteRepo.Close()
		_ = os.Remove(dbPath)
	}()

	runRepo := sqlite.NewAgentRunRepository(sqliteRepo.DB)
	stepRepo := sqlite.NewAgentStepRepository(sqliteRepo.DB)

	pl := planner.NewDummyPlanner()

	reg := agent.NewRegistry()
	reg.Register("agent.echo", agent.NewEchoAgent())

	toolRegistry := tools.NewRegistry()
	toolExecutor := tools.NewRegistryExecutor(toolRegistry)

	engine := orchestrator.NewEngine(
		pl,
		reg,
		toolExecutor,
		nil,
		runRepo,
		stepRepo,
		nil,
	)

	runID := "run-persist-1"

	result, err := engine.Execute(
		context.Background(),
		orchestrator.ExecutionRequest{
			RunID:  runID,
			TaskID: "task.persist",
			Input: map[string]any{
				"msg": "hello persistence",
			},
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Status != orchestrator.StatusSucceeded {
		t.Fatalf("expected StatusSucceeded, got %v", result.Status)
	}

	persistedRun, err := runRepo.GetByID(runID)
	if err != nil {
		t.Fatalf("failed to load persisted run: %v", err)
	}

	if persistedRun.RunID != runID {
		t.Fatalf("expected run_id %q, got %q", runID, persistedRun.RunID)
	}

	if persistedRun.Status != agent.AgentRunCompleted {
		t.Fatalf("expected persisted run status %q, got %q", agent.AgentRunCompleted, persistedRun.Status)
	}

	if persistedRun.CompletedAt == nil {
		t.Fatalf("expected persisted run to have CompletedAt set")
	}

	steps, err := stepRepo.GetByRunID(runID)
	if err != nil {
		t.Fatalf("failed to load persisted steps: %v", err)
	}

	if len(steps) == 0 {
		t.Fatalf("expected at least one persisted step, got 0")
	}

	first := steps[0]
	if first.RunID != runID {
		t.Fatalf("expected first step RunID %q, got %q", runID, first.RunID)
	}

	if first.Status != agent.StepSucceeded {
		t.Fatalf("expected first step status %q, got %q", agent.StepSucceeded, first.Status)
	}

	if first.FinishedAt == nil {
		t.Fatalf("expected first step FinishedAt to be set")
	}
}

