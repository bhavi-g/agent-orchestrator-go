package storage

import "agent-orchestrator/agent"

// ToolCallRepository persists individual tool invocations.
type ToolCallRepository interface {
	Create(tc *agent.ToolCall) error
	GetByRunID(runID string) ([]*agent.ToolCall, error)
}
