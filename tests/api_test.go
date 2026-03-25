package tests

import (
	"bytes"
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

// --- In-memory repositories for testing ---

type memRunRepo struct {
	runs map[string]*agent.AgentRun
}

func newMemRunRepo() *memRunRepo {
	return &memRunRepo{runs: make(map[string]*agent.AgentRun)}
}

func (r *memRunRepo) Create(run *agent.AgentRun) error {
	r.runs[run.RunID] = run
	return nil
}

func (r *memRunRepo) GetByID(runID string) (*agent.AgentRun, error) {
	run, ok := r.runs[runID]
	if !ok {
		return nil, &notFoundError{runID}
	}
	return run, nil
}

func (r *memRunRepo) Update(run *agent.AgentRun) error {
	r.runs[run.RunID] = run
	return nil
}

func (r *memRunRepo) List() ([]*agent.AgentRun, error) {
	var result []*agent.AgentRun
	for _, run := range r.runs {
		result = append(result, run)
	}
	return result, nil
}

type notFoundError struct{ id string }

func (e *notFoundError) Error() string { return "not found: " + e.id }

type memStepRepo struct {
	steps []*agent.AgentStep
}

func newMemStepRepo() *memStepRepo {
	return &memStepRepo{}
}

func (r *memStepRepo) Create(step *agent.AgentStep) error {
	r.steps = append(r.steps, step)
	return nil
}

func (r *memStepRepo) GetByRunID(runID string) ([]*agent.AgentStep, error) {
	var result []*agent.AgentStep
	for _, s := range r.steps {
		if s.RunID == runID {
			result = append(result, s)
		}
	}
	return result, nil
}

// --- Helper to build a test server ---

func newTestServer() (*httptest.Server, *memRunRepo, *memStepRepo) {
	runRepo := newMemRunRepo()
	stepRepo := newMemStepRepo()

	pl := planner.NewDummyPlanner()
	reg := agent.NewRegistry()
	reg.Register("agent.echo", agent.NewEchoAgent())
	toolExec := tools.NewRegistryExecutor(tools.NewRegistry())

	engine := orchestrator.NewEngine(pl, reg, toolExec, nil, runRepo, stepRepo, nil)
	rh := handlers.NewRunHandler(engine, runRepo, stepRepo, nil)
	router := api.NewRouter(rh)

	return httptest.NewServer(router), runRepo, stepRepo
}

// --- Tests ---

func TestAPI_PostRuns_Success(t *testing.T) {
	srv, _, _ := newTestServer()
	defer srv.Close()

	body := `{"task_id": "task.test", "input": {"msg": "hello"}}`
	resp, err := http.Post(srv.URL+"/runs", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", resp.StatusCode)
	}

	var result handlers.RunResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode error: %v", err)
	}

	if result.RunID == "" {
		t.Error("expected non-empty run_id")
	}
	if result.Goal != "task.test" {
		t.Errorf("expected goal task.test, got %s", result.Goal)
	}
}

func TestAPI_PostRuns_MissingTaskID(t *testing.T) {
	srv, _, _ := newTestServer()
	defer srv.Close()

	body := `{"input": {"msg": "hello"}}`
	resp, err := http.Post(srv.URL+"/runs", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestAPI_PostRuns_InvalidJSON(t *testing.T) {
	srv, _, _ := newTestServer()
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/runs", "application/json", bytes.NewBufferString("not json"))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestAPI_GetRun_Success(t *testing.T) {
	srv, runRepo, _ := newTestServer()
	defer srv.Close()

	// Seed a run directly
	now := time.Now()
	runRepo.Create(&agent.AgentRun{
		RunID:     "run-123",
		Goal:      "task.demo",
		Status:    agent.AgentRunCompleted,
		MaxSteps:  2,
		CreatedAt: now,
	})

	resp, err := http.Get(srv.URL + "/runs/run-123")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result handlers.RunResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode error: %v", err)
	}

	if result.RunID != "run-123" {
		t.Errorf("expected run-123, got %s", result.RunID)
	}
	if result.Goal != "task.demo" {
		t.Errorf("expected task.demo, got %s", result.Goal)
	}
	if result.Status != string(agent.AgentRunCompleted) {
		t.Errorf("expected COMPLETED, got %s", result.Status)
	}
}

func TestAPI_GetRun_NotFound(t *testing.T) {
	srv, _, _ := newTestServer()
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/runs/nonexistent")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestAPI_GetRunSteps_Success(t *testing.T) {
	srv, runRepo, stepRepo := newTestServer()
	defer srv.Close()

	now := time.Now()
	runRepo.Create(&agent.AgentRun{
		RunID:     "run-456",
		Goal:      "task.steps",
		Status:    agent.AgentRunCompleted,
		CreatedAt: now,
	})

	finished := now.Add(time.Second)
	stepRepo.Create(&agent.AgentStep{
		StepID:     "step-1",
		RunID:      "run-456",
		Type:       agent.StepPlan,
		Status:     agent.StepSucceeded,
		Input:      "input-1",
		Output:     "output-1",
		StartedAt:  now,
		FinishedAt: &finished,
	})
	stepRepo.Create(&agent.AgentStep{
		StepID:    "step-2",
		RunID:     "run-456",
		Type:      agent.StepToolCall,
		Status:    agent.StepFailed,
		Input:     "input-2",
		Output:    "error",
		StartedAt: now,
	})

	resp, err := http.Get(srv.URL + "/runs/run-456/steps")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var steps []handlers.StepResponse
	if err := json.NewDecoder(resp.Body).Decode(&steps); err != nil {
		t.Fatalf("decode error: %v", err)
	}

	if len(steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(steps))
	}
	if steps[0].StepID != "step-1" {
		t.Errorf("expected step-1, got %s", steps[0].StepID)
	}
	if steps[1].Status != string(agent.StepFailed) {
		t.Errorf("expected FAILED, got %s", steps[1].Status)
	}
}

func TestAPI_GetRunSteps_RunNotFound(t *testing.T) {
	srv, _, _ := newTestServer()
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/runs/nonexistent/steps")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestAPI_GetRunSteps_EmptySteps(t *testing.T) {
	srv, runRepo, _ := newTestServer()
	defer srv.Close()

	runRepo.Create(&agent.AgentRun{
		RunID:     "run-empty",
		Goal:      "task.empty",
		Status:    agent.AgentRunCreated,
		CreatedAt: time.Now(),
	})

	resp, err := http.Get(srv.URL + "/runs/run-empty/steps")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var steps []handlers.StepResponse
	if err := json.NewDecoder(resp.Body).Decode(&steps); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if len(steps) != 0 {
		t.Errorf("expected 0 steps, got %d", len(steps))
	}
}

func TestAPI_Health(t *testing.T) {
	srv, _, _ := newTestServer()
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/health")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if result["status"] != "ok" {
		t.Errorf("expected ok, got %s", result["status"])
	}
}

func TestAPI_MethodNotAllowed(t *testing.T) {
	srv, runRepo, _ := newTestServer()
	defer srv.Close()

	// Seed a run so the path is valid
	runRepo.Create(&agent.AgentRun{
		RunID: "run-x", Goal: "t", Status: agent.AgentRunCreated, CreatedAt: time.Now(),
	})

	// DELETE on /runs/run-x should be rejected by GetRun handler
	req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/runs/run-x", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", resp.StatusCode)
	}
}
