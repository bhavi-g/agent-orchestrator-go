package handlers

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"agent-orchestrator/agent"
	"agent-orchestrator/orchestrator"
	"agent-orchestrator/storage"

	"github.com/google/uuid"
)

// RunHandler handles HTTP requests for agent runs.
type RunHandler struct {
	engine   *orchestrator.Engine
	runs     storage.AgentRunRepository
	steps    storage.AgentStepRepository
}

// NewRunHandler creates a handler wired to the engine and repositories.
func NewRunHandler(
	engine *orchestrator.Engine,
	runs storage.AgentRunRepository,
	steps storage.AgentStepRepository,
) *RunHandler {
	return &RunHandler{engine: engine, runs: runs, steps: steps}
}

// --- Request / Response types ------------------------------------------------

// CreateRunRequest is the JSON body for POST /runs.
type CreateRunRequest struct {
	TaskID string         `json:"task_id"`
	Input  map[string]any `json:"input,omitempty"`
}

// RunResponse is the JSON representation of an agent run.
type RunResponse struct {
	RunID            string     `json:"run_id"`
	Goal             string     `json:"goal"`
	Status           string     `json:"status"`
	CurrentStepIndex int        `json:"current_step_index"`
	MaxSteps         int        `json:"max_steps"`
	CreatedAt        time.Time  `json:"created_at"`
	CompletedAt      *time.Time `json:"completed_at,omitempty"`
}

// StepResponse is the JSON representation of an agent step.
type StepResponse struct {
	StepID     string     `json:"step_id"`
	RunID      string     `json:"run_id"`
	Type       string     `json:"type"`
	Status     string     `json:"status"`
	Input      string     `json:"input"`
	Output     string     `json:"output"`
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
}

// ErrorResponse is a standard error envelope.
type ErrorResponse struct {
	Error string `json:"error"`
}

// --- Handlers ----------------------------------------------------------------

// CreateRun handles POST /runs.
func (h *RunHandler) CreateRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req CreateRunRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if req.TaskID == "" {
		writeError(w, http.StatusBadRequest, "task_id is required")
		return
	}

	runID := uuid.New().String()

	// Execute asynchronously — the run is created inside Execute,
	// so we kick it off in a goroutine and return the run ID immediately.
	go func() {
		_, _ = h.engine.Execute(r.Context(), orchestrator.ExecutionRequest{
			RunID:  runID,
			TaskID: req.TaskID,
			Input:  req.Input,
		})
	}()

	// Return the initial run state.
	writeJSON(w, http.StatusAccepted, RunResponse{
		RunID:     runID,
		Goal:      req.TaskID,
		Status:    string(agent.AgentRunCreated),
		CreatedAt: time.Now().UTC(),
	})
}

// GetRun handles GET /runs/:id.
func (h *RunHandler) GetRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	runID := extractRunID(r.URL.Path, "/runs/")
	if runID == "" {
		writeError(w, http.StatusBadRequest, "run_id is required")
		return
	}

	// Strip trailing /steps if present (defensive)
	if strings.Contains(runID, "/") {
		writeError(w, http.StatusBadRequest, "invalid run_id")
		return
	}

	run, err := h.runs.GetByID(runID)
	if err != nil {
		writeError(w, http.StatusNotFound, "run not found")
		return
	}

	writeJSON(w, http.StatusOK, toRunResponse(run))
}

// GetRunSteps handles GET /runs/:id/steps.
func (h *RunHandler) GetRunSteps(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// Extract run ID from /runs/<id>/steps
	path := strings.TrimPrefix(r.URL.Path, "/runs/")
	path = strings.TrimSuffix(path, "/steps")
	runID := path
	if runID == "" || strings.Contains(runID, "/") {
		writeError(w, http.StatusBadRequest, "invalid run_id")
		return
	}

	// Verify run exists
	if _, err := h.runs.GetByID(runID); err != nil {
		writeError(w, http.StatusNotFound, "run not found")
		return
	}

	steps, err := h.steps.GetByRunID(runID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to retrieve steps")
		return
	}

	resp := make([]StepResponse, len(steps))
	for i, s := range steps {
		resp[i] = toStepResponse(s)
	}
	writeJSON(w, http.StatusOK, resp)
}

// --- Helpers -----------------------------------------------------------------

func extractRunID(path, prefix string) string {
	s := strings.TrimPrefix(path, prefix)
	// Remove anything after the first slash (e.g. /steps)
	if idx := strings.Index(s, "/"); idx >= 0 {
		s = s[:idx]
	}
	return s
}

func toRunResponse(run *agent.AgentRun) RunResponse {
	return RunResponse{
		RunID:            run.RunID,
		Goal:             run.Goal,
		Status:           string(run.Status),
		CurrentStepIndex: run.CurrentStepIndex,
		MaxSteps:         run.MaxSteps,
		CreatedAt:        run.CreatedAt,
		CompletedAt:      run.CompletedAt,
	}
}

func toStepResponse(s *agent.AgentStep) StepResponse {
	return StepResponse{
		StepID:     s.StepID,
		RunID:      s.RunID,
		Type:       string(s.Type),
		Status:     string(s.Status),
		Input:      s.Input,
		Output:     s.Output,
		StartedAt:  s.StartedAt,
		FinishedAt: s.FinishedAt,
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, ErrorResponse{Error: msg})
}
