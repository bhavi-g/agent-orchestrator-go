package tests

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"agent-orchestrator/agent"
	"agent-orchestrator/orchestrator"
	"agent-orchestrator/planner"
	"agent-orchestrator/tools"
)

// --- Grounding validator unit tests ----------------------------------------

func TestGroundingValidator_PassesGroundedEvidence(t *testing.T) {
	// Simulate tool call outputs that the evidence references.
	collector := tools.NewToolCallCollector(nilExecutor{})

	// Manually feed the collector with what tool calls would produce.
	gv := orchestrator.NewGroundingValidator()
	gv.SetCollector(collector)

	// Simulate the collector having seen these outputs from grep calls.
	simulateGrepOutput(t, collector, "app.log", "ERROR Failed to connect")
	simulateGrepOutput(t, collector, "app.log", "WARN  Retrying connection")

	output := map[string]any{
		"error_summary":        "1 error",
		"suspected_root_cause": "connection failure",
		"confidence_level":     "High",
		"supporting_evidence": []map[string]any{
			{"file": "app.log", "line_number": 2, "text": "ERROR Failed to connect"},
			{"file": "app.log", "line_number": 3, "text": "WARN  Retrying connection"},
		},
		"suggested_next_steps": []string{"check connection"},
	}

	err := gv.Validate("agent.log_analyzer", output)
	if err != nil {
		t.Fatalf("grounded evidence should pass, got: %v", err)
	}
}

func TestGroundingValidator_DetectsHallucinatedFile(t *testing.T) {
	collector := tools.NewToolCallCollector(nilExecutor{})
	gv := orchestrator.NewGroundingValidator()
	gv.SetCollector(collector)

	// Only app.log was accessed via tools.
	simulateGrepOutput(t, collector, "app.log", "ERROR something")

	output := map[string]any{
		"error_summary":        "1 error",
		"suspected_root_cause": "unknown",
		"confidence_level":     "Low",
		"supporting_evidence": []map[string]any{
			{"file": "app.log", "line_number": 1, "text": "ERROR something"},
			{"file": "phantom.log", "line_number": 5, "text": "ERROR ghost error"}, // hallucinated
		},
		"suggested_next_steps": []string{"investigate"},
	}

	err := gv.Validate("agent.log_analyzer", output)
	if err == nil {
		t.Fatal("expected grounding violation for hallucinated file")
	}
	if !strings.Contains(err.Error(), "phantom.log") {
		t.Fatalf("error should mention phantom.log, got: %v", err)
	}
}

func TestGroundingValidator_DetectsHallucinatedText(t *testing.T) {
	collector := tools.NewToolCallCollector(nilExecutor{})
	gv := orchestrator.NewGroundingValidator()
	gv.SetCollector(collector)

	simulateGrepOutput(t, collector, "app.log", "ERROR real error message")

	output := map[string]any{
		"error_summary":        "1 error",
		"suspected_root_cause": "unknown",
		"confidence_level":     "Low",
		"supporting_evidence": []map[string]any{
			{"file": "app.log", "line_number": 1, "text": "ERROR completely fabricated message"}, // hallucinated
		},
		"suggested_next_steps": []string{"check"},
	}

	err := gv.Validate("agent.log_analyzer", output)
	if err == nil {
		t.Fatal("expected grounding violation for hallucinated text")
	}
	if !strings.Contains(err.Error(), "not found in any tool call output") {
		t.Fatalf("error should mention text not found, got: %v", err)
	}
}

func TestGroundingValidator_SkipsNonAnalyzerStep(t *testing.T) {
	gv := orchestrator.NewGroundingValidator()
	// No collector set — should still pass for non-analyzer steps.
	err := gv.Validate("agent.echo", map[string]any{"anything": true})
	if err != nil {
		t.Fatalf("non-analyzer step should pass, got: %v", err)
	}
}

