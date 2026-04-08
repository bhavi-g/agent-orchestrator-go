package main

import (
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"agent-orchestrator/agent"
	"agent-orchestrator/api"
	"agent-orchestrator/api/handlers"
	"agent-orchestrator/config"
	"agent-orchestrator/demo"
	"agent-orchestrator/orchestrator"
	"agent-orchestrator/planner"
	"agent-orchestrator/repair"
	"agent-orchestrator/storage/sqlite"
	"agent-orchestrator/tools"
	"agent-orchestrator/web"
)

func main() {
	cfgPath := os.Getenv("CONFIG_PATH")
	if cfgPath == "" {
		cfgPath = "config/local.yaml"
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	// Allow overriding sqlite path via env for deployed environments
	if dbPath := os.Getenv("SQLITE_PATH"); dbPath != "" {
		cfg.Storage.SQLitePath = dbPath
	}

	repo := sqlite.New(cfg.Storage.SQLitePath)
	if err := repo.Open(); err != nil {
		log.Fatalf("failed to open database: %v", err)
	}
	defer repo.Close()

	// SQLite-backed run/step/tool-call repositories
	runRepo := sqlite.NewAgentRunRepository(repo.DB)
	stepRepo := sqlite.NewAgentStepRepository(repo.DB)
	toolCallRepo := sqlite.NewToolCallRepository(repo.DB)

	// ---- Orchestrator wiring ----
	pl := planner.NewLogAnalysisPlanner()

	agentRegistry := agent.NewRegistry()
	agentRegistry.Register("agent.echo", agent.NewEchoAgent())
	agentRegistry.Register("agent.log_reader", agent.NewLogReaderAgent())
	agentRegistry.Register("agent.log_analyzer", agent.NewLogAnalyzerAgent())

	// Tools wiring — root directory defaults to current working directory
	toolRegistry := tools.NewRegistry()
	toolRegistry.Register(tools.NewReadFileTool("."))
	toolRegistry.Register(tools.NewListDirTool("."))
	toolRegistry.Register(tools.NewGrepFileTool("."))
	toolExecutor := tools.NewRegistryExecutor(toolRegistry)

	// Repair engine wiring (optional)
	repairStrategy := repair.NewSimpleRetryStrategy()
	repairEngine := repair.NewEngine(repairStrategy, 3)

	// Engine wiring with report + grounding validators
	validator := orchestrator.NewCompositeValidator(
		orchestrator.NewReportValidator(),
		orchestrator.NewGroundingValidator(),
	)
	engine := orchestrator.NewEngine(
		pl,
		agentRegistry,
		toolExecutor,
		validator,
		runRepo,
		stepRepo,
		repairEngine,
	)
	engine.SetToolCallRepository(toolCallRepo)
	engine.SetToolCallReader(toolCallRepo)

	// ---- HTTP Server ----
	// Upload directory (for deployed environments)
	uploadDir := os.Getenv("UPLOAD_DIR")
	if uploadDir == "" {
		uploadDir = filepath.Join(os.TempDir(), "agent-orchestrator-uploads")
	}
	_ = os.MkdirAll(uploadDir, 0o755)

	// Extract embedded demo logs on startup
	demoDir := handlers.GetDemoDir(uploadDir)
	extractDemoLogs(demoDir)

	runHandler := handlers.NewRunHandler(engine, runRepo, stepRepo, toolCallRepo)
	uploadHandler := handlers.NewUploadHandler(uploadDir)
	metricsEval := orchestrator.NewMetricsEvaluator(runRepo, stepRepo, toolCallRepo)
	metricsHandler := handlers.NewMetricsHandler(metricsEval, runRepo)
	router := api.NewRouter(runHandler, uploadHandler, metricsHandler)

	// Serve embedded web dashboard at /
	staticFS, _ := fs.Sub(web.Static, "static")
	router.Handle("/", http.FileServer(http.FS(staticFS)))

	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	log.Printf("Agent Orchestrator listening on %s", addr)
	if err := http.ListenAndServe(addr, router); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

// extractDemoLogs writes the embedded demo log files to disk so the agent can read them.
func extractDemoLogs(dir string) {
	_ = os.MkdirAll(dir, 0o755)
	entries, err := demo.Logs.ReadDir("logs")
	if err != nil {
		log.Printf("warning: could not read embedded demo logs: %v", err)
		return
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		data, err := demo.Logs.ReadFile("logs/" + e.Name())
		if err != nil {
			continue
		}
		_ = os.WriteFile(filepath.Join(dir, e.Name()), data, 0o644)
	}
	log.Printf("demo logs extracted to %s", dir)
}
