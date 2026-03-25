package orchestrator

import (
	"fmt"
	"strings"

	"agent-orchestrator/tools"
)

// GroundingValidator checks that the agent's supporting_evidence references
// only data that actually appeared in tool call outputs. This detects
// hallucinated file names, log lines, and error codes.
//
// It requires a *tools.ToolCallCollector to be set before Validate is called.
type GroundingValidator struct {
	collector *tools.ToolCallCollector
}

func NewGroundingValidator() *GroundingValidator {
	return &GroundingValidator{}
}

// SetCollector sets the collector that provides tool call outputs for the
// current step. Must be called by the step executor before validation.
func (v *GroundingValidator) SetCollector(c *tools.ToolCallCollector) {
	v.collector = c
}

func (v *GroundingValidator) Validate(stepID string, output map[string]any) error {
	// Only validate steps that produce supporting_evidence.
	if _, hasEvidence := output["supporting_evidence"]; !hasEvidence {
		return nil
	}

	// If no collector was wired, skip grounding (degrade gracefully).
	if v.collector == nil {
		return nil
	}

	evidence := extractEvidenceItems(output)
	if len(evidence) == 0 {
		return nil // nothing to ground-check
	}

	// Build the corpus: all tool call outputs concatenated, plus the set
	// of file paths that were actually accessed via tools.
	toolOutputs := v.collector.Outputs()
	corpus, fileSet := buildCorpus(toolOutputs)

	var violations []string

	for i, ev := range evidence {
		file := ev.file
		text := ev.text

		// 1. Check file name is grounded (appeared in a tool call).
		if file != "" && !fileSet[file] {
			violations = append(violations,
				fmt.Sprintf("evidence[%d]: file %q not found in any tool call output", i, file))
		}

		// 2. Check that the claimed log line text appears in the corpus.
		if text != "" && !isGrounded(text, corpus) {
			violations = append(violations,
				fmt.Sprintf("evidence[%d]: text %q not found in any tool call output", i, truncate(text, 80)))
		}
	}

	if len(violations) > 0 {
		return fmt.Errorf("grounding check failed: %d violation(s):\n  - %s",
			len(violations), strings.Join(violations, "\n  - "))
	}

	return nil
}

// --- helpers ---

type evidenceItem struct {
	file       string
	lineNumber int
	text       string
}

func extractEvidenceItems(output map[string]any) []evidenceItem {
	raw, ok := output["supporting_evidence"]
	if !ok {
		return nil
	}

	var items []evidenceItem
	switch ev := raw.(type) {
	case []map[string]any:
		for _, m := range ev {
			items = append(items, evidenceItem{
				file:       strVal(m, "file"),
				lineNumber: intVal(m, "line_number"),
				text:       strVal(m, "text"),
			})
		}
	case []any:
		for _, r := range ev {
			if m, ok := r.(map[string]any); ok {
				items = append(items, evidenceItem{
					file:       strVal(m, "file"),
					lineNumber: intVal(m, "line_number"),
					text:       strVal(m, "text"),
				})
			}
		}
	}
	return items
}

// buildCorpus collects all tool output text into a single searchable string
// and tracks which file paths were accessed.
func buildCorpus(outputs []tools.CollectedOutput) (string, map[string]bool) {
	var sb strings.Builder
	fileSet := make(map[string]bool)

	for _, o := range outputs {
		// Track file names from tool inputs and outputs.
		if p, ok := o.Input["path"].(string); ok {
			fileSet[p] = true
			// Also add the base name (agent evidence often uses just the name).
			if idx := strings.LastIndex(p, "/"); idx >= 0 {
				fileSet[p[idx+1:]] = true
			}
		}

		// Collect text from tool outputs.
		for _, v := range o.Output {
			appendValue(&sb, v)
		}
	}

	return sb.String(), fileSet
}

// appendValue recursively converts a value to string and appends it.
func appendValue(sb *strings.Builder, v any) {
	switch val := v.(type) {
	case string:
		sb.WriteString(val)
		sb.WriteByte('\n')
	case []any:
		for _, item := range val {
			appendValue(sb, item)
		}
	case []map[string]any:
		for _, m := range val {
			for _, mv := range m {
				appendValue(sb, mv)
			}
		}
	case map[string]any:
		for _, mv := range val {
			appendValue(sb, mv)
		}
	default:
		sb.WriteString(fmt.Sprintf("%v", val))
		sb.WriteByte('\n')
	}
}

// isGrounded checks whether the evidence text appears in the corpus.
// It uses a normalized substring search to handle minor whitespace differences.
func isGrounded(text, corpus string) bool {
	// Normalize whitespace for comparison.
	normText := normalizeWS(text)
	normCorpus := normalizeWS(corpus)
	return strings.Contains(normCorpus, normText)
}

func normalizeWS(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func strVal(m map[string]any, key string) string {
	v, _ := m[key].(string)
	return v
}

func intVal(m map[string]any, key string) int {
	v, _ := m[key].(int)
	return v
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
