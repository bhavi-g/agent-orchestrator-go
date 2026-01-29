package main

import (
	"fmt"
	"log"

	"agent-orchestrator/config"
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

	fmt.Println("Agent Orchestrator starting...")
	fmt.Println("SQLite storage initialized")
}
