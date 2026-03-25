package tests

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"agent-orchestrator/agent"
	"agent-orchestrator/orchestrator"
	"agent-orchestrator/planner"
	"agent-orchestrator/tools"
)

// ---- File-system tool unit tests ------------------------------------------

func TestReadFileTool(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "test.log"), []byte("line1\nline2\n"), 0644)

	tool := tools.NewReadFileTool(dir)

	t.Run("reads existing file", func(t *testing.T) {
		res, err := tool.Execute(context.Background(), tools.Call{
			ToolName: "fs.read_file",
			Args:     map[string]any{"path": "test.log"},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		content, _ := res.Data["content"].(string)
		if content != "line1\nline2\n" {
			t.Fatalf("wrong content: %q", content)
		}
		if res.Data["filename"] != "test.log" {
			t.Fatalf("wrong filename: %v", res.Data["filename"])
		}
	})

	t.Run("rejects missing file", func(t *testing.T) {
		_, err := tool.Execute(context.Background(), tools.Call{
			ToolName: "fs.read_file",
			Args:     map[string]any{"path": "nonexistent.log"},
		})
		if err == nil {
			t.Fatal("expected error for missing file")
		}
	})

	t.Run("rejects path traversal", func(t *testing.T) {
		_, err := tool.Execute(context.Background(), tools.Call{
			ToolName: "fs.read_file",
			Args:     map[string]any{"path": "../../../etc/passwd"},
		})
		if err == nil {
			t.Fatal("expected error for path traversal")
		}
	})

	t.Run("allows absolute path to existing file", func(t *testing.T) {
		// Absolute paths are now allowed — this is a local dev tool and the user
		// already has filesystem access. The old rejection was overly restrictive.
		tmpFile, _ := os.CreateTemp("", "mcp-test-*.txt")
		tmpFile.WriteString("test content")
		tmpFile.Close()
		defer os.Remove(tmpFile.Name())

		res, err := tool.Execute(context.Background(), tools.Call{
			ToolName: "fs.read_file",
			Args:     map[string]any{"path": tmpFile.Name()},
		})
		if err != nil {
			t.Fatalf("expected absolute path to work, got error: %v", err)
		}
		if res.Data["content"] != "test content" {
			t.Fatalf("unexpected content: %v", res.Data["content"])
		}
	})
}

func TestListDirTool(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "app.log"), []byte("log data"), 0644)
	os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("notes"), 0644)
	os.Mkdir(filepath.Join(dir, "subdir"), 0755)

	tool := tools.NewListDirTool(dir)

	t.Run("lists directory contents", func(t *testing.T) {
		res, err := tool.Execute(context.Background(), tools.Call{
			ToolName: "fs.list_dir",
			Args:     map[string]any{"path": "."},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		count, _ := res.Data["count"].(int)
		if count != 3 {
			t.Fatalf("expected 3 entries, got %d", count)
		}
	})

	t.Run("rejects traversal", func(t *testing.T) {
		_, err := tool.Execute(context.Background(), tools.Call{
			ToolName: "fs.list_dir",
			Args:     map[string]any{"path": "../../"},
		})
		if err == nil {
			t.Fatal("expected error for path traversal")
		}
	})
}

func TestGrepFileTool(t *testing.T) {
	dir := t.TempDir()
	content := `2024-01-01 INFO  Starting application
2024-01-01 ERROR Failed to connect to database
2024-01-01 WARN  Retrying connection
2024-01-01 ERROR Timeout exceeded
2024-01-01 INFO  Shutdown complete
`
	os.WriteFile(filepath.Join(dir, "app.log"), []byte(content), 0644)

	tool := tools.NewGrepFileTool(dir)

	t.Run("finds matching lines", func(t *testing.T) {
		res, err := tool.Execute(context.Background(), tools.Call{
			ToolName: "fs.grep_file",
			Args:     map[string]any{"path": "app.log", "keyword": "ERROR"},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		mc, _ := res.Data["match_count"].(int)
		if mc != 2 {
			t.Fatalf("expected 2 matches, got %d", mc)
		}
	})

	t.Run("case-insensitive search", func(t *testing.T) {
		res, err := tool.Execute(context.Background(), tools.Call{
			ToolName: "fs.grep_file",
			Args:     map[string]any{"path": "app.log", "keyword": "error"},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		mc, _ := res.Data["match_count"].(int)
		if mc != 2 {
			t.Fatalf("expected 2 case-insensitive matches, got %d", mc)
		}
	})

	t.Run("no matches returns zero", func(t *testing.T) {
		res, err := tool.Execute(context.Background(), tools.Call{
			ToolName: "fs.grep_file",
			Args:     map[string]any{"path": "app.log", "keyword": "CRITICAL"},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		mc, _ := res.Data["match_count"].(int)
		if mc != 0 {
			t.Fatalf("expected 0 matches, got %d", mc)
		}
	})
}

// ---- LogReaderAgent integration test --------------------------------------

func TestLogReaderAgent_EndToEnd(t *testing.T) {
	// Create a temp directory with sample log files.
	dir := t.TempDir()

	appLog := `2024-03-01 10:00:01 INFO  Application started
2024-03-01 10:00:02 ERROR Database connection failed: timeout
2024-03-01 10:00:03 WARN  Falling back to read-only mode
2024-03-01 10:00:04 ERROR Request handler panic: nil pointer dereference
2024-03-01 10:00:05 INFO  Graceful shutdown initiated
`
	accessLog := `192.168.1.1 - - [01/Mar/2024:10:00:01] "GET / HTTP/1.1" 200 1234
192.168.1.2 - - [01/Mar/2024:10:00:02] "POST /api/data HTTP/1.1" 500 56
192.168.1.3 - - [01/Mar/2024:10:00:03] "GET /health HTTP/1.1" 200 2
`
	os.WriteFile(filepath.Join(dir, "app.log"), []byte(appLog), 0644)
	os.WriteFile(filepath.Join(dir, "access.log"), []byte(accessLog), 0644)
	os.WriteFile(filepath.Join(dir, "readme.md"), []byte("not a log"), 0644) // should be skipped

	// Build tool registry rooted at the temp dir
	toolRegistry := tools.NewRegistry()
	toolRegistry.Register(tools.NewReadFileTool(dir))
	toolRegistry.Register(tools.NewListDirTool(dir))
	toolRegistry.Register(tools.NewGrepFileTool(dir))
	toolExecutor := tools.NewRegistryExecutor(toolRegistry)

	// Register agent
	agentReg := agent.NewRegistry()
	agentReg.Register("agent.log_reader", agent.NewLogReaderAgent())

	// Build a planner that runs the log_reader agent
	pl := &singleStepPlanner{agentID: "agent.log_reader"}

	engine := orchestrator.NewEngine(pl, agentReg, toolExecutor, nil, nil, nil, nil)

	result, err := engine.Execute(context.Background(), orchestrator.ExecutionRequest{
		RunID:  "run-log-reader",
		TaskID: "analyze-logs",
		Input: map[string]any{
			"directory": ".",
		},
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.Status != orchestrator.StatusSucceeded {
		t.Fatalf("expected success, got %v (err=%v)", result.Status, result.Err)
	}

	output := result.Output
	filesScanned, _ := output["files_scanned"].(int)
	totalMatches, _ := output["total_matches"].(int)
	summary, _ := output["summary"].(string)

	if filesScanned != 2 {
		t.Errorf("expected 2 files scanned (app.log, access.log), got %d", filesScanned)
	}
	if totalMatches == 0 {
		t.Error("expected at least some matches, got 0")
	}
	if summary == "" {
		t.Error("expected non-empty summary")
	}

	t.Logf("Files scanned: %d", filesScanned)
	t.Logf("Total matches: %d", totalMatches)
	t.Logf("Summary:\n%s", summary)
}

// singleStepPlanner creates a plan with one step.
type singleStepPlanner struct {
	agentID string
}

func (p *singleStepPlanner) CreatePlan(_ context.Context, taskID string) (*planner.Plan, error) {
	return &planner.Plan{
		TaskID: taskID,
		Steps:  []planner.PlanStep{{AgentID: p.agentID}},
	}, nil
}

func TestLogReaderAgent_EmptyDir(t *testing.T) {
	dir := t.TempDir() // empty directory

	toolRegistry := tools.NewRegistry()
	toolRegistry.Register(tools.NewReadFileTool(dir))
	toolRegistry.Register(tools.NewListDirTool(dir))
	toolRegistry.Register(tools.NewGrepFileTool(dir))
	toolExecutor := tools.NewRegistryExecutor(toolRegistry)

	agentReg := agent.NewRegistry()
	agentReg.Register("agent.log_reader", agent.NewLogReaderAgent())

	pl := &singleStepPlanner{agentID: "agent.log_reader"}
	engine := orchestrator.NewEngine(pl, agentReg, toolExecutor, nil, nil, nil, nil)

	result, err := engine.Execute(context.Background(), orchestrator.ExecutionRequest{
		RunID:  "run-empty-dir",
		TaskID: "analyze-empty",
		Input:  map[string]any{"directory": "."},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != orchestrator.StatusSucceeded {
		t.Fatalf("expected success even with empty dir, got %v", result.Status)
	}

	totalMatches, _ := result.Output["total_matches"].(int)
	if totalMatches != 0 {
		t.Fatalf("expected 0 matches in empty dir, got %d", totalMatches)
	}
}

func TestLogReaderAgent_CustomKeywords(t *testing.T) {
	dir := t.TempDir()
	content := `line1: SUCCESS
line2: FAILURE detected
line3: ok
line4: FAILURE again
`
	os.WriteFile(filepath.Join(dir, "custom.log"), []byte(content), 0644)

	toolRegistry := tools.NewRegistry()
	toolRegistry.Register(tools.NewReadFileTool(dir))
	toolRegistry.Register(tools.NewListDirTool(dir))
	toolRegistry.Register(tools.NewGrepFileTool(dir))
	toolExecutor := tools.NewRegistryExecutor(toolRegistry)

	agentReg := agent.NewRegistry()
	agentReg.Register("agent.log_reader", agent.NewLogReaderAgent())

	pl := &singleStepPlanner{agentID: "agent.log_reader"}
	engine := orchestrator.NewEngine(pl, agentReg, toolExecutor, nil, nil, nil, nil)

	result, err := engine.Execute(context.Background(), orchestrator.ExecutionRequest{
		RunID:  "run-custom-kw",
		TaskID: "analyze-custom",
		Input: map[string]any{
			"directory": ".",
			"keywords":  "FAILURE",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != orchestrator.StatusSucceeded {
		t.Fatalf("expected success, got %v", result.Status)
	}

	totalMatches, _ := result.Output["total_matches"].(int)
	if totalMatches != 2 {
		t.Fatalf("expected 2 FAILURE matches, got %d", totalMatches)
	}
}
