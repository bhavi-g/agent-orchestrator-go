package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// OllamaClient talks to a local Ollama server.
type OllamaClient struct {
	baseURL string
	http    *http.Client
}

// NewOllamaClient creates a client pointing at the given base URL
// (e.g. "http://localhost:11434").
func NewOllamaClient(baseURL string) *OllamaClient {
	return &OllamaClient{
		baseURL: baseURL,
		http: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

// ollamaChatRequest is the Ollama /api/chat wire format.
type ollamaChatRequest struct {
	Model    string          `json:"model"`
	Messages []ollamaMessage `json:"messages"`
	Stream   bool            `json:"stream"`
	Format   string          `json:"format,omitempty"`
	Options  *ollamaOptions  `json:"options,omitempty"`
}

type ollamaMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ollamaOptions struct {
	Temperature float64 `json:"temperature,omitempty"`
	NumPredict  int     `json:"num_predict,omitempty"`
}

// ollamaChatResponse is the Ollama /api/chat wire format (non-streaming).
type ollamaChatResponse struct {
	Model   string        `json:"model"`
	Message ollamaMessage `json:"message"`
	// Token counts (Ollama returns these at the top level).
	PromptEvalCount int `json:"prompt_eval_count"`
	EvalCount       int `json:"eval_count"`
}

// Chat implements Client.
func (c *OllamaClient) Chat(ctx context.Context, req Request) (*Response, error) {
	msgs := make([]ollamaMessage, len(req.Messages))
	for i, m := range req.Messages {
		msgs[i] = ollamaMessage{Role: m.Role, Content: m.Content}
	}

	body := ollamaChatRequest{
		Model:    req.Model,
		Messages: msgs,
		Stream:   false,
		Format:   req.Format,
	}

	if req.Temperature > 0 || req.MaxTokens > 0 {
		body.Options = &ollamaOptions{
			Temperature: req.Temperature,
			NumPredict:  req.MaxTokens,
		}
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal ollama request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/chat", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("create http request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	httpResp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("ollama request failed: %w", err)
	}
	defer httpResp.Body.Close()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("read ollama response: %w", err)
	}

	if httpResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama returned %d: %s", httpResp.StatusCode, string(respBody))
	}

	var ollamaResp ollamaChatResponse
	if err := json.Unmarshal(respBody, &ollamaResp); err != nil {
		return nil, fmt.Errorf("decode ollama response: %w", err)
	}

	return &Response{
		Content:      ollamaResp.Message.Content,
		Model:        ollamaResp.Model,
		PromptTokens: ollamaResp.PromptEvalCount,
		OutputTokens: ollamaResp.EvalCount,
	}, nil
}
