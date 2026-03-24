package repair

import (
	"agent-orchestrator/failure"
	"fmt"
	"strings"
)

// InputCorrector modifies input based on failure analysis
type InputCorrector struct{}

// NewInputCorrector creates a new input corrector
func NewInputCorrector() *InputCorrector {
	return &InputCorrector{}
}

// CorrectInput analyzes a failure and suggests corrected input
func (ic *InputCorrector) CorrectInput(f *failure.FailureEvent, originalInput map[string]any) map[string]any {
	// Create a copy of the original input
	correctedInput := make(map[string]any)
	for k, v := range originalInput {
		correctedInput[k] = v
	}

	// Analyze the error message for hints
	errorLower := strings.ToLower(f.Error)

	// Common corrections based on error patterns
	switch {
	case strings.Contains(errorLower, "missing") && strings.Contains(errorLower, "required"):
		// Try to add missing fields
		correctedInput = ic.addMissingFields(correctedInput, errorLower)
	
	case strings.Contains(errorLower, "invalid format"):
		// Try to fix format issues
		correctedInput = ic.fixFormatIssues(correctedInput, errorLower)
	
	case strings.Contains(errorLower, "too large") || strings.Contains(errorLower, "too long"):
		// Try to reduce size
		correctedInput = ic.reduceSizes(correctedInput)
	
	case strings.Contains(errorLower, "empty") || strings.Contains(errorLower, "null"):
		// Try to provide default values
		correctedInput = ic.addDefaults(correctedInput)
	}

	// Add metadata about correction
	correctedInput["__corrected__"] = true
	correctedInput["__correction_reason__"] = f.Error

	return correctedInput
}

// addMissingFields attempts to add missing required fields
func (ic *InputCorrector) addMissingFields(input map[string]any, errorMsg string) map[string]any {
	// Extract field name from error if possible
	// e.g., "missing required field: username"
	if strings.Contains(errorMsg, "field:") {
		parts := strings.Split(errorMsg, "field:")
		if len(parts) > 1 {
			fieldName := strings.TrimSpace(strings.Split(parts[1], " ")[0])
			fieldName = strings.Trim(fieldName, "\"'")
			if fieldName != "" && input[fieldName] == nil {
				input[fieldName] = "" // Add empty string as placeholder
			}
		}
	}
	return input
}

// fixFormatIssues attempts to fix common format problems
func (ic *InputCorrector) fixFormatIssues(input map[string]any, errorMsg string) map[string]any {
	// Example: convert strings to numbers if needed
	for k, v := range input {
		if _, ok := v.(string); ok {
			// Check if error mentions this field should be numeric
			if strings.Contains(errorMsg, k) && strings.Contains(errorMsg, "number") {
				// Try to provide a default number
				input[k] = 0
			}
		}
	}
	return input
}

// reduceSizes attempts to reduce data size
func (ic *InputCorrector) reduceSizes(input map[string]any) map[string]any {
	for k, v := range input {
		if str, ok := v.(string); ok && len(str) > 100 {
			// Truncate long strings
			input[k] = str[:100]
		}
		if arr, ok := v.([]any); ok && len(arr) > 10 {
			// Reduce array size
			input[k] = arr[:10]
		}
	}
	return input
}

// addDefaults adds default values for nil/empty fields
func (ic *InputCorrector) addDefaults(input map[string]any) map[string]any {
	// Common field defaults
	defaults := map[string]any{
		"limit":  10,
		"offset": 0,
		"page":   1,
		"id":     "default",
	}

	for k, defaultVal := range defaults {
		if input[k] == nil || input[k] == "" {
			input[k] = defaultVal
		}
	}
	return input
}

// CanCorrect determines if input correction is likely to help
func (ic *InputCorrector) CanCorrect(f *failure.FailureEvent) bool {
	if f.Type != failure.ValidationFailure {
		return false // Only validation failures can be corrected via input
	}

	errorLower := strings.ToLower(f.Error)
	
	// Check for correctable error patterns
	correctablePatterns := []string{
		"missing", "required", "invalid format", "empty", "null",
		"too large", "too long", "invalid type",
	}

	for _, pattern := range correctablePatterns {
		if strings.Contains(errorLower, pattern) {
			return true
		}
	}

	return false
}

// InputModificationStrategy creates repair plans that modify input
type InputModificationStrategy struct {
	corrector *InputCorrector
	baseInput map[string]any
}

// NewInputModificationStrategy creates a strategy that modifies input
func NewInputModificationStrategy(baseInput map[string]any) *InputModificationStrategy {
	return &InputModificationStrategy{
		corrector: NewInputCorrector(),
		baseInput: baseInput,
	}
}

// CreateRepairPlan creates a plan that modifies input
func (s *InputModificationStrategy) CreateRepairPlan(f *failure.FailureEvent) (*RepairPlan, error) {
	plan := NewRepairPlan(f)

	if s.corrector.CanCorrect(f) {
		correctedInput := s.corrector.CorrectInput(f, s.baseInput)
		
		plan.AddAction(RepairAction{
			Type:          ModifyInput,
			StepIndex:     f.StepIndex,
			AgentID:       f.AgentID,
			ModifiedInput: correctedInput,
		})
		
		plan.WithReasoning(fmt.Sprintf("correcting input based on error: %s", f.Error))
	} else {
		// Cannot correct, abort
		plan.AddAction(RepairAction{
			Type:      Abort,
			StepIndex: f.StepIndex,
		})
		plan.WithReasoning("input correction not applicable")
	}

	return plan, nil
}
