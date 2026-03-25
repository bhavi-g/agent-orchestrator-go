package handlers

import (
	"net/http"
	"strings"

	"agent-orchestrator/orchestrator"
)

// MetricsHandler handles HTTP requests for evaluation metrics.
type MetricsHandler struct {
	evaluator *orchestrator.MetricsEvaluator
	runs      orchestrator.RunRepository
}

// NewMetricsHandler creates a handler wired to the metrics evaluator.
func NewMetricsHandler(
	evaluator *orchestrator.MetricsEvaluator,
	runs orchestrator.RunRepository,
) *MetricsHandler {
	return &MetricsHandler{evaluator: evaluator, runs: runs}
}

// GetMetrics handles GET /metrics and GET /metrics/:runID.
func (h *MetricsHandler) GetMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// Extract optional run ID: /metrics/<runID>
	path := strings.TrimPrefix(r.URL.Path, "/metrics")
	path = strings.TrimPrefix(path, "/")
	runID := path

	if runID != "" {
		// Single-run metrics.
		metrics, err := h.evaluator.Evaluate(runID)
		if err != nil {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, metrics)
		return
	}

	// Aggregate metrics across all runs.
	allRuns, err := h.runs.List()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list runs")
		return
	}
	if len(allRuns) == 0 {
		writeJSON(w, http.StatusOK, &orchestrator.Metrics{})
		return
	}

	metrics, err := h.evaluator.EvaluateAll(allRuns)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, metrics)
}
