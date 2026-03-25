package orchestrator

import "fmt"

// ReportValidator validates that the final output of the log analysis agent
// conforms to the Golden Path structured report schema:
//
//   - error_summary        (string, non-empty)
//   - suspected_root_cause (string, non-empty)
//   - supporting_evidence  ([]map, at least 0 items but must be present)
//   - confidence_level     (string, one of "Low", "Medium", "High")
//   - suggested_next_steps ([]string or []any, non-empty)
//
// It only validates the "agent.log_analyzer" step; other steps pass through.
type ReportValidator struct{}

func NewReportValidator() *ReportValidator {
	return &ReportValidator{}
}

func (v *ReportValidator) Validate(stepID string, output map[string]any) error {
	// Only validate the analyzer step.
	if stepID != "agent.log_analyzer" {
		return nil
	}

	// Required string fields.
	for _, field := range []string{"error_summary", "suspected_root_cause"} {
		val, ok := output[field].(string)
		if !ok || val == "" {
			return fmt.Errorf("report validation: %q is required and must be a non-empty string", field)
		}
	}

	// confidence_level must be one of the allowed values.
	conf, ok := output["confidence_level"].(string)
	if !ok || conf == "" {
		return fmt.Errorf("report validation: \"confidence_level\" is required")
	}
	switch conf {
	case "Low", "Medium", "High":
		// ok
	default:
		return fmt.Errorf("report validation: \"confidence_level\" must be Low, Medium, or High, got %q", conf)
	}

	// supporting_evidence must be present (array).
	if _, ok := output["supporting_evidence"]; !ok {
		return fmt.Errorf("report validation: \"supporting_evidence\" is required")
	}

	// suggested_next_steps must be present and non-empty.
	switch ns := output["suggested_next_steps"].(type) {
	case []string:
		if len(ns) == 0 {
			return fmt.Errorf("report validation: \"suggested_next_steps\" must not be empty")
		}
	case []any:
		if len(ns) == 0 {
			return fmt.Errorf("report validation: \"suggested_next_steps\" must not be empty")
		}
	default:
		return fmt.Errorf("report validation: \"suggested_next_steps\" is required and must be an array")
	}

	return nil
}
