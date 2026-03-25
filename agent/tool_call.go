package agent

import "time"

// ToolCall records a single tool invocation within a run.
type ToolCall struct {
	ToolCallID string
	RunID      string
	StepID     string
	ToolName   string
	Input      string // JSON-encoded args
	Output     string // JSON-encoded result or error
	Status     ToolCallStatus
	StartedAt  time.Time
	FinishedAt *time.Time
}

type ToolCallStatus string

const (
	ToolCallSucceeded ToolCallStatus = "SUCCEEDED"
	ToolCallFailed    ToolCallStatus = "FAILED"
)
