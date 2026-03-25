package llm

import (
	"strings"
	"testing"
)

func TestEmptyResponseGuardrail(t *testing.T) {
	g := &EmptyResponseGuardrail{}

	if err := g.Check(&Response{Content: "hello"}); err != nil {
		t.Errorf("unexpected error for non-empty: %v", err)
	}
	if err := g.Check(&Response{Content: ""}); err == nil {
		t.Fatal("expected error for empty response")
	}
	if err := g.Check(&Response{Content: "   "}); err == nil {
		t.Fatal("expected error for whitespace-only response")
	}
}

func TestMaxTokenGuardrail(t *testing.T) {
	g := &MaxTokenGuardrail{Limit: 100}

	if err := g.Check(&Response{OutputTokens: 50}); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if err := g.Check(&Response{OutputTokens: 150}); err == nil {
		t.Fatal("expected error for exceeding token limit")
	}
}

func TestJSONGuardrail_Valid(t *testing.T) {
	g := &JSONGuardrail{Schema: PlanStepSchema}

	resp := &Response{Content: `[{"agent_id": "agent.read"}]`}
	if err := g.Check(resp); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestJSONGuardrail_Invalid(t *testing.T) {
	g := &JSONGuardrail{Schema: PlanStepSchema}

	resp := &Response{Content: `not json`}
	if err := g.Check(resp); err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestJSONGuardrail_WithMarkdownFences(t *testing.T) {
	g := &JSONGuardrail{Schema: PlanStepSchema}

	resp := &Response{Content: "```json\n[{\"agent_id\": \"agent.read\"}]\n```"}
	if err := g.Check(resp); err != nil {
		t.Errorf("unexpected error for fenced JSON: %v", err)
	}
}

func TestBlockedContentGuardrail(t *testing.T) {
	g := &BlockedContentGuardrail{Phrases: []string{"DROP TABLE", "rm -rf"}}

	if err := g.Check(&Response{Content: "analyze the logs"}); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if err := g.Check(&Response{Content: "run DROP TABLE users"}); err == nil {
		t.Fatal("expected error for blocked content")
	}
	// Case insensitive
	if err := g.Check(&Response{Content: "run drop table users"}); err == nil {
		t.Fatal("expected error for case-insensitive blocked content")
	}
}

func TestGuardrailChain_AllPass(t *testing.T) {
	chain := NewGuardrailChain(
		&EmptyResponseGuardrail{},
		&MaxTokenGuardrail{Limit: 1000},
	)
	if err := chain.Check(&Response{Content: "hello", OutputTokens: 10}); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestGuardrailChain_FirstFailureStops(t *testing.T) {
	chain := NewGuardrailChain(
		&EmptyResponseGuardrail{},
		&MaxTokenGuardrail{Limit: 1000},
	)
	err := chain.Check(&Response{Content: "", OutputTokens: 10})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "empty response") {
		t.Errorf("expected empty response error, got: %v", err)
	}
}

func TestExtractJSON_PlainArray(t *testing.T) {
	got := extractJSON(`[{"a":1}]`)
	if got != `[{"a":1}]` {
		t.Errorf("unexpected: %s", got)
	}
}

func TestExtractJSON_MarkdownFenced(t *testing.T) {
	input := "```json\n[{\"a\":1}]\n```"
	got := extractJSON(input)
	if got != `[{"a":1}]` {
		t.Errorf("unexpected: %s", got)
	}
}

func TestExtractJSON_LeadingProse(t *testing.T) {
	input := "Here is the plan:\n[{\"agent_id\": \"agent.read\"}]"
	got := extractJSON(input)
	if got != `[{"agent_id": "agent.read"}]` {
		t.Errorf("unexpected: %s", got)
	}
}

func TestExtractJSON_Object(t *testing.T) {
	input := `some text {"key": "val"} done`
	got := extractJSON(input)
	if got != `{"key": "val"}` {
		t.Errorf("unexpected: %s", got)
	}
}
