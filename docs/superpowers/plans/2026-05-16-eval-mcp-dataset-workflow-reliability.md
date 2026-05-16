# Eval MCP Dataset Workflow Reliability Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the eval MCP service discover curated dataset fixtures, create eval API datasets from them, validate RAG collections, and prevent failed or incomplete run comparisons.

**Architecture:** Add three focused units under `go/eval-mcp-service`: an eval dataset creation method on the existing eval API client, a new ingestion API client, and a fixture catalog package. Wire them into `evalworkflow.Service`, then expose the workflow through MCP tools and updated prompt/resource text.

**Tech Stack:** Go, `net/http`, stdlib JSON/filepath APIs, existing `github.com/modelcontextprotocol/go-sdk/mcp`, existing Go unit tests.

---

## File Structure

- Create `go/eval-mcp-service/internal/fixturecatalog/catalog.go`: scan fixture roots, parse curated JSON fixtures, validate item fields and source references.
- Create `go/eval-mcp-service/internal/fixturecatalog/catalog_test.go`: fixture discovery and validation tests.
- Create `go/eval-mcp-service/internal/ingestionapi/client.go`: ingestion `/collections` and `/collections/{name}/config` HTTP client.
- Create `go/eval-mcp-service/internal/ingestionapi/client_test.go`: ingestion client tests.
- Modify `go/eval-mcp-service/internal/evalapi/client.go`: add dataset create request/response types and `CreateDataset`.
- Modify `go/eval-mcp-service/internal/evalapi/client_test.go`: add `POST /datasets` test.
- Modify `go/eval-mcp-service/internal/config/config.go`: add fixture roots and ingestion URL config.
- Modify `go/eval-mcp-service/internal/config/config_test.go`: cover new defaults and env overrides.
- Modify `go/eval-mcp-service/cmd/eval-mcp/main.go`: construct fixture catalog and ingestion client, pass them to workflow service.
- Modify `go/eval-mcp-service/cmd/eval-mcp/main_test.go`: update service wiring assertions if constructor signatures require it.
- Modify `go/eval-mcp-service/internal/evalworkflow/service.go`: add dataset fixture import, RAG collection methods, collection validation, and completed-only comparison/summary guards.
- Modify `go/eval-mcp-service/internal/evalworkflow/service_test.go`: workflow tests for new behavior.
- Modify `go/eval-mcp-service/internal/mcpserver/server.go`: add MCP tools, schemas, handlers, interface methods, prompt/resource text.
- Modify `go/eval-mcp-service/internal/mcpserver/server_test.go`: registration, handler, and prompt/resource tests.
- Modify `go/eval-mcp-service/README.md`: document new config and workflow.

### Task 1: Fixture Catalog

**Files:**
- Create: `go/eval-mcp-service/internal/fixturecatalog/catalog.go`
- Create: `go/eval-mcp-service/internal/fixturecatalog/catalog_test.go`

- [ ] **Step 1: Write failing tests for valid fixture discovery**

Create `go/eval-mcp-service/internal/fixturecatalog/catalog_test.go` with:

```go
package fixturecatalog

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCatalogListsValidFixture(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "laptop-pro-15-specs.pdf", "%PDF-1.4")
	writeFile(t, root, "rag-eval-dataset-product-docs.json", `{
		"name": "product-docs-rag-v1",
		"items": [{
			"query": "How long does the battery last?",
			"expected_answer": "Up to 10 hours of mixed use.",
			"expected_sources": ["laptop-pro-15-specs.pdf"]
		}]
	}`)

	catalog := New([]string{root})
	fixtures, err := catalog.List()
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(fixtures) != 1 {
		t.Fatalf("fixtures len = %d", len(fixtures))
	}
	got := fixtures[0]
	if got.ID != "rag-eval-dataset-product-docs.json" || got.Name != "product-docs-rag-v1" || got.ItemCount != 1 || !got.Valid {
		t.Fatalf("unexpected fixture: %#v", got)
	}
	if len(got.ReferencedSources) != 1 || got.ReferencedSources[0] != "laptop-pro-15-specs.pdf" {
		t.Fatalf("unexpected sources: %#v", got.ReferencedSources)
	}
}

func writeFile(t *testing.T, root, name, content string) {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
}
```

- [ ] **Step 2: Run fixture catalog tests and verify they fail**

Run:

```bash
cd go/eval-mcp-service
go test ./internal/fixturecatalog
```

Expected: FAIL because `internal/fixturecatalog` does not exist.

- [ ] **Step 3: Implement minimal fixture catalog**

Create `go/eval-mcp-service/internal/fixturecatalog/catalog.go`:

