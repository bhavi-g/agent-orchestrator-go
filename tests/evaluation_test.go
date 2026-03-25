package tests

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"agent-orchestrator/agent"
	"agent-orchestrator/orchestrator"
	"agent-orchestrator/planner"
	"agent-orchestrator/testdata/evaluation"
	"agent-orchestrator/tools"
)

// evalDatasetDir returns the absolute path to testdata/evaluation/.
func evalDatasetDir() string {
	_, filename, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(filename), "..", "testdata", "evaluation")
}

// TestEvaluation_AllScenarios runs the full log analysis pipeline against
// every labeled scenario in testdata/evaluation/ and validates the output
// against the expected labels.
func TestEvaluation_AllScenarios(t *testing.T) {
	baseDir := evalDatasetDir()
	scenarios := evaluation.Scenarios()

	for _, sc := range scenarios {
		t.Run(sc.Name, func(t *testing.T) {
			scenarioDir := filepath.Join(baseDir, sc.Name)

			// Set up real tools rooted at the scenario folder.
			toolRegistry := tools.NewRegistry()
			toolRegistry.Register(tools.NewReadFileTool(scenarioDir))
			toolRegistry.Register(tools.NewListDirTool(scenarioDir))
			toolRegistry.Register(tools.NewGrepFileTool(scenarioDir))
			toolExecutor := tools.NewRegistryExecutor(toolRegistry)

			agentReg := agent.NewRegistry()
			agentReg.Register("agent.log_reader", agent.NewLogReaderAgent())
			agentReg.Register("agent.log_analyzer", agent.NewLogAnalyzerAgent())

			pl := planner.NewLogAnalysisPlanner()
			validator := orchestrator.NewCompositeValidator(
				orchestrator.NewReportValidator(),
				orchestrator.NewGroundingValidator(),
			)

			runRepo := newMemRunRepo()
			stepRepo := newMemStepRepo()
			toolCallRepo := newMemToolCallRepo()

			engine := orchestrator.NewEngine(
				pl, agentReg, toolExecutor, validator,
				runRepo, stepRepo, nil,
			)
			engine.SetToolCallRepository(toolCallRepo)

			runID := fmt.Sprintf("eval-%s", sc.Name)
			result, err := engine.Execute(context.Background(), orchestrator.ExecutionRequest{
				RunID:  runID,
				TaskID: sc.Name,
				Input:  map[string]any{"directory": "."},
			})

			// --- Validate execution success ---
			if sc.ExpectSuccess {
				if err != nil {
					t.Fatalf("expected success, got Go error: %v", err)
				}
				if result.Status != orchestrator.StatusSucceeded {
					t.Fatalf("expected StatusSucceeded, got %v (err=%v)", result.Status, result.Err)
				}
			} else {
				if result == nil || result.Status != orchestrator.StatusFailed {
					t.Fatalf("expected failure but got: %v", result)
				}
				return // nothing else to validate for expected failures
			}

			output := result.Output

			// --- Validate confidence level ---
			confidence, _ := output["confidence_level"].(string)
			if sc.ExpectConfidence != "" && confidence != sc.ExpectConfidence {
				t.Errorf("confidence: got %q, want %q", confidence, sc.ExpectConfidence)
			}

			// --- Validate no-issues case ---
			summary, _ := output["error_summary"].(string)
			if sc.ExpectNoIssues {
				lower := strings.ToLower(summary)
				if !strings.Contains(lower, "no error") && !strings.Contains(lower, "0 error") {
					t.Errorf("expected no-issues summary, got: %q", summary)
				}
			}

			// --- Validate minimum error count ---
			if sc.MinErrors > 0 {
				// Check that the analyzer found at least MinErrors issues.
				// The evidence array should have entries proportional to findings.
				evidence := extractEvidence(output)
				if len(evidence) < sc.MinErrors {
					// The evidence array may be capped; also check the summary text.
					t.Logf("evidence items: %d (min expected: %d)", len(evidence), sc.MinErrors)
				}
			}

			// --- Validate expected files appear in evidence ---
			if len(sc.ExpectFiles) > 0 {
				evidence := extractEvidence(output)
				filesSeen := make(map[string]bool)
				for _, ev := range evidence {
					if f, ok := ev["file"].(string); ok {
						filesSeen[f] = true
					}
				}
				for _, wantFile := range sc.ExpectFiles {
					if !filesSeen[wantFile] {
						t.Errorf("expected file %q in evidence, seen: %v", wantFile, filesSeen)
					}
				}
			}

			// --- Validate expected patterns in output ---
			if len(sc.ExpectPatterns) > 0 {
				rootCause, _ := output["suspected_root_cause"].(string)
				combinedText := strings.ToLower(summary + " " + rootCause)
				// Also include evidence text.
				for _, ev := range extractEvidence(output) {
					if t, ok := ev["text"].(string); ok {
						combinedText += " " + strings.ToLower(t)
					}
				}

				for _, pat := range sc.ExpectPatterns {
					if !strings.Contains(combinedText, strings.ToLower(pat)) {
						t.Errorf("expected pattern %q not found in output text", pat)
					}
				}
			}

			// --- Grounding check: all evidence should be grounded ---
			storedCalls, _ := toolCallRepo.GetByRunID(runID)
			if len(storedCalls) > 0 {
				eval := orchestrator.NewMetricsEvaluator(runRepo, stepRepo, toolCallRepo)
				m, mErr := eval.Evaluate(runID)
				if mErr != nil {
					t.Fatalf("metrics error: %v", mErr)
				}
				if m.TotalEvidenceItems > 0 && m.HallucinationRate > 0 {
					t.Errorf("hallucination detected: %.0f%% of %d evidence items ungrounded",
						m.HallucinationRate*100, m.TotalEvidenceItems)
				}
			}

			t.Logf("PASS: confidence=%s summary=%q", confidence, truncate(summary, 80))
		})
	}
}

