package orchestrator

import (
	"encoding/json"
	"fmt"
	"strings"

	"agent-orchestrator/agent"
)

// Metrics holds computed evaluation metrics across a set of agent runs.
type Metrics struct {
	TotalRuns              int     `json:"total_runs"`
	SucceededRuns          int     `json:"succeeded_runs"`
	FailedRuns             int     `json:"failed_runs"`
	StructuredOutputRate   float64 `json:"structured_output_rate"`
	EvidenceCoverage       float64 `json:"evidence_coverage"`
	HallucinationRate      float64 `json:"hallucination_rate"`
	RepairSuccessRate      float64 `json:"repair_success_rate"`
	TotalSteps             int     `json:"total_steps"`
	TotalToolCalls         int     `json:"total_tool_calls"`
	TotalEvidenceItems     int     `json:"total_evidence_items"`
	GroundedEvidenceItems  int     `json:"grounded_evidence_items"`
	TotalRepairableSteps   int     `json:"total_repairable_steps"`
	SuccessfullyRepaired   int     `json:"successfully_repaired"`
}

// MetricsEvaluator computes evaluation metrics from persisted run data.
type MetricsEvaluator struct {
	runs      RunRepository
	steps     StepRepository
	toolCalls ToolCallRepository
}

// NewMetricsEvaluator creates an evaluator wired to the storage repositories.
func NewMetricsEvaluator(
	runs RunRepository,
	steps StepRepository,
	toolCalls ToolCallRepository,
) *MetricsEvaluator {
	return &MetricsEvaluator{
		runs:      runs,
		steps:     steps,
		toolCalls: toolCalls,
	}
}

// Evaluate computes metrics for a given run ID.
func (m *MetricsEvaluator) Evaluate(runID string) (*Metrics, error) {
	run, err := m.runs.GetByID(runID)
	if err != nil {
		return nil, fmt.Errorf("metrics: run %s not found: %w", runID, err)
	}
	return m.evaluateRuns([]*agent.AgentRun{run})
}

// EvaluateAll computes aggregate metrics across all provided runs.
func (m *MetricsEvaluator) EvaluateAll(runs []*agent.AgentRun) (*Metrics, error) {
	return m.evaluateRuns(runs)
}

func (m *MetricsEvaluator) evaluateRuns(runs []*agent.AgentRun) (*Metrics, error) {
	metrics := &Metrics{
		TotalRuns: len(runs),
	}

	var validStructureCount int

	for _, run := range runs {
		switch run.Status {
		case agent.AgentRunCompleted:
			metrics.SucceededRuns++
		case agent.AgentRunFailed:
			metrics.FailedRuns++
		}

		steps, err := m.steps.GetByRunID(run.RunID)
		if err != nil {
			return nil, fmt.Errorf("metrics: failed to load steps for %s: %w", run.RunID, err)
		}
		metrics.TotalSteps += len(steps)

		toolCalls, err := m.toolCalls.GetByRunID(run.RunID)
		if err != nil {
			return nil, fmt.Errorf("metrics: failed to load tool calls for %s: %w", run.RunID, err)
		}
		metrics.TotalToolCalls += len(toolCalls)

		// --- Structured output success rate ---
		if hasValidStructuredOutput(steps) {
			validStructureCount++
		}

		// --- Evidence coverage & hallucination rate ---
		evidenceItems, groundedItems := computeGrounding(steps, toolCalls)
		metrics.TotalEvidenceItems += evidenceItems
		metrics.GroundedEvidenceItems += groundedItems

		// --- Repair success rate ---
		repairable, repaired := computeRepairRate(steps)
		metrics.TotalRepairableSteps += repairable
		metrics.SuccessfullyRepaired += repaired
	}

	// Compute rates.
	if metrics.TotalRuns > 0 {
		metrics.StructuredOutputRate = float64(validStructureCount) / float64(metrics.TotalRuns)
	}
	if metrics.TotalEvidenceItems > 0 {
		metrics.HallucinationRate = 1.0 - float64(metrics.GroundedEvidenceItems)/float64(metrics.TotalEvidenceItems)
		metrics.EvidenceCoverage = float64(metrics.GroundedEvidenceItems) / float64(metrics.TotalEvidenceItems)
	}
	if metrics.TotalRepairableSteps > 0 {
		metrics.RepairSuccessRate = float64(metrics.SuccessfullyRepaired) / float64(metrics.TotalRepairableSteps)
	}

	return metrics, nil
}

// hasValidStructuredOutput checks whether any succeeded step in the run
// produced a valid structured output (has the golden-path required fields).
func hasValidStructuredOutput(steps []*agent.AgentStep) bool {
	for _, s := range steps {
		if s.Status != agent.StepSucceeded {
			continue
		}
		output := parseOutputMap(s.Output)
		if output == nil {
			continue
		}
		// Check for required golden-path fields.
		if hasString(output, "error_summary") &&
			hasString(output, "suspected_root_cause") &&
			hasField(output, "supporting_evidence") &&
			hasString(output, "confidence_level") &&
			hasField(output, "suggested_next_steps") {
			return true
		}
	}
	return false
}

// computeGrounding counts total evidence items and how many are grounded
// in the tool call outputs for a single run.
func computeGrounding(steps []*agent.AgentStep, toolCalls []*agent.ToolCall) (total, grounded int) {
	// Build corpus from all successful tool call outputs.
	corpus, fileSet := buildToolCallCorpus(toolCalls)

	for _, s := range steps {
		if s.Status != agent.StepSucceeded {
			continue
		}
		output := parseOutputMap(s.Output)
		if output == nil {
			continue
		}
		evidence := extractEvidenceFromOutput(output)
		for _, ev := range evidence {
			total++
			fileOK := ev.file == "" || fileSet[ev.file]
			textOK := ev.text == "" || isSubstringNormalized(corpus, ev.text)
			if fileOK && textOK {
				grounded++
			}
		}
	}
	return
}

