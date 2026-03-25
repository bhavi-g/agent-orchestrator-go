package sqlite

func agentRunTableSQL() string {
	return `
	CREATE TABLE IF NOT EXISTS agent_runs (
		run_id TEXT PRIMARY KEY,
		goal TEXT NOT NULL,
		status TEXT NOT NULL,
		current_step_index INTEGER NOT NULL,
		prompt_version TEXT NOT NULL,
		model_version TEXT NOT NULL,
		max_steps INTEGER NOT NULL,
		created_at DATETIME NOT NULL,
		completed_at DATETIME
	);
	`
}

func agentStepTableSQL() string {
	return `
	CREATE TABLE IF NOT EXISTS agent_steps (
		step_id TEXT PRIMARY KEY,
		run_id TEXT NOT NULL,
		type TEXT NOT NULL,
		status TEXT NOT NULL,
		input TEXT NOT NULL,
		output TEXT NOT NULL,
		started_at DATETIME NOT NULL,
		finished_at DATETIME
	);
	`
}

func toolCallTableSQL() string {
	return `
	CREATE TABLE IF NOT EXISTS tool_calls (
		tool_call_id TEXT PRIMARY KEY,
		run_id TEXT NOT NULL,
		step_id TEXT NOT NULL,
		tool_name TEXT NOT NULL,
		input TEXT NOT NULL,
		output TEXT NOT NULL,
		status TEXT NOT NULL,
		started_at DATETIME NOT NULL,
		finished_at DATETIME
	);
	`
}
