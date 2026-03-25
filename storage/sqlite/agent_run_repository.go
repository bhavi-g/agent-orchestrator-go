package sqlite

import (
	"agent-orchestrator/agent"
	"database/sql"
)

type AgentRunSQLiteRepository struct {
	DB *sql.DB
}

func NewAgentRunRepository(db *sql.DB) *AgentRunSQLiteRepository {
	return &AgentRunSQLiteRepository{DB: db}
}

func (r *AgentRunSQLiteRepository) Create(run *agent.AgentRun) error {
	_, err := r.DB.Exec(`
		INSERT INTO agent_runs (
			run_id, goal, status, current_step_index,
			prompt_version, model_version, max_steps,
			created_at, completed_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		run.RunID,
		run.Goal,
		run.Status,
		run.CurrentStepIndex,
		run.PromptVersion,
		run.ModelVersion,
		run.MaxSteps,
		run.CreatedAt,
		run.CompletedAt,
	)

	return err
}

func (r *AgentRunSQLiteRepository) GetByID(runID string) (*agent.AgentRun, error) {
	row := r.DB.QueryRow(`
		SELECT run_id, goal, status, current_step_index,
		       prompt_version, model_version, max_steps,
		       created_at, completed_at
		FROM agent_runs
		WHERE run_id = ?
	`, runID)

	var run agent.AgentRun
	err := row.Scan(
		&run.RunID,
		&run.Goal,
		&run.Status,
		&run.CurrentStepIndex,
		&run.PromptVersion,
		&run.ModelVersion,
		&run.MaxSteps,
		&run.CreatedAt,
		&run.CompletedAt,
	)

	if err != nil {
		return nil, err
	} 

	return &run, nil
}

func (r *AgentRunSQLiteRepository) Update(run *agent.AgentRun) error {
	_, err := r.DB.Exec(`
		UPDATE agent_runs
		SET status = ?, current_step_index = ?, completed_at = ?
		WHERE run_id = ?
	`,
		run.Status,
		run.CurrentStepIndex,
		run.CompletedAt,
		run.RunID,
	)

	return err
}

func (r *AgentRunSQLiteRepository) List() ([]*agent.AgentRun, error) {
	rows, err := r.DB.Query(`
		SELECT run_id, goal, status, current_step_index,
		       prompt_version, model_version, max_steps,
		       created_at, completed_at
		FROM agent_runs
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var runs []*agent.AgentRun
	for rows.Next() {
		var run agent.AgentRun
		if err := rows.Scan(
			&run.RunID,
			&run.Goal,
			&run.Status,
			&run.CurrentStepIndex,
			&run.PromptVersion,
			&run.ModelVersion,
			&run.MaxSteps,
			&run.CreatedAt,
			&run.CompletedAt,
		); err != nil {
			return nil, err
		}
		runs = append(runs, &run)
	}
	return runs, nil
}