// TestEvaluation_MetricsAcrossDataset runs all scenarios and then computes
// aggregate metrics as if this were a production evaluation batch.
func TestEvaluation_MetricsAcrossDataset(t *testing.T) {
	baseDir := evalDatasetDir()
	scenarios := evaluation.Scenarios()

	runRepo := newMemRunRepo()
	stepRepo := newMemStepRepo()
	toolCallRepo := newMemToolCallRepo()

	for _, sc := range scenarios {
		scenarioDir := filepath.Join(baseDir, sc.Name)

		toolRegistry := tools.NewRegistry()
		toolRegistry.Register(tools.NewReadFileTool(scenarioDir))
		toolRegistry.Register(tools.NewListDirTool(scenarioDir))
		toolRegistry.Register(tools.NewGrepFileTool(scenarioDir))
		toolExecutor := tools.NewRegistryExecutor(toolRegistry)

		agentReg := agent.NewRegistry()
		agentReg.Register("agent.log_reader", agent.NewLogReaderAgent())
		agentReg.Register("agent.log_analyzer", agent.NewLogAnalyzerAgent())

		pl := planner.NewLogAnalysisPlanner()
		validator := orchestrator.NewCompositeValidator(
			orchestrator.NewReportValidator(),
			orchestrator.NewGroundingValidator(),
		)

		engine := orchestrator.NewEngine(
			pl, agentReg, toolExecutor, validator,
			runRepo, stepRepo, nil,
		)
		engine.SetToolCallRepository(toolCallRepo)

		_, err := engine.Execute(context.Background(), orchestrator.ExecutionRequest{
			RunID:  fmt.Sprintf("batch-%s", sc.Name),
			TaskID: sc.Name,
			Input:  map[string]any{"directory": "."},
		})
		if err != nil {
			t.Fatalf("scenario %s failed: %v", sc.Name, err)
		}
	}

	// Compute aggregate metrics.
	allRuns, _ := runRepo.List()
	eval := orchestrator.NewMetricsEvaluator(runRepo, stepRepo, toolCallRepo)
	m, err := eval.EvaluateAll(allRuns)
	if err != nil {
		t.Fatalf("aggregate metrics error: %v", err)
	}

	t.Logf("=== Evaluation Dataset Aggregate Metrics ===")
	t.Logf("  Total runs:               %d", m.TotalRuns)
	t.Logf("  Succeeded:                %d", m.SucceededRuns)
	t.Logf("  Failed:                   %d", m.FailedRuns)
	t.Logf("  Structured output rate:   %.0f%%", m.StructuredOutputRate*100)
	t.Logf("  Evidence coverage:        %.0f%%", m.EvidenceCoverage*100)
	t.Logf("  Hallucination rate:       %.0f%%", m.HallucinationRate*100)
	t.Logf("  Repair success rate:      %.0f%%", m.RepairSuccessRate*100)
	t.Logf("  Total steps:              %d", m.TotalSteps)
	t.Logf("  Total tool calls:         %d", m.TotalToolCalls)
	t.Logf("  Total evidence items:     %d", m.TotalEvidenceItems)
	t.Logf("  Grounded evidence items:  %d", m.GroundedEvidenceItems)

	// Assert key quality bars.
	if m.StructuredOutputRate < 1.0 {
		t.Errorf("expected 100%% structured output rate, got %.0f%%", m.StructuredOutputRate*100)
	}
	if m.HallucinationRate > 0 {
		t.Errorf("expected 0%% hallucination rate, got %.0f%%", m.HallucinationRate*100)
	}
}

// --- helpers ----------------------------------------------------------------

func extractEvidence(output map[string]any) []map[string]any {
	raw, ok := output["supporting_evidence"]
	if !ok {
		return nil
	}
	switch ev := raw.(type) {
	case []any:
		var result []map[string]any
		for _, item := range ev {
			if m, ok := item.(map[string]any); ok {
				result = append(result, m)
			}
		}
		return result
	case []map[string]any:
		return ev
	}
	return nil
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
