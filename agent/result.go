package agent

// Result represents the output of an agent execution.
// It is intentionally simple and owned by the agent layer.
type Result struct {
	Output map[string]any
}
