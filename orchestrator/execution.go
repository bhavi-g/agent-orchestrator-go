package orchestrator

import "time"

type ExecutionStatus string

const (
	StatusPending   ExecutionStatus = "pending"
	StatusRunning   ExecutionStatus = "running"
	StatusSucceeded ExecutionStatus = "succeeded"
	StatusFailed    ExecutionStatus = "failed"
)

type ExecutionRequest struct {
	RunID    string
	AgentID string
	TaskID  string
	Input   map[string]any
}

type ExecutionState struct {
	RunID     string
	Status    ExecutionStatus
	StartedAt time.Time
	EndedAt   *time.Time
	Error     string
}
