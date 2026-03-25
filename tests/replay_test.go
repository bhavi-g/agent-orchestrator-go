package tests

import (
	"context"
	"encoding/json"
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

// --- ReplayExecutor unit tests ----------------------------------------------

func TestReplayExecutor_ReturnsStoredOutput(t *testing.T) {
	records := []tools.ToolCallRecord{
		{
			ToolCallID: "tc-1",
			RunID:      "run-1",
			StepID:     "step-0",
			ToolName:   "math.add",
			Input:      `{"a":1,"b":2}`,
			Output:     `{"sum":3}`,
			Succeeded:  true,
		},
	}

	exec, err := tools.NewReplayExecutor(records)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result, err := exec.Execute(context.Background(), tools.Call{
		ToolName: "math.add",
		Args:     map[string]any{"a": float64(1), "b": float64(2)},
	})
	if err != nil {
		t.Fatalf("replay execute failed: %v", err)
	}

	sum, ok := result.Data["sum"]
	if !ok {
		t.Fatal("expected 'sum' in result data")
	}
	// JSON numbers unmarshal to float64
	if sum != float64(3) {
		t.Fatalf("expected sum=3, got %v", sum)
	}
}

func TestReplayExecutor_ErrorOnMissingCall(t *testing.T) {
	exec, err := tools.NewReplayExecutor(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = exec.Execute(context.Background(), tools.Call{
		ToolName: "math.add",
		Args:     map[string]any{"a": float64(1), "b": float64(2)},
	})
	if err == nil {
		t.Fatal("expected error for unmatched call")
	}
	if !strings.Contains(err.Error(), "no stored output") {
		t.Fatalf("expected 'no stored output' error, got: %v", err)
	}
}

func TestReplayExecutor_SkipsFailedRecords(t *testing.T) {
	records := []tools.ToolCallRecord{
		{
			ToolCallID: "tc-1",
			ToolName:   "math.add",
			Input:      `{"a":1,"b":2}`,
			Output:     "some error",
			Succeeded:  false,
		},
	}

	exec, err := tools.NewReplayExecutor(records)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = exec.Execute(context.Background(), tools.Call{
		ToolName: "math.add",
		Args:     map[string]any{"a": float64(1), "b": float64(2)},
	})
	if err == nil {
		t.Fatal("expected error because the only record was a failure")
	}
}

func TestReplayExecutor_ConsumesInOrder(t *testing.T) {
	records := []tools.ToolCallRecord{
		{ToolCallID: "tc-1", ToolName: "fs.read_file", Input: `{"path":"a.txt"}`, Output: `{"content":"AAA"}`, Succeeded: true},
		{ToolCallID: "tc-2", ToolName: "fs.read_file", Input: `{"path":"a.txt"}`, Output: `{"content":"BBB"}`, Succeeded: true},
	}

	exec, err := tools.NewReplayExecutor(records)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// First call should get first record.
	r1, _ := exec.Execute(context.Background(), tools.Call{
		ToolName: "fs.read_file",
		Args:     map[string]any{"path": "a.txt"},
	})
	if r1.Data["content"] != "AAA" {
		t.Fatalf("expected AAA, got %v", r1.Data["content"])
	}

	// Second identical call should get the second record.
	r2, _ := exec.Execute(context.Background(), tools.Call{
		ToolName: "fs.read_file",
		Args:     map[string]any{"path": "a.txt"},
	})
	if r2.Data["content"] != "BBB" {
		t.Fatalf("expected BBB, got %v", r2.Data["content"])
	}

	if exec.Unconsumed() != 0 {
		t.Fatalf("expected 0 unconsumed, got %d", exec.Unconsumed())
	}
}

func TestReplayExecutor_Unconsumed(t *testing.T) {
	records := []tools.ToolCallRecord{
		{ToolCallID: "tc-1", ToolName: "math.add", Input: `{"a":1,"b":2}`, Output: `{"sum":3}`, Succeeded: true},
		{ToolCallID: "tc-2", ToolName: "math.mul", Input: `{"a":3,"b":4}`, Output: `{"product":12}`, Succeeded: true},
	}

	exec, err := tools.NewReplayExecutor(records)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if exec.Unconsumed() != 2 {
		t.Fatalf("expected 2 unconsumed, got %d", exec.Unconsumed())
	}

	// Consume one.
	exec.Execute(context.Background(), tools.Call{
		ToolName: "math.add",
		Args:     map[string]any{"a": float64(1), "b": float64(2)},
	})

	if exec.Unconsumed() != 1 {
		t.Fatalf("expected 1 unconsumed, got %d", exec.Unconsumed())
	}
}

// --- Engine.Replay E2E tests ------------------------------------------------

func TestReplay_EndToEnd(t *testing.T) {
	// 1. Run the real log analysis pipeline to populate tool call records.
	dir := t.TempDir()
	logContent := "2024-03-01 10:00:00 ERROR disk full\n2024-03-01 10:00:01 WARN  cleanup started\n"
	os.WriteFile(filepath.Join(dir, "system.log"), []byte(logContent), 0644)

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
	engine.SetToolCallReader(toolCallRepo)

	// Execute original run.
	origResult, err := engine.Execute(context.Background(), orchestrator.ExecutionRequest{
		RunID:  "original-run",
		TaskID: "replay-test",
		Input:  map[string]any{"directory": "."},
	})
	if err != nil {
		t.Fatalf("original run error: %v", err)
	}
	if origResult.Status != orchestrator.StatusSucceeded {
		t.Fatalf("original run failed: %v", origResult.Err)
	}

	// Verify tool calls were recorded.
	storedCalls, _ := toolCallRepo.GetByRunID("original-run")
	if len(storedCalls) == 0 {
		t.Fatal("expected persisted tool calls from original run")
	}
	t.Logf("Original run recorded %d tool calls", len(storedCalls))

	// 2. Replay from stored state — no real tools needed.
	replayResult, err := engine.Replay(context.Background(), "original-run")
	if err != nil {
		t.Fatalf("replay error: %v", err)
	}
	if replayResult.Status != orchestrator.StatusSucceeded {
		t.Fatalf("replay failed: %v", replayResult.Err)
	}

	// 3. Verify replay metadata.
	if replayResult.Output["_replay_source"] != "original-run" {
		t.Fatalf("expected _replay_source=original-run, got %v", replayResult.Output["_replay_source"])
	}

	t.Logf("Replay succeeded: error_summary=%v, confidence=%v, unconsumed=%v",
		replayResult.Output["error_summary"],
		replayResult.Output["confidence_level"],
		replayResult.Output["_replay_unconsumed_calls"])
}

func TestReplay_FailsForMissingRun(t *testing.T) {
	pl := planner.NewLogAnalysisPlanner()
	agentReg := agent.NewRegistry()
	runRepo := newMemRunRepo()
	toolCallRepo := newMemToolCallRepo()

	engine := orchestrator.NewEngine(pl, agentReg, nil, nil, runRepo, nil, nil)
	engine.SetToolCallReader(toolCallRepo)

	_, err := engine.Replay(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for missing run")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected 'not found' error, got: %v", err)
	}
}

func TestReplay_FailsWithoutRepos(t *testing.T) {
	pl := planner.NewLogAnalysisPlanner()
	agentReg := agent.NewRegistry()

	engine := orchestrator.NewEngine(pl, agentReg, nil, nil, nil, nil, nil)

	_, err := engine.Replay(context.Background(), "any-run")
	if err == nil {
		t.Fatal("expected error without repositories")
	}
	if !strings.Contains(err.Error(), "repository is required") {
		t.Fatalf("expected repository required error, got: %v", err)
	}
}

// --- HTTP replay handler test -----------------------------------------------

func TestReplayHTTP_PostReplay(t *testing.T) {
	// Set up a real pipeline, execute, then replay via HTTP.
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "app.log"),
		[]byte("2024-03-01 10:00:00 ERROR connection refused\n"), 0644)

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
	engine.SetToolCallReader(toolCallRepo)

	// Execute original.
	_, err := engine.Execute(context.Background(), orchestrator.ExecutionRequest{
		RunID:  "http-replay-run",
		TaskID: "http-replay-test",
		Input:  map[string]any{"directory": "."},
	})
	if err != nil {
		t.Fatalf("original run error: %v", err)
	}

	// HTTP test server.
	rh := handlers.NewRunHandler(engine, runRepo, stepRepo, toolCallRepo)
	router := api.NewRouter(rh)
	ts := httptest.NewServer(router)
	defer ts.Close()

	// POST /runs/http-replay-run/replay
	resp, err := http.Post(ts.URL+"/runs/http-replay-run/replay", "application/json", nil)
	if err != nil {
		t.Fatalf("HTTP error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body handlers.ReplayResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if body.Status != "succeeded" {
		t.Fatalf("expected status=succeeded, got %s", body.Status)
	}
	if body.SourceRunID != "http-replay-run" {
		t.Fatalf("expected source_run_id=http-replay-run, got %s", body.SourceRunID)
	}

	t.Logf("HTTP replay: status=%s source=%s", body.Status, body.SourceRunID)
}
