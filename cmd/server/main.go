package main

import (
	"fmt"
	"log"

	"agent-orchestrator/agent"
	"agent-orchestrator/config"
	"agent-orchestrator/orchestrator"
	"agent-orchestrator/planner"
	"agent-orchestrator/storage/sqlite"
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

	// ---- Orchestrator wiring ONLY (no execution) ----

	pl := planner.NewDummyPlanner()

	agentRegistry := agent.NewRegistry()
	agentRegistry.Register("agent.echo", agent.NewEchoAgent())

	engine := orchestrator.NewEngine(pl, agentRegistry)
	_ = engine // intentionally unused for now

	fmt.Println("Agent Orchestrator starting...")
	fmt.Println("SQLite storage initialized")
}
