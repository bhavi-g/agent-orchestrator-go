package sqlite

import (
	"agent-orchestrator/agent"
	"agent-orchestrator/tools"
	"database/sql"
)

type ToolCallSQLiteRepository struct {
	DB *sql.DB
}

func NewToolCallRepository(db *sql.DB) *ToolCallSQLiteRepository {
	return &ToolCallSQLiteRepository{DB: db}
}

// Record implements tools.ToolCallRecorder.
func (r *ToolCallSQLiteRepository) Record(rec tools.ToolCallRecord) error {
	status := agent.ToolCallFailed
	if rec.Succeeded {
		status = agent.ToolCallSucceeded
	}
	_, err := r.DB.Exec(`
		INSERT INTO tool_calls (
			tool_call_id, run_id, step_id, tool_name,
			input, output, status, started_at, finished_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		rec.ToolCallID,
		rec.RunID,
		rec.StepID,
		rec.ToolName,
		rec.Input,
		rec.Output,
		status,
		rec.StartedAt,
		rec.FinishedAt,
	)
	return err
}

// Create implements storage.ToolCallRepository.
func (r *ToolCallSQLiteRepository) Create(tc *agent.ToolCall) error {
	_, err := r.DB.Exec(`
		INSERT INTO tool_calls (
			tool_call_id, run_id, step_id, tool_name,
			input, output, status, started_at, finished_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		tc.ToolCallID,
		tc.RunID,
		tc.StepID,
		tc.ToolName,
		tc.Input,
		tc.Output,
		tc.Status,
		tc.StartedAt,
		tc.FinishedAt,
	)
	return err
}

func (r *ToolCallSQLiteRepository) GetByRunID(runID string) ([]*agent.ToolCall, error) {
	rows, err := r.DB.Query(`
		SELECT tool_call_id, run_id, step_id, tool_name,
		       input, output, status, started_at, finished_at
		FROM tool_calls
		WHERE run_id = ?
		ORDER BY started_at ASC
	`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var calls []*agent.ToolCall
	for rows.Next() {
		var tc agent.ToolCall
		if err := rows.Scan(
			&tc.ToolCallID,
			&tc.RunID,
			&tc.StepID,
			&tc.ToolName,
			&tc.Input,
			&tc.Output,
			&tc.Status,
			&tc.StartedAt,
			&tc.FinishedAt,
		); err != nil {
			return nil, err
		}
		calls = append(calls, &tc)
	}
	return calls, nil
}
