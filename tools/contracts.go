package tools

import (
	"context"
	"errors"
	"fmt"
)

// Tool is a stateless, synchronous capability callable by agents.
// Tools must be deterministic for a given ToolCall (no hidden randomness/time).
type Tool interface {
	// Name is the stable identifier used by plans/agents, e.g. "math.add".
	Name() string

	// Spec returns the tool's contract metadata (inputs/outputs) for validation/UI.
	Spec() Spec

	// Execute runs the tool with the provided call payload.
	Execute(ctx context.Context, call Call) (Result, error)
}

// Spec describes the contract of a tool.
type Spec struct {
	Name        string
	Description string
	Input       map[string]Field
	Output      map[string]Field
}

type Field struct {
	Type        string
	Description string
	Required    bool
}

// Call is a single tool invocation request.
type Call struct {
	ToolName string
	Args     map[string]any
}

// Result is a single tool invocation response.
type Result struct {
	ToolName string
	Data     map[string]any
}

var (
	ErrToolNotFound = errors.New("tool not found")
	ErrInvalidArgs  = errors.New("invalid tool args")
	ErrToolFailed   = errors.New("tool execution failed")
)

func InvalidArgsf(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidArgs, fmt.Sprintf(format, a...))
}

func ToolFailedf(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrToolFailed, fmt.Sprintf(format, a...))
}
