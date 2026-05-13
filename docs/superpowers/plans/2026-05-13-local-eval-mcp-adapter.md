# Local Eval MCP Adapter Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a local-only stdio MCP service that lets agents run, compare, and summarize RAG eval experiments through the existing Python eval API.

**Architecture:** Add `go/eval-mcp-service` following the existing local MCP service pattern. The service has stateless API wrappers over `services/eval` plus a small SQLite experiment metadata store for labels, hypotheses, and conclusions. The Python eval API remains the source of truth for datasets, runs, scores, and per-query records.

**Tech Stack:** Go 1.26.1, `github.com/modelcontextprotocol/go-sdk` v1.5.0, `modernc.org/sqlite`, stdlib `net/http`, stdio MCP transport.

---

## File Structure

Create these files:

- `go/eval-mcp-service/go.mod`: module definition and dependencies.
- `go/eval-mcp-service/cmd/eval-mcp/main.go`: stdio entrypoint, config loading, store migration, service wiring.
- `go/eval-mcp-service/cmd/eval-mcp/main_test.go`: startup wiring tests.
- `go/eval-mcp-service/internal/config/config.go`: environment parsing and validation.
- `go/eval-mcp-service/internal/config/config_test.go`: config tests.
- `go/eval-mcp-service/internal/evalapi/client.go`: typed HTTP client for the Python eval API.
- `go/eval-mcp-service/internal/evalapi/client_test.go`: `httptest` coverage for API client behavior.
- `go/eval-mcp-service/internal/store/sqlite.go`: SQLite experiment metadata store.
- `go/eval-mcp-service/internal/store/sqlite_test.go`: store migration and CRUD tests.
- `go/eval-mcp-service/internal/evalworkflow/service.go`: experiment orchestration, labels, summaries, worst-case sorting.
- `go/eval-mcp-service/internal/evalworkflow/service_test.go`: workflow tests with fake API and store.
- `go/eval-mcp-service/internal/mcpserver/server.go`: MCP prompt, resource, tool schemas, handlers, JSON helpers.
- `go/eval-mcp-service/internal/mcpserver/server_test.go`: MCP handler tests.
- `go/eval-mcp-service/README.md`: local registration and usage docs.

No existing application files should be modified in the first implementation except generated `go.sum` from `go mod tidy`.

## Task 1: Module Skeleton And Config

**Files:**
- Create: `go/eval-mcp-service/go.mod`
- Create: `go/eval-mcp-service/internal/config/config.go`
- Create: `go/eval-mcp-service/internal/config/config_test.go`

- [ ] **Step 1: Create module file**

Create `go/eval-mcp-service/go.mod`:

```go
module github.com/kabradshaw1/portfolio/go/eval-mcp-service

go 1.26.1

require (
	github.com/modelcontextprotocol/go-sdk v1.5.0
	modernc.org/sqlite v1.39.1
)
```

- [ ] **Step 2: Write failing config tests**

Create `go/eval-mcp-service/internal/config/config_test.go`:

