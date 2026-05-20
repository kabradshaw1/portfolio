package triageapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const defaultHTTPTimeout = 30 * time.Second

type TokenProvider interface {
	Token(context.Context) (string, error)
	Invalidate()
}

type Client struct {
	baseURL       string
	tokenProvider TokenProvider
	httpClient    *http.Client
}

type TriageRequest struct {
	EvalID               string `json:"eval_id"`
	BaselineEvalID       string `json:"baseline_eval_id,omitempty"`
	Metric               string `json:"metric,omitempty"`
	Limit                int    `json:"limit,omitempty"`
	IncludeObservability bool   `json:"include_observability,omitempty"`
}

func New(baseURL string, tokenProvider TokenProvider, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: defaultHTTPTimeout}
	}
	return &Client{
		baseURL:       strings.TrimRight(baseURL, "/"),
		tokenProvider: tokenProvider,
		httpClient:    httpClient,
	}
}

func (c *Client) TriageRAGRegression(ctx context.Context, in TriageRequest) (map[string]any, error) {
	path := "/triage/eval-run"
	body := map[string]any{
		"eval_id": in.EvalID,
	}
	if in.BaselineEvalID != "" {
		path = "/triage/comparison"
		body = map[string]any{
			"baseline_eval_id":  in.BaselineEvalID,
			"candidate_eval_id": in.EvalID,
		}
	}
	if in.Metric != "" {
		body["metric"] = in.Metric
	}
	if in.Limit > 0 {
		body["limit"] = in.Limit
	}
	if in.IncludeObservability {
		body["include_observability"] = true
	}

	var out map[string]any
	if err := c.do(ctx, http.MethodPost, path, body, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) do(ctx context.Context, method, path string, body any, out any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("%s %s: encode request: %w", method, path, err)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("%s %s: create request: %w", method, path, err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.tokenProvider != nil {
		token, err := c.tokenProvider.Token(ctx)
		if err != nil {
			return fmt.Errorf("%s %s: token: %w", method, path, err)
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("%s %s: read response: %w", method, path, err)
	}
	if resp.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("%s %s: status %d: %s", method, path, resp.StatusCode, strings.TrimSpace(string(data)))
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("%s %s: decode response: %w", method, path, err)
	}
	return nil
}
