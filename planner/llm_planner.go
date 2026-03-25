package planner

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"agent-orchestrator/llm"
)

// AgentDescriptor tells the LLM planner what agents are available.
type AgentDescriptor struct {
	ID          string
	Description string
}

// LLMPlanner uses an LLM to generate and revise execution plans.
// It implements both Planner and Replanner.
type LLMPlanner struct {
	client     llm.Client
	model      string
	agents     []AgentDescriptor
	guardrails *llm.GuardrailChain
}

// LLMPlannerOption configures the LLMPlanner.
type LLMPlannerOption func(*LLMPlanner)

// WithModel sets the Ollama model name (default: "llama3").
func WithModel(model string) LLMPlannerOption {
	return func(p *LLMPlanner) { p.model = model }
}

// WithGuardrails attaches a guardrail chain to every LLM call.
func WithGuardrails(gc *llm.GuardrailChain) LLMPlannerOption {
	return func(p *LLMPlanner) { p.guardrails = gc }
}

// NewLLMPlanner creates a planner backed by the given LLM client.
func NewLLMPlanner(client llm.Client, agents []AgentDescriptor, opts ...LLMPlannerOption) *LLMPlanner {
	p := &LLMPlanner{
		client: client,
		model:  "llama3",
		agents: agents,
		guardrails: llm.NewGuardrailChain(
			&llm.EmptyResponseGuardrail{},
			&llm.JSONGuardrail{Schema: llm.PlanStepSchema},
		),
	}
	for _, o := range opts {
		o(p)
	}
	return p
}

// CreatePlan implements Planner.
func (p *LLMPlanner) CreatePlan(ctx context.Context, taskID string) (*Plan, error) {
	systemMsg, err := llm.PlanSystemPrompt.Render(struct{ Agents []AgentDescriptor }{p.agents})
	if err != nil {
		return nil, fmt.Errorf("render system prompt: %w", err)
	}

	userMsg, err := llm.PlanUserPrompt.Render(struct {
		TaskID      string
		Description string
		Context     string
	}{TaskID: taskID})
	if err != nil {
		return nil, fmt.Errorf("render user prompt: %w", err)
	}

	resp, err := p.client.Chat(ctx, llm.Request{
		Model:    p.model,
		Messages: []llm.Message{systemMsg, userMsg},
		Format:   "json",
	})
	if err != nil {
		return nil, fmt.Errorf("llm chat: %w", err)
	}

	// Run guardrails
	if p.guardrails != nil {
		if err := p.guardrails.Check(resp); err != nil {
			return nil, err
		}
	}

	steps, err := parsePlanSteps(resp.Content, p.agents)
	if err != nil {
		return nil, err
	}

	return &Plan{TaskID: taskID, Steps: steps}, nil
}

// Replan implements Replanner.
func (p *LLMPlanner) Replan(ctx context.Context, rctx ReplanContext) (*Plan, error) {
	if rctx.Attempt > rctx.MaxReplans {
		return nil, fmt.Errorf("exceeded max replans (%d)", rctx.MaxReplans)
	}

	systemMsg, err := llm.ReplanSystemPrompt.Render(struct{ Agents []AgentDescriptor }{p.agents})
	if err != nil {
		return nil, fmt.Errorf("render replan system prompt: %w", err)
	}

	type completedInfo struct {
		StepIndex int
		AgentID   string
	}
	completed := make([]completedInfo, len(rctx.CompletedSteps))
	for i, cs := range rctx.CompletedSteps {
		completed[i] = completedInfo{StepIndex: cs.StepIndex, AgentID: cs.AgentID}
	}

	userMsg, err := llm.ReplanUserPrompt.Render(struct {
		TaskID          string
		FailedStepIndex int
		FailedAgentID   string
		FailureError    string
		FailureType     string
		Attempt         int
		MaxReplans      int
		CompletedSteps  []completedInfo
	}{
		TaskID:          rctx.TaskID,
		FailedStepIndex: rctx.FailedStepIndex,
		FailedAgentID:   rctx.FailedAgentID,
		FailureError:    rctx.FailureError,
		FailureType:     rctx.FailureType,
		Attempt:         rctx.Attempt,
		MaxReplans:      rctx.MaxReplans,
		CompletedSteps:  completed,
	})
	if err != nil {
		return nil, fmt.Errorf("render replan user prompt: %w", err)
	}

	resp, err := p.client.Chat(ctx, llm.Request{
		Model:    p.model,
		Messages: []llm.Message{systemMsg, userMsg},
		Format:   "json",
	})
	if err != nil {
		return nil, fmt.Errorf("llm replan chat: %w", err)
	}

	if p.guardrails != nil {
		if err := p.guardrails.Check(resp); err != nil {
			return nil, err
		}
	}

	steps, err := parsePlanSteps(resp.Content, p.agents)
	if err != nil {
		return nil, err
	}

	return &Plan{TaskID: rctx.TaskID, Steps: steps}, nil
}

// ---- JSON parsing + validation ------------------------------------------------

// rawStep is the JSON shape the LLM is expected to produce.
type rawStep struct {
	AgentID   string         `json:"agent_id"`
	Input     map[string]any `json:"input,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	StepID    string         `json:"step_id,omitempty"`
	DependsOn []string       `json:"depends_on,omitempty"`
}

// parsePlanSteps extracts plan steps from the LLM's JSON response,
// validates agent IDs against the known list, and returns typed PlanSteps.
func parsePlanSteps(content string, agents []AgentDescriptor) ([]PlanStep, error) {
	body := extractJSON(content)

	var raw []rawStep
	if err := json.Unmarshal([]byte(body), &raw); err != nil {
		return nil, fmt.Errorf("parse plan JSON: %w", err)
	}

	if len(raw) == 0 {
		return nil, fmt.Errorf("LLM returned empty plan")
	}

	knownAgents := make(map[string]bool, len(agents))
	for _, a := range agents {
		knownAgents[a.ID] = true
	}

	steps := make([]PlanStep, 0, len(raw))
	for i, r := range raw {
		if r.AgentID == "" {
			return nil, fmt.Errorf("step %d: missing agent_id", i)
		}
		if !knownAgents[r.AgentID] {
			return nil, fmt.Errorf("step %d: unknown agent_id %q", i, r.AgentID)
		}
		steps = append(steps, PlanStep{
			AgentID:   r.AgentID,
			Input:     r.Input,
			Metadata:  r.Metadata,
			StepID:    r.StepID,
			DependsOn: r.DependsOn,
		})
	}

	return steps, nil
}

// extractJSON pulls JSON from text that may contain markdown fences.
func extractJSON(s string) string {
	s = strings.TrimSpace(s)

	if strings.HasPrefix(s, "```") {
		if idx := strings.Index(s[3:], "\n"); idx >= 0 {
			s = s[3+idx+1:]
		}
		if idx := strings.LastIndex(s, "```"); idx >= 0 {
			s = s[:idx]
		}
		s = strings.TrimSpace(s)
	}

	startArr := strings.Index(s, "[")
	startObj := strings.Index(s, "{")
	start := -1
	endChar := byte(']')

	if startArr >= 0 && (startObj < 0 || startArr < startObj) {
		start = startArr
		endChar = ']'
	} else if startObj >= 0 {
		start = startObj
		endChar = '}'
	}

	if start < 0 {
		return s
	}

	end := strings.LastIndexByte(s, endChar)
	if end < start {
		return s
	}

	return s[start : end+1]
}
