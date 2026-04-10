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

// OpenAIClient talks to any OpenAI-compatible API (OpenAI, Groq, Together, OpenRouter).
type OpenAIClient struct {
	baseURL string
	model   string
	apiKey  string
	http    *http.Client
}

// NewOpenAIClient returns a Client pointed at baseURL (e.g. "https://api.groq.com/openai/v1").
func NewOpenAIClient(baseURL, model, apiKey string) *OpenAIClient {
	return &OpenAIClient{
		baseURL: baseURL,
		model:   model,
		apiKey:  apiKey,
		http:    &http.Client{Timeout: 60 * time.Second},
	}
}

type openaiReq struct {
	Model    string        `json:"model"`
	Messages []openaiMsg   `json:"messages"`
	Tools    []openaiTool  `json:"tools,omitempty"`
	Stream   bool          `json:"stream"`
}

type openaiMsg struct {
	Role       string          `json:"role"`
	Content    string          `json:"content,omitempty"`
	ToolCalls  []openaiTC      `json:"tool_calls,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
	Name       string          `json:"name,omitempty"`
}

type openaiTC struct {
	ID       string   `json:"id"`
	Type     string   `json:"type"`
	Function openaiTF `json:"function"`
}

type openaiTF struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type openaiTool struct {
	Type     string      `json:"type"`
	Function openaiToolF `json:"function"`
}

type openaiToolF struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type openaiResp struct {
	Choices []struct {
		Message struct {
			Role      string     `json:"role"`
			Content   string     `json:"content"`
			ToolCalls []openaiTC `json:"tool_calls"`
		} `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
}

func (c *OpenAIClient) Chat(ctx context.Context, messages []Message, tools []ToolSchema) (ChatResponse, error) {
	// Convert messages to OpenAI format
	oaiMsgs := make([]openaiMsg, 0, len(messages))
	for _, m := range messages {
		msg := openaiMsg{
			Role:       string(m.Role),
			Content:    m.Content,
			ToolCallID: m.ToolCallID,
			Name:       m.Name,
		}
		for _, tc := range m.ToolCalls {
			msg.ToolCalls = append(msg.ToolCalls, openaiTC{
				ID:   tc.ID,
				Type: "function",
				Function: openaiTF{
					Name:      tc.Name,
					Arguments: string(tc.Args),
				},
			})
		}
		oaiMsgs = append(oaiMsgs, msg)
	}

	reqBody := openaiReq{
		Model:    c.model,
		Messages: oaiMsgs,
		Stream:   false,
	}
	for _, t := range tools {
		reqBody.Tools = append(reqBody.Tools, openaiTool{
			Type: "function",
			Function: openaiToolF{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.Parameters,
			},
		})
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return ChatResponse{}, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return ChatResponse{}, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	start := time.Now()
	resp, err := c.http.Do(req)
	if err != nil {
		return ChatResponse{}, fmt.Errorf("openai request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		payload, _ := io.ReadAll(resp.Body)
		return ChatResponse{}, fmt.Errorf("openai status %d: %s", resp.StatusCode, string(payload))
	}

	var parsed openaiResp
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return ChatResponse{}, fmt.Errorf("decode response: %w", err)
	}

	if len(parsed.Choices) == 0 {
		return ChatResponse{}, fmt.Errorf("openai returned no choices")
	}

	choice := parsed.Choices[0]
	out := ChatResponse{
		Content:         choice.Message.Content,
		PromptEvalCount: parsed.Usage.PromptTokens,
		EvalCount:       parsed.Usage.CompletionTokens,
		RequestDuration: time.Since(start),
	}
	for _, tc := range choice.Message.ToolCalls {
		out.ToolCalls = append(out.ToolCalls, ToolCall{
			ID:   tc.ID,
			Name: tc.Function.Name,
			Args: json.RawMessage(tc.Function.Arguments),
		})
	}
	return out, nil
}
