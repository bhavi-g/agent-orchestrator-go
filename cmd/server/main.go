package main

import (
	"fmt"
	"log"

	"agent-orchestrator/agent"
	"agent-orchestrator/config"
	"agent-orchestrator/orchestrator"
	"agent-orchestrator/planner"
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

	// SQLite-backed run/step repositories
	runRepo := sqlite.NewAgentRunRepository(repo.DB)
	stepRepo := sqlite.NewAgentStepRepository(repo.DB)

	// ---- Orchestrator wiring ONLY (no execution) ----
	pl := planner.NewDummyPlanner()

	agentRegistry := agent.NewRegistry()
	agentRegistry.Register("agent.echo", agent.NewEchoAgent())

	// Tools wiring
	toolRegistry := tools.NewRegistry()
	toolExecutor := tools.NewRegistryExecutor(toolRegistry)

	// Engine wiring
	engine := orchestrator.NewEngine(
		pl,
		agentRegistry,
		toolExecutor,
		nil,
		runRepo,
		stepRepo,
	)
	_ = engine // intentionally unused for now

	fmt.Println("Agent Orchestrator starting...")
	fmt.Println("SQLite storage initialized")
}
