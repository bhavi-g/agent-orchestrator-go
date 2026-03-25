package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// ToolCallRecord is a persistence-friendly snapshot of a single tool invocation.
// It lives in the tools package to avoid import cycles with agent/.
type ToolCallRecord struct {
	ToolCallID string
	RunID      string
	StepID     string
	ToolName   string
	Input      string
	Output     string
	Succeeded  bool
	StartedAt  time.Time
	FinishedAt time.Time
}

// ToolCallRecorder persists tool call records.
type ToolCallRecorder interface {
	Record(rec ToolCallRecord) error
}

// PersistingExecutor wraps an Executor and records every tool call.
type PersistingExecutor struct {
	inner    Executor
	recorder ToolCallRecorder
	runID    string
	stepID   string
	seq      int
}

// NewPersistingExecutor decorates inner with tool-call persistence.
func NewPersistingExecutor(inner Executor, recorder ToolCallRecorder, runID, stepID string) *PersistingExecutor {
	return &PersistingExecutor{
		inner:    inner,
		recorder: recorder,
		runID:    runID,
		stepID:   stepID,
	}
}

func (e *PersistingExecutor) Execute(ctx context.Context, call Call) (Result, error) {
	e.seq++
	callID := fmt.Sprintf("%s-%s-tc-%d", e.runID, e.stepID, e.seq)

	inputJSON, _ := json.Marshal(call.Args)
	start := time.Now()

	result, execErr := e.inner.Execute(ctx, call)

	now := time.Now()
	rec := ToolCallRecord{
		ToolCallID: callID,
		RunID:      e.runID,
		StepID:     e.stepID,
		ToolName:   call.ToolName,
		Input:      string(inputJSON),
		StartedAt:  start,
		FinishedAt: now,
	}

	if execErr != nil {
		rec.Succeeded = false
		rec.Output = execErr.Error()
	} else {
		rec.Succeeded = true
		outputJSON, _ := json.Marshal(result.Data)
		rec.Output = string(outputJSON)
	}

	// Best-effort persistence — don't fail the tool call if recording fails.
	if e.recorder != nil {
		_ = e.recorder.Record(rec)
	}

	return result, execErr
}
