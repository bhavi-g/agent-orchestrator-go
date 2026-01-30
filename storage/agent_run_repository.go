package storage

import "agent-orchestrator/agent"

type AgentRunRepository interface {
	Create(run *agent.AgentRun) error
	GetByID(runID string) (*agent.AgentRun, error)
	Update(run *agent.AgentRun) error
}

