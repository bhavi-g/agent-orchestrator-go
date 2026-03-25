package tests

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"agent-orchestrator/agent"
	"agent-orchestrator/api"
	"agent-orchestrator/api/handlers"
	"agent-orchestrator/orchestrator"
	"agent-orchestrator/planner"
	"agent-orchestrator/tools"
)

// --- In-memory tool call recorder/repo for tests ---

type memToolCallRepo struct {
	records []tools.ToolCallRecord
}

func newMemToolCallRepo() *memToolCallRepo {
	return &memToolCallRepo{}
}

// Record implements tools.ToolCallRecorder.
func (r *memToolCallRepo) Record(rec tools.ToolCallRecord) error {
	r.records = append(r.records, rec)
	return nil
}

// GetByRunID implements storage.ToolCallRepository (for API handler tests).
func (r *memToolCallRepo) GetByRunID(runID string) ([]*agent.ToolCall, error) {
	var result []*agent.ToolCall
	for _, rec := range r.records {
		if rec.RunID == runID {
			status := agent.ToolCallFailed
			if rec.Succeeded {
				status = agent.ToolCallSucceeded
			}
			finished := rec.FinishedAt
			result = append(result, &agent.ToolCall{
				ToolCallID: rec.ToolCallID,
				RunID:      rec.RunID,
				StepID:     rec.StepID,
				ToolName:   rec.ToolName,
				Input:      rec.Input,
				Output:     rec.Output,
				Status:     status,
				StartedAt:  rec.StartedAt,
				FinishedAt: &finished,
			})
		}
	}
	return result, nil
}

func (r *memToolCallRepo) Create(tc *agent.ToolCall) error {
	succeeded := tc.Status == agent.ToolCallSucceeded
	finished := time.Now()
	if tc.FinishedAt != nil {
		finished = *tc.FinishedAt
	}
	r.records = append(r.records, tools.ToolCallRecord{
		ToolCallID: tc.ToolCallID,
		RunID:      tc.RunID,
		StepID:     tc.StepID,
		ToolName:   tc.ToolName,
		Input:      tc.Input,
		Output:     tc.Output,
		Succeeded:  succeeded,
		StartedAt:  tc.StartedAt,
		FinishedAt: finished,
	})
	return nil
}

// --- Test tool ---

type addTool struct{}

func (t *addTool) Name() string { return "math.add" }
func (t *addTool) Spec() tools.Spec {
	return tools.Spec{
		Name:        "math.add",
		Description: "adds two numbers",
		Input: map[string]tools.Field{
			"a": {Type: "number", Required: true},
			"b": {Type: "number", Required: true},
		},
	}
}
func (t *addTool) Execute(_ context.Context, call tools.Call) (tools.Result, error) {
	a, _ := call.Args["a"].(float64)
	b, _ := call.Args["b"].(float64)
	return tools.Result{
		ToolName: "math.add",
		Data:     map[string]any{"sum": a + b},
	}, nil
}

// --- PersistingExecutor unit tests ---

func TestPersistingExecutor_RecordsSuccess(t *testing.T) {
	reg := tools.NewRegistry()
	reg.Register(&addTool{})
	inner := tools.NewRegistryExecutor(reg)

	recorder := newMemToolCallRepo()
	pe := tools.NewPersistingExecutor(inner, recorder, "run-1", "step-0")

	result, err := pe.Execute(context.Background(), tools.Call{
		ToolName: "math.add",
		Args:     map[string]any{"a": float64(2), "b": float64(3)},
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Data["sum"] != float64(5) {
		t.Errorf("expected sum=5, got %v", result.Data["sum"])
	}

	if len(recorder.records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(recorder.records))
	}

	rec := recorder.records[0]
	if rec.RunID != "run-1" {
		t.Errorf("expected run-1, got %s", rec.RunID)
	}
	if rec.StepID != "step-0" {
		t.Errorf("expected step-0, got %s", rec.StepID)
	}
	if rec.ToolName != "math.add" {
		t.Errorf("expected math.add, got %s", rec.ToolName)
	}
	if !rec.Succeeded {
		t.Error("expected Succeeded=true")
	}
	if rec.ToolCallID == "" {
		t.Error("expected non-empty ToolCallID")
	}
}

func TestPersistingExecutor_RecordsFailure(t *testing.T) {
	reg := tools.NewRegistry()
	inner := tools.NewRegistryExecutor(reg)

	recorder := newMemToolCallRepo()
	pe := tools.NewPersistingExecutor(inner, recorder, "run-2", "step-0")

	_, err := pe.Execute(context.Background(), tools.Call{
		ToolName: "nonexistent",
		Args:     map[string]any{},
	})

	if err == nil {
		t.Fatal("expected error for missing tool")
	}

	if len(recorder.records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(recorder.records))
	}

	rec := recorder.records[0]
	if rec.Succeeded {
		t.Error("expected Succeeded=false for failed tool")
	}
	if rec.Output == "" {
		t.Error("expected error message in output")
	}
}

