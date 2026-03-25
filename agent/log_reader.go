package agent

import (
	"context"
	"fmt"
	"strings"

	"agent-orchestrator/tools"
)

// LogReaderAgent scans a directory for log files, searches for error/warning
// patterns, and produces a structured summary of findings.
//
// Expected input keys:
//   - "directory" (string)  — relative path to list (default ".")
//   - "keywords"  (string)  — comma-separated search terms (default "error,ERROR,panic,FATAL,fatal,warn,WARN")
//
// Output keys:
//   - "files_scanned" (int)
//   - "total_matches" (int)
//   - "findings"      ([]map) — per-file list of matching lines
//   - "summary"       (string)
type LogReaderAgent struct{}

func NewLogReaderAgent() *LogReaderAgent {
	return &LogReaderAgent{}
}

func (a *LogReaderAgent) Run(ctx context.Context, input map[string]any) (*Result, error) {
	return &Result{Output: map[string]any{
		"error": "LogReaderAgent requires RunWithContext for tool access",
	}}, nil
}

func (a *LogReaderAgent) RunWithContext(ctx context.Context, rtx RuntimeContext) (*Result, error) {
	if rtx.Tools == nil {
		return nil, fmt.Errorf("LogReaderAgent requires tools (fs.list_dir, fs.grep_file)")
	}

	dir := "."
	if d, ok := rtx.Input["directory"].(string); ok && d != "" {
		dir = d
	}

	keywords := []string{"error", "ERROR", "panic", "FATAL", "fatal", "warn", "WARN"}
	if kw, ok := rtx.Input["keywords"].(string); ok && kw != "" {
		keywords = strings.Split(kw, ",")
		for i := range keywords {
			keywords[i] = strings.TrimSpace(keywords[i])
		}
	}

	// 1. List directory
	listResult, err := rtx.Tools.Execute(ctx, tools.Call{
		ToolName: "fs.list_dir",
		Args:     map[string]any{"path": dir},
	})
	if err != nil {
		return nil, fmt.Errorf("list_dir failed: %w", err)
	}

	entries, _ := listResult.Data["entries"].([]map[string]any)
	if entries == nil {
		// Try interface slice (runtime type from JSON/tool output)
		if raw, ok := listResult.Data["entries"].([]any); ok {
			for _, r := range raw {
				if m, ok := r.(map[string]any); ok {
					entries = append(entries, m)
				}
			}
		}
	}

	// Filter to files only (skip directories), keep .log .txt and extensionless files
	var logFiles []string
	for _, entry := range entries {
		isDir, _ := entry["is_dir"].(bool)
		if isDir {
			continue
		}
		name, _ := entry["name"].(string)
		if name == "" {
			continue
		}
		lower := strings.ToLower(name)
		if strings.HasSuffix(lower, ".log") ||
			strings.HasSuffix(lower, ".txt") ||
			strings.HasSuffix(lower, ".out") ||
			!strings.Contains(name, ".") { // extensionless
			logFiles = append(logFiles, name)
		}
	}

	// 2. Grep each log file for each keyword
	type fileFindings struct {
		File    string           `json:"file"`
		Matches []map[string]any `json:"matches"`
	}

	var allFindings []map[string]any
	totalMatches := 0

	for _, file := range logFiles {
		var matches []map[string]any

		relPath := file
		if dir != "." {
			relPath = dir + "/" + file
		}

		for _, kw := range keywords {
			grepResult, err := rtx.Tools.Execute(ctx, tools.Call{
				ToolName: "fs.grep_file",
				Args:     map[string]any{"path": relPath, "keyword": kw},
			})
			if err != nil {
				continue // skip individual grep failures
			}

			matchCount, _ := grepResult.Data["match_count"].(int)
			if matchCount == 0 {
				continue
			}

			rawMatches, _ := grepResult.Data["matches"].([]map[string]any)
			if rawMatches == nil {
				if raw, ok := grepResult.Data["matches"].([]any); ok {
					for _, r := range raw {
						if m, ok := r.(map[string]any); ok {
							rawMatches = append(rawMatches, m)
						}
					}
				}
			}

			for _, m := range rawMatches {
				m["keyword"] = kw
				matches = append(matches, m)
			}
			totalMatches += matchCount
		}

		if len(matches) > 0 {
			allFindings = append(allFindings, map[string]any{
				"file":        file,
				"matches":     matches,
				"match_count": len(matches),
			})
		}
	}

	// 3. Build summary
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Scanned %d log files in '%s'. ", len(logFiles), dir))
	sb.WriteString(fmt.Sprintf("Found %d matching lines across %d files.\n", totalMatches, len(allFindings)))

	for _, f := range allFindings {
		fname, _ := f["file"].(string)
		mc, _ := f["match_count"].(int)
		sb.WriteString(fmt.Sprintf("  - %s: %d matches\n", fname, mc))
	}

	if totalMatches == 0 {
		sb.WriteString("No errors or warnings found.")
	}

	return &Result{
		Output: map[string]any{
			"files_scanned": len(logFiles),
			"total_matches": totalMatches,
			"findings":      allFindings,
			"summary":       sb.String(),
		},
	}, nil
}