```go
package fixturecatalog

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const (
	maxNameLen   = 100
	maxItems     = 100
	maxQueryLen  = 2000
	maxAnswerLen = 5000
)

var datasetNamePattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

type Catalog struct {
	roots []string
}

type Fixture struct {
	ID                string        `json:"id"`
	Name              string        `json:"name"`
	Path              string        `json:"path"`
	DocumentRoot      string        `json:"document_root"`
	ItemCount         int           `json:"item_count"`
	ReferencedSources []string      `json:"referenced_sources"`
	Valid             bool          `json:"valid"`
	Errors            []string      `json:"errors,omitempty"`
	Items             []GoldenItem  `json:"-"`
}

type GoldenItem struct {
	Query           string   `json:"query"`
	ExpectedAnswer  string   `json:"expected_answer"`
	ExpectedSources []string `json:"expected_sources"`
}

type fixtureFile struct {
	Name  string       `json:"name"`
	Items []GoldenItem `json:"items"`
}

func New(roots []string) *Catalog {
	cleaned := make([]string, 0, len(roots))
	for _, root := range roots {
		if strings.TrimSpace(root) != "" {
			cleaned = append(cleaned, root)
		}
	}
	return &Catalog{roots: cleaned}
}

func (c *Catalog) List() ([]Fixture, error) {
	var fixtures []Fixture
	for _, root := range c.roots {
		matches, err := filepath.Glob(filepath.Join(root, "*.json"))
		if err != nil {
			return nil, err
		}
		sort.Strings(matches)
		for _, path := range matches {
			fixture, err := c.Load(path)
			if err != nil {
				fixtures = append(fixtures, Fixture{
					ID:           filepath.Base(path),
					Path:         path,
					DocumentRoot: root,
					Valid:        false,
					Errors:       []string{err.Error()},
				})
				continue
			}
			fixtures = append(fixtures, fixture)
		}
	}
	sort.Slice(fixtures, func(i, j int) bool { return fixtures[i].ID < fixtures[j].ID })
	return fixtures, nil
}

func (c *Catalog) Load(idOrPath string) (Fixture, error) {
	path, root, err := c.resolve(idOrPath)
	if err != nil {
		return Fixture{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Fixture{}, fmt.Errorf("read fixture %q: %w", path, err)
	}
	var raw fixtureFile
	if err := json.Unmarshal(data, &raw); err != nil {
		return Fixture{}, fmt.Errorf("parse fixture %q: %w", path, err)
	}
	fixture := Fixture{
		ID:           filepath.Base(path),
		Name:         raw.Name,
		Path:         path,
		DocumentRoot: root,
		ItemCount:    len(raw.Items),
		Items:        raw.Items,
	}
	fixture.Errors = validateFixture(raw, root)
	fixture.ReferencedSources = referencedSources(raw.Items)
	fixture.Valid = len(fixture.Errors) == 0
	if !fixture.Valid {
		return fixture, fmt.Errorf("invalid fixture %q: %s", path, strings.Join(fixture.Errors, "; "))
	}
	return fixture, nil
}

func (c *Catalog) resolve(idOrPath string) (string, string, error) {
	for _, root := range c.roots {
		candidate := idOrPath
		if !filepath.IsAbs(candidate) {
			candidate = filepath.Join(root, idOrPath)
		}
		absRoot, err := filepath.Abs(root)
		if err != nil {
			return "", "", err
		}
		absCandidate, err := filepath.Abs(candidate)
		if err != nil {
			return "", "", err
		}
		rel, err := filepath.Rel(absRoot, absCandidate)
		if err != nil || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." || filepath.IsAbs(rel) {
			continue
		}
		if _, err := os.Stat(absCandidate); err == nil {
			return absCandidate, absRoot, nil
		}
	}
	return "", "", fmt.Errorf("fixture %q not found", idOrPath)
}

func validateFixture(raw fixtureFile, root string) []string {
	var errs []string
	if raw.Name == "" || len(raw.Name) > maxNameLen || !datasetNamePattern.MatchString(raw.Name) {
		errs = append(errs, "name must match ^[a-zA-Z0-9_-]+$ and be 1-100 characters")
	}
	if len(raw.Items) == 0 || len(raw.Items) > maxItems {
		errs = append(errs, "items must contain 1-100 entries")
	}
	for i, item := range raw.Items {
		if strings.TrimSpace(item.Query) == "" || len(item.Query) > maxQueryLen {
			errs = append(errs, fmt.Sprintf("items[%d].query must be 1-2000 characters", i))
		}
		if strings.TrimSpace(item.ExpectedAnswer) == "" || len(item.ExpectedAnswer) > maxAnswerLen {
			errs = append(errs, fmt.Sprintf("items[%d].expected_answer must be 1-5000 characters", i))
		}
		for _, source := range item.ExpectedSources {
			if err := validateSource(root, source); err != nil {
				errs = append(errs, fmt.Sprintf("items[%d].expected_sources %q: %v", i, source, err))
			}
		}
	}
	return errs
}

func validateSource(root, source string) error {
	if source == "" || filepath.IsAbs(source) {
		return fmt.Errorf("must be a relative path")
	}
	ext := strings.ToLower(filepath.Ext(source))
	if ext != ".pdf" && ext != ".md" {
		return fmt.Errorf("must reference a .pdf or .md file")
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	absSource, err := filepath.Abs(filepath.Join(root, source))
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(absRoot, absSource)
	if err != nil || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." || filepath.IsAbs(rel) {
		return fmt.Errorf("must stay under document root")
	}
	if _, err := os.Stat(absSource); err != nil {
		return err
	}
	return nil
}

func referencedSources(items []GoldenItem) []string {
	seen := map[string]struct{}{}
	for _, item := range items {
		for _, source := range item.ExpectedSources {
			seen[source] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for source := range seen {
		out = append(out, source)
	}
	sort.Strings(out)
	return out
}
```