```go
package config

import (
	"testing"
	"time"
)

func TestFromEnvDefaults(t *testing.T) {
	t.Setenv("EVAL_MCP_DB_PATH", "")
	t.Setenv("EVAL_API_URL", "")
	t.Setenv("EVAL_API_TOKEN", "")
	t.Setenv("EVAL_MCP_POLL_INTERVAL", "")
	t.Setenv("EVAL_MCP_WAIT_TIMEOUT", "")

	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv returned error: %v", err)
	}
	if cfg.DBPath != "data/eval-mcp.db" {
		t.Fatalf("DBPath = %q", cfg.DBPath)
	}
	if cfg.EvalAPIURL != "http://localhost:8000/eval" {
		t.Fatalf("EvalAPIURL = %q", cfg.EvalAPIURL)
	}
	if cfg.APIToken != "" {
		t.Fatalf("APIToken = %q", cfg.APIToken)
	}
	if cfg.PollInterval != time.Second {
		t.Fatalf("PollInterval = %s", cfg.PollInterval)
	}
	if cfg.WaitTimeout != 5*time.Minute {
		t.Fatalf("WaitTimeout = %s", cfg.WaitTimeout)
	}
}

func TestFromEnvOverrides(t *testing.T) {
	t.Setenv("EVAL_MCP_DB_PATH", "/tmp/eval.db")
	t.Setenv("EVAL_API_URL", "http://127.0.0.1:9000/eval")
	t.Setenv("EVAL_API_TOKEN", "token-123")
	t.Setenv("EVAL_MCP_POLL_INTERVAL", "250ms")
	t.Setenv("EVAL_MCP_WAIT_TIMEOUT", "30s")

	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv returned error: %v", err)
	}
	if cfg.DBPath != "/tmp/eval.db" || cfg.EvalAPIURL != "http://127.0.0.1:9000/eval" || cfg.APIToken != "token-123" {
		t.Fatalf("unexpected config: %#v", cfg)
	}
	if cfg.PollInterval != 250*time.Millisecond || cfg.WaitTimeout != 30*time.Second {
		t.Fatalf("unexpected durations: %#v", cfg)
	}
}

func TestFromEnvRejectsBadDuration(t *testing.T) {
	t.Setenv("EVAL_MCP_POLL_INTERVAL", "not-a-duration")

	_, err := FromEnv()
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestFromEnvRejectsNonPositiveDurations(t *testing.T) {
	t.Setenv("EVAL_MCP_POLL_INTERVAL", "0s")

	_, err := FromEnv()
	if err == nil {
		t.Fatal("expected error")
	}
}
```

- [ ] **Step 3: Run tests and verify they fail**

Run:

```bash
cd go/eval-mcp-service
go test ./internal/config
```

Expected: FAIL because `FromEnv` and `Config` are undefined.

- [ ] **Step 4: Implement config parsing**

Create `go/eval-mcp-service/internal/config/config.go`:

```go
package config

import (
	"fmt"
	"os"
	"time"
)

const (
	defaultDBPath       = "data/eval-mcp.db"
	defaultEvalAPIURL   = "http://localhost:8000/eval"
	defaultPollInterval = time.Second
	defaultWaitTimeout  = 5 * time.Minute
)

type Config struct {
	DBPath       string
	EvalAPIURL   string
	APIToken     string
	PollInterval time.Duration
	WaitTimeout  time.Duration
}

func FromEnv() (Config, error) {
	pollInterval, err := durationEnv("EVAL_MCP_POLL_INTERVAL", defaultPollInterval)
	if err != nil {
		return Config{}, err
	}
	waitTimeout, err := durationEnv("EVAL_MCP_WAIT_TIMEOUT", defaultWaitTimeout)
	if err != nil {
		return Config{}, err
	}
	if pollInterval <= 0 {
		return Config{}, fmt.Errorf("EVAL_MCP_POLL_INTERVAL must be positive")
	}
	if waitTimeout <= 0 {
		return Config{}, fmt.Errorf("EVAL_MCP_WAIT_TIMEOUT must be positive")
	}
	return Config{
		DBPath:       getenv("EVAL_MCP_DB_PATH", defaultDBPath),
		EvalAPIURL:   getenv("EVAL_API_URL", defaultEvalAPIURL),
		APIToken:     os.Getenv("EVAL_API_TOKEN"),
		PollInterval: pollInterval,
		WaitTimeout:  waitTimeout,
	}, nil
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func durationEnv(key string, fallback time.Duration) (time.Duration, error) {
	value := os.Getenv(key)
	if value == "" {
		return fallback, nil
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", key, err)
	}
	return parsed, nil
}
```

- [ ] **Step 5: Run config tests**

Run:

```bash
cd go/eval-mcp-service
go test ./internal/config
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add go/eval-mcp-service/go.mod go/eval-mcp-service/internal/config
git commit -m "feat: add eval mcp config"
```

## Task 2: Eval API Client

**Files:**
- Create: `go/eval-mcp-service/internal/evalapi/client.go`
- Create: `go/eval-mcp-service/internal/evalapi/client_test.go`

- [ ] **Step 1: Write failing API client tests**

Create `go/eval-mcp-service/internal/evalapi/client_test.go` with tests for:

