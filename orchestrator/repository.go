package orchestrator

import "agent-orchestrator/agent"

type RunRepository interface {
	Create(run *agent.AgentRun) error
	GetByID(runID string) (*agent.AgentRun, error)
	Update(run *agent.AgentRun) error
}

type StepRepository interface {
	Create(step *agent.AgentStep) error
	GetByRunID(runID string) ([]*agent.AgentStep, error)
}

type ToolCallRepository interface {
	Create(tc *agent.ToolCall) error
	GetByRunID(runID string) ([]*agent.ToolCall, error)
}
