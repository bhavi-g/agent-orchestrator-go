package orchestrator

type ExecutionResult struct {
	RunID  string
	Output map[string]any
	Err    error
}
