package orchestrator

type ExecutionResult struct {
	RunID  string
	Status ExecutionStatus
	Output map[string]any
	Err    error
}
