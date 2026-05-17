package evalapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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
	baseURL           string
	tokenProvider     TokenProvider
	retryUnauthorized bool
	httpClient        *http.Client
}

type TokenProvider interface {
	Token(context.Context) (string, error)
	Invalidate()
}

type staticTokenProvider struct {
	token string
}

func (p staticTokenProvider) Token(context.Context) (string, error) {
	return p.token, nil
}

func (p staticTokenProvider) Invalidate() {}

type HTTPError struct {
	Method     string
	Path       string
	StatusCode int
	Excerpt    string
	RetryAfter time.Duration
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("%s %s: status %d: %s", e.Method, e.Path, e.StatusCode, e.Excerpt)
}

type Dataset struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	CreatedAt string `json:"created_at"`
	ItemCount int    `json:"item_count"`
}

type GoldenItem struct {
	Query           string   `json:"query"`
	ExpectedAnswer  string   `json:"expected_answer"`
	ExpectedSources []string `json:"expected_sources"`
}

type CreateDatasetRequest struct {
	Name  string       `json:"name"`
	Items []GoldenItem `json:"items"`
}

type CreateDatasetResponse struct {
	ID string `json:"id"`
}

type RetrievalConfig struct {
	TopK *int `json:"top_k,omitempty"`
}

type StartEvaluationRequest struct {
	DatasetID       string           `json:"dataset_id"`
	Collection      string           `json:"collection,omitempty"`
	Notes           string           `json:"notes,omitempty"`
	BaselineEvalID  string           `json:"baseline_eval_id,omitempty"`
	ExperimentID    string           `json:"experiment_id,omitempty"`
	ExperimentLabel string           `json:"experiment_label,omitempty"`
	Rerank          bool             `json:"rerank"`
	RetrievalConfig *RetrievalConfig `json:"retrieval_config,omitempty"`
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

type Experiment struct {
	ID             string          `json:"id"`
	Name           string          `json:"name"`
	Hypothesis     string          `json:"hypothesis"`
	DatasetID      string          `json:"dataset_id"`
	Collection     string          `json:"collection"`
	BaselineEvalID *string         `json:"baseline_eval_id"`
	FocusMetric    string          `json:"focus_metric"`
	Status         string          `json:"status"`
	Decision       string          `json:"decision"`
	Conclusion     string          `json:"conclusion"`
	Evidence       map[string]any  `json:"evidence"`
	Notes          *string         `json:"notes"`
	CreatedAt      string          `json:"created_at"`
	UpdatedAt      string          `json:"updated_at"`
	Runs           []ExperimentRun `json:"runs,omitempty"`
}

type ExperimentRun struct {
	EvaluationID string           `json:"evaluation_id"`
	Label        string           `json:"label"`
	Notes        *string          `json:"notes"`
	AttachedAt   string           `json:"attached_at"`
	Evaluation   EvaluationDetail `json:"evaluation"`
}

type CreateExperimentRequest struct {
	Name           string `json:"name"`
	Hypothesis     string `json:"hypothesis"`
	DatasetID      string `json:"dataset_id"`
	Collection     string `json:"collection"`
	BaselineEvalID string `json:"baseline_eval_id,omitempty"`
	FocusMetric    string `json:"focus_metric,omitempty"`
	Status         string `json:"status,omitempty"`
	Notes          string `json:"notes,omitempty"`
}

type UpdateExperimentRequest struct {
	Hypothesis     string         `json:"hypothesis,omitempty"`
	BaselineEvalID string         `json:"baseline_eval_id,omitempty"`
	FocusMetric    string         `json:"focus_metric,omitempty"`
	Status         string         `json:"status,omitempty"`
	Decision       string         `json:"decision,omitempty"`
	Conclusion     string         `json:"conclusion,omitempty"`
	Evidence       map[string]any `json:"evidence,omitempty"`
	Notes          string         `json:"notes,omitempty"`
}

type AttachExperimentRunRequest struct {
	EvaluationID string `json:"evaluation_id"`
	Label        string `json:"label"`
	Notes        string `json:"notes,omitempty"`
}

func New(baseURL, token string, httpClient *http.Client) *Client {
	var provider TokenProvider
	if token != "" {
		provider = staticTokenProvider{token: token}
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: defaultHTTPTimeout}
	}
	return &Client{
		baseURL:       strings.TrimRight(baseURL, "/"),
		tokenProvider: provider,
		httpClient:    httpClient,
	}
}