func TestGroundingValidator_SkipsWhenNoCollector(t *testing.T) {
	gv := orchestrator.NewGroundingValidator()
	// No collector — graceful degradation.
	err := gv.Validate("agent.log_analyzer", map[string]any{
		"supporting_evidence": []map[string]any{
			{"file": "ghost.log", "text": "invented"},
		},
	})
	if err != nil {
		t.Fatalf("should degrade gracefully without collector, got: %v", err)
	}
}

func TestGroundingValidator_EmptyEvidence(t *testing.T) {
	collector := tools.NewToolCallCollector(nilExecutor{})
	gv := orchestrator.NewGroundingValidator()
	gv.SetCollector(collector)

	output := map[string]any{
		"supporting_evidence": []map[string]any{},
	}
	err := gv.Validate("agent.log_analyzer", output)
	if err != nil {
		t.Fatalf("empty evidence should pass, got: %v", err)
	}
}

// --- Composite validator test ---

func TestCompositeValidator_RunsBothValidators(t *testing.T) {
	collector := tools.NewToolCallCollector(nilExecutor{})
	gv := orchestrator.NewGroundingValidator()
	gv.SetCollector(collector)

	cv := orchestrator.NewCompositeValidator(
		orchestrator.NewReportValidator(),
		gv,
	)

	// Missing required field — ReportValidator should catch it first.
	err := cv.Validate("agent.log_analyzer", map[string]any{
		"suspected_root_cause": "x",
	})
	if err == nil {
		t.Fatal("expected report validation error")
	}
	if !strings.Contains(err.Error(), "error_summary") {
		t.Fatalf("expected report validator error, got: %v", err)
	}
}

// --- End-to-end grounding test with engine ----------------------------------

