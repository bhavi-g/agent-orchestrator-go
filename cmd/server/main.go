package main

import (
	"fmt"
	"log"

	"agent-orchestrator/config"
)

func main() {
	cfg, err := config.Load("config/local.yaml")
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	fmt.Println("Agent Orchestrator starting...")
	fmt.Printf("Loaded config: %+v\n", cfg)
}