```go
package evalapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestListDatasetsAddsBearerToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/datasets" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer token-123" {
			t.Fatalf("Authorization = %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"datasets": []Dataset{{ID: "ds-1", Name: "rag", ItemCount: 2}}})
	}))
	defer server.Close()

	client := New(server.URL, "token-123", server.Client())
	got, err := client.ListDatasets(context.Background())
	if err != nil {
		t.Fatalf("ListDatasets error: %v", err)
	}
	if len(got) != 1 || got[0].ID != "ds-1" {
		t.Fatalf("datasets = %#v", got)
	}
}

func TestStartEvaluationSendsOptionalFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body StartEvaluationRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body.DatasetID != "ds-1" || body.Collection != "documents" || body.Notes != "candidate" || body.BaselineEvalID != "eval-base" || !body.Rerank {
			t.Fatalf("body = %#v", body)
		}
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(StartEvaluationResponse{ID: "eval-2", Status: "running"})
	}))
	defer server.Close()

	client := New(server.URL, "", server.Client())
	got, err := client.StartEvaluation(context.Background(), StartEvaluationRequest{
		DatasetID:       "ds-1",
		Collection:      "documents",
		Notes:           "candidate",
		BaselineEvalID:  "eval-base",
		Rerank:          true,
	})
	if err != nil {
		t.Fatalf("StartEvaluation error: %v", err)
	}
	if got.ID != "eval-2" || got.Status != "running" {
		t.Fatalf("response = %#v", got)
	}
}

func TestGetEvaluationAndCompare(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/evaluations/eval-1":
			_ = json.NewEncoder(w).Encode(EvaluationDetail{
				ID: "eval-1", Status: "completed",
				AggregateScores: &Scores{ContextPrecision: ptr(0.42)},
			})
		case "/evaluations/compare":
			if got := r.URL.Query().Get("ids"); got != "eval-1,eval-2" {
				t.Fatalf("ids = %q", got)
			}
			_ = json.NewEncoder(w).Encode(Comparison{Runs: []EvaluationDetail{{ID: "eval-1"}, {ID: "eval-2"}}, Deltas: map[string][]float64{"context_precision": {0, 0.1}}})
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := New(server.URL, "", server.Client())
	run, err := client.GetEvaluation(context.Background(), "eval-1")
	if err != nil {
		t.Fatalf("GetEvaluation error: %v", err)
	}
	if run.AggregateScores == nil || run.AggregateScores.ContextPrecision == nil || *run.AggregateScores.ContextPrecision != 0.42 {
		t.Fatalf("run = %#v", run)
	}
	comp, err := client.CompareEvaluations(context.Background(), []string{"eval-1", "eval-2"})
	if err != nil {
		t.Fatalf("CompareEvaluations error: %v", err)
	}
	if len(comp.Runs) != 2 {
		t.Fatalf("comparison = %#v", comp)
	}
}

func TestHTTPErrorIncludesStatusAndExcerpt(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, strings.Repeat("x", 300), http.StatusUnauthorized)
	}))
	defer server.Close()

	client := New(server.URL, "", server.Client())
	_, err := client.ListDatasets(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "status 401") {
		t.Fatalf("error = %v", err)
	}
}

func ptr(v float64) *float64 { return &v }

func TestNewUsesDefaultHTTPClient(t *testing.T) {
	client := New("http://example.test/eval", "", nil)
	if client.httpClient == nil {
		t.Fatal("httpClient is nil")
	}
	if client.httpClient.Timeout != 30*time.Second {
		t.Fatalf("timeout = %s", client.httpClient.Timeout)
	}
}
```

- [ ] **Step 2: Run tests and verify they fail**

Run:

```bash
cd go/eval-mcp-service
go test ./internal/evalapi
```

Expected: FAIL because package implementation is missing.

- [ ] **Step 3: Implement API client**

Create `go/eval-mcp-service/internal/evalapi/client.go` with exported structs matching the eval API:

