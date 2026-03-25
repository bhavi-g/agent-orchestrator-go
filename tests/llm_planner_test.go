package tests

import (
	"context"
	"errors"
	"testing"

	"agent-orchestrator/llm"
	"agent-orchestrator/planner"
)

// mockLLMClient returns a canned response for every Chat call.
type mockLLMClient struct {
	response string
	err      error
	calls    int
}

func (m *mockLLMClient) Chat(_ context.Context, _ llm.Request) (*llm.Response, error) {
	m.calls++
	if m.err != nil {
		return nil, m.err
	}
	return &llm.Response{
		Content:      m.response,
		Model:        "mock",
		PromptTokens: 10,
		OutputTokens: 5,
	}, nil
}

var testAgents = []planner.AgentDescriptor{
	{ID: "agent.read_file", Description: "reads files from disk"},
	{ID: "agent.summarize", Description: "summarizes text"},
	{ID: "agent.echo", Description: "echoes input back"},
}

// --- CreatePlan tests ---

func TestLLMPlanner_CreatePlan_ValidJSON(t *testing.T) {
	mock := &mockLLMClient{
		response: `[{"agent_id": "agent.read_file", "input": {"path": "/var/log/app.log"}}, {"agent_id": "agent.summarize"}]`,
	}

	p := planner.NewLLMPlanner(mock, testAgents)
	plan, err := p.CreatePlan(context.Background(), "task.analyze_logs")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if plan.TaskID != "task.analyze_logs" {
		t.Errorf("expected taskID task.analyze_logs, got %s", plan.TaskID)
	}
	if len(plan.Steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(plan.Steps))
	}
	if plan.Steps[0].AgentID != "agent.read_file" {
		t.Errorf("step 0: expected agent.read_file, got %s", plan.Steps[0].AgentID)
	}
	if plan.Steps[0].Input["path"] != "/var/log/app.log" {
		t.Errorf("step 0: expected path input, got %v", plan.Steps[0].Input)
	}
	if plan.Steps[1].AgentID != "agent.summarize" {
		t.Errorf("step 1: expected agent.summarize, got %s", plan.Steps[1].AgentID)
	}
}

func TestLLMPlanner_CreatePlan_MarkdownFenced(t *testing.T) {
	mock := &mockLLMClient{
		response: "```json\n[{\"agent_id\": \"agent.echo\"}]\n```",
	}

	p := planner.NewLLMPlanner(mock, testAgents)
	plan, err := p.CreatePlan(context.Background(), "task.test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(plan.Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(plan.Steps))
	}
	if plan.Steps[0].AgentID != "agent.echo" {
		t.Errorf("expected agent.echo, got %s", plan.Steps[0].AgentID)
	}
}

func TestLLMPlanner_CreatePlan_UnknownAgent(t *testing.T) {
	mock := &mockLLMClient{
		response: `[{"agent_id": "agent.unknown"}]`,
	}

	p := planner.NewLLMPlanner(mock, testAgents)
	_, err := p.CreatePlan(context.Background(), "task.test")
	if err == nil {
		t.Fatal("expected error for unknown agent")
	}
}

func TestLLMPlanner_CreatePlan_EmptyResponse(t *testing.T) {
	mock := &mockLLMClient{response: ""}

	p := planner.NewLLMPlanner(mock, testAgents)
	_, err := p.CreatePlan(context.Background(), "task.test")
	if err == nil {
		t.Fatal("expected error for empty response")
	}
}

func TestLLMPlanner_CreatePlan_LLMError(t *testing.T) {
	mock := &mockLLMClient{err: errors.New("connection refused")}

	p := planner.NewLLMPlanner(mock, testAgents)
	_, err := p.CreatePlan(context.Background(), "task.test")
	if err == nil {
		t.Fatal("expected error when LLM fails")
	}
}

func TestLLMPlanner_CreatePlan_InvalidJSON(t *testing.T) {
	mock := &mockLLMClient{response: "Sure! Here's a plan for you..."}

	p := planner.NewLLMPlanner(mock, testAgents)
	_, err := p.CreatePlan(context.Background(), "task.test")
	if err == nil {
		t.Fatal("expected error for non-JSON response")
	}
}

