package failure

import (
	"errors"
	"testing"
	"time"
)

func TestNewFailureEvent(t *testing.T) {
	err := errors.New("test error")
	event := NewFailureEvent("run-123", 2, "agent-abc", ValidationFailure, err)

	if event.RunID != "run-123" {
		t.Errorf("expected RunID 'run-123', got '%s'", event.RunID)
	}
	if event.StepIndex != 2 {
		t.Errorf("expected StepIndex 2, got %d", event.StepIndex)
	}
	if event.AgentID != "agent-abc" {
		t.Errorf("expected AgentID 'agent-abc', got '%s'", event.AgentID)
	}
	if event.Type != ValidationFailure {
		t.Errorf("expected Type ValidationFailure, got %s", event.Type)
	}
	if event.Error != "test error" {
		t.Errorf("expected Error 'test error', got '%s'", event.Error)
	}
	if event.Attempt != 1 {
		t.Errorf("expected Attempt 1, got %d", event.Attempt)
	}
	if event.Output == nil {
		t.Error("expected Output to be initialized")
	}
	if time.Since(event.Timestamp) > time.Second {
		t.Error("expected Timestamp to be recent")
	}
}

func TestFailureEvent_WithOutput(t *testing.T) {
	event := NewFailureEvent("run-123", 0, "agent-1", ToolFailure, nil)
	output := map[string]any{"result": "failed"}

	event = event.WithOutput(output)

	if event.Output["result"] != "failed" {
		t.Error("expected output to be set")
	}
}

func TestFailureEvent_WithAttempt(t *testing.T) {
	event := NewFailureEvent("run-123", 0, "agent-1", AgentFailure, nil)

	event = event.WithAttempt(3)

	if event.Attempt != 3 {
		t.Errorf("expected Attempt 3, got %d", event.Attempt)
	}
}

func TestFailureEvent_IsRetryable(t *testing.T) {
	tests := []struct {
		name       string
		failureType FailureType
		expected   bool
	}{
		{"ValidationFailure is retryable", ValidationFailure, true},
		{"ToolFailure is retryable", ToolFailure, true},
		{"AgentFailure is retryable", AgentFailure, true},
		{"UnknownFailure is not retryable", UnknownFailure, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := NewFailureEvent("run-1", 0, "agent-1", tt.failureType, nil)
			if event.IsRetryable() != tt.expected {
				t.Errorf("expected IsRetryable to be %v for %s", tt.expected, tt.failureType)
			}
		})
	}
}

func TestClassifier_Classify(t *testing.T) {
	classifier := NewClassifier()

	tests := []struct {
		name     string
		err      error
		expected FailureType
	}{
		{
			name:     "validation error",
			err:      errors.New("validation failed: invalid input"),
			expected: ValidationFailure,
		},
		{
			name:     "tool error",
			err:      errors.New("tool execution timeout"),
			expected: ToolFailure,
		},
		{
			name:     "agent error",
			err:      errors.New("agent runtime error"),
			expected: AgentFailure,
		},
		{
			name:     "unknown error",
			err:      errors.New("something went wrong"),
			expected: UnknownFailure,
		},
		{
			name:     "nil error",
			err:      nil,
			expected: UnknownFailure,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := classifier.Classify(tt.err)
			if result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestClassifier_ClassifyWithContext(t *testing.T) {
	classifier := NewClassifier()

	// Generic error but context helps classify
	err := errors.New("operation failed")

	t.Run("context indicates validation", func(t *testing.T) {
		result := classifier.ClassifyWithContext(err, "validation stage")
		if result != ValidationFailure {
			t.Errorf("expected ValidationFailure, got %s", result)
		}
	})

	t.Run("context indicates tool", func(t *testing.T) {
		result := classifier.ClassifyWithContext(err, "tool execution")
		if result != ToolFailure {
			t.Errorf("expected ToolFailure, got %s", result)
		}
	})

	t.Run("context indicates agent", func(t *testing.T) {
		result := classifier.ClassifyWithContext(err, "agent runtime")
		if result != AgentFailure {
			t.Errorf("expected AgentFailure, got %s", result)
		}
	})
}
