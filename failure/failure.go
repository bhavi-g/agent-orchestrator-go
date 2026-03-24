package failure

import "time"

// FailureType categorizes the type of failure that occurred
type FailureType string

const (
	// ValidationFailure indicates output validation failed
	ValidationFailure FailureType = "validation_failure"
	// ToolFailure indicates a tool execution failed
	ToolFailure FailureType = "tool_failure"
	// AgentFailure indicates the agent itself failed
	AgentFailure FailureType = "agent_failure"
	// UnknownFailure indicates an unclassified error
	UnknownFailure FailureType = "unknown_failure"
)

// FailureEvent represents a detailed failure that occurred during execution
type FailureEvent struct {
	// RunID identifies the execution run
	RunID string
	// StepIndex identifies which step in the plan failed (0-based)
	StepIndex int
	// AgentID identifies which agent was executing
	AgentID string
	// Type classifies the failure
	Type FailureType
	// Error is the error message
	Error string
	// Output contains the agent/tool output at time of failure (if any)
	Output map[string]any
	// Timestamp when the failure occurred
	Timestamp time.Time
	// Attempt tracks which attempt this was (1-based)
	Attempt int
}

// NewFailureEvent creates a new failure event with the current timestamp
func NewFailureEvent(runID string, stepIndex int, agentID string, failureType FailureType, err error) *FailureEvent {
	errorMsg := ""
	if err != nil {
		errorMsg = err.Error()
	}

	return &FailureEvent{
		RunID:     runID,
		StepIndex: stepIndex,
		AgentID:   agentID,
		Type:      failureType,
		Error:     errorMsg,
		Output:    make(map[string]any),
		Timestamp: time.Now(),
		Attempt:   1,
	}
}

// WithOutput adds output data to the failure event
func (f *FailureEvent) WithOutput(output map[string]any) *FailureEvent {
	f.Output = output
	return f
}

// WithAttempt sets the attempt number
func (f *FailureEvent) WithAttempt(attempt int) *FailureEvent {
	f.Attempt = attempt
	return f
}

// IsRetryable determines if the failure can be retried
func (f *FailureEvent) IsRetryable() bool {
	switch f.Type {
	case ToolFailure, AgentFailure:
		// Tool and agent failures are generally retryable
		return true
	case ValidationFailure:
		// Validation failures might be retryable with modified input
		return true
	case UnknownFailure:
		// Unknown failures should not be automatically retried
		return false
	default:
		return false
	}
}
