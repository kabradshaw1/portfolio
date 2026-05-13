package evalapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	defaultHTTPTimeout = 30 * time.Second
	errorExcerptLimit  = 256
)

type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

type Dataset struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	CreatedAt string `json:"created_at"`
	ItemCount int    `json:"item_count"`
}

type StartEvaluationRequest struct {
	DatasetID      string `json:"dataset_id"`
	Collection     string `json:"collection,omitempty"`
	Notes          string `json:"notes,omitempty"`
	BaselineEvalID string `json:"baseline_eval_id,omitempty"`
	Rerank         bool   `json:"rerank"`
}

type StartEvaluationResponse struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

type Scores struct {
	Faithfulness     *float64 `json:"faithfulness"`
	AnswerRelevancy  *float64 `json:"answer_relevancy"`
	ContextPrecision *float64 `json:"context_precision"`
	ContextRecall    *float64 `json:"context_recall"`
}

type QueryResult struct {
	Query        string            `json:"query"`
	Answer       string            `json:"answer"`
	Contexts     []string          `json:"contexts"`
	Scores       Scores            `json:"scores"`
	ScoreReasons map[string]string `json:"score_reasons,omitempty"`
}

type EvaluationDetail struct {
	ID              string         `json:"id"`
	DatasetID       string         `json:"dataset_id"`
	Status          string         `json:"status"`
	Collection      *string        `json:"collection"`
	AggregateScores *Scores        `json:"aggregate_scores"`
	Results         []QueryResult  `json:"results"`
	Error           *string        `json:"error"`
	CreatedAt       string         `json:"created_at"`
	CompletedAt     *string        `json:"completed_at"`
	Notes           *string        `json:"notes"`
	Config          map[string]any `json:"config"`
	BaselineEvalID  *string        `json:"baseline_eval_id"`
}

type Comparison struct {
	Runs   []EvaluationDetail   `json:"runs"`
	Deltas map[string][]float64 `json:"deltas"`
}

func New(baseURL, token string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: defaultHTTPTimeout}
	}
	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		token:      token,
		httpClient: httpClient,
	}
}

func (c *Client) ListDatasets(ctx context.Context) ([]Dataset, error) {
	var response struct {
		Datasets []Dataset `json:"datasets"`
	}
	if err := c.do(ctx, http.MethodGet, "/datasets", nil, &response); err != nil {
		return nil, err
	}
	return response.Datasets, nil
}

func (c *Client) StartEvaluation(ctx context.Context, body StartEvaluationRequest) (StartEvaluationResponse, error) {
	var response StartEvaluationResponse
	if err := c.do(ctx, http.MethodPost, "/evaluations", body, &response); err != nil {
		return StartEvaluationResponse{}, err
	}
	return response, nil
}

func (c *Client) GetEvaluation(ctx context.Context, id string) (EvaluationDetail, error) {
	var response EvaluationDetail
	path := "/evaluations/" + url.PathEscape(id)
	if err := c.do(ctx, http.MethodGet, path, nil, &response); err != nil {
		return EvaluationDetail{}, err
	}
	return response, nil
}

func (c *Client) CompareEvaluations(ctx context.Context, ids []string) (Comparison, error) {
	values := url.Values{}
	values.Set("ids", strings.Join(ids, ","))
	path := "/evaluations/compare?" + values.Encode()

	var response Comparison
	if err := c.do(ctx, http.MethodGet, path, nil, &response); err != nil {
		return Comparison{}, err
	}
	return response, nil
}

func (c *Client) do(ctx context.Context, method, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		var buf bytes.Buffer
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			return fmt.Errorf("%s %s: encode request: %w", method, path, err)
		}
		reader = &buf
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return fmt.Errorf("%s %s: create request: %w", method, path, err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		excerpt, _ := io.ReadAll(io.LimitReader(resp.Body, errorExcerptLimit))
		return fmt.Errorf("%s %s: status %d: %s", method, path, resp.StatusCode, strings.TrimSpace(string(excerpt)))
	}

	if out == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("%s %s: decode response: %w", method, path, err)
	}
	return nil
}
