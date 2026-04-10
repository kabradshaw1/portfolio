package llm

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// OllamaClient talks to an Ollama server's /api/chat endpoint using
// the OpenAI-compatible tool-calling shape that Qwen 2.5 supports.
type OllamaClient struct {
	baseURL string
	model   string
	http    *http.Client
}

// NewOllamaClient returns a Client pointed at baseURL (e.g. "http://ollama:11434").
func NewOllamaClient(baseURL, model string) *OllamaClient {
	return &OllamaClient{
		baseURL: baseURL,
		model:   model,
		http:    &http.Client{Timeout: 60 * time.Second},
	}
}

type ollamaTool struct {
	Type     string      `json:"type"`
	Function ollamaToolF `json:"function"`
}
type ollamaToolF struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type ollamaReq struct {
	Model    string       `json:"model"`
	Messages []Message    `json:"messages"`
	Tools    []ollamaTool `json:"tools,omitempty"`
	Stream   bool         `json:"stream"`
}

type ollamaResp struct {
	Message struct {
		Role      Role   `json:"role"`
		Content   string `json:"content"`
		ToolCalls []struct {
			Function struct {
				Name      string          `json:"name"`
				Arguments json.RawMessage `json:"arguments"`
			} `json:"function"`
		} `json:"tool_calls"`
	} `json:"message"`
	Done bool `json:"done"`
}

func (c *OllamaClient) Chat(ctx context.Context, messages []Message, tools []ToolSchema) (ChatResponse, error) {
	reqBody := ollamaReq{
		Model:    c.model,
		Messages: messages,
		Stream:   false,
	}
	for _, t := range tools {
		reqBody.Tools = append(reqBody.Tools, ollamaTool{
			Type:     "function",
			Function: ollamaToolF(t),
		})
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return ChatResponse{}, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return ChatResponse{}, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return ChatResponse{}, fmt.Errorf("ollama request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		payload, _ := io.ReadAll(resp.Body)
		return ChatResponse{}, fmt.Errorf("ollama status %d: %s", resp.StatusCode, string(payload))
	}

	var parsed ollamaResp
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return ChatResponse{}, fmt.Errorf("decode response: %w", err)
	}

	out := ChatResponse{Content: parsed.Message.Content}
	for _, tc := range parsed.Message.ToolCalls {
		out.ToolCalls = append(out.ToolCalls, ToolCall{
			ID:   newCallID(),
			Name: tc.Function.Name,
			Args: tc.Function.Arguments,
		})
	}
	return out, nil
}

func newCallID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return "call_" + hex.EncodeToString(b[:])
}