// computeRepairRate analyzes step attempts to determine how many logical
// steps had failures (multiple attempts) and how many eventually succeeded.
//
// A "repairable step" is a logical step (identified by run-step prefix) where
// at least one attempt failed. It counts as "successfully repaired" if any
// subsequent attempt succeeded.
func computeRepairRate(steps []*agent.AgentStep) (repairable, repaired int) {
	// Group steps by logical step prefix: "{runID}-step-{idx}"
	type stepGroup struct {
		hasFailed    bool
		hasSucceeded bool
	}
	groups := make(map[string]*stepGroup)

	for _, s := range steps {
		// StepID format: "{runID}-step-{idx}-attempt-{n}"
		prefix := logicalStepPrefix(s.StepID)
		if prefix == "" {
			continue
		}
		g, ok := groups[prefix]
		if !ok {
			g = &stepGroup{}
			groups[prefix] = g
		}
		if s.Status == agent.StepFailed {
			g.hasFailed = true
		}
		if s.Status == agent.StepSucceeded {
			g.hasSucceeded = true
		}
	}

	for _, g := range groups {
		if g.hasFailed {
			repairable++
			if g.hasSucceeded {
				repaired++
			}
		}
	}
	return
}

// --- helpers ----------------------------------------------------------------

type metricsEvidence struct {
	file string
	text string
}

func extractEvidenceFromOutput(output map[string]any) []metricsEvidence {
	raw, ok := output["supporting_evidence"]
	if !ok {
		return nil
	}

	var items []metricsEvidence

	switch ev := raw.(type) {
	case []any:
		for _, item := range ev {
			if m, ok := item.(map[string]any); ok {
				items = append(items, metricsEvidence{
					file: anyString(m["file"]),
					text: anyString(m["text"]),
				})
			}
		}
	case []map[string]any:
		for _, m := range ev {
			items = append(items, metricsEvidence{
				file: anyString(m["file"]),
				text: anyString(m["text"]),
			})
		}
	}
	return items
}

func buildToolCallCorpus(toolCalls []*agent.ToolCall) (string, map[string]bool) {
	var sb strings.Builder
	fileSet := make(map[string]bool)

	for _, tc := range toolCalls {
		if tc.Status != agent.ToolCallSucceeded {
			continue
		}

		var output map[string]any
		if err := json.Unmarshal([]byte(tc.Output), &output); err != nil {
			continue
		}

		appendValues(&sb, fileSet, output)
	}

	return sb.String(), fileSet
}

func appendValues(sb *strings.Builder, fileSet map[string]bool, m map[string]any) {
	for k, v := range m {
		switch val := v.(type) {
		case string:
			sb.WriteString(val)
			sb.WriteString("\n")
			// Track file-like values.
			if k == "path" || k == "file" || strings.HasSuffix(k, "_path") {
				fileSet[val] = true
				// Also track the basename.
				if idx := strings.LastIndex(val, "/"); idx >= 0 {
					fileSet[val[idx+1:]] = true
				}
			}
		case map[string]any:
			appendValues(sb, fileSet, val)
		case []any:
			for _, item := range val {
				if m2, ok := item.(map[string]any); ok {
					appendValues(sb, fileSet, m2)
				} else if s, ok := item.(string); ok {
					sb.WriteString(s)
					sb.WriteString("\n")
				}
			}
		}
	}
}

func isSubstringNormalized(corpus, text string) bool {
	norm := func(s string) string {
		return strings.ToLower(strings.Join(strings.Fields(s), " "))
	}
	return strings.Contains(norm(corpus), norm(text))
}

func parseOutputMap(raw string) map[string]any {
	// Step outputs are stored as fmt.Sprintf("%v", output) which produces
	// Go map literal format: map[key:value ...]. Try JSON first, then
	// fall back to a simple Go-map parser.
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err == nil {
		return m
	}
	return parseGoMapLiteral(raw)
}

// parseGoMapLiteral is a best-effort parser for Go's fmt "%v" map output.
// It handles the common case: map[key:value key2:value2].
// For nested structures it falls back to treating the whole value as a string.
func parseGoMapLiteral(s string) map[string]any {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "map[") || !strings.HasSuffix(s, "]") {
		return nil
	}
	inner := s[4 : len(s)-1]
	if inner == "" {
		return map[string]any{}
	}

	result := make(map[string]any)
	// Simple tokenizer: split on space, then split each token on first ':'
	tokens := strings.Fields(inner)
	for _, tok := range tokens {
		idx := strings.Index(tok, ":")
		if idx < 0 {
			continue
		}
		key := tok[:idx]
		val := tok[idx+1:]
		result[key] = val
	}
	return result
}

func hasString(m map[string]any, key string) bool {
	v, ok := m[key]
	if !ok {
		return false
	}
	s, ok := v.(string)
	return ok && s != ""
}

func hasField(m map[string]any, key string) bool {
	_, ok := m[key]
	return ok
}

func anyString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// logicalStepPrefix extracts the "{runID}-step-{idx}" portion from a step ID
// like "{runID}-step-{idx}-attempt-{n}".
func logicalStepPrefix(stepID string) string {
	idx := strings.LastIndex(stepID, "-attempt-")
	if idx < 0 {
		return ""
	}
	return stepID[:idx]
}