- [ ] **Step 4: Run fixture catalog tests and verify they pass**

Run:

```bash
cd go/eval-mcp-service
go test ./internal/fixturecatalog
```

Expected: PASS.

- [ ] **Step 5: Add failing validation tests**

Append tests covering invalid sources and item validation:

```go
func TestCatalogRejectsPathTraversalSource(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "fixture.json", `{
		"name": "bad",
		"items": [{
			"query": "q",
			"expected_answer": "a",
			"expected_sources": ["../secret.pdf"]
		}]
	}`)

	_, err := New([]string{root}).Load("fixture.json")
	if err == nil || !strings.Contains(err.Error(), "must stay under document root") {
		t.Fatalf("error = %v", err)
	}
}

func TestCatalogRejectsMissingSource(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "fixture.json", `{
		"name": "bad",
		"items": [{
			"query": "q",
			"expected_answer": "a",
			"expected_sources": ["missing.pdf"]
		}]
	}`)

	_, err := New([]string{root}).Load("fixture.json")
	if err == nil || !strings.Contains(err.Error(), "missing.pdf") {
		t.Fatalf("error = %v", err)
	}
}

func TestCatalogRejectsInvalidShape(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "fixture.json", `{"name": "bad name", "items": []}`)

	_, err := New([]string{root}).Load("fixture.json")
	if err == nil || !strings.Contains(err.Error(), "name must match") || !strings.Contains(err.Error(), "items must contain") {
		t.Fatalf("error = %v", err)
	}
}
```

Also add `strings` to the test imports.

- [ ] **Step 6: Run validation tests**

Run:

```bash
cd go/eval-mcp-service
go test ./internal/fixturecatalog
```

Expected: PASS.

- [ ] **Step 7: Commit fixture catalog**

```bash
git add go/eval-mcp-service/internal/fixturecatalog/catalog.go go/eval-mcp-service/internal/fixturecatalog/catalog_test.go
git commit -m "feat: add eval dataset fixture catalog"
```

### Task 2: Eval And Ingestion API Clients

**Files:**
- Modify: `go/eval-mcp-service/internal/evalapi/client.go`
- Modify: `go/eval-mcp-service/internal/evalapi/client_test.go`
- Create: `go/eval-mcp-service/internal/ingestionapi/client.go`
- Create: `go/eval-mcp-service/internal/ingestionapi/client_test.go`

- [ ] **Step 1: Add failing eval API dataset creation test**

Append to `go/eval-mcp-service/internal/evalapi/client_test.go`:

```go
func TestClientCreateDataset(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/datasets" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var body CreateDatasetRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body.Name != "product-docs-rag-v1" || len(body.Items) != 1 || body.Items[0].ExpectedSources[0] != "laptop-pro-15-specs.pdf" {
			t.Fatalf("unexpected body: %#v", body)
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"id": "ds-123"})
	}))
	defer server.Close()

	client := New(server.URL, "", server.Client())
	got, err := client.CreateDataset(context.Background(), CreateDatasetRequest{
		Name: "product-docs-rag-v1",
		Items: []GoldenItem{{
			Query:           "How long?",
			ExpectedAnswer:  "Up to 10 hours.",
			ExpectedSources: []string{"laptop-pro-15-specs.pdf"},
		}},
	})
	if err != nil {
		t.Fatalf("CreateDataset returned error: %v", err)
	}
	if got.ID != "ds-123" {
		t.Fatalf("ID = %q", got.ID)
	}
}
```

- [ ] **Step 2: Run eval API tests and verify they fail**

Run:

```bash
cd go/eval-mcp-service
go test ./internal/evalapi -run TestClientCreateDataset
```

Expected: FAIL because `CreateDatasetRequest` and `CreateDataset` are undefined.

- [ ] **Step 3: Implement eval API dataset creation**

In `go/eval-mcp-service/internal/evalapi/client.go`, add near dataset types:

```go
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
```

Add method after `ListDatasets`:

```go
func (c *Client) CreateDataset(ctx context.Context, body CreateDatasetRequest) (CreateDatasetResponse, error) {
	var response CreateDatasetResponse
	if err := c.do(ctx, http.MethodPost, "/datasets", body, &response); err != nil {
		return CreateDatasetResponse{}, err
	}
	return response, nil
}
```

- [ ] **Step 4: Run eval API tests**

Run:

```bash
cd go/eval-mcp-service
go test ./internal/evalapi
```

Expected: PASS.

- [ ] **Step 5: Add failing ingestion client tests**

Create `go/eval-mcp-service/internal/ingestionapi/client_test.go`:

```go
package ingestionapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientListCollections(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/collections" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"collections": []map[string]any{{"name": "documents", "points_count": 15}},
		})
	}))
	defer server.Close()

	got, err := New(server.URL, "", server.Client()).ListCollections(context.Background())
	if err != nil {
		t.Fatalf("ListCollections returned error: %v", err)
	}
	if len(got) != 1 || got[0].Name != "documents" || got[0].PointsCount != 15 {
		t.Fatalf("unexpected collections: %#v", got)
	}
}

func TestClientGetCollectionConfig(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/collections/documents/config" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"chunk_size": 1000, "embedding_model": "nomic-embed-text"})
	}))
	defer server.Close()

	got, err := New(server.URL, "", server.Client()).GetCollectionConfig(context.Background(), "documents")
	if err != nil {
		t.Fatalf("GetCollectionConfig returned error: %v", err)
	}
	if got["embedding_model"] != "nomic-embed-text" {
		t.Fatalf("unexpected config: %#v", got)
	}
}
```

