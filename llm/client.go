package llm

import "context"

// Message represents a single message in a conversation.
type Message struct {
	Role    string `json:"role"`    // "system", "user", "assistant"
	Content string `json:"content"`
}

// Request is the input to an LLM call.
type Request struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Temperature float64   `json:"temperature,omitempty"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
	// Format requests structured output (e.g. "json" for Ollama).
	Format string `json:"format,omitempty"`
}

// Response is the output of an LLM call.
type Response struct {
	Content      string `json:"content"`
	Model        string `json:"model"`
	PromptTokens int    `json:"prompt_tokens"`
	OutputTokens int    `json:"output_tokens"`
}

// Client abstracts the LLM backend (Ollama, OpenAI, etc.).
type Client interface {
	// Chat sends a conversation to the model and returns the reply.
	Chat(ctx context.Context, req Request) (*Response, error)
}
