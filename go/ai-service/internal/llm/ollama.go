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
	"strings"
	"time"

	gobreaker "github.com/sony/gobreaker/v2"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/kabradshaw1/portfolio/go/pkg/resilience"
)

// OllamaClient talks to an Ollama server's /api/chat endpoint using
// the OpenAI-compatible tool-calling shape that Qwen 2.5 supports.
type OllamaClient struct {
	baseURL  string
	model    string
	http     *http.Client
	breaker  *gobreaker.CircuitBreaker[any]
	retryCfg resilience.RetryConfig
}

// NewOllamaClient returns a Client pointed at baseURL (e.g. "http://ollama:11434").
func NewOllamaClient(baseURL, model string, breaker *gobreaker.CircuitBreaker[any]) *OllamaClient {
	cfg := resilience.RetryConfig{
		MaxAttempts: 3,
		BaseDelay:   500 * time.Millisecond,
		MaxDelay:    5 * time.Second,
		IsRetryable: func(err error) bool {
			if err == nil {
				return false
			}
			msg := err.Error()
			// Don't retry 4xx.
			return !strings.Contains(msg, "status 4")
		},
	}
	return &OllamaClient{
		baseURL:  baseURL,
		model:    model,
		http:     &http.Client{Timeout: 60 * time.Second, Transport: otelhttp.NewTransport(http.DefaultTransport)},
		breaker:  breaker,
		retryCfg: cfg,
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
	Done            bool `json:"done"`
	PromptEvalCount int  `json:"prompt_eval_count"`
	EvalCount       int  `json:"eval_count"`
	EvalDuration    int  `json:"eval_duration"` // nanoseconds
}

func (c *OllamaClient) Chat(ctx context.Context, messages []Message, tools []ToolSchema) (ChatResponse, error) {
	return resilience.Call(ctx, c.breaker, c.retryCfg, func(ctx context.Context) (ChatResponse, error) {
		ctx, span := otel.Tracer("llm").Start(ctx, "ollama.chat",
			trace.WithAttributes(attribute.String("llm.model", c.model)),
		)
		defer span.End()
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

		start := time.Now()
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

		span.SetAttributes(
			attribute.Int("llm.prompt_eval_count", parsed.PromptEvalCount),
			attribute.Int("llm.eval_count", parsed.EvalCount),
		)

		out := ChatResponse{
			Content:         parsed.Message.Content,
			PromptEvalCount: parsed.PromptEvalCount,
			EvalCount:       parsed.EvalCount,
			EvalDurationNs:  parsed.EvalDuration,
			RequestDuration: time.Since(start),
		}
		for _, tc := range parsed.Message.ToolCalls {
			out.ToolCalls = append(out.ToolCalls, ToolCall{
				ID:   newCallID(),
				Name: tc.Function.Name,
				Args: tc.Function.Arguments,
			})
		}
		return out, nil
	})
}

func newCallID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return "call_" + hex.EncodeToString(b[:])
}