- [ ] **Step 6: Run ingestion client tests and verify they fail**

Run:

```bash
cd go/eval-mcp-service
go test ./internal/ingestionapi
```

Expected: FAIL because the package does not exist.

- [ ] **Step 7: Implement ingestion API client**

Create `go/eval-mcp-service/internal/ingestionapi/client.go`:

```go
package ingestionapi

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

const errorExcerptLimit = 256

type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

type Collection struct {
	Name        string `json:"name"`
	PointsCount int    `json:"points_count"`
}

type HTTPError struct {
	Method     string
	Path       string
	StatusCode int
	Excerpt    string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("%s %s: status %d: %s", e.Method, e.Path, e.StatusCode, e.Excerpt)
}

func New(baseURL, token string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), token: token, httpClient: httpClient}
}

func (c *Client) ListCollections(ctx context.Context) ([]Collection, error) {
	var response struct {
		Collections []Collection `json:"collections"`
	}
	if err := c.do(ctx, http.MethodGet, "/collections", nil, &response); err != nil {
		return nil, err
	}
	return response.Collections, nil
}

func (c *Client) GetCollectionConfig(ctx context.Context, name string) (map[string]any, error) {
	var response map[string]any
	path := "/collections/" + url.PathEscape(name) + "/config"
	if err := c.do(ctx, http.MethodGet, path, nil, &response); err != nil {
		return nil, err
	}
	return response, nil
}

func (c *Client) do(ctx context.Context, method, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, errorExcerptLimit))
		return &HTTPError{Method: method, Path: path, StatusCode: resp.StatusCode, Excerpt: string(data)}
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
```

- [ ] **Step 8: Run client tests**

Run:

```bash
cd go/eval-mcp-service
go test ./internal/evalapi ./internal/ingestionapi
```

Expected: PASS.

- [ ] **Step 9: Commit API clients**

```bash
git add go/eval-mcp-service/internal/evalapi/client.go go/eval-mcp-service/internal/evalapi/client_test.go go/eval-mcp-service/internal/ingestionapi/client.go go/eval-mcp-service/internal/ingestionapi/client_test.go
git commit -m "feat: add eval dataset and ingestion clients"
```

### Task 3: Configuration And App Wiring

**Files:**
- Modify: `go/eval-mcp-service/internal/config/config.go`
- Modify: `go/eval-mcp-service/internal/config/config_test.go`
- Modify: `go/eval-mcp-service/cmd/eval-mcp/main.go`
- Modify: `go/eval-mcp-service/cmd/eval-mcp/main_test.go`

- [ ] **Step 1: Add failing config tests**

In `go/eval-mcp-service/internal/config/config_test.go`, update `TestFromEnvDefaults` to clear:

```go
t.Setenv("EVAL_MCP_INGESTION_URL", "")
t.Setenv("EVAL_MCP_DATASET_FIXTURE_ROOTS", "")
```

Add assertions:

```go
if cfg.IngestionURL != "http://localhost:8000/ingestion" {
	t.Fatalf("IngestionURL = %q", cfg.IngestionURL)
}
if len(cfg.DatasetFixtureRoots) != 1 || !strings.HasSuffix(cfg.DatasetFixtureRoots[0], "docs/product-catalog") {
	t.Fatalf("DatasetFixtureRoots = %#v", cfg.DatasetFixtureRoots)
}
```

In `TestFromEnvOverrides`, add:

```go
t.Setenv("EVAL_MCP_INGESTION_URL", "http://127.0.0.1:8000/ingestion")
t.Setenv("EVAL_MCP_DATASET_FIXTURE_ROOTS", "/tmp/a:/tmp/b")
```

Add assertions:

```go
if cfg.IngestionURL != "http://127.0.0.1:8000/ingestion" {
	t.Fatalf("IngestionURL = %q", cfg.IngestionURL)
}
if !slices.Equal(cfg.DatasetFixtureRoots, []string{"/tmp/a", "/tmp/b"}) {
	t.Fatalf("DatasetFixtureRoots = %#v", cfg.DatasetFixtureRoots)
}
```

Add `slices` to imports.

- [ ] **Step 2: Run config tests and verify they fail**

Run:

```bash
cd go/eval-mcp-service
go test ./internal/config
```

Expected: FAIL because new config fields do not exist.

- [ ] **Step 3: Implement config fields**

In `go/eval-mcp-service/internal/config/config.go`, add imports:

```go
	"path/filepath"
	"strings"
```

Add constants:

```go
	defaultIngestionURL = "http://localhost:8000/ingestion"
```

Add fields:

```go
	IngestionURL        string
	DatasetFixtureRoots []string
```

Add to `FromEnv` return:

```go
		IngestionURL:        getenv("EVAL_MCP_INGESTION_URL", defaultIngestionURL),
		DatasetFixtureRoots: datasetFixtureRoots(),
```