```go
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

type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

type Dataset struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	CreatedAt string `json:"created_at,omitempty"`
	ItemCount int    `json:"item_count"`
}

type StartEvaluationRequest struct {
	DatasetID      string `json:"dataset_id"`
	Collection     string `json:"collection,omitempty"`
	Notes          string `json:"notes,omitempty"`
	BaselineEvalID string `json:"baseline_eval_id,omitempty"`
	Rerank         bool   `json:"rerank,omitempty"`
}

type StartEvaluationResponse struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

type Scores struct {
	Faithfulness    *float64 `json:"faithfulness"`
	AnswerRelevancy *float64 `json:"answer_relevancy"`
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
	Collection      string         `json:"collection"`
	AggregateScores *Scores        `json:"aggregate_scores"`
	Results         []QueryResult  `json:"results"`
	Error           string         `json:"error"`
	CreatedAt       string         `json:"created_at"`
	CompletedAt     string         `json:"completed_at"`
	Notes           string         `json:"notes"`
	Config          map[string]any `json:"config"`
	BaselineEvalID  string         `json:"baseline_eval_id"`
}

type Comparison struct {
	Runs   []EvaluationDetail `json:"runs"`
	Deltas map[string][]float64 `json:"deltas"`
}

func New(baseURL, token string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), token: token, httpClient: httpClient}
}

func (c *Client) ListDatasets(ctx context.Context) ([]Dataset, error) {
	var out struct {
		Datasets []Dataset `json:"datasets"`
	}
	if err := c.do(ctx, http.MethodGet, "/datasets", nil, &out); err != nil {
		return nil, err
	}
	return out.Datasets, nil
}

func (c *Client) StartEvaluation(ctx context.Context, body StartEvaluationRequest) (StartEvaluationResponse, error) {
	var out StartEvaluationResponse
	if err := c.do(ctx, http.MethodPost, "/evaluations", body, &out); err != nil {
		return StartEvaluationResponse{}, err
	}
	return out, nil
}

func (c *Client) GetEvaluation(ctx context.Context, id string) (EvaluationDetail, error) {
	var out EvaluationDetail
	if err := c.do(ctx, http.MethodGet, "/evaluations/"+url.PathEscape(id), nil, &out); err != nil {
		return EvaluationDetail{}, err
	}
	return out, nil
}

func (c *Client) CompareEvaluations(ctx context.Context, ids []string) (Comparison, error) {
	values := url.Values{}
	values.Set("ids", strings.Join(ids, ","))
	var out Comparison
	if err := c.do(ctx, http.MethodGet, "/evaluations/compare?"+values.Encode(), nil, &out); err != nil {
		return Comparison{}, err
	}
	return out, nil
}

func (c *Client) do(ctx context.Context, method, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		var buf bytes.Buffer
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			return fmt.Errorf("encode request: %w", err)
		}
		reader = &buf
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return fmt.Errorf("new request: %w", err)
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
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 240))
		return fmt.Errorf("%s %s: status %d: %s", method, path, resp.StatusCode, strings.TrimSpace(string(b)))
	}
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}
```

- [ ] **Step 4: Run API client tests**

Run:

```bash
cd go/eval-mcp-service
go test ./internal/evalapi
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add go/eval-mcp-service/internal/evalapi
git commit -m "feat: add eval api client"
```

## Task 3: SQLite Experiment Store

**Files:**
- Create: `go/eval-mcp-service/internal/store/sqlite.go`
- Create: `go/eval-mcp-service/internal/store/sqlite_test.go`

- [ ] **Step 1: Write failing store tests**

Create tests that cover migration, experiment creation, label attachment, listing, detail loading, conclusion update, and missing rows. Use `t.TempDir()` and real SQLite files.

Key test functions:

```go
func TestExperimentLifecycle(t *testing.T)
func TestGetExperimentNotFound(t *testing.T)
func TestAttachRunReplacesExistingLabel(t *testing.T)
func TestListExperimentsNewestFirst(t *testing.T)
```

Expected assertions:

- `CreateExperiment` returns an ID.
- `GetExperiment` includes `Runs` with labels and eval IDs.
- Re-attaching `label="baseline"` updates the eval ID.
- `ErrNotFound` is returned for missing experiment IDs.

- [ ] **Step 2: Run tests and verify they fail**

Run:

```bash
cd go/eval-mcp-service
go test ./internal/store
```

Expected: FAIL because store package is missing.

- [ ] **Step 3: Implement SQLite store**

Create `go/eval-mcp-service/internal/store/sqlite.go` with:

