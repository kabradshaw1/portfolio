package clients

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	gobreaker "github.com/sony/gobreaker/v2"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/kabradshaw1/portfolio/go/pkg/resilience"
)

type SearchResult struct {
	Text       string  `json:"text"`
	Filename   string  `json:"filename"`
	PageNumber int     `json:"page_number"`
	Score      float64 `json:"score"`
}

type AskAnswer struct {
	Answer  string      `json:"answer"`
	Sources []AskSource `json:"sources"`
}

type AskSource struct {
	File string `json:"file"`
	Page int    `json:"page"`
}

type Collection struct {
	Name       string `json:"name"`
	PointCount int    `json:"point_count"`
}

type RAGClient struct {
	chatURL      string
	ingestionURL string
	http         *http.Client
	breaker      *gobreaker.CircuitBreaker[any]
	retryCfg     resilience.RetryConfig
}

func NewRAGClient(chatURL, ingestionURL string, breaker *gobreaker.CircuitBreaker[any]) *RAGClient {
	cfg := resilience.DefaultRetryConfig()
	cfg.IsRetryable = func(err error) bool {
		if err == nil {
			return false
		}
		// Don't retry 4xx (client errors).
		msg := err.Error()
		return !strings.Contains(msg, "status 4")
	}
	return &RAGClient{
		chatURL:      chatURL,
		ingestionURL: ingestionURL,
		// 30s timeout — longer than ecommerce's 5s because RAG includes LLM generation.
		http:     &http.Client{Timeout: 30 * time.Second, Transport: otelhttp.NewTransport(http.DefaultTransport)},
		breaker:  breaker,
		retryCfg: cfg,
	}
}

func (c *RAGClient) Search(ctx context.Context, query, collection string, limit int) ([]SearchResult, error) {
	body := map[string]any{"query": query, "limit": limit}
	if collection != "" {
		body["collection"] = collection
	}
	payload, _ := json.Marshal(body)

	return resilience.Call(ctx, c.breaker, c.retryCfg, func(ctx context.Context) ([]SearchResult, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.chatURL+"/search", bytes.NewReader(payload))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := c.http.Do(req)
		if err != nil {
			return nil, fmt.Errorf("rag search: %w", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 400 {
			b, _ := io.ReadAll(resp.Body)
			return nil, fmt.Errorf("rag search: status %d: %s", resp.StatusCode, string(b))
		}
		var result struct {
			Results []SearchResult `json:"results"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return nil, fmt.Errorf("decode search results: %w", err)
		}
		return result.Results, nil
	})
}

func (c *RAGClient) Ask(ctx context.Context, question, collection string) (AskAnswer, error) {
	body := map[string]any{"question": question}
	if collection != "" {
		body["collection"] = collection
	}
	payload, _ := json.Marshal(body)

	return resilience.Call(ctx, c.breaker, c.retryCfg, func(ctx context.Context) (AskAnswer, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.chatURL+"/chat", bytes.NewReader(payload))
		if err != nil {
			return AskAnswer{}, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")

		resp, err := c.http.Do(req)
		if err != nil {
			return AskAnswer{}, fmt.Errorf("rag ask: %w", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 400 {
			b, _ := io.ReadAll(resp.Body)
			return AskAnswer{}, fmt.Errorf("rag ask: status %d: %s", resp.StatusCode, string(b))
		}
		var answer AskAnswer
		if err := json.NewDecoder(resp.Body).Decode(&answer); err != nil {
			return AskAnswer{}, fmt.Errorf("decode ask answer: %w", err)
		}
		return answer, nil
	})
}

func (c *RAGClient) ListCollections(ctx context.Context) ([]Collection, error) {
	return resilience.Call(ctx, c.breaker, c.retryCfg, func(ctx context.Context) ([]Collection, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.ingestionURL+"/collections", nil)
		if err != nil {
			return nil, err
		}

		resp, err := c.http.Do(req)
		if err != nil {
			return nil, fmt.Errorf("list collections: %w", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 400 {
			b, _ := io.ReadAll(resp.Body)
			return nil, fmt.Errorf("list collections: status %d: %s", resp.StatusCode, string(b))
		}
		var result struct {
			Collections []Collection `json:"collections"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return nil, fmt.Errorf("decode collections: %w", err)
		}
		return result.Collections, nil
	})
}