func TestPersistingExecutor_MultipleCalls(t *testing.T) {
	reg := tools.NewRegistry()
	reg.Register(&addTool{})
	inner := tools.NewRegistryExecutor(reg)

	recorder := newMemToolCallRepo()
	pe := tools.NewPersistingExecutor(inner, recorder, "run-3", "step-0")

	pe.Execute(context.Background(), tools.Call{ToolName: "math.add", Args: map[string]any{"a": 1.0, "b": 2.0}})
	pe.Execute(context.Background(), tools.Call{ToolName: "math.add", Args: map[string]any{"a": 3.0, "b": 4.0}})

	if len(recorder.records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(recorder.records))
	}

	// Verify sequential IDs
	if recorder.records[0].ToolCallID == recorder.records[1].ToolCallID {
		t.Error("expected unique ToolCallIDs")
	}
}

// --- Engine integration: tool calls persisted during execution ---

// toolUsingAgent always calls a tool then returns
type toolUsingAgent struct{}

func (a *toolUsingAgent) Run(ctx context.Context, input map[string]any) (*agent.Result, error) {
	return &agent.Result{Output: map[string]any{"done": true}}, nil
}

func (a *toolUsingAgent) RunWithContext(ctx context.Context, rt agent.RuntimeContext) (*agent.Result, error) {
	if rt.Tools != nil {
		res, err := rt.Tools.Execute(ctx, tools.Call{
			ToolName: "math.add",
			Args:     map[string]any{"a": 10.0, "b": 20.0},
		})
		if err != nil {
			return nil, err
		}
		return &agent.Result{Output: res.Data}, nil
	}
	return &agent.Result{Output: map[string]any{"no_tool": true}}, nil
}

