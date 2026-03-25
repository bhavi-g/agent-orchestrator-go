package agent

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// LogAnalyzerAgent takes raw log findings (produced by LogReaderAgent) and
// produces the Golden Path structured report:
//
//   - error_summary
//   - suspected_root_cause
//   - supporting_evidence  (list of {file, line_number, text})
//   - confidence_level     ("Low" | "Medium" | "High")
//   - suggested_next_steps (list of strings)
//
// It reads "findings" and "total_matches" from input (typically merged from
// Vars of a prior LogReaderAgent step).
type LogAnalyzerAgent struct{}

func NewLogAnalyzerAgent() *LogAnalyzerAgent {
	return &LogAnalyzerAgent{}
}

func (a *LogAnalyzerAgent) Run(ctx context.Context, input map[string]any) (*Result, error) {
	return a.analyze(input)
}

func (a *LogAnalyzerAgent) analyze(input map[string]any) (*Result, error) {
	// Extract findings from Vars/input (set by prior LogReaderAgent step).
	findings := extractFindings(input)
	totalMatches, _ := input["total_matches"].(int)

	// If no findings, produce an "all clear" report.
	if len(findings) == 0 || totalMatches == 0 {
		return &Result{Output: map[string]any{
			"error_summary":       "No errors or warnings detected in the scanned log files.",
			"suspected_root_cause": "N/A — no issues found.",
			"supporting_evidence":  []map[string]any{},
			"confidence_level":     "High",
			"suggested_next_steps": []string{
				"Continue monitoring logs for future issues.",
			},
		}}, nil
	}

	// --- Analyze findings ---

	// Collect all matched lines across files.
	type evidence struct {
		File       string
		LineNumber int
		Text       string
		Keyword    string
	}
	var allEvidence []evidence

	for _, f := range findings {
		file, _ := f["file"].(string)
		matches := extractMatches(f)
		for _, m := range matches {
			lineNum, _ := m["line_number"].(int)
			text, _ := m["text"].(string)
			kw, _ := m["keyword"].(string)
			allEvidence = append(allEvidence, evidence{
				File: file, LineNumber: lineNum, Text: text, Keyword: kw,
			})
		}
	}

	// Count error types by keyword category.
	errorCount := 0
	panicCount := 0
	warnCount := 0

	for _, ev := range allEvidence {
		kwLower := strings.ToLower(ev.Keyword)
		switch {
		case kwLower == "panic" || kwLower == "fatal":
			panicCount++
		case kwLower == "error":
			errorCount++
		case kwLower == "warn" || kwLower == "warning":
			warnCount++
		default:
			errorCount++ // treat unknown keywords as errors
		}
	}

	// --- Build error summary ---
	var summaryParts []string
	if panicCount > 0 {
		summaryParts = append(summaryParts, fmt.Sprintf("%d panic/fatal events", panicCount))
	}
	if errorCount > 0 {
		summaryParts = append(summaryParts, fmt.Sprintf("%d errors", errorCount))
	}
	if warnCount > 0 {
		summaryParts = append(summaryParts, fmt.Sprintf("%d warnings", warnCount))
	}
	errorSummary := fmt.Sprintf("Detected %s across %d log files.",
		strings.Join(summaryParts, ", "), len(findings))

	// --- Determine root cause (heuristic: most common error pattern) ---
	patternCounts := make(map[string]int)
	for _, ev := range allEvidence {
		kwLower := strings.ToLower(ev.Keyword)
		if kwLower == "warn" || kwLower == "warning" {
			continue // skip warnings for root cause
		}
		// Use a simplified version of the line as a pattern key.
		pattern := simplifyPattern(ev.Text)
		patternCounts[pattern]++
	}

	rootCause := "Unable to determine a specific root cause from available evidence."
	topPattern := ""
	topCount := 0
	for p, c := range patternCounts {
		if c > topCount {
			topCount = c
			topPattern = p
		}
	}
	if topPattern != "" {
		rootCause = fmt.Sprintf("Most frequent error pattern (%d occurrences): %s", topCount, topPattern)
	}

	// --- Build supporting evidence (top entries, capped) ---
	// Prioritise panics/fatals, then errors, then warnings.
	sort.Slice(allEvidence, func(i, j int) bool {
		return severityRank(allEvidence[i].Keyword) > severityRank(allEvidence[j].Keyword)
	})

	maxEvidence := 20
	if len(allEvidence) < maxEvidence {
		maxEvidence = len(allEvidence)
	}
	evidenceList := make([]map[string]any, maxEvidence)
	for i := 0; i < maxEvidence; i++ {
		ev := allEvidence[i]
		evidenceList[i] = map[string]any{
			"file":        ev.File,
			"line_number": ev.LineNumber,
			"text":        ev.Text,
		}
	}

	// --- Confidence level ---
	confidence := "Medium"
	if panicCount > 0 || topCount >= 3 {
		confidence = "High"
	} else if totalMatches <= 2 {
		confidence = "Low"
	}

	// --- Suggested next steps ---
	var nextSteps []string
	if panicCount > 0 {
		nextSteps = append(nextSteps, "Investigate panic/fatal events immediately — these indicate crashes.")
	}
	if errorCount > 0 {
		nextSteps = append(nextSteps, "Review the most frequent error pattern and trace its origin in the codebase.")
	}
	if warnCount > 0 {
		nextSteps = append(nextSteps, "Evaluate warnings for potential issues that could escalate.")
	}
	nextSteps = append(nextSteps, "Check if errors correlate with a recent deployment or configuration change.")

	return &Result{Output: map[string]any{
		"error_summary":        errorSummary,
		"suspected_root_cause": rootCause,
		"supporting_evidence":  evidenceList,
		"confidence_level":     confidence,
		"suggested_next_steps": nextSteps,
	}}, nil
}

// --- helpers ---

func extractFindings(input map[string]any) []map[string]any {
	if f, ok := input["findings"].([]map[string]any); ok {
		return f
	}
	if raw, ok := input["findings"].([]any); ok {
		var out []map[string]any
		for _, r := range raw {
			if m, ok := r.(map[string]any); ok {
				out = append(out, m)
			}
		}
		return out
	}
	return nil
}

func extractMatches(finding map[string]any) []map[string]any {
	if m, ok := finding["matches"].([]map[string]any); ok {
		return m
	}
	if raw, ok := finding["matches"].([]any); ok {
		var out []map[string]any
		for _, r := range raw {
			if m, ok := r.(map[string]any); ok {
				out = append(out, m)
			}
		}
		return out
	}
	return nil
}

// simplifyPattern extracts a rough "pattern" from a log line by removing
// timestamps and normalising whitespace.
func simplifyPattern(line string) string {
	// Strip common timestamp prefixes (YYYY-MM-DD HH:MM:SS or similar).
	parts := strings.Fields(line)
	// Drop leading tokens that look like dates/times.
	start := 0
	for start < len(parts) && start < 3 {
		p := parts[start]
		if looksLikeTimestamp(p) {
			start++
			continue
		}
		break
	}
	if start >= len(parts) {
		return line
	}
	return strings.Join(parts[start:], " ")
}

func looksLikeTimestamp(s string) bool {
	if len(s) < 4 {
		return false
	}
	digits := 0
	for _, c := range s {
		if c >= '0' && c <= '9' {
			digits++
		}
	}
	return digits >= 3 // e.g. "2024-01-01" or "10:00:01"
}

func severityRank(keyword string) int {
	switch strings.ToLower(keyword) {
	case "panic", "fatal":
		return 3
	case "error":
		return 2
	case "warn", "warning":
		return 1
	default:
		return 0
	}
}
