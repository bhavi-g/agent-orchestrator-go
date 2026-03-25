// Agent Orchestrator — MCP Server
//
// Exposes log analysis as MCP tools so Claude Desktop, Cursor, and VS Code
// Copilot can call it directly from chat — no browser, no form, no terminal.
//
// ── Build ──────────────────────────────────────────────────────────────────
//
//	go build -o ~/bin/agent-orchestrator-mcp ./cmd/mcp
//
// ── Claude Desktop ──────────────────────────────────────────────────────────
// Edit ~/.claude/claude_desktop_config.json:
//
//	{
//	  "mcpServers": {
//	    "agent-orchestrator": {
//	      "command": "/Users/YOU/bin/agent-orchestrator-mcp"
//	    }
//	  }
//	}
//
// Restart Claude Desktop. Then just ask:
//   "Analyze the logs in /path/to/my/logs"
//
// ── Cursor ──────────────────────────────────────────────────────────────────
// Settings → MCP → Add → stdio → /Users/YOU/bin/agent-orchestrator-mcp
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"agent-orchestrator/agent"
	"agent-orchestrator/orchestrator"
	"agent-orchestrator/planner"
	"agent-orchestrator/repair"
	"agent-orchestrator/storage/sqlite"
	"agent-orchestrator/tools"

	"github.com/google/uuid"
)

// ── JSON-RPC 2.0 types ───────────────────────────────────────────────────────

type rpcMsg struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcResp struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type mcpContent struct {
	Type string `json:"type"` // "text"
	Text string `json:"text"`
}

type mcpToolResult struct {
	Content []mcpContent `json:"content"`
	IsError bool         `json:"isError,omitempty"`
}

// ── Server ───────────────────────────────────────────────────────────────────

type srv struct {
	agentReg    *agent.Registry
	validator   orchestrator.Validator
	repairEng   *repair.Engine
	runRepo     *sqlite.AgentRunSQLiteRepository
	stepRepo    *sqlite.AgentStepSQLiteRepository
	toolCallRepo *sqlite.ToolCallSQLiteRepository
}

func main() {
	// Log to stderr — stdout is the MCP protocol channel.
	log.SetOutput(os.Stderr)
	log.SetFlags(log.Ltime)

	// Persistent DB in ~/.agent-orchestrator/ shared across sessions.
	home, _ := os.UserHomeDir()
	dbDir := filepath.Join(home, ".agent-orchestrator")
	_ = os.MkdirAll(dbDir, 0755)
	dbPath := filepath.Join(dbDir, "runs.db")
	if len(os.Args) > 1 && os.Args[1] != "" {
		dbPath = os.Args[1]
	}

	repo := sqlite.New(dbPath)
	if err := repo.Open(); err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer repo.Close()

	agentReg := agent.NewRegistry()
	agentReg.Register("agent.log_reader", agent.NewLogReaderAgent())
	agentReg.Register("agent.log_analyzer", agent.NewLogAnalyzerAgent())

	repairEng := repair.NewEngine(repair.NewSimpleRetryStrategy(), 3)
	validator := orchestrator.NewCompositeValidator(
		orchestrator.NewReportValidator(),
		orchestrator.NewGroundingValidator(),
	)

	s := &srv{
		agentReg:    agentReg,
		validator:   validator,
		repairEng:   repairEng,
		runRepo:     sqlite.NewAgentRunRepository(repo.DB),
		stepRepo:    sqlite.NewAgentStepRepository(repo.DB),
		toolCallRepo: sqlite.NewToolCallRepository(repo.DB),
	}

	log.Printf("agent-orchestrator MCP server ready (db: %s)", dbPath)
	s.loop()
}

func (s *srv) loop() {
	enc := json.NewEncoder(os.Stdout)
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 4*1024*1024), 4*1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var msg rpcMsg
		if err := json.Unmarshal(line, &msg); err != nil {
			log.Printf("parse error: %v", err)
			continue
		}
		// Notifications (no ID) are fire-and-forget — no response.
		if len(msg.ID) == 0 || string(msg.ID) == "null" {
			continue
		}
		if err := enc.Encode(s.handle(msg)); err != nil {
			log.Printf("encode error: %v", err)
		}
	}
}

