package llm

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"
)

// PromptTemplate is a reusable, parameterized prompt.
type PromptTemplate struct {
	name     string
	role     string // message role: "system" or "user"
	tmpl     *template.Template
}

// NewPromptTemplate compiles a Go text/template with the given name and role.
func NewPromptTemplate(name, role, text string) (*PromptTemplate, error) {
	t, err := template.New(name).Parse(text)
	if err != nil {
		return nil, fmt.Errorf("parse template %s: %w", name, err)
	}
	return &PromptTemplate{name: name, role: role, tmpl: t}, nil
}

// MustPromptTemplate is like NewPromptTemplate but panics on error.
func MustPromptTemplate(name, role, text string) *PromptTemplate {
	pt, err := NewPromptTemplate(name, role, text)
	if err != nil {
		panic(err)
	}
	return pt
}

// Render executes the template with the given data and returns a Message.
func (pt *PromptTemplate) Render(data any) (Message, error) {
	var buf bytes.Buffer
	if err := pt.tmpl.Execute(&buf, data); err != nil {
		return Message{}, fmt.Errorf("render template %s: %w", pt.name, err)
	}
	return Message{Role: pt.role, Content: buf.String()}, nil
}

// ---- Built-in planner prompts ------------------------------------------------

// PlanSystemPrompt instructs the LLM to act as a planner.
var PlanSystemPrompt = MustPromptTemplate("plan_system", "system", strings.TrimSpace(`
You are a task planner for an agent orchestration system.
Given a task description and a list of available agents, produce an execution plan as a JSON array.

Available agents:
{{- range .Agents }}
- {{ .ID }}: {{ .Description }}
{{- end }}

Rules:
1. Return ONLY a JSON array of step objects.
2. Each step object MUST have an "agent_id" field (string) matching one of the available agents above.
3. Each step MAY have an "input" field (object) with key-value pairs the agent needs.
4. Each step MAY have a "metadata" field (object) for extra context.
5. Order the steps so that later steps can use outputs of earlier steps.
6. Do NOT include explanation text — return raw JSON only.

Example output:
[
  {"agent_id": "agent.read_file", "input": {"path": "/var/log/app.log"}},
  {"agent_id": "agent.summarize", "input": {"focus": "errors"}}
]
`))

// PlanUserPrompt provides the task for plan generation.
var PlanUserPrompt = MustPromptTemplate("plan_user", "user", strings.TrimSpace(`
Task: {{ .TaskID }}
{{ if .Description }}Description: {{ .Description }}{{ end }}
{{ if .Context }}Additional context: {{ .Context }}{{ end }}
`))

// ReplanSystemPrompt instructs the LLM to replan after a failure.
var ReplanSystemPrompt = MustPromptTemplate("replan_system", "system", strings.TrimSpace(`
You are a task replanner for an agent orchestration system.
A previous execution plan failed. Analyze the failure and produce a NEW plan (JSON array) for the remaining work.

Available agents:
{{- range .Agents }}
- {{ .ID }}: {{ .Description }}
{{- end }}

Rules:
1. Return ONLY a JSON array of step objects.
2. Each step MUST have "agent_id" matching an available agent.
3. Steps MAY have "input" and "metadata" fields.
4. Do NOT repeat steps that already completed successfully.
5. Address the failure if possible (e.g. use a different agent or approach).
6. Return raw JSON only — no explanation text.
`))

// ReplanUserPrompt provides failure context to the replanner.
var ReplanUserPrompt = MustPromptTemplate("replan_user", "user", strings.TrimSpace(`
Task: {{ .TaskID }}
Failure at step {{ .FailedStepIndex }} (agent: {{ .FailedAgentID }}): {{ .FailureError }}
Failure type: {{ .FailureType }}
Replan attempt: {{ .Attempt }}/{{ .MaxReplans }}

Completed steps so far:
{{- range .CompletedSteps }}
- Step {{ .StepIndex }} ({{ .AgentID }}): succeeded
{{- end }}
{{ if not .CompletedSteps }}- (none){{ end }}
`))
