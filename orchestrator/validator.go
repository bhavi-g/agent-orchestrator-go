package orchestrator

type Validator interface {
	Validate(stepID string, output map[string]any) error
}

// CompositeValidator runs multiple validators in order and returns
// the first error encountered.
type CompositeValidator struct {
	validators []Validator
}

func NewCompositeValidator(validators ...Validator) *CompositeValidator {
	return &CompositeValidator{validators: validators}
}

func (c *CompositeValidator) Validate(stepID string, output map[string]any) error {
	for _, v := range c.validators {
		if err := v.Validate(stepID, output); err != nil {
			return err
		}
	}
	return nil
}
