package agent

import "time"

type AgentStepType string

const (
	StepPlan       AgentStepType = "PLAN"
	StepToolCall   AgentStepType = "TOOL_CALL"
	StepValidation AgentStepType = "VALIDATION"
	StepRepair     AgentStepType = "REPAIR"
)

type AgentStepStatus string

const (
	StepPending   AgentStepStatus = "PENDING"
	StepSucceeded AgentStepStatus = "SUCCEEDED"
	StepFailed    AgentStepStatus = "FAILED"
)

type AgentStep struct {
	StepID     string
	RunID      string
	Type       AgentStepType
	Status     AgentStepStatus
	Input      string
	Output     string
	StartedAt  time.Time
	FinishedAt *time.Time
}