Add helper:

```go
func datasetFixtureRoots() []string {
	value := os.Getenv("EVAL_MCP_DATASET_FIXTURE_ROOTS")
	if value != "" {
		parts := strings.Split(value, string(os.PathListSeparator))
		roots := make([]string, 0, len(parts))
		for _, part := range parts {
			if strings.TrimSpace(part) != "" {
				roots = append(roots, part)
			}
		}
		return roots
	}
	return []string{filepath.Clean(filepath.Join("..", "..", "docs", "product-catalog"))}
}
```

- [ ] **Step 4: Run config tests**

Run:

```bash
cd go/eval-mcp-service
go test ./internal/config
```

Expected: PASS.

- [ ] **Step 5: Update app wiring**

In `go/eval-mcp-service/cmd/eval-mcp/main.go`, add imports:

```go
	"github.com/kabradshaw1/portfolio/go/eval-mcp-service/internal/fixturecatalog"
	"github.com/kabradshaw1/portfolio/go/eval-mcp-service/internal/ingestionapi"
```

Construct clients:

```go
	ingestion := ingestionapi.New(cfg.IngestionURL, cfg.APIToken, httpClient)
	fixtures := fixturecatalog.New(cfg.DatasetFixtureRoots)
	service := evalworkflow.New(api, ingestion, fixtures, cfg.PollInterval, cfg.WaitTimeout)
	logger.Printf("eval MCP server running on stdio eval_api_url=%s ingestion_url=%s", cfg.EvalAPIURL, cfg.IngestionURL)
```

- [ ] **Step 6: Run compile tests and verify expected constructor failures**

Run:

```bash
cd go/eval-mcp-service
go test ./cmd/eval-mcp ./internal/evalworkflow
```

Expected: FAIL until `evalworkflow.New` is updated in Task 4.

- [ ] **Step 7: Commit config and wiring after Task 4 passes**

Defer commit until Task 4 updates the workflow constructor, then include these files in the Task 4 commit.

### Task 4: Workflow Service

**Files:**
- Modify: `go/eval-mcp-service/internal/evalworkflow/service.go`
- Modify: `go/eval-mcp-service/internal/evalworkflow/service_test.go`
- Modify: files from Task 3 if not committed

- [ ] **Step 1: Add workflow interface and fake support in tests**

In `go/eval-mcp-service/internal/evalworkflow/service_test.go`, extend the fake API with:

```go
createDatasetRequest evalapi.CreateDatasetRequest
createdDatasetID      string
evaluationsByID       map[string]evalapi.EvaluationDetail
collections           []ingestionapi.Collection
collectionConfigs     map[string]map[string]any
fixture               fixturecatalog.Fixture
```

Add fake methods:

```go
func (f *fakeAPI) CreateDataset(_ context.Context, req evalapi.CreateDatasetRequest) (evalapi.CreateDatasetResponse, error) {
	f.createDatasetRequest = req
	id := f.createdDatasetID
	if id == "" {
		id = "ds-created"
	}
	return evalapi.CreateDatasetResponse{ID: id}, nil
}
```

Create these separate fake types:

```go
type fakeIngestion struct {
	collections []ingestionapi.Collection
	configs     map[string]map[string]any
}

func (f fakeIngestion) ListCollections(context.Context) ([]ingestionapi.Collection, error) {
	return f.collections, nil
}

func (f fakeIngestion) GetCollectionConfig(_ context.Context, name string) (map[string]any, error) {
	return f.configs[name], nil
}

type fakeFixtures struct {
	fixtures []fixturecatalog.Fixture
	fixture  fixturecatalog.Fixture
}

func (f fakeFixtures) List() ([]fixturecatalog.Fixture, error) { return f.fixtures, nil }
func (f fakeFixtures) Load(string) (fixturecatalog.Fixture, error) { return f.fixture, nil }
```

- [ ] **Step 2: Add failing workflow tests**

Add tests:

```go
func TestCreateDatasetFromFixture(t *testing.T) {
	api := &fakeAPI{}
	fixture := fixturecatalog.Fixture{
		Name: "product-docs-rag-v1",
		Items: []fixturecatalog.GoldenItem{{
			Query: "q", ExpectedAnswer: "a", ExpectedSources: []string{"doc.pdf"},
		}},
		Valid: true,
	}
	service := New(api, fakeIngestion{}, fakeFixtures{fixture: fixture}, time.Second, time.Minute)

	got, err := service.CreateDatasetFromFixture(context.Background(), "rag-eval-dataset-product-docs.json")
	if err != nil {
		t.Fatalf("CreateDatasetFromFixture returned error: %v", err)
	}
	if got.ID != "ds-created" || got.Name != "product-docs-rag-v1" {
		t.Fatalf("unexpected result: %#v", got)
	}
	if api.createDatasetRequest.Name != "product-docs-rag-v1" || len(api.createDatasetRequest.Items) != 1 {
		t.Fatalf("unexpected request: %#v", api.createDatasetRequest)
	}
}

func TestStartRunRejectsMissingCollection(t *testing.T) {
	api := &fakeAPI{}
	service := New(api, fakeIngestion{collections: []ingestionapi.Collection{{Name: "documents"}}}, fakeFixtures{}, time.Second, time.Minute)
	_, err := service.StartRun(context.Background(), StartRunInput{DatasetID: "ds-1", Collection: "missing"})
	if err == nil || !strings.Contains(err.Error(), `retrieval collection "missing" does not exist`) {
		t.Fatalf("error = %v", err)
	}
}

func TestCompareRejectsNonCompletedRuns(t *testing.T) {
	api := &fakeAPI{evaluationsByID: map[string]evalapi.EvaluationDetail{
		"base": {ID: "base", Status: "completed"},
		"bad":  {ID: "bad", Status: "failed"},
	}}
	service := New(api, fakeIngestion{}, fakeFixtures{}, time.Second, time.Minute)
	_, err := service.Compare(context.Background(), CompareInput{EvalIDs: []string{"base", "bad"}})
	if err == nil || !strings.Contains(err.Error(), `bad=failed`) {
		t.Fatalf("error = %v", err)
	}
}
```

