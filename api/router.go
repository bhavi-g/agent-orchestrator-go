package api

import (
	"net/http"
	"strings"

	"agent-orchestrator/api/handlers"
)

// NewRouter builds an http.ServeMux with all API routes.
func NewRouter(rh *handlers.RunHandler, mh ...*handlers.MetricsHandler) *http.ServeMux {
	mux := http.NewServeMux()

	// POST /runs
	// GET  /runs/<id>
	// GET  /runs/<id>/steps
	mux.HandleFunc("/runs", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/runs" || r.URL.Path == "/runs/" {
			if r.Method == http.MethodGet {
				rh.ListRuns(w, r)
			} else {
				rh.CreateRun(w, r)
			}
			return
		}
		http.NotFound(w, r)
	})

	mux.HandleFunc("/runs/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/replay") {
			rh.ReplayRun(w, r)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/steps") {
			rh.GetRunSteps(w, r)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/tools") {
			rh.GetRunToolCalls(w, r)
			return
		}
		rh.GetRun(w, r)
	})

	// Health check
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	// Metrics: GET /metrics, GET /metrics/<runID>
	if len(mh) > 0 && mh[0] != nil {
		mux.HandleFunc("/metrics", mh[0].GetMetrics)
		mux.HandleFunc("/metrics/", mh[0].GetMetrics)
	}

	return mux
}