func TestEngine_ToolCallPersistence(t *testing.T) {
	toolReg := tools.NewRegistry()
	toolReg.Register(&addTool{})
	toolExec := tools.NewRegistryExecutor(toolReg)

	agentReg := agent.NewRegistry()
	agentReg.Register("agent.echo", &toolUsingAgent{})

	pl := planner.NewDummyPlanner()
	recorder := newMemToolCallRepo()

	engine := orchestrator.NewEngine(pl, agentReg, toolExec, nil, nil, nil, nil)
	engine.SetToolCallRepository(recorder)

	result, err := engine.Execute(context.Background(), orchestrator.ExecutionRequest{
		RunID:  "run-tool-1",
		TaskID: "task.test",
		Input:  map[string]any{"x": 1},
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != orchestrator.StatusSucceeded {
		t.Fatalf("expected succeeded, got %v", result.Status)
	}

	// Verify tool call was recorded
	if len(recorder.records) != 1 {
		t.Fatalf("expected 1 tool call recorded, got %d", len(recorder.records))
	}

	rec := recorder.records[0]
	if rec.RunID != "run-tool-1" {
		t.Errorf("expected run-tool-1, got %s", rec.RunID)
	}
	if rec.ToolName != "math.add" {
		t.Errorf("expected math.add, got %s", rec.ToolName)
	}
	if !rec.Succeeded {
		t.Error("expected Succeeded=true")
	}
}

func TestEngine_NoRecorder_ToolCallsStillWork(t *testing.T) {
	toolReg := tools.NewRegistry()
	toolReg.Register(&addTool{})
	toolExec := tools.NewRegistryExecutor(toolReg)

	agentReg := agent.NewRegistry()
	agentReg.Register("agent.echo", &toolUsingAgent{})

	pl := planner.NewDummyPlanner()

	// No recorder set
	engine := orchestrator.NewEngine(pl, agentReg, toolExec, nil, nil, nil, nil)

	result, err := engine.Execute(context.Background(), orchestrator.ExecutionRequest{
		RunID:  "run-no-rec",
		TaskID: "task.test",
		Input:  map[string]any{"x": 1},
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != orchestrator.StatusSucceeded {
		t.Fatalf("expected succeeded, got %v", result.Status)
	}
}

// --- API endpoint tests ---

func TestAPI_GetRunToolCalls_Success(t *testing.T) {
	runRepo := newMemRunRepo()
	stepRepo := newMemStepRepo()
	toolCallRepo := newMemToolCallRepo()

	now := time.Now()
	runRepo.Create(&agent.AgentRun{
		RunID: "run-tc-api", Goal: "task", Status: agent.AgentRunCompleted, CreatedAt: now,
	})

	// Seed tool calls
	toolCallRepo.Record(tools.ToolCallRecord{
		ToolCallID: "tc-1", RunID: "run-tc-api", StepID: "step-0",
		ToolName: "math.add", Input: `{"a":1,"b":2}`, Output: `{"sum":3}`,
		Succeeded: true, StartedAt: now, FinishedAt: now,
	})
	toolCallRepo.Record(tools.ToolCallRecord{
		ToolCallID: "tc-2", RunID: "run-tc-api", StepID: "step-0",
		ToolName: "math.add", Input: `{"a":3,"b":4}`, Output: `{"sum":7}`,
		Succeeded: true, StartedAt: now, FinishedAt: now,
	})

	pl := planner.NewDummyPlanner()
	reg := agent.NewRegistry()
	reg.Register("agent.echo", agent.NewEchoAgent())
	toolExec := tools.NewRegistryExecutor(tools.NewRegistry())
	engine := orchestrator.NewEngine(pl, reg, toolExec, nil, runRepo, stepRepo, nil)

	rh := handlers.NewRunHandler(engine, runRepo, stepRepo, toolCallRepo)
	router := api.NewRouter(rh)
	srv := httptest.NewServer(router)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/runs/run-tc-api/tools")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var calls []handlers.ToolCallResponse
	if err := json.NewDecoder(resp.Body).Decode(&calls); err != nil {
		t.Fatalf("decode error: %v", err)
	}

	if len(calls) != 2 {
		t.Fatalf("expected 2 tool calls, got %d", len(calls))
	}
	if calls[0].ToolCallID != "tc-1" {
		t.Errorf("expected tc-1, got %s", calls[0].ToolCallID)
	}
	if calls[0].ToolName != "math.add" {
		t.Errorf("expected math.add, got %s", calls[0].ToolName)
	}
}

func TestAPI_GetRunToolCalls_RunNotFound(t *testing.T) {
	runRepo := newMemRunRepo()
	stepRepo := newMemStepRepo()
	toolCallRepo := newMemToolCallRepo()

	pl := planner.NewDummyPlanner()
	reg := agent.NewRegistry()
	reg.Register("agent.echo", agent.NewEchoAgent())
	toolExec := tools.NewRegistryExecutor(tools.NewRegistry())
	engine := orchestrator.NewEngine(pl, reg, toolExec, nil, runRepo, stepRepo, nil)

	rh := handlers.NewRunHandler(engine, runRepo, stepRepo, toolCallRepo)
	router := api.NewRouter(rh)
	srv := httptest.NewServer(router)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/runs/nonexistent/tools")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestAPI_GetRunToolCalls_NoRepo(t *testing.T) {
	runRepo := newMemRunRepo()
	stepRepo := newMemStepRepo()

	runRepo.Create(&agent.AgentRun{
		RunID: "run-no-tc", Goal: "task", Status: agent.AgentRunCompleted, CreatedAt: time.Now(),
	})

	pl := planner.NewDummyPlanner()
	reg := agent.NewRegistry()
	reg.Register("agent.echo", agent.NewEchoAgent())
	toolExec := tools.NewRegistryExecutor(tools.NewRegistry())
	engine := orchestrator.NewEngine(pl, reg, toolExec, nil, runRepo, stepRepo, nil)

	// nil toolCallRepo
	rh := handlers.NewRunHandler(engine, runRepo, stepRepo, nil)
	router := api.NewRouter(rh)
	srv := httptest.NewServer(router)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/runs/run-no-tc/tools")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var calls []handlers.ToolCallResponse
	json.NewDecoder(resp.Body).Decode(&calls)
	if len(calls) != 0 {
		t.Errorf("expected 0 calls with nil repo, got %d", len(calls))
	}
}
