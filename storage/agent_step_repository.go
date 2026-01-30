package storage

import "agent-orchestrator/agent"

type AgentStepRepository interface {
	Create(step *agent.AgentStep) error
	GetByRunID(runID string) ([]*agent.AgentStep, error)
}
