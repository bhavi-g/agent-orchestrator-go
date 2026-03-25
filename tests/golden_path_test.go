package tests

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"agent-orchestrator/agent"
	"agent-orchestrator/orchestrator"
	"agent-orchestrator/planner"
	"agent-orchestrator/storage/sqlite"
	"agent-orchestrator/tools"
)

// --- Golden Path end-to-end test -------------------------------------------

func TestGoldenPath_LogAnalysis_EndToEnd(t *testing.T) {
	dir := t.TempDir()

	// Realistic multi-file log scenario.
	appLog := `2024-03-01 10:00:01 INFO  Application started on port 8080
2024-03-01 10:00:02 INFO  Connected to database
2024-03-01 10:00:15 WARN  Slow query detected (query_time=3200ms)
2024-03-01 10:01:00 ERROR Failed to process request: connection reset by peer
2024-03-01 10:01:05 ERROR Failed to process request: connection reset by peer
2024-03-01 10:01:10 ERROR Retry exhausted for request handler
2024-03-01 10:02:00 WARN  Connection pool running low (3/50 available)
2024-03-01 10:03:00 ERROR Failed to process request: connection reset by peer
2024-03-01 10:04:00 FATAL panic: nil pointer dereference in handler.ServeHTTP
2024-03-01 10:04:01 INFO  Graceful shutdown initiated
`
	workerLog := `2024-03-01 10:00:01 INFO  Worker started
2024-03-01 10:01:00 WARN  Job queue backlog: 150 pending
2024-03-01 10:02:00 ERROR Job 12345 failed: timeout waiting for database
2024-03-01 10:03:00 ERROR Job 12346 failed: timeout waiting for database
`
	os.WriteFile(filepath.Join(dir, "app.log"), []byte(appLog), 0644)
	os.WriteFile(filepath.Join(dir, "worker.log"), []byte(workerLog), 0644)
	os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("port: 8080"), 0644) // should be skipped

	// Build the full pipeline.
	toolRegistry := tools.NewRegistry()
	toolRegistry.Register(tools.NewReadFileTool(dir))
	toolRegistry.Register(tools.NewListDirTool(dir))
	toolRegistry.Register(tools.NewGrepFileTool(dir))
	toolExecutor := tools.NewRegistryExecutor(toolRegistry)

	agentReg := agent.NewRegistry()
	agentReg.Register("agent.log_reader", agent.NewLogReaderAgent())
	agentReg.Register("agent.log_analyzer", agent.NewLogAnalyzerAgent())

	pl := planner.NewLogAnalysisPlanner()
	validator := orchestrator.NewReportValidator()

	engine := orchestrator.NewEngine(pl, agentReg, toolExecutor, validator, nil, nil, nil)

	result, err := engine.Execute(context.Background(), orchestrator.ExecutionRequest{
		RunID:  "golden-path-1",
		TaskID: "analyze-app-logs",
		Input:  map[string]any{"directory": "."},
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.Status != orchestrator.StatusSucceeded {
		t.Fatalf("expected success, got %v (err=%v)", result.Status, result.Err)
	}

	out := result.Output

	// Validate all Golden Path output fields.
	errorSummary, ok := out["error_summary"].(string)
	if !ok || errorSummary == "" {
		t.Fatalf("missing or empty error_summary: %v", out["error_summary"])
	}

	rootCause, ok := out["suspected_root_cause"].(string)
	if !ok || rootCause == "" {
		t.Fatalf("missing or empty suspected_root_cause: %v", out["suspected_root_cause"])
	}

	confidence, ok := out["confidence_level"].(string)
	if !ok {
		t.Fatalf("missing confidence_level")
	}
	if confidence != "Low" && confidence != "Medium" && confidence != "High" {
		t.Fatalf("invalid confidence_level: %q", confidence)
	}

	evidence, ok := out["supporting_evidence"].([]map[string]any)
	if !ok || len(evidence) == 0 {
		t.Fatalf("expected non-empty supporting_evidence, got %v", out["supporting_evidence"])
	}
	// Each evidence item must have file, line_number, text.
	for i, ev := range evidence {
		if _, ok := ev["file"].(string); !ok {
			t.Fatalf("evidence[%d] missing file", i)
		}
		if _, ok := ev["line_number"].(int); !ok {
			t.Fatalf("evidence[%d] missing line_number", i)
		}
		if _, ok := ev["text"].(string); !ok {
			t.Fatalf("evidence[%d] missing text", i)
		}
	}

	nextSteps := out["suggested_next_steps"]
	switch ns := nextSteps.(type) {
	case []string:
		if len(ns) == 0 {
			t.Fatal("suggested_next_steps is empty")
		}
	case []any:
		if len(ns) == 0 {
			t.Fatal("suggested_next_steps is empty")
		}
	default:
		t.Fatalf("suggested_next_steps wrong type: %T", nextSteps)
	}

	t.Logf("=== Golden Path Report ===")
	t.Logf("Error Summary:  %s", errorSummary)
	t.Logf("Root Cause:     %s", rootCause)
	t.Logf("Confidence:     %s", confidence)
	t.Logf("Evidence items: %d", len(evidence))
	t.Logf("Next Steps:     %v", nextSteps)
}

func TestGoldenPath_NoErrors(t *testing.T) {
	dir := t.TempDir()
	cleanLog := `2024-03-01 10:00:01 INFO  Application started
2024-03-01 10:00:02 INFO  Health check passed
2024-03-01 10:00:03 INFO  Serving traffic
`
	os.WriteFile(filepath.Join(dir, "clean.log"), []byte(cleanLog), 0644)

	toolRegistry := tools.NewRegistry()
	toolRegistry.Register(tools.NewReadFileTool(dir))
	toolRegistry.Register(tools.NewListDirTool(dir))
	toolRegistry.Register(tools.NewGrepFileTool(dir))
	toolExecutor := tools.NewRegistryExecutor(toolRegistry)

	agentReg := agent.NewRegistry()
	agentReg.Register("agent.log_reader", agent.NewLogReaderAgent())
	agentReg.Register("agent.log_analyzer", agent.NewLogAnalyzerAgent())

	pl := planner.NewLogAnalysisPlanner()
	validator := orchestrator.NewReportValidator()
	engine := orchestrator.NewEngine(pl, agentReg, toolExecutor, validator, nil, nil, nil)

	result, err := engine.Execute(context.Background(), orchestrator.ExecutionRequest{
		RunID:  "golden-path-clean",
		TaskID: "analyze-clean-logs",
		Input:  map[string]any{"directory": "."},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != orchestrator.StatusSucceeded {
		t.Fatalf("expected success, got %v (err=%v)", result.Status, result.Err)
	}

	out := result.Output
	confidence := out["confidence_level"].(string)
	if confidence != "High" {
		t.Errorf("clean logs should produce High confidence, got %s", confidence)
	}

	summary := out["error_summary"].(string)
	t.Logf("Clean log report: %s (confidence=%s)", summary, confidence)
}

func TestGoldenPath_WithPersistence(t *testing.T) {
	dir := t.TempDir()
	logContent := `2024-03-01 12:00:00 ERROR disk full: /var/data
2024-03-01 12:00:01 FATAL shutting down due to disk full
`
	os.WriteFile(filepath.Join(dir, "system.log"), []byte(logContent), 0644)

	// Set up SQLite persistence.
	dbPath := filepath.Join(t.TempDir(), "golden_test.db")
	sqliteRepo := sqlite.New(dbPath)
	if err := sqliteRepo.Open(); err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	defer sqliteRepo.Close()

	runRepo := sqlite.NewAgentRunRepository(sqliteRepo.DB)
	stepRepo := sqlite.NewAgentStepRepository(sqliteRepo.DB)
	toolCallRepo := sqlite.NewToolCallRepository(sqliteRepo.DB)

	toolRegistry := tools.NewRegistry()
	toolRegistry.Register(tools.NewReadFileTool(dir))
	toolRegistry.Register(tools.NewListDirTool(dir))
	toolRegistry.Register(tools.NewGrepFileTool(dir))
	toolExecutor := tools.NewRegistryExecutor(toolRegistry)

	agentReg := agent.NewRegistry()
	agentReg.Register("agent.log_reader", agent.NewLogReaderAgent())
	agentReg.Register("agent.log_analyzer", agent.NewLogAnalyzerAgent())

	pl := planner.NewLogAnalysisPlanner()
	validator := orchestrator.NewReportValidator()

	engine := orchestrator.NewEngine(pl, agentReg, toolExecutor, validator,
		runRepo, stepRepo, nil)
	engine.SetToolCallRepository(toolCallRepo)

	result, err := engine.Execute(context.Background(), orchestrator.ExecutionRequest{
		RunID:  "golden-persisted",
		TaskID: "analyze-disk-full",
		Input:  map[string]any{"directory": "."},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != orchestrator.StatusSucceeded {
		t.Fatalf("expected success, got %v (err=%v)", result.Status, result.Err)
	}

	// Verify persistence: run should exist.
	run, err := runRepo.GetByID("golden-persisted")
	if err != nil {
		t.Fatalf("failed to get persisted run: %v", err)
	}
	if run.Status != agent.AgentRunCompleted {
		t.Fatalf("persisted run status = %s, want Completed", run.Status)
	}

	// Steps should be persisted.
	steps, err := stepRepo.GetByRunID("golden-persisted")
	if err != nil {
		t.Fatalf("failed to list steps: %v", err)
	}
	if len(steps) == 0 {
		t.Fatal("expected persisted steps, got none")
	}

	// Tool calls should be persisted (fs.list_dir and fs.grep_file at minimum).
	toolCalls, err := toolCallRepo.GetByRunID("golden-persisted")
	if err != nil {
		t.Fatalf("failed to list tool calls: %v", err)
	}
	if len(toolCalls) == 0 {
		t.Fatal("expected persisted tool calls, got none")
	}

	t.Logf("Persisted: %d steps, %d tool calls", len(steps), len(toolCalls))
	t.Logf("Report: %s (confidence=%s)",
		result.Output["error_summary"], result.Output["confidence_level"])
}

func TestGoldenPath_ReportValidation(t *testing.T) {
	v := orchestrator.NewReportValidator()

	// Valid report — should pass.
	err := v.Validate("agent.log_analyzer", map[string]any{
		"error_summary":        "1 error found",
		"suspected_root_cause": "disk full",
		"supporting_evidence":  []map[string]any{{"file": "a.log", "line_number": 1, "text": "err"}},
		"confidence_level":     "High",
		"suggested_next_steps": []string{"check disk"},
	})
	if err != nil {
		t.Fatalf("valid report rejected: %v", err)
	}

	// Missing error_summary.
	err = v.Validate("agent.log_analyzer", map[string]any{
		"suspected_root_cause": "x",
		"supporting_evidence":  []map[string]any{},
		"confidence_level":     "Low",
		"suggested_next_steps": []string{"y"},
	})
	if err == nil {
		t.Fatal("expected validation error for missing error_summary")
	}

	// Invalid confidence level.
	err = v.Validate("agent.log_analyzer", map[string]any{
		"error_summary":        "x",
		"suspected_root_cause": "x",
		"supporting_evidence":  []map[string]any{},
		"confidence_level":     "Invalid",
		"suggested_next_steps": []string{"y"},
	})
	if err == nil {
		t.Fatal("expected validation error for bad confidence_level")
	}

	// Non-analyzer step — should pass regardless.
	err = v.Validate("agent.echo", map[string]any{})
	if err != nil {
		t.Fatalf("non-analyzer step should pass, got: %v", err)
	}
}
