package failure

import (
	"errors"
	"strings"
)

// Classifier analyzes errors and determines failure types
type Classifier struct{}

// NewClassifier creates a new failure classifier
func NewClassifier() *Classifier {
	return &Classifier{}
}

// Classify determines the type of failure based on the error
func (c *Classifier) Classify(err error) FailureType {
	if err == nil {
		return UnknownFailure
	}

	errMsg := err.Error()
	errMsgLower := strings.ToLower(errMsg)

	// Check for validation errors
	if strings.Contains(errMsgLower, "validation") ||
		strings.Contains(errMsgLower, "invalid") ||
		strings.Contains(errMsgLower, "constraint") {
		return ValidationFailure
	}

	// Check for tool errors
	if strings.Contains(errMsgLower, "tool") ||
		strings.Contains(errMsgLower, "execute") ||
		strings.Contains(errMsgLower, "timeout") {
		return ToolFailure
	}

	// Check for agent errors
	if strings.Contains(errMsgLower, "agent") ||
		strings.Contains(errMsgLower, "runtime") {
		return AgentFailure
	}

	// Default to unknown
	return UnknownFailure
}

// ClassifyWithContext provides additional context for classification
func (c *Classifier) ClassifyWithContext(err error, context string) FailureType {
	if err == nil {
		return UnknownFailure
	}

	// First try standard classification
	failureType := c.Classify(err)
	if failureType != UnknownFailure {
		return failureType
	}

	// Use context to help classify
	contextLower := strings.ToLower(context)
	if strings.Contains(contextLower, "validation") {
		return ValidationFailure
	}
	if strings.Contains(contextLower, "tool") {
		return ToolFailure
	}
	if strings.Contains(contextLower, "agent") {
		return AgentFailure
	}

	return UnknownFailure
}

// Common error types for easy creation
var (
	ErrValidation = errors.New("validation error")
	ErrTool       = errors.New("tool execution error")
	ErrAgent      = errors.New("agent execution error")
)
