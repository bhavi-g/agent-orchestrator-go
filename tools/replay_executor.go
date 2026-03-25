package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
)

// ToolCallReader loads persisted tool call records for a given run.
// This is the read-side counterpart to ToolCallRecorder.
type ToolCallReader interface {
	GetByRunID(runID string) ([]ToolCallRecord, error)
}

// ReplayExecutor replays stored tool call outputs instead of executing real
// tools. Calls are matched by (tool name, JSON-serialised input). If a call
// is not found in the stored records the executor returns an error so that
// divergence from the original run is surfaced immediately.
type ReplayExecutor struct {
	mu      sync.Mutex
	entries map[string][]replayEntry // key = toolName
}

type replayEntry struct {
	inputJSON string
	output    map[string]any
	consumed  bool
}

// NewReplayExecutor builds an executor pre-loaded with persisted records.
func NewReplayExecutor(records []ToolCallRecord) (*ReplayExecutor, error) {
	entries := make(map[string][]replayEntry)

	for _, rec := range records {
		if !rec.Succeeded {
			continue // skip failed calls – they shouldn't be replayed
		}

		var output map[string]any
		if err := json.Unmarshal([]byte(rec.Output), &output); err != nil {
			return nil, fmt.Errorf("replay: corrupt output for call %s: %w", rec.ToolCallID, err)
		}

		entries[rec.ToolName] = append(entries[rec.ToolName], replayEntry{
			inputJSON: rec.Input,
			output:    output,
		})
	}

	return &ReplayExecutor{entries: entries}, nil
}

// Execute returns the stored output for the matching tool call.
// Matching is exact: tool name + canonical JSON input.
func (r *ReplayExecutor) Execute(_ context.Context, call Call) (Result, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	callInput, _ := json.Marshal(call.Args)
	callInputStr := string(callInput)

	candidates := r.entries[call.ToolName]
	for i := range candidates {
		if candidates[i].consumed {
			continue
		}
		if candidates[i].inputJSON == callInputStr {
			candidates[i].consumed = true
			return Result{
				ToolName: call.ToolName,
				Data:     candidates[i].output,
			}, nil
		}
	}

	return Result{}, fmt.Errorf("replay: no stored output for tool %q with input %s", call.ToolName, callInputStr)
}

// Unconsumed returns the number of stored records that were never matched
// during the replay. Useful for detecting divergence.
func (r *ReplayExecutor) Unconsumed() int {
	r.mu.Lock()
	defer r.mu.Unlock()

	n := 0
	for _, entries := range r.entries {
		for _, e := range entries {
			if !e.consumed {
				n++
			}
		}
	}
	return n
}