func (s *srv) handle(msg rpcMsg) rpcResp {
	switch msg.Method {
	case "initialize":
		return rpcResp{
			JSONRPC: "2.0", ID: msg.ID,
			Result: map[string]any{
				"protocolVersion": "2024-11-05",
				"capabilities":    map[string]any{"tools": map[string]any{}},
				"serverInfo":      map[string]any{"name": "agent-orchestrator", "version": "1.0.0"},
			},
		}

	case "tools/list":
		return rpcResp{JSONRPC: "2.0", ID: msg.ID, Result: map[string]any{"tools": toolDefs()}}

	case "tools/call":
		var p struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		}
		if err := json.Unmarshal(msg.Params, &p); err != nil {
			return errResp(msg.ID, -32600, "invalid params")
		}
		return rpcResp{JSONRPC: "2.0", ID: msg.ID, Result: s.callTool(p.Name, p.Arguments)}

	default:
		return errResp(msg.ID, -32601, "method not found: "+msg.Method)
	}
}

func errResp(id json.RawMessage, code int, msg string) rpcResp {
	return rpcResp{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: code, Message: msg}}
}

// ── Tool definitions ─────────────────────────────────────────────────────────

func toolDefs() []map[string]any {
	return []map[string]any{
		{
			"name":        "analyze_logs",
			"description": "Scan a directory of log files, identify errors and root causes, and return a structured report with evidence and recommended next steps. Works on any local directory.",
			"inputSchema": map[string]any{
				"type":     "object",
				"required": []string{"directory"},
				"properties": map[string]any{
					"directory": map[string]any{
						"type":        "string",
						"description": "Absolute path to the directory containing log files, e.g. /var/log/myapp or /Users/me/project/logs",
					},
					"task_id": map[string]any{
						"type":        "string",
						"description": "Optional short label for this run, e.g. 'diagnose-payment-crash'",
					},
				},
			},
		},
		{
			"name":        "list_runs",
			"description": "List recent log analysis runs with their status, goal, and run ID.",
			"inputSchema": map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
		{
			"name":        "get_report",
			"description": "Retrieve the full analysis report for a completed run by its run ID.",
			"inputSchema": map[string]any{
				"type":     "object",
				"required": []string{"run_id"},
				"properties": map[string]any{
					"run_id": map[string]any{
						"type":        "string",
						"description": "Run ID from analyze_logs or list_runs",
					},
				},
			},
		},
	}
}

// ── Tool dispatch ─────────────────────────────────────────────────────────────

func (s *srv) callTool(name string, args map[string]any) mcpToolResult {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	switch name {
	case "analyze_logs":
		return s.analyzeLogs(ctx, args)
	case "list_runs":
		return s.listRuns()
	case "get_report":
		runID, _ := args["run_id"].(string)
		return s.getReport(runID)
	default:
		return errTool("unknown tool: " + name)
	}
}

// ── analyze_logs ──────────────────────────────────────────────────────────────

func (s *srv) analyzeLogs(ctx context.Context, args map[string]any) mcpToolResult {
	directory, _ := args["directory"].(string)
	if directory == "" {
		return errTool("directory is required")
	}
	taskID, _ := args["task_id"].(string)
	if taskID == "" {
		taskID = "mcp-analyze"
	}

	log.Printf("analyzing logs in: %s", directory)

	// Create tools scoped to the user's directory.
	// The agent uses "." as its working path, so all file ops resolve within this root.
	toolReg := tools.NewRegistry()
	toolReg.Register(tools.NewListDirTool(directory))
	toolReg.Register(tools.NewReadFileTool(directory))
	toolReg.Register(tools.NewGrepFileTool(directory))
	toolExec := tools.NewRegistryExecutor(toolReg)

	engine := orchestrator.NewEngine(
		planner.NewLogAnalysisPlanner(),
		s.agentReg,
		toolExec,
		s.validator,
		s.runRepo,
		s.stepRepo,
		s.repairEng,
	)
	engine.SetToolCallRepository(s.toolCallRepo)
	engine.SetToolCallReader(s.toolCallRepo)

	runID := uuid.New().String()
	result, err := engine.Execute(ctx, orchestrator.ExecutionRequest{
		RunID:  runID,
		TaskID: taskID,
		Input:  map[string]any{"directory": "."}, // "." = root of the scoped tool registry
	})
	if err != nil {
		return errTool("engine error: " + err.Error())
	}
	if result.Err != nil {
		return errTool("analysis failed: " + result.Err.Error())
	}

	return mcpToolResult{Content: []mcpContent{{Type: "text", Text: formatReport(result.Output, directory, runID)}}}
}

// ── list_runs ─────────────────────────────────────────────────────────────────

func (s *srv) listRuns() mcpToolResult {
	runs, err := s.runRepo.List()
	if err != nil {
		return errTool("failed to list runs: " + err.Error())
	}
	if len(runs) == 0 {
		return mcpToolResult{Content: []mcpContent{{Type: "text", Text: "No runs yet. Use analyze_logs to start an analysis."}}}
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## Recent Analysis Runs (%d total)\n\n", len(runs)))
	for _, r := range runs {
		sb.WriteString(fmt.Sprintf("- **%s** — `%s` — %s (%s)\n",
			r.Goal, r.RunID[:8], r.Status, r.CreatedAt.Format("Jan 2 15:04")))
	}
	sb.WriteString("\nUse `get_report` with the full run ID to retrieve a report.")
	return mcpToolResult{Content: []mcpContent{{Type: "text", Text: sb.String()}}}
}

// ── get_report ────────────────────────────────────────────────────────────────

func (s *srv) getReport(runID string) mcpToolResult {
	if runID == "" {
		return errTool("run_id is required")
	}
	steps, err := s.stepRepo.GetByRunID(runID)
	if err != nil || len(steps) == 0 {
		return errTool("no steps found for run " + runID + " — it may still be running or the ID is wrong")
	}
	for i := len(steps) - 1; i >= 0; i-- {
		var output map[string]any
		if err := json.Unmarshal([]byte(steps[i].Output), &output); err != nil {
			continue
		}
		if _, ok := output["error_summary"]; ok {
			return mcpToolResult{Content: []mcpContent{{Type: "text", Text: formatReport(output, "", runID)}}}
		}
	}
	return errTool("no report found — the run may still be in progress")
}

// ── formatReport ──────────────────────────────────────────────────────────────

func formatReport(output map[string]any, directory, runID string) string {
	var sb strings.Builder
	sb.WriteString("## Log Analysis Report\n\n")
	if directory != "" {
		sb.WriteString(fmt.Sprintf("**Directory:** `%s`\n", directory))
	}
	if runID != "" {
		sb.WriteString(fmt.Sprintf("**Run ID:** `%s`\n\n", runID))
	}

	if v, ok := output["confidence_level"].(string); ok {
		sb.WriteString(fmt.Sprintf("**Confidence:** %s\n\n", v))
	}

	if v, ok := output["error_summary"].(string); ok {
		sb.WriteString("### Error Summary\n")
		sb.WriteString(v + "\n\n")
	}

	if v, ok := output["suspected_root_cause"].(string); ok && v != "" && v != "N/A — no issues found." {
		sb.WriteString("### Root Cause\n")
		sb.WriteString(v + "\n\n")
	}

	if ev, ok := output["supporting_evidence"].([]any); ok && len(ev) > 0 {
		sb.WriteString("### Supporting Evidence\n\n")
		for _, e := range ev {
			if m, ok := e.(map[string]any); ok {
				file, _ := m["file"].(string)
				text, _ := m["text"].(string)
				line := 0
				switch v := m["line_number"].(type) {
				case int:
					line = v
				case float64:
					line = int(v)
				}
				if line > 0 {
					sb.WriteString(fmt.Sprintf("- `%s:%d` — %s\n", file, line, text))
				} else {
					sb.WriteString(fmt.Sprintf("- `%s` — %s\n", file, text))
				}
			}
		}
		sb.WriteString("\n")
	}

	if steps, ok := output["suggested_next_steps"].([]any); ok && len(steps) > 0 {
		sb.WriteString("### Recommended Next Steps\n\n")
		for i, step := range steps {
			if str, ok := step.(string); ok {
				sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, str))
			}
		}
	}

	return sb.String()
}

func errTool(msg string) mcpToolResult {
	return mcpToolResult{IsError: true, Content: []mcpContent{{Type: "text", Text: "Error: " + msg}}}
}
