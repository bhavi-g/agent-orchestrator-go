package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"agent-orchestrator/agent"
	"agent-orchestrator/api"
	"agent-orchestrator/api/handlers"
	"agent-orchestrator/orchestrator"
	"agent-orchestrator/planner"
	"agent-orchestrator/tools"
)

// --- MetricsEvaluator unit tests -------------------------------------------

func TestMetrics_SuccessfulRun(t *testing.T) {
	runRepo := newMemRunRepo()
	stepRepo := newMemStepRepo()
	toolCallRepo := newMemToolCallRepo()

	// Simulate a successful run.
	runRepo.Create(&agent.AgentRun{RunID: "r1", Status: agent.AgentRunCompleted})

	// Step with valid structured output.
	stepRepo.Create(&agent.AgentStep{
		StepID: "r1-step-1-attempt-1",
		RunID:  "r1",
		Status: agent.StepSucceeded,
		Output: mustJSON(map[string]any{
			"error_summary":        "1 error found",
			"suspected_root_cause": "disk full",
			"supporting_evidence": []any{
				map[string]any{"file": "app.log", "text": "ERROR disk full"},
			},
			"confidence_level":     "High",
			"suggested_next_steps": []any{"clear disk"},
		}),
	})

	// Tool call that the evidence references.
	toolCallRepo.Create(&agent.ToolCall{
		ToolCallID: "tc1", RunID: "r1", StepID: "r1-step-0",
		ToolName: "fs.grep_file",
		Input:    `{"path":"app.log","keyword":"ERROR"}`,
		Output:   mustJSON(map[string]any{"path": "app.log", "matches": []any{map[string]any{"text": "ERROR disk full"}}}),
		Status:   agent.ToolCallSucceeded,
	})

	eval := orchestrator.NewMetricsEvaluator(runRepo, stepRepo, toolCallRepo)
	m, err := eval.Evaluate("r1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertEqual(t, "TotalRuns", m.TotalRuns, 1)
	assertEqual(t, "SucceededRuns", m.SucceededRuns, 1)
	assertEqual(t, "FailedRuns", m.FailedRuns, 0)
	assertRate(t, "StructuredOutputRate", m.StructuredOutputRate, 1.0)
	assertRate(t, "EvidenceCoverage", m.EvidenceCoverage, 1.0)
	assertRate(t, "HallucinationRate", m.HallucinationRate, 0.0)
	assertRate(t, "RepairSuccessRate", m.RepairSuccessRate, 0.0) // no retries
}

func TestMetrics_FailedRun(t *testing.T) {
	runRepo := newMemRunRepo()
	stepRepo := newMemStepRepo()
	toolCallRepo := newMemToolCallRepo()

	runRepo.Create(&agent.AgentRun{RunID: "r1", Status: agent.AgentRunFailed})
	stepRepo.Create(&agent.AgentStep{
		StepID: "r1-step-0-attempt-1",
		RunID:  "r1",
		Status: agent.StepFailed,
		Output: "agent not found",
	})

	eval := orchestrator.NewMetricsEvaluator(runRepo, stepRepo, toolCallRepo)
	m, err := eval.Evaluate("r1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertEqual(t, "TotalRuns", m.TotalRuns, 1)
	assertEqual(t, "SucceededRuns", m.SucceededRuns, 0)
	assertEqual(t, "FailedRuns", m.FailedRuns, 1)
	assertRate(t, "StructuredOutputRate", m.StructuredOutputRate, 0.0)
}

func TestMetrics_HallucinationDetected(t *testing.T) {
	runRepo := newMemRunRepo()
	stepRepo := newMemStepRepo()
	toolCallRepo := newMemToolCallRepo()

	runRepo.Create(&agent.AgentRun{RunID: "r1", Status: agent.AgentRunCompleted})

	// Step with hallucinated evidence.
	stepRepo.Create(&agent.AgentStep{
		StepID: "r1-step-1-attempt-1",
		RunID:  "r1",
		Status: agent.StepSucceeded,
		Output: mustJSON(map[string]any{
			"error_summary":        "1 error",
			"suspected_root_cause": "unknown",
			"supporting_evidence": []any{
				map[string]any{"file": "real.log", "text": "ERROR real"},
				map[string]any{"file": "phantom.log", "text": "FAKE fabricated error"},
			},
			"confidence_level":     "Low",
			"suggested_next_steps": []any{"check"},
		}),
	})

	// Only real.log was accessed via tools.
	toolCallRepo.Create(&agent.ToolCall{
		ToolCallID: "tc1", RunID: "r1", StepID: "r1-step-0",
		ToolName: "fs.grep_file",
		Input:    `{"path":"real.log","keyword":"ERROR"}`,
		Output:   mustJSON(map[string]any{"path": "real.log", "matches": []any{map[string]any{"text": "ERROR real"}}}),
		Status:   agent.ToolCallSucceeded,
	})

	eval := orchestrator.NewMetricsEvaluator(runRepo, stepRepo, toolCallRepo)
	m, err := eval.Evaluate("r1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 2 evidence items, 1 grounded, 1 hallucinated → 50% hallucination
	assertEqual(t, "TotalEvidenceItems", m.TotalEvidenceItems, 2)
	assertEqual(t, "GroundedEvidenceItems", m.GroundedEvidenceItems, 1)
	assertRate(t, "HallucinationRate", m.HallucinationRate, 0.5)
	assertRate(t, "EvidenceCoverage", m.EvidenceCoverage, 0.5)
}

func TestMetrics_RepairSuccess(t *testing.T) {
	runRepo := newMemRunRepo()
	stepRepo := newMemStepRepo()
	toolCallRepo := newMemToolCallRepo()

	runRepo.Create(&agent.AgentRun{RunID: "r1", Status: agent.AgentRunCompleted})

	// Step 0: first attempt failed, second attempt succeeded → repaired.
	stepRepo.Create(&agent.AgentStep{
		StepID: "r1-step-0-attempt-1",
		RunID:  "r1",
		Status: agent.StepFailed,
		Output: "timeout",
	})
	stepRepo.Create(&agent.AgentStep{
		StepID: "r1-step-0-attempt-2",
		RunID:  "r1",
		Status: agent.StepSucceeded,
		Output: mustJSON(map[string]any{"message": "ok"}),
	})

	// Step 1: single attempt succeeded → not repairable.
	stepRepo.Create(&agent.AgentStep{
		StepID: "r1-step-1-attempt-1",
		RunID:  "r1",
		Status: agent.StepSucceeded,
		Output: mustJSON(map[string]any{
			"error_summary":        "none",
			"suspected_root_cause": "n/a",
			"supporting_evidence":  []any{},
			"confidence_level":     "High",
			"suggested_next_steps": []any{"nothing"},
		}),
	})

	eval := orchestrator.NewMetricsEvaluator(runRepo, stepRepo, toolCallRepo)
	m, err := eval.Evaluate("r1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertEqual(t, "TotalRepairableSteps", m.TotalRepairableSteps, 1)
	assertEqual(t, "SuccessfullyRepaired", m.SuccessfullyRepaired, 1)
	assertRate(t, "RepairSuccessRate", m.RepairSuccessRate, 1.0)
}

func TestMetrics_RepairFailed(t *testing.T) {
	runRepo := newMemRunRepo()
	stepRepo := newMemStepRepo()
	toolCallRepo := newMemToolCallRepo()

	runRepo.Create(&agent.AgentRun{RunID: "r1", Status: agent.AgentRunFailed})

	// Step 0: two failed attempts, no success → failed repair.
	stepRepo.Create(&agent.AgentStep{
		StepID: "r1-step-0-attempt-1",
		RunID:  "r1",
		Status: agent.StepFailed,
		Output: "error 1",
	})
	stepRepo.Create(&agent.AgentStep{
		StepID: "r1-step-0-attempt-2",
		RunID:  "r1",
		Status: agent.StepFailed,
		Output: "error 2",
	})

	eval := orchestrator.NewMetricsEvaluator(runRepo, stepRepo, toolCallRepo)
	m, err := eval.Evaluate("r1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertEqual(t, "TotalRepairableSteps", m.TotalRepairableSteps, 1)
	assertEqual(t, "SuccessfullyRepaired", m.SuccessfullyRepaired, 0)
	assertRate(t, "RepairSuccessRate", m.RepairSuccessRate, 0.0)
}

func TestMetrics_AggregateMultipleRuns(t *testing.T) {
	runRepo := newMemRunRepo()
	stepRepo := newMemStepRepo()
	toolCallRepo := newMemToolCallRepo()

	// Run 1: succeeded with structured output.
	runRepo.Create(&agent.AgentRun{RunID: "r1", Status: agent.AgentRunCompleted})
	stepRepo.Create(&agent.AgentStep{
		StepID: "r1-step-1-attempt-1",
		RunID:  "r1",
		Status: agent.StepSucceeded,
		Output: mustJSON(map[string]any{
			"error_summary":        "1 error",
			"suspected_root_cause": "oom",
			"supporting_evidence":  []any{},
			"confidence_level":     "High",
			"suggested_next_steps": []any{"restart"},
		}),
	})

	// Run 2: failed with no structured output.
	runRepo.Create(&agent.AgentRun{RunID: "r2", Status: agent.AgentRunFailed})
	stepRepo.Create(&agent.AgentStep{
		StepID: "r2-step-0-attempt-1",
		RunID:  "r2",
		Status: agent.StepFailed,
		Output: "crash",
	})

	allRuns, _ := runRepo.List()
	eval := orchestrator.NewMetricsEvaluator(runRepo, stepRepo, toolCallRepo)
	m, err := eval.EvaluateAll(allRuns)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertEqual(t, "TotalRuns", m.TotalRuns, 2)
	assertEqual(t, "SucceededRuns", m.SucceededRuns, 1)
	assertEqual(t, "FailedRuns", m.FailedRuns, 1)
	assertRate(t, "StructuredOutputRate", m.StructuredOutputRate, 0.5)
}

// --- E2E metrics from real pipeline -----------------------------------------

func TestMetrics_EndToEnd(t *testing.T) {
	dir := t.TempDir()
	logContent := "2024-03-01 10:00:00 ERROR out of memory\n2024-03-01 10:00:01 WARN  gc pressure\n"
	os.WriteFile(filepath.Join(dir, "server.log"), []byte(logContent), 0644)

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

	runRepo := newMemRunRepo()
	stepRepo := newMemStepRepo()
	toolCallRepo := newMemToolCallRepo()

	engine := orchestrator.NewEngine(pl, agentReg, toolExecutor, validator, runRepo, stepRepo, nil)
	engine.SetToolCallRepository(toolCallRepo)

	result, err := engine.Execute(context.Background(), orchestrator.ExecutionRequest{
		RunID:  "metrics-e2e",
		TaskID: "metrics-test",
		Input:  map[string]any{"directory": "."},
	})
	if err != nil {
		t.Fatalf("execution error: %v", err)
	}
	if result.Status != orchestrator.StatusSucceeded {
		t.Fatalf("execution failed: %v", result.Err)
	}

	// Now compute metrics for this run.
	eval := orchestrator.NewMetricsEvaluator(runRepo, stepRepo, toolCallRepo)
	m, err := eval.Evaluate("metrics-e2e")
	if err != nil {
		t.Fatalf("metrics error: %v", err)
	}

	// Verify key metrics.
	assertEqual(t, "TotalRuns", m.TotalRuns, 1)
	assertEqual(t, "SucceededRuns", m.SucceededRuns, 1)
	assertRate(t, "StructuredOutputRate", m.StructuredOutputRate, 1.0)

	// Evidence should be fully grounded (no hallucination from real pipeline).
	assertRate(t, "HallucinationRate", m.HallucinationRate, 0.0)
	assertRate(t, "EvidenceCoverage", m.EvidenceCoverage, 1.0)

	if m.TotalToolCalls == 0 {
		t.Fatal("expected tool calls to be recorded")
	}

	t.Logf("E2E Metrics: runs=%d, success_rate=%.0f%%, evidence_coverage=%.0f%%, hallucination=%.0f%%, tool_calls=%d",
		m.TotalRuns,
		m.StructuredOutputRate*100,
		m.EvidenceCoverage*100,
		m.HallucinationRate*100,
		m.TotalToolCalls)
}

// --- HTTP metrics handler test ----------------------------------------------

func TestMetricsHTTP_GetMetrics(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "app.log"),
		[]byte("2024-03-01 10:00:00 ERROR timeout\n"), 0644)

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

	runRepo := newMemRunRepo()
	stepRepo := newMemStepRepo()
	toolCallRepo := newMemToolCallRepo()

	engine := orchestrator.NewEngine(pl, agentReg, toolExecutor, validator, runRepo, stepRepo, nil)
	engine.SetToolCallRepository(toolCallRepo)

	// Execute a run.
	engine.Execute(context.Background(), orchestrator.ExecutionRequest{
		RunID: "http-metrics-run", TaskID: "http-metrics", Input: map[string]any{"directory": "."},
	})

	// Set up HTTP test server.
	eval := orchestrator.NewMetricsEvaluator(runRepo, stepRepo, toolCallRepo)
	mh := handlers.NewMetricsHandler(eval, runRepo)
	rh := handlers.NewRunHandler(engine, runRepo, stepRepo, toolCallRepo)
	router := api.NewRouter(rh, nil, mh)
	ts := httptest.NewServer(router)
	defer ts.Close()

	// GET /metrics (aggregate)
	resp, err := http.Get(ts.URL + "/metrics")
	if err != nil {
		t.Fatalf("HTTP error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var m orchestrator.Metrics
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}

	assertEqual(t, "TotalRuns", m.TotalRuns, 1)
	if m.StructuredOutputRate <= 0 {
		t.Fatalf("expected positive structured output rate, got %f", m.StructuredOutputRate)
	}

	t.Logf("HTTP metrics: %+v", m)

	// GET /metrics/<runID> (single run)
	resp2, err := http.Get(ts.URL + "/metrics/http-metrics-run")
	if err != nil {
		t.Fatalf("HTTP error: %v", err)
	}
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp2.StatusCode)
	}

	var m2 orchestrator.Metrics
	if err := json.NewDecoder(resp2.Body).Decode(&m2); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}

	assertEqual(t, "SingleRunTotal", m2.TotalRuns, 1)
}

func TestMetricsHTTP_NotFound(t *testing.T) {
	runRepo := newMemRunRepo()
	stepRepo := newMemStepRepo()
	toolCallRepo := newMemToolCallRepo()

	eval := orchestrator.NewMetricsEvaluator(runRepo, stepRepo, toolCallRepo)
	mh := handlers.NewMetricsHandler(eval, runRepo)
	rh := handlers.NewRunHandler(nil, runRepo, stepRepo, toolCallRepo)
	router := api.NewRouter(rh, nil, mh)
	ts := httptest.NewServer(router)
	defer ts.Close()

	resp, _ := http.Get(ts.URL + "/metrics/nonexistent")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

// --- test helpers ---

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("mustJSON: %v", err))
	}
	return string(b)
}

func assertEqual(t *testing.T, name string, got, want int) {
	t.Helper()
	if got != want {
		t.Fatalf("%s: got %d, want %d", name, got, want)
	}
}

func assertRate(t *testing.T, name string, got, want float64) {
	t.Helper()
	const epsilon = 0.001
	diff := got - want
	if diff < 0 {
		diff = -diff
	}
	if diff > epsilon {
		t.Fatalf("%s: got %.3f, want %.3f", name, got, want)
	}
}

// verify unused imports are not present — just use strings to avoid
// compiler warning for the test package imports
var _ = strings.Contains