```go
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

var ErrNotFound = errors.New("not found")

type DB struct { db *sql.DB }

type Experiment struct {
	ID             int64      `json:"id"`
	Name           string     `json:"name"`
	DatasetID      string     `json:"dataset_id"`
	Collection     string     `json:"collection"`
	BaselineEvalID string     `json:"baseline_eval_id,omitempty"`
	FocusMetric    string     `json:"focus_metric"`
	Hypothesis     string     `json:"hypothesis,omitempty"`
	Notes          string     `json:"notes,omitempty"`
	Conclusion     string     `json:"conclusion,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	Runs           []RunLabel  `json:"runs,omitempty"`
}

type RunLabel struct {
	Label     string    `json:"label"`
	EvalID    string    `json:"eval_id"`
	Notes     string    `json:"notes,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type CreateExperimentInput struct {
	Name           string
	DatasetID      string
	Collection     string
	BaselineEvalID string
	FocusMetric    string
	Hypothesis     string
	Notes          string
}

func Open(path string) (*DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create db dir: %w", err)
	}
	sqlDB, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	return &DB{db: sqlDB}, nil
}

func OpenSQL(db *sql.DB) *DB { return &DB{db: db} }
func (d *DB) Close() error { return d.db.Close() }
```

Also implement methods:

```go
func (d *DB) Migrate(ctx context.Context) error
func (d *DB) CreateExperiment(ctx context.Context, in CreateExperimentInput) (int64, error)
func (d *DB) ListExperiments(ctx context.Context) ([]Experiment, error)
func (d *DB) GetExperiment(ctx context.Context, id int64) (Experiment, error)
func (d *DB) AttachRun(ctx context.Context, experimentID int64, label, evalID, notes string) error
func (d *DB) RecordConclusion(ctx context.Context, experimentID int64, conclusion string) error
```

Migration SQL:

```sql
CREATE TABLE IF NOT EXISTS experiments (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL UNIQUE,
	dataset_id TEXT NOT NULL,
	collection TEXT NOT NULL DEFAULT 'documents',
	baseline_eval_id TEXT NOT NULL DEFAULT '',
	focus_metric TEXT NOT NULL DEFAULT 'context_precision',
	hypothesis TEXT NOT NULL DEFAULT '',
	notes TEXT NOT NULL DEFAULT '',
	conclusion TEXT NOT NULL DEFAULT '',
	created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS experiment_runs (
	experiment_id INTEGER NOT NULL REFERENCES experiments(id) ON DELETE CASCADE,
	label TEXT NOT NULL,
	eval_id TEXT NOT NULL,
	notes TEXT NOT NULL DEFAULT '',
	created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
	PRIMARY KEY (experiment_id, label)
);
```

- [ ] **Step 4: Run store tests**

Run:

```bash
cd go/eval-mcp-service
go test ./internal/store
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add go/eval-mcp-service/internal/store
git commit -m "feat: add eval experiment store"
```

## Task 4: Workflow Service

**Files:**
- Create: `go/eval-mcp-service/internal/evalworkflow/service.go`
- Create: `go/eval-mcp-service/internal/evalworkflow/service_test.go`

- [ ] **Step 1: Write failing workflow tests**

Cover these behaviors with fakes:

```go
func TestStartExperimentDefaultsCollectionAndFocusMetric(t *testing.T)
func TestStartRunAttachesReturnedEvalIDWhenExperimentProvided(t *testing.T)
func TestWaitForRunReturnsCompletedRun(t *testing.T)
func TestWaitForRunTimesOutWithLatestStatus(t *testing.T)
func TestWorstCasesSortsAscendingByMetric(t *testing.T)
func TestCompareResolvesExperimentLabels(t *testing.T)
func TestSummarizeExperimentReturnsBaselineCandidateAndWorstCases(t *testing.T)
```

Use a fake API implementing:

```go
type API interface {
	ListDatasets(context.Context) ([]evalapi.Dataset, error)
	StartEvaluation(context.Context, evalapi.StartEvaluationRequest) (evalapi.StartEvaluationResponse, error)
	GetEvaluation(context.Context, string) (evalapi.EvaluationDetail, error)
	CompareEvaluations(context.Context, []string) (evalapi.Comparison, error)
}
```

- [ ] **Step 2: Run tests and verify they fail**

Run:

```bash
cd go/eval-mcp-service
go test ./internal/evalworkflow
```

Expected: FAIL because workflow service is missing.

- [ ] **Step 3: Implement workflow service**

Create `go/eval-mcp-service/internal/evalworkflow/service.go` with:

```go
package evalworkflow

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/kabradshaw1/portfolio/go/eval-mcp-service/internal/evalapi"
	"github.com/kabradshaw1/portfolio/go/eval-mcp-service/internal/store"
)

const (
	DefaultCollection  = "documents"
	DefaultFocusMetric = "context_precision"
)

type API interface {
	ListDatasets(context.Context) ([]evalapi.Dataset, error)
	StartEvaluation(context.Context, evalapi.StartEvaluationRequest) (evalapi.StartEvaluationResponse, error)
	GetEvaluation(context.Context, string) (evalapi.EvaluationDetail, error)
	CompareEvaluations(context.Context, []string) (evalapi.Comparison, error)
}

type Store interface {
	CreateExperiment(context.Context, store.CreateExperimentInput) (int64, error)
	ListExperiments(context.Context) ([]store.Experiment, error)
	GetExperiment(context.Context, int64) (store.Experiment, error)
	AttachRun(context.Context, int64, string, string, string) error
	RecordConclusion(context.Context, int64, string) error
}

type Service struct {
	api          API
	store        Store
	pollInterval time.Duration
	waitTimeout  time.Duration
}
```

Implement methods:

```go
func New(api API, st Store, pollInterval, waitTimeout time.Duration) *Service
func (s *Service) StartExperiment(ctx context.Context, in StartExperimentInput) (store.Experiment, error)
func (s *Service) ListExperiments(ctx context.Context) ([]store.Experiment, error)
func (s *Service) GetExperiment(ctx context.Context, id int64) (store.Experiment, error)
func (s *Service) ListDatasets(ctx context.Context) ([]evalapi.Dataset, error)
func (s *Service) StartRun(ctx context.Context, in StartRunInput) (StartRunResult, error)
func (s *Service) WaitForRun(ctx context.Context, evalID string) (WaitResult, error)
func (s *Service) AttachRun(ctx context.Context, experimentID int64, label, evalID, notes string) error
func (s *Service) GetRun(ctx context.Context, evalID string) (evalapi.EvaluationDetail, error)
func (s *Service) Compare(ctx context.Context, in CompareInput) (evalapi.Comparison, error)
func (s *Service) WorstCases(ctx context.Context, in WorstCasesInput) (WorstCasesResult, error)
func (s *Service) SummarizeExperiment(ctx context.Context, experimentID int64) (ExperimentSummary, error)
func (s *Service) RecordConclusion(ctx context.Context, experimentID int64, conclusion string) error
```

Worst-case sorting must be ascending by selected metric, with a default limit of
5 and a max limit of 20. Supported metrics are `faithfulness`,
`answer_relevancy`, `context_precision`, and `context_recall`.

- [ ] **Step 4: Run workflow tests**

Run:

```bash
cd go/eval-mcp-service
go test ./internal/evalworkflow
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add go/eval-mcp-service/internal/evalworkflow
git commit -m "feat: add eval workflow service"
```

## Task 5: MCP Server Tools

**Files:**
- Create: `go/eval-mcp-service/internal/mcpserver/server.go`
- Create: `go/eval-mcp-service/internal/mcpserver/server_test.go`

- [ ] **Step 1: Write failing MCP handler tests**

Follow the `coding-exercises-mcp-service/internal/mcpserver/server_test.go`
pattern. Cover:

```go
func TestServerRegistersPromptResourceAndTools(t *testing.T)
func TestStartEvalExperimentHandler(t *testing.T)
func TestStartEvalRunValidationError(t *testing.T)
func TestWorstCasesHandlerReturnsJSON(t *testing.T)
func TestEvalPromptHandler(t *testing.T)
func TestWorkflowResourceHandler(t *testing.T)
```

- [ ] **Step 2: Run tests and verify they fail**

Run:

```bash
cd go/eval-mcp-service
go test ./internal/mcpserver
```

Expected: FAIL because MCP server is missing.

- [ ] **Step 3: Implement MCP server**

Create `go/eval-mcp-service/internal/mcpserver/server.go`.

Register:

```go
func New(service EvalService) *sdkmcp.Server {
	srv := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "eval-mcp-service", Version: "0.1.0"}, nil)
	addPrompt(srv, "eval", "Eval", "Start an agent-led RAG eval experiment.", evalPromptHandler())
	addResource(srv, "eval://workflow", "Eval Workflow", "Workflow instructions for agent-led RAG eval experiments.", workflowResourceHandler())
	addTool(srv, "start_eval_experiment", "Start or define a local eval experiment session.", startEvalExperimentSchema(), startEvalExperimentHandler(service))
	addTool(srv, "list_eval_experiments", "List local eval experiment sessions.", emptySchema(), listEvalExperimentsHandler(service))
	addTool(srv, "get_eval_experiment", "Get one local eval experiment with run labels.", experimentIDSchema(), getEvalExperimentHandler(service))
	addTool(srv, "list_eval_datasets", "List datasets from the eval API.", emptySchema(), listEvalDatasetsHandler(service))
	addTool(srv, "start_eval_run", "Start an eval API run and optionally attach it to an experiment label.", startEvalRunSchema(), startEvalRunHandler(service))
	addTool(srv, "wait_for_eval_run", "Poll one eval run until completion, failure, or timeout.", waitEvalRunSchema(), waitForEvalRunHandler(service))
	addTool(srv, "attach_eval_run", "Attach an existing eval run ID to a local experiment label.", attachEvalRunSchema(), attachEvalRunHandler(service))
	addTool(srv, "get_eval_run", "Fetch one eval run from the eval API.", evalIDSchema(), getEvalRunHandler(service))
	addTool(srv, "compare_eval_runs", "Compare eval runs by explicit IDs or experiment labels.", compareEvalRunsSchema(), compareEvalRunsHandler(service))
	addTool(srv, "get_worst_eval_cases", "Return the lowest-scoring per-query cases for a metric.", worstCasesSchema(), worstCasesHandler(service))
	addTool(srv, "summarize_eval_experiment", "Summarize baseline, candidates, and worst cases for an experiment.", experimentIDSchema(), summarizeExperimentHandler(service))
	addTool(srv, "record_eval_experiment_conclusion", "Record the approved conclusion for a local eval experiment.", recordConclusionSchema(), recordConclusionHandler(service))
	return srv
}
```

Define an `EvalService` interface over the workflow service methods. Return JSON
as `sdkmcp.TextContent`, matching the existing local MCP services. Tool errors
should be returned as MCP tool errors, not Go errors.

- [ ] **Step 4: Run MCP tests**

Run:

```bash
cd go/eval-mcp-service
go test ./internal/mcpserver
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add go/eval-mcp-service/internal/mcpserver
git commit -m "feat: expose eval mcp tools"
```

## Task 6: Command Entrypoint And README

**Files:**
- Create: `go/eval-mcp-service/cmd/eval-mcp/main.go`
- Create: `go/eval-mcp-service/cmd/eval-mcp/main_test.go`
- Create: `go/eval-mcp-service/README.md`

- [ ] **Step 1: Write failing command tests**

Create `main_test.go` with:

```go
func TestRunWiresDependenciesAndCallsServer(t *testing.T)
func TestRunReturnsConfigError(t *testing.T)
```

The first test should use `t.TempDir()` for `EVAL_MCP_DB_PATH`, an
`httptest.Server` URL for `EVAL_API_URL`, and a fake `serverRunner` that asserts
`application.service != nil`.

- [ ] **Step 2: Run tests and verify they fail**

Run:

```bash
cd go/eval-mcp-service
go test ./cmd/eval-mcp
```

Expected: FAIL because command entrypoint is missing.

- [ ] **Step 3: Implement command entrypoint**

Create `go/eval-mcp-service/cmd/eval-mcp/main.go`:

```go
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kabradshaw1/portfolio/go/eval-mcp-service/internal/config"
	"github.com/kabradshaw1/portfolio/go/eval-mcp-service/internal/evalapi"
	"github.com/kabradshaw1/portfolio/go/eval-mcp-service/internal/evalworkflow"
	"github.com/kabradshaw1/portfolio/go/eval-mcp-service/internal/mcpserver"
	"github.com/kabradshaw1/portfolio/go/eval-mcp-service/internal/store"
)

type app struct {
	service *evalworkflow.Service
	cfg     config.Config
}

type serverRunner func(context.Context, *app) error

func main() {
	logger := log.New(os.Stderr, "", log.LstdFlags)
	if err := run(context.Background(), logger, runMCPServer); err != nil {
		logger.Fatalf("eval MCP server failed: %v", err)
	}
}

func run(ctx context.Context, logger *log.Logger, runServer serverRunner) error {
	cfg, err := config.FromEnv()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	db, err := store.Open(cfg.DBPath)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer db.Close()
	if err := db.Migrate(ctx); err != nil {
		return fmt.Errorf("migrate store: %w", err)
	}
	api := evalapi.New(cfg.EvalAPIURL, cfg.APIToken, &http.Client{Timeout: cfg.WaitTimeout})
	service := evalworkflow.New(api, db, cfg.PollInterval, cfg.WaitTimeout)
	logger.Printf("eval MCP server running on stdio eval_api_url=%s db_path=%s", cfg.EvalAPIURL, cfg.DBPath)
	return runServer(ctx, &app{service: service, cfg: cfg})
}

func runMCPServer(ctx context.Context, application *app) error {
	server := mcpserver.New(application.service)
	return server.Run(ctx, &sdkmcp.StdioTransport{})
}
```

- [ ] **Step 4: Add README**

Create `go/eval-mcp-service/README.md` with:

```markdown
# Eval MCP Service

Local-only MCP stdio server for agent-driven RAG evaluation experiments.

The Python eval API remains the source of truth for datasets, runs, scores, and
per-query results. This MCP service stores only local experiment metadata such
as labels, hypotheses, notes, and conclusions.

## Run Directly

```bash
go run ./cmd/eval-mcp
```

The service logs to stderr and reserves stdout for MCP protocol messages.

## Register With Codex

```bash
codex mcp add eval -- zsh -lc 'cd /Users/kylebradshaw/repos/gen_ai_engineer/go/eval-mcp-service && exec go run ./cmd/eval-mcp'
```

## Configuration

- `EVAL_MCP_DB_PATH`: SQLite path, defaults to `data/eval-mcp.db`.
- `EVAL_API_URL`: eval API base URL, defaults to `http://localhost:8000/eval`.
- `EVAL_API_TOKEN`: optional bearer token for authenticated eval API calls.
- `EVAL_MCP_POLL_INTERVAL`: polling interval, defaults to `1s`.
- `EVAL_MCP_WAIT_TIMEOUT`: wait timeout, defaults to `5m`.

## Tools

- `start_eval_experiment`
- `list_eval_experiments`
- `get_eval_experiment`
- `list_eval_datasets`
- `start_eval_run`
- `wait_for_eval_run`
- `attach_eval_run`
- `get_eval_run`
- `compare_eval_runs`
- `get_worst_eval_cases`
- `summarize_eval_experiment`
- `record_eval_experiment_conclusion`
```

- [ ] **Step 5: Run command tests**

Run:

```bash
cd go/eval-mcp-service
go test ./cmd/eval-mcp
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add go/eval-mcp-service/cmd go/eval-mcp-service/README.md
git commit -m "feat: add eval mcp entrypoint"
```

## Task 7: Integration Verification And Repository Wiring Check

**Files:**
- Modify: `go/eval-mcp-service/go.sum`

- [ ] **Step 1: Tidy the module**

Run:

```bash
cd go/eval-mcp-service
go mod tidy
```

Expected: `go.sum` is created or updated with MCP and SQLite dependencies.

- [ ] **Step 2: Run service-local tests**

Run:

```bash
cd go/eval-mcp-service
go test ./...
```

Expected: PASS.

- [ ] **Step 3: Run Go preflight from repo root**

Run:

```bash
make preflight-go
```

Expected: PASS. If this target does not automatically include the new module,
record the gap and add a follow-up task to wire the module into the repo's Go
preflight discovery before merging implementation.

- [ ] **Step 4: Smoke run the stdio server**

Run:

```bash
cd go/eval-mcp-service
timeout 2s go run ./cmd/eval-mcp
```

Expected: process starts, logs `eval MCP server running on stdio` to stderr, and
then exits due to `timeout`.

- [ ] **Step 5: Commit final verification artifacts**

```bash
git add go/eval-mcp-service/go.sum
git commit -m "chore: verify eval mcp module"
```

If `go.sum` was already committed by earlier tasks and no files changed, skip
this commit and record that there were no final artifact changes.