func NewWithTokenProvider(baseURL string, tokenProvider TokenProvider, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: defaultHTTPTimeout}
	}
	return &Client{
		baseURL:           strings.TrimRight(baseURL, "/"),
		tokenProvider:     tokenProvider,
		retryUnauthorized: tokenProvider != nil,
		httpClient:        httpClient,
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

func (c *Client) CreateDataset(ctx context.Context, body CreateDatasetRequest) (CreateDatasetResponse, error) {
	var response CreateDatasetResponse
	if err := c.do(ctx, http.MethodPost, "/datasets", body, &response); err != nil {
		return CreateDatasetResponse{}, err
	}
	return response, nil
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

func (c *Client) CreateExperiment(ctx context.Context, body CreateExperimentRequest) (Experiment, error) {
	var response Experiment
	if err := c.do(ctx, http.MethodPost, "/experiments", body, &response); err != nil {
		return Experiment{}, err
	}
	return response, nil
}

func (c *Client) ListExperiments(ctx context.Context) ([]Experiment, error) {
	var response struct {
		Experiments []Experiment `json:"experiments"`
	}
	if err := c.do(ctx, http.MethodGet, "/experiments", nil, &response); err != nil {
		return nil, err
	}
	return response.Experiments, nil
}

func (c *Client) GetExperiment(ctx context.Context, id string) (Experiment, error) {
	var response Experiment
	if err := c.do(ctx, http.MethodGet, "/experiments/"+url.PathEscape(id), nil, &response); err != nil {
		return Experiment{}, err
	}
	return response, nil
}

func (c *Client) UpdateExperiment(ctx context.Context, id string, body UpdateExperimentRequest) (Experiment, error) {
	var response Experiment
	if err := c.do(ctx, http.MethodPatch, "/experiments/"+url.PathEscape(id), body, &response); err != nil {
		return Experiment{}, err
	}
	return response, nil
}

func (c *Client) AttachExperimentRun(ctx context.Context, id string, body AttachExperimentRunRequest) (Experiment, error) {
	var response Experiment
	if err := c.do(ctx, http.MethodPost, "/experiments/"+url.PathEscape(id)+"/runs", body, &response); err != nil {
		return Experiment{}, err
	}
	return response, nil
}

func (c *Client) do(ctx context.Context, method, path string, body any, out any) error {
	var payload []byte
	if body != nil {
		var buf bytes.Buffer
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			return fmt.Errorf("%s %s: encode request: %w", method, path, err)
		}
		payload = buf.Bytes()
	}

	err := c.doOnce(ctx, method, path, payload, body != nil, out)
	if err == nil || c.tokenProvider == nil || !c.retryUnauthorized {
		return err
	}

	var httpErr *HTTPError
	if !errors.As(err, &httpErr) || httpErr.StatusCode != http.StatusUnauthorized {
		return err
	}

	c.tokenProvider.Invalidate()
	return c.doOnce(ctx, method, path, payload, body != nil, out)
}

func (c *Client) doOnce(ctx context.Context, method, path string, payload []byte, hasBody bool, out any) error {
	var reader io.Reader
	if hasBody {
		reader = bytes.NewReader(payload)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return fmt.Errorf("%s %s: create request: %w", method, path, err)
	}
	if hasBody {
		req.Header.Set("Content-Type", "application/json")
	}
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

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		excerpt, _ := io.ReadAll(io.LimitReader(resp.Body, errorExcerptLimit))
		return &HTTPError{
			Method:     method,
			Path:       path,
			StatusCode: resp.StatusCode,
			Excerpt:    strings.TrimSpace(string(excerpt)),
			RetryAfter: parseRetryAfter(resp.Header.Get("Retry-After")),
		}
	}

	if out == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("%s %s: decode response: %w", method, path, err)
	}
	return nil
}

func parseRetryAfter(value string) time.Duration {
	if value == "" {
		return 0
	}
	seconds, err := time.ParseDuration(value + "s")
	if err != nil {
		return 0
	}
	return seconds
}