func TestGrounding_EndToEnd_Passes(t *testing.T) {
	// The real pipeline should pass grounding because LogAnalyzerAgent
	// only uses data from LogReaderAgent which comes from real tool calls.
	dir := t.TempDir()
	logContent := `2024-03-01 10:00:00 ERROR database timeout
2024-03-01 10:00:01 WARN  retrying query
`
	os.WriteFile(filepath.Join(dir, "db.log"), []byte(logContent), 0644)

	toolRegistry := tools.NewRegistry()
	toolRegistry.Register(tools.NewReadFileTool(dir))
	toolRegistry.Register(tools.NewListDirTool(dir))
	toolRegistry.Register(tools.NewGrepFileTool(dir))
	toolExecutor := tools.NewRegistryExecutor(toolRegistry)

	agentReg := agent.NewRegistry()
	agentReg.Register("agent.log_reader", agent.NewLogReaderAgent())
	agentReg.Register("agent.log_analyzer", agent.NewLogAnalyzerAgent())

	pl := planner.NewLogAnalysisPlanner()
	validator := orchestrator.NewCompositeValidator(
		orchestrator.NewReportValidator(),
		orchestrator.NewGroundingValidator(),
	)

	engine := orchestrator.NewEngine(pl, agentReg, toolExecutor, validator, nil, nil, nil)

	result, err := engine.Execute(context.Background(), orchestrator.ExecutionRequest{
		RunID:  "grounding-e2e",
		TaskID: "check-grounding",
		Input:  map[string]any{"directory": "."},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != orchestrator.StatusSucceeded {
		t.Fatalf("expected success, got %v (err=%v)", result.Status, result.Err)
	}

	t.Logf("Grounding E2E passed: %s (confidence=%s)",
		result.Output["error_summary"], result.Output["confidence_level"])
}

func TestGrounding_EndToEnd_DetectsHallucinatingAgent(t *testing.T) {
	// Use a custom agent that fabricates evidence not from tools.
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "real.log"), []byte("2024-03-01 INFO  ok\n"), 0644)

	toolRegistry := tools.NewRegistry()
	toolRegistry.Register(tools.NewReadFileTool(dir))
	toolRegistry.Register(tools.NewListDirTool(dir))
	toolRegistry.Register(tools.NewGrepFileTool(dir))
	toolExecutor := tools.NewRegistryExecutor(toolRegistry)

	agentReg := agent.NewRegistry()
	agentReg.Register("agent.log_reader", agent.NewLogReaderAgent())
	agentReg.Register("agent.hallucinator", &hallucinatingAgent{})

	// Custom planner: log_reader → hallucinator
	pl := &hallucinatorPlanner{}

	validator := orchestrator.NewCompositeValidator(
		orchestrator.NewReportValidator(),
		orchestrator.NewGroundingValidator(),
	)

	engine := orchestrator.NewEngine(pl, agentReg, toolExecutor, validator, nil, nil, nil)

	result, err := engine.Execute(context.Background(), orchestrator.ExecutionRequest{
		RunID:  "grounding-hallucination",
		TaskID: "detect-hallucination",
		Input:  map[string]any{"directory": "."},
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}

	// Should fail because the hallucinator references phantom files/text.
	if result.Status != orchestrator.StatusFailed {
		t.Fatalf("expected failure from grounding check, got %v", result.Status)
	}
	if result.Err == nil || !strings.Contains(result.Err.Error(), "grounding check failed") {
		t.Fatalf("expected grounding violation error, got: %v", result.Err)
	}

	t.Logf("Correctly detected hallucination: %v", result.Err)
}

// --- test helpers ---

// nilExecutor never executes — used only so the collector satisfies the interface.
type nilExecutor struct{}

func (nilExecutor) Execute(ctx context.Context, call tools.Call) (tools.Result, error) {
	return tools.Result{}, nil
}

// simulateGrepOutput feeds a collector as if fs.grep_file had been called.
func simulateGrepOutput(t *testing.T, collector *tools.ToolCallCollector, file, text string) {
	t.Helper()
	// We call through the collector so it records the output.
	// Use a fake executor that returns grep-like data.
	fakeExec := &fakeGrepExecutor{file: file, text: text}
	c := tools.NewToolCallCollector(fakeExec)
	c.Execute(context.Background(), tools.Call{
		ToolName: "fs.grep_file",
		Args:     map[string]any{"path": file, "keyword": "test"},
	})
	// Copy outputs into the target collector.
	for _, o := range c.Outputs() {
		collector.AddOutput(o)
	}
}

type fakeGrepExecutor struct {
	file string
	text string
}

func (f *fakeGrepExecutor) Execute(ctx context.Context, call tools.Call) (tools.Result, error) {
	return tools.Result{
		ToolName: "fs.grep_file",
		Data: map[string]any{
			"matches": []map[string]any{
				{"line_number": 1, "text": f.text},
			},
			"match_count": 1,
			"path":        f.file,
			"keyword":     "test",
		},
	}, nil
}

// hallucinatingAgent produces a report with fabricated evidence.
type hallucinatingAgent struct{}

func (a *hallucinatingAgent) Run(ctx context.Context, input map[string]any) (*agent.Result, error) {
	return &agent.Result{Output: map[string]any{
		"error_summary":        "Found critical errors in phantom systems",
		"suspected_root_cause": "Quantum flux capacitor overload",
		"supporting_evidence": []map[string]any{
			{"file": "phantom_system.log", "line_number": 42, "text": "CRITICAL quantum flux overflow detected"},
			{"file": "nonexistent.log", "line_number": 99, "text": "FATAL warp core breach imminent"},
		},
		"confidence_level":     "High",
		"suggested_next_steps": []string{"Recalibrate the flux capacitor"},
	}}, nil
}

type hallucinatorPlanner struct{}

func (p *hallucinatorPlanner) CreatePlan(_ context.Context, taskID string) (*planner.Plan, error) {
	return &planner.Plan{
		TaskID: taskID,
		Steps: []planner.PlanStep{
			{AgentID: "agent.log_reader", StepID: "scan"},
			{AgentID: "agent.hallucinator", StepID: "analyze", DependsOn: []string{"scan"}},
		},
	}, nil
}
