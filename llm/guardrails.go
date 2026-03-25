package llm

import (
	"fmt"
	"strings"
)

// Guardrail inspects an LLM response and returns an error if it violates a rule.
type Guardrail interface {
	Check(resp *Response) error
}

// GuardrailChain runs a sequence of guardrails in order. The first violation
// stops the chain and returns the error.
type GuardrailChain struct {
	guards []Guardrail
}

// NewGuardrailChain creates a chain from the given guardrails.
func NewGuardrailChain(guards ...Guardrail) *GuardrailChain {
	return &GuardrailChain{guards: guards}
}

// Check runs every guardrail in order.
func (gc *GuardrailChain) Check(resp *Response) error {
	for _, g := range gc.guards {
		if err := g.Check(resp); err != nil {
			return err
		}
	}
	return nil
}

// --- Concrete guardrails ---

// MaxTokenGuardrail rejects responses exceeding a token budget.
type MaxTokenGuardrail struct {
	Limit int
}

func (g *MaxTokenGuardrail) Check(resp *Response) error {
	if resp.OutputTokens > g.Limit {
		return fmt.Errorf("guardrail: response used %d tokens, limit is %d", resp.OutputTokens, g.Limit)
	}
	return nil
}

// EmptyResponseGuardrail rejects blank responses.
type EmptyResponseGuardrail struct{}

func (g *EmptyResponseGuardrail) Check(resp *Response) error {
	if strings.TrimSpace(resp.Content) == "" {
		return fmt.Errorf("guardrail: LLM returned empty response")
	}
	return nil
}

// JSONGuardrail ensures the response is valid JSON and passes a Schema.
type JSONGuardrail struct {
	Schema *Schema
}

func (g *JSONGuardrail) Check(resp *Response) error {
	content := extractJSON(resp.Content)
	errs := g.Schema.ValidateJSON([]byte(content))
	if len(errs) > 0 {
		msgs := make([]string, len(errs))
		for i, e := range errs {
			msgs[i] = e.Error()
		}
		return fmt.Errorf("guardrail: schema validation failed: %s", strings.Join(msgs, "; "))
	}
	return nil
}

// BlockedContentGuardrail rejects responses containing any of the blocked phrases.
type BlockedContentGuardrail struct {
	Phrases []string
}

func (g *BlockedContentGuardrail) Check(resp *Response) error {
	lower := strings.ToLower(resp.Content)
	for _, p := range g.Phrases {
		if strings.Contains(lower, strings.ToLower(p)) {
			return fmt.Errorf("guardrail: response contains blocked content")
		}
	}
	return nil
}

// extractJSON tries to pull a JSON array or object from text that might
// contain markdown fences or leading prose.
func extractJSON(s string) string {
	s = strings.TrimSpace(s)

	// Strip markdown code fences
	if strings.HasPrefix(s, "```") {
		if idx := strings.Index(s[3:], "\n"); idx >= 0 {
			s = s[3+idx+1:]
		}
		if idx := strings.LastIndex(s, "```"); idx >= 0 {
			s = s[:idx]
		}
		s = strings.TrimSpace(s)
	}

	// Find first [ or { and last ] or }
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
