package main

import (
	"fmt"
	"log"
	"net/http"

	"agent-orchestrator/agent"
	"agent-orchestrator/api"
	"agent-orchestrator/api/handlers"
	"agent-orchestrator/config"
	"agent-orchestrator/orchestrator"
	"agent-orchestrator/planner"
	"agent-orchestrator/repair"
	"agent-orchestrator/storage/sqlite"
	"agent-orchestrator/tools"
)

func main() {
	cfg, err := config.Load("config/local.yaml")
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
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

	// ---- HTTP Server ----
	runHandler := handlers.NewRunHandler(engine, runRepo, stepRepo, toolCallRepo)
	router := api.NewRouter(runHandler)

	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	log.Printf("Agent Orchestrator listening on %s", addr)
	if err := http.ListenAndServe(addr, router); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
