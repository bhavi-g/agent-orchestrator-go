package llm

import "testing"

func TestPromptTemplate_Render(t *testing.T) {
	pt := MustPromptTemplate("test", "user", "Hello {{ .Name }}, you have {{ .Count }} items.")

	msg, err := pt.Render(struct {
		Name  string
		Count int
	}{"Alice", 3})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg.Role != "user" {
		t.Errorf("expected role user, got %s", msg.Role)
	}
	if msg.Content != "Hello Alice, you have 3 items." {
		t.Errorf("unexpected content: %s", msg.Content)
	}
}

func TestPromptTemplate_RenderError(t *testing.T) {
	pt := MustPromptTemplate("bad", "user", "{{ .Missing }}")

	_, err := pt.Render(struct{}{})
	if err == nil {
		t.Fatal("expected error for missing field")
	}
}

func TestMustPromptTemplate_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for invalid template")
		}
	}()
	MustPromptTemplate("bad", "user", "{{ .Bad")
}

func TestPlanSystemPrompt_Renders(t *testing.T) {
	type agentDesc struct {
		ID          string
		Description string
	}
	msg, err := PlanSystemPrompt.Render(struct{ Agents []agentDesc }{
		Agents: []agentDesc{
			{ID: "agent.read", Description: "reads files"},
			{ID: "agent.summarize", Description: "summarizes text"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg.Role != "system" {
		t.Errorf("expected system role, got %s", msg.Role)
	}
	if len(msg.Content) < 50 {
		t.Errorf("system prompt too short: %s", msg.Content)
	}
}

func TestPlanUserPrompt_Renders(t *testing.T) {
	msg, err := PlanUserPrompt.Render(struct {
		TaskID      string
		Description string
		Context     string
	}{TaskID: "task.analyze", Description: "analyze logs", Context: "prod env"})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg.Role != "user" {
		t.Errorf("expected user role, got %s", msg.Role)
	}
}
