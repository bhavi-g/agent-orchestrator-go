package tools

import (
	"context"
	"encoding/json"
	"sync"
)

// CollectedOutput stores a snapshot of what a tool call returned.
type CollectedOutput struct {
	ToolName string
	Input    map[string]any
	Output   map[string]any
}

// ToolCallCollector wraps an Executor, forwarding all calls while keeping
// an in-memory log of successful outputs. It is created once per run so that
// validators can cross-reference agent claims against tool data from all steps.
type ToolCallCollector struct {
	inner   Executor
	mu      sync.Mutex
	outputs []CollectedOutput
}

// NewToolCallCollector decorates inner with in-memory output collection.
func NewToolCallCollector(inner Executor) *ToolCallCollector {
	return &ToolCallCollector{inner: inner}
}

func (c *ToolCallCollector) Execute(ctx context.Context, call Call) (Result, error) {
	result, err := c.inner.Execute(ctx, call)
	if err == nil {
		c.mu.Lock()
		c.outputs = append(c.outputs, CollectedOutput{
			ToolName: call.ToolName,
			Input:    call.Args,
			Output:   result.Data,
		})
		c.mu.Unlock()
	}
	return result, err
}

// Outputs returns a copy of all collected outputs so far.
func (c *ToolCallCollector) Outputs() []CollectedOutput {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]CollectedOutput, len(c.outputs))
	copy(out, c.outputs)
	return out
}

// AddOutput manually adds an output record. Useful for testing.
func (c *ToolCallCollector) AddOutput(o CollectedOutput) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.outputs = append(c.outputs, o)
}

// OutputsJSON returns the collected outputs serialized as JSON strings,
// useful for substring matching.
func (c *ToolCallCollector) OutputsJSON() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	result := make([]string, len(c.outputs))
	for i, o := range c.outputs {
		b, _ := json.Marshal(o.Output)
		result[i] = string(b)
	}
	return result
}