Add imports for `strings`, `fixturecatalog`, and `ingestionapi`.

- [ ] **Step 3: Run workflow tests and verify they fail**

Run:

```bash
cd go/eval-mcp-service
go test ./internal/evalworkflow
```

Expected: FAIL because workflow constructor and methods are not implemented.

- [ ] **Step 4: Update workflow interfaces and constructor**

In `go/eval-mcp-service/internal/evalworkflow/service.go`, update imports:

```go
	"github.com/kabradshaw1/portfolio/go/eval-mcp-service/internal/fixturecatalog"
	"github.com/kabradshaw1/portfolio/go/eval-mcp-service/internal/ingestionapi"
```

Add to `API`:

```go
	CreateDataset(context.Context, evalapi.CreateDatasetRequest) (evalapi.CreateDatasetResponse, error)
```

Add interfaces:

```go
type Ingestion interface {
	ListCollections(context.Context) ([]ingestionapi.Collection, error)
	GetCollectionConfig(context.Context, string) (map[string]any, error)
}

type Fixtures interface {
	List() ([]fixturecatalog.Fixture, error)
	Load(string) (fixturecatalog.Fixture, error)
}
```

Update service fields and constructor:

```go
type Service struct {
	api          API
	ingestion    Ingestion
	fixtures     Fixtures
	pollInterval time.Duration
	waitTimeout  time.Duration
}

func New(api API, ingestion Ingestion, fixtures Fixtures, pollInterval, waitTimeout time.Duration) *Service {
	return &Service{api: api, ingestion: ingestion, fixtures: fixtures, pollInterval: pollInterval, waitTimeout: waitTimeout}
}
```

- [ ] **Step 5: Implement fixture and collection workflow methods**

Add types:

```go
type CreateDatasetResult struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
```

Add methods:

```go
func (s *Service) ListDatasetFixtures() ([]fixturecatalog.Fixture, error) {
	return s.fixtures.List()
}

func (s *Service) CreateDatasetFromFixture(ctx context.Context, fixtureID string) (CreateDatasetResult, error) {
	fixture, err := s.fixtures.Load(fixtureID)
	if err != nil {
		return CreateDatasetResult{}, err
	}
	items := make([]evalapi.GoldenItem, 0, len(fixture.Items))
	for _, item := range fixture.Items {
		items = append(items, evalapi.GoldenItem{
			Query:           item.Query,
			ExpectedAnswer:  item.ExpectedAnswer,
			ExpectedSources: item.ExpectedSources,
		})
	}
	created, err := s.api.CreateDataset(ctx, evalapi.CreateDatasetRequest{Name: fixture.Name, Items: items})
	if err != nil {
		return CreateDatasetResult{}, err
	}
	return CreateDatasetResult{ID: created.ID, Name: fixture.Name}, nil
}

func (s *Service) ListRAGCollections(ctx context.Context) ([]ingestionapi.Collection, error) {
	return s.ingestion.ListCollections(ctx)
}

func (s *Service) GetRAGCollectionConfig(ctx context.Context, name string) (map[string]any, error) {
	return s.ingestion.GetCollectionConfig(ctx, name)
}
```

- [ ] **Step 6: Add collection validation to experiment and run creation**

Add helper:

```go
func (s *Service) validateCollectionExists(ctx context.Context, collection string) error {
	collections, err := s.ingestion.ListCollections(ctx)
	if err != nil {
		return fmt.Errorf("list RAG collections: %w", err)
	}
	for _, candidate := range collections {
		if candidate.Name == collection {
			return nil
		}
	}
	return fmt.Errorf("retrieval collection %q does not exist; call list_rag_collections and choose an existing collection", collection)
}
```

Call it in `StartExperiment` after defaulting `collection`, before `CreateExperiment`:

```go
	if err := s.validateCollectionExists(ctx, collection); err != nil {
		return evalapi.Experiment{}, err
	}
```

Call it in `StartRun` before `StartEvaluation`:

```go
	if err := s.validateCollectionExists(ctx, in.Collection); err != nil {
		return StartRunResult{}, err
	}
```

- [ ] **Step 7: Guard comparisons and summaries**

Add helper:

