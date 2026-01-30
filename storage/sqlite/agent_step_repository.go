package sqlite

import (
	"agent-orchestrator/agent"
	"database/sql"
)

type AgentStepSQLiteRepository struct {
	DB *sql.DB
}

func NewAgentStepRepository(db *sql.DB) *AgentStepSQLiteRepository {
	return &AgentStepSQLiteRepository{DB: db}
}

func (r *AgentStepSQLiteRepository) Create(step *agent.AgentStep) error {
	_, err := r.DB.Exec(`
		INSERT INTO agent_steps (
			step_id, run_id, type, status,
			input, output, started_at, finished_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`,
		step.StepID,
		step.RunID,
		step.Type,
		step.Status,
		step.Input,
		step.Output,
		step.StartedAt,
		step.FinishedAt,
	)

	return err
}

func (r *AgentStepSQLiteRepository) GetByRunID(runID string) ([]*agent.AgentStep, error) {
	rows, err := r.DB.Query(`
		SELECT step_id, run_id, type, status,
		       input, output, started_at, finished_at
		FROM agent_steps
		WHERE run_id = ?
		ORDER BY started_at ASC
	`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var steps []*agent.AgentStep
	for rows.Next() {
		var step agent.AgentStep
		if err := rows.Scan(
			&step.StepID,
			&step.RunID,
			&step.Type,
			&step.Status,
			&step.Input,
			&step.Output,
			&step.StartedAt,
			&step.FinishedAt,
		); err != nil {
			return nil, err
		}
		steps = append(steps, &step)
	}

	return steps, nil
}