func TestLLMPlanner_CreatePlan_MissingAgentID(t *testing.T) {
	mock := &mockLLMClient{response: `[{"input": {"path": "/tmp"}}]`}

	p := planner.NewLLMPlanner(mock, testAgents)
	_, err := p.CreatePlan(context.Background(), "task.test")
	if err == nil {
		t.Fatal("expected error for missing agent_id")
	}
}

func TestLLMPlanner_CreatePlan_EmptyArray(t *testing.T) {
	mock := &mockLLMClient{response: `[]`}

	p := planner.NewLLMPlanner(mock, testAgents)
	_, err := p.CreatePlan(context.Background(), "task.test")
	if err == nil {
		t.Fatal("expected error for empty plan array")
	}
}

func TestLLMPlanner_CreatePlan_WithModel(t *testing.T) {
	mock := &mockLLMClient{
		response: `[{"agent_id": "agent.echo"}]`,
	}

	p := planner.NewLLMPlanner(mock, testAgents, planner.WithModel("mistral"))
	plan, err := p.CreatePlan(context.Background(), "task.test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(plan.Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(plan.Steps))
	}
}

// --- Replan tests ---

func TestLLMPlanner_Replan_ValidJSON(t *testing.T) {
	mock := &mockLLMClient{
		response: `[{"agent_id": "agent.echo", "metadata": {"replan": true}}]`,
	}

	p := planner.NewLLMPlanner(mock, testAgents)
	plan, err := p.Replan(context.Background(), planner.ReplanContext{
		TaskID:          "task.test",
		FailedStepIndex: 0,
		FailedAgentID:   "agent.read_file",
		FailureError:    "file not found",
		FailureType:     "tool_failure",
		Attempt:         1,
		MaxReplans:      3,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(plan.Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(plan.Steps))
	}
	if plan.Steps[0].AgentID != "agent.echo" {
		t.Errorf("expected agent.echo, got %s", plan.Steps[0].AgentID)
	}
}

func TestLLMPlanner_Replan_ExceedsMaxReplans(t *testing.T) {
	mock := &mockLLMClient{response: `[{"agent_id": "agent.echo"}]`}

	p := planner.NewLLMPlanner(mock, testAgents)
	_, err := p.Replan(context.Background(), planner.ReplanContext{
		TaskID:     "task.test",
		Attempt:    4,
		MaxReplans: 3,
	})
	if err == nil {
		t.Fatal("expected error for exceeding max replans")
	}
	// LLM should not have been called
	if mock.calls != 0 {
		t.Errorf("expected 0 LLM calls, got %d", mock.calls)
	}
}

func TestLLMPlanner_Replan_LLMError(t *testing.T) {
	mock := &mockLLMClient{err: errors.New("timeout")}

	p := planner.NewLLMPlanner(mock, testAgents)
	_, err := p.Replan(context.Background(), planner.ReplanContext{
		TaskID:     "task.test",
		Attempt:    1,
		MaxReplans: 3,
	})
	if err == nil {
		t.Fatal("expected error when LLM fails")
	}
}

// --- Guardrail integration ---

func TestLLMPlanner_CustomGuardrails(t *testing.T) {
	mock := &mockLLMClient{
		response: `[{"agent_id": "agent.echo"}]`,
	}

	chain := llm.NewGuardrailChain(
		&llm.EmptyResponseGuardrail{},
		&llm.BlockedContentGuardrail{Phrases: []string{"DROP TABLE"}},
		&llm.JSONGuardrail{Schema: llm.PlanStepSchema},
	)

	p := planner.NewLLMPlanner(mock, testAgents, planner.WithGuardrails(chain))
	plan, err := p.CreatePlan(context.Background(), "task.test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(plan.Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(plan.Steps))
	}
}

func TestLLMPlanner_GuardrailBlocks(t *testing.T) {
	mock := &mockLLMClient{
		response: `DROP TABLE users; [{"agent_id": "agent.echo"}]`,
	}

	chain := llm.NewGuardrailChain(
		&llm.EmptyResponseGuardrail{},
		&llm.BlockedContentGuardrail{Phrases: []string{"DROP TABLE"}},
	)

	p := planner.NewLLMPlanner(mock, testAgents, planner.WithGuardrails(chain))
	_, err := p.CreatePlan(context.Background(), "task.test")
	if err == nil {
		t.Fatal("expected error from blocked content guardrail")
	}
}