```go
func (s *Service) requireCompletedRuns(ctx context.Context, ids []string) error {
	var invalid []string
	for _, id := range ids {
		run, err := s.api.GetEvaluation(ctx, id)
		if err != nil {
			return fmt.Errorf("get run %q: %w", id, err)
		}
		if run.Status != "completed" {
			invalid = append(invalid, fmt.Sprintf("%s=%s", id, run.Status))
		}
	}
	if len(invalid) > 0 {
		return fmt.Errorf("compare requires completed runs; invalid statuses: %s", strings.Join(invalid, ", "))
	}
	return nil
}
```

Call it in `Compare` after `validateCompareIDs(ids)` and before `CompareEvaluations`.

In `SummarizeExperiment`, only append completed run IDs to `evalIDs`; if any attached run is not completed, return:

```go
return ExperimentSummary{}, fmt.Errorf("summarize requires completed runs; %s=%s", run.ID, run.Status)
```

- [ ] **Step 8: Run workflow and app tests**

Run:

```bash
cd go/eval-mcp-service
go test ./internal/evalworkflow ./cmd/eval-mcp
```

Expected: PASS after updating old constructor calls in tests to pass fakes.

- [ ] **Step 9: Commit workflow and wiring**

```bash
git add go/eval-mcp-service/internal/config/config.go go/eval-mcp-service/internal/config/config_test.go go/eval-mcp-service/cmd/eval-mcp/main.go go/eval-mcp-service/cmd/eval-mcp/main_test.go go/eval-mcp-service/internal/evalworkflow/service.go go/eval-mcp-service/internal/evalworkflow/service_test.go
git commit -m "feat: validate eval MCP datasets and collections"
```

### Task 5: MCP Tools And Prompt

**Files:**
- Modify: `go/eval-mcp-service/internal/mcpserver/server.go`
- Modify: `go/eval-mcp-service/internal/mcpserver/server_test.go`

- [ ] **Step 1: Add failing MCP registration tests**

In `go/eval-mcp-service/internal/mcpserver/server_test.go`, extend `fakeEvalService` with methods:

```go
func (f *fakeEvalService) ListDatasetFixtures(context.Context) ([]fixturecatalog.Fixture, error) {
	return []fixturecatalog.Fixture{{ID: "rag-eval-dataset-product-docs.json", Name: "product-docs-rag-v1", Valid: true}}, nil
}

func (f *fakeEvalService) CreateDatasetFromFixture(context.Context, string) (evalworkflow.CreateDatasetResult, error) {
	return evalworkflow.CreateDatasetResult{ID: "ds-created", Name: "product-docs-rag-v1"}, nil
}

func (f *fakeEvalService) ListRAGCollections(context.Context) ([]ingestionapi.Collection, error) {
	return []ingestionapi.Collection{{Name: "documents", PointsCount: 15}}, nil
}

func (f *fakeEvalService) GetRAGCollectionConfig(context.Context, string) (map[string]any, error) {
	return map[string]any{"chunk_size": 1000}, nil
}
```

Update `wantTools` to include:

```go
"create_eval_dataset",
"get_rag_collection_config",
"list_eval_dataset_fixtures",
"list_rag_collections",
```

Add imports for `fixturecatalog` and `ingestionapi`.

- [ ] **Step 2: Run MCP tests and verify they fail**

Run:

```bash
cd go/eval-mcp-service
go test ./internal/mcpserver -run TestServerRegistersPromptResourceAndTools
```

Expected: FAIL because tools are not registered and interface methods do not exist.

- [ ] **Step 3: Update MCP service interface and registration**

In `go/eval-mcp-service/internal/mcpserver/server.go`, add imports:

```go
	"github.com/kabradshaw1/portfolio/go/eval-mcp-service/internal/fixturecatalog"
	"github.com/kabradshaw1/portfolio/go/eval-mcp-service/internal/ingestionapi"
```

Add to `EvalService`:

```go
	ListDatasetFixtures(context.Context) ([]fixturecatalog.Fixture, error)
	CreateDatasetFromFixture(context.Context, string) (evalworkflow.CreateDatasetResult, error)
	ListRAGCollections(context.Context) ([]ingestionapi.Collection, error)
	GetRAGCollectionConfig(context.Context, string) (map[string]any, error)
```

Register tools in `New`:

```go
	addTool(srv, "list_eval_dataset_fixtures", "List curated eval dataset fixtures available in the repo.", emptySchema(), listEvalDatasetFixturesHandler(service))
	addTool(srv, "create_eval_dataset", "Create an eval API dataset from a curated repo fixture.", createEvalDatasetSchema(), createEvalDatasetHandler(service))
	addTool(srv, "list_rag_collections", "List Qdrant retrieval collections from ingestion.", emptySchema(), listRAGCollectionsHandler(service))
	addTool(srv, "get_rag_collection_config", "Fetch ingestion metadata for one RAG collection.", ragCollectionConfigSchema(), getRAGCollectionConfigHandler(service))
```

- [ ] **Step 4: Add handlers and schemas**

Add handlers:

