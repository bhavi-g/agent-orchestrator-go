package agent

import "time"

type AgentRunStatus string

const (
	AgentRunCreated   AgentRunStatus = "CREATED"
	AgentRunRunning   AgentRunStatus = "RUNNING"
	AgentRunFailed    AgentRunStatus = "FAILED"
	AgentRunCompleted AgentRunStatus = "COMPLETED"
)

type AgentRun struct {
	RunID            string
	Goal             string
	Status           AgentRunStatus
	CurrentStepIndex int
	PromptVersion    string
	ModelVersion     string
	MaxSteps         int
	CreatedAt        time.Time
	CompletedAt      *time.Time
}
