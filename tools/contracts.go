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
// Keep this lightweight for now; richer schemas can be added later.
type Spec struct {
	Name        string            // must match Tool.Name()
	Description string
	Input       map[string]Field  // field name -> field description
	Output      map[string]Field  // field name -> field description
}

type Field struct {
	Type        string // e.g. "string", "number", "boolean", "object"
	Description string
	Required    bool
}

// Call is a single tool invocation request.
type Call struct {
	ToolName string         // required
	Args     map[string]any // required; validated by tool (and optionally by a validator layer later)
	// TraceID etc can be added later if telemetry needs it, but avoid now.
}

// Result is a single tool invocation response.
type Result struct {
	ToolName string
	Data     map[string]any
}

// Common, stable errors for deterministic behavior and testability.
var (
	ErrToolNotFound = errors.New("tool not found")
	ErrInvalidArgs  = errors.New("invalid tool args")
	ErrToolFailed   = errors.New("tool execution failed")
)

// Wrap helpers so callers can use errors.Is(err, ErrInvalidArgs), etc.
func InvalidArgsf(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidArgs, fmt.Sprintf(format, a...))
}

func ToolFailedf(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrToolFailed, fmt.Sprintf(format, a...))
}