```go
func listEvalDatasetFixturesHandler(service EvalService) sdkmcp.ToolHandler {
	return func(ctx context.Context, _ *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		result, err := service.ListDatasetFixtures(ctx)
		return resultOrError(result, err), nil
	}
}

func createEvalDatasetHandler(service EvalService) sdkmcp.ToolHandler {
	return func(ctx context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		var args struct{ Fixture string `json:"fixture"` }
		if err := decodeArgs(req, &args); err != nil {
			return toolError(err.Error()), nil
		}
		if strings.TrimSpace(args.Fixture) == "" {
			return toolError("fixture is required"), nil
		}
		result, err := service.CreateDatasetFromFixture(ctx, args.Fixture)
		return resultOrError(result, err), nil
	}
}

func listRAGCollectionsHandler(service EvalService) sdkmcp.ToolHandler {
	return func(ctx context.Context, _ *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		result, err := service.ListRAGCollections(ctx)
		return resultOrError(result, err), nil
	}
}

func getRAGCollectionConfigHandler(service EvalService) sdkmcp.ToolHandler {
	return func(ctx context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		var args struct{ Name string `json:"name"` }
		if err := decodeArgs(req, &args); err != nil {
			return toolError(err.Error()), nil
		}
		if strings.TrimSpace(args.Name) == "" {
			return toolError("name is required"), nil
		}
		result, err := service.GetRAGCollectionConfig(ctx, args.Name)
		return resultOrError(result, err), nil
	}
}
```

Add schemas:

```go
func createEvalDatasetSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"fixture":{"type":"string","minLength":1}},"required":["fixture"],"additionalProperties":false}`)
}

func ragCollectionConfigSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"name":{"type":"string","minLength":1}},"required":["name"],"additionalProperties":false}`)
}
```

- [ ] **Step 5: Add handler tests**

Add these exact handler tests:

```go
func TestCreateEvalDatasetHandlerRequiresFixture(t *testing.T) {
	result, err := createEvalDatasetHandler(&fakeEvalService{})(context.Background(), callReq(map[string]any{}))
	if err != nil {
		t.Fatalf("handler returned transport error: %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected MCP tool error")
	}
}

func TestListRAGCollectionsHandler(t *testing.T) {
	result, err := listRAGCollectionsHandler(&fakeEvalService{})(context.Background(), callReq(map[string]any{}))
	if err != nil || result.IsError {
		t.Fatalf("handler failed: result=%#v err=%v", result, err)
	}
	var payload []ingestionapi.Collection
	unmarshalTextResult(t, result, &payload)
	if len(payload) != 1 || payload[0].Name != "documents" {
		t.Fatalf("unexpected payload: %#v", payload)
	}
}
```

- [ ] **Step 6: Update prompt and workflow text**

In `evalPromptHandler` and `workflowResourceHandler`, add explicit text:

```text
Datasets are golden questions and expected answers. Collections are Qdrant retrieval corpora. Never infer a collection from a dataset name. Use list_eval_dataset_fixtures and create_eval_dataset for curated repo fixtures, then use list_rag_collections and get_rag_collection_config before start_eval_experiment or start_eval_run. Run baseline to completion before starting rerank while runtime hardening is pending. Compare only completed runs.
```

Update prompt/resource tests to assert the new tool names and the phrases `Never infer a collection from a dataset name` and `Compare only completed runs`.

- [ ] **Step 7: Run MCP tests**

Run:

```bash
cd go/eval-mcp-service
go test ./internal/mcpserver
```

Expected: PASS.

- [ ] **Step 8: Commit MCP tools**

```bash
git add go/eval-mcp-service/internal/mcpserver/server.go go/eval-mcp-service/internal/mcpserver/server_test.go
git commit -m "feat: expose reliable eval MCP workflow tools"
```

### Task 6: README And Verification

**Files:**
- Modify: `go/eval-mcp-service/README.md`

- [ ] **Step 1: Update README configuration**

In `go/eval-mcp-service/README.md`, add config bullets:

```markdown
- `EVAL_MCP_INGESTION_URL`: ingestion API base URL for collection discovery,
  defaults to `http://localhost:8000/ingestion`.
- `EVAL_MCP_DATASET_FIXTURE_ROOTS`: path-list of curated eval fixture roots,
  defaults to the repo `docs/product-catalog` directory.
```

- [ ] **Step 2: Update README tools list**

Add tools:

```markdown
- `list_eval_dataset_fixtures`
- `create_eval_dataset`
- `list_rag_collections`
- `get_rag_collection_config`
```

- [ ] **Step 3: Add workflow note**

Add a short paragraph:

```markdown
Eval datasets and RAG collections are separate concepts. Datasets contain
golden questions and expected answers. Collections are Qdrant retrieval corpora.
Use curated fixture tools to create missing datasets, then validate the chosen
RAG collection before starting baseline and rerank runs.
```

- [ ] **Step 4: Run focused Go tests**

Run:

```bash
cd go/eval-mcp-service
go test ./...
```

Expected: PASS.

- [ ] **Step 5: Run Go preflight**

Run:

```bash
make preflight-go
```

Expected: PASS. If blocked by local environment, capture the exact blocker and leave verification to CI.

- [ ] **Step 6: Commit README and any final fixes**

```bash
git add go/eval-mcp-service/README.md
git commit -m "docs: document eval MCP dataset workflow"
```

- [ ] **Step 7: Push and open PR**

Use a feature branch/worktree for implementation. Push the branch and create a PR to `qa`.
