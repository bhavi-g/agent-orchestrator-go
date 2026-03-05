package orchestrator

type Validator interface {
	Validate(stepID string, output map[string]any) error
}
