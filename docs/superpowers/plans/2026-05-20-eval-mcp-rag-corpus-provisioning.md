# Eval MCP RAG Corpus Provisioning Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add controlled eval MCP tools that provision curated RAG corpus fixtures into managed Qdrant collections through the ingestion service.

**Architecture:** Keep ingestion as the only Qdrant writer. Add corpus manifest storage to `services/ingestion`, then add Go eval MCP fixture/provisioning code that uploads only curated repo PDFs into namespaced `eval_*` collections and exposes the workflow as MCP tools.

**Tech Stack:** Python FastAPI, aiosqlite, pytest, Go stdlib HTTP multipart client, modelcontextprotocol Go SDK, existing eval MCP workflow service.

---

## File Map

- `services/ingestion/app/collection_meta.py`: extend SQLite metadata storage with corpus manifest table and methods.
- `services/ingestion/app/main.py`: add authenticated manifest `PUT`/`GET` endpoints.
- `services/ingestion/tests/test_collection_meta.py`: unit tests for manifest persistence.
- `services/ingestion/tests/test_main.py`: endpoint tests for manifest behavior and validation.
- `go/eval-mcp-service/internal/config/config.go`: add `EVAL_MCP_CORPUS_FIXTURE_ROOTS` config.
- `go/eval-mcp-service/internal/corpusfixture/catalog.go`: new curated RAG corpus fixture catalog.
- `go/eval-mcp-service/internal/corpusfixture/catalog_test.go`: catalog hashing, validation, and naming tests.
- `go/eval-mcp-service/internal/ingestionapi/client.go`: add multipart upload, manifest get/put, and collection delete methods.
- `go/eval-mcp-service/internal/ingestionapi/client_test.go`: HTTP contract tests for new ingestion client methods.
- `go/eval-mcp-service/internal/evalworkflow/service.go`: add corpus fixture/provision/delete methods and result types.
- `go/eval-mcp-service/internal/evalworkflow/service_test.go`: workflow tests for idempotency, guardrails, and force recreate.
- `go/eval-mcp-service/internal/mcpserver/server.go`: add MCP interface methods, schemas, handlers, tool registration, and workflow text.
- `go/eval-mcp-service/internal/mcpserver/server_test.go`: assert tool exposure and handler behavior.
- `go/eval-mcp-service/cmd/eval-mcp/main.go`: wire corpus fixture catalog into the service.
- `go/eval-mcp-service/README.md`: document new env var and tools.

## Task 1: Create Feature Worktree

**Files:**
- No code files changed in this task.

- [ ] **Step 1: Create the feature worktree**

Run from repo root:

```bash
git fetch origin
mkdir -p .codex/worktrees
git worktree add .codex/worktrees/eval-mcp-rag-corpus-provisioning -b eval-mcp-rag-corpus-provisioning main
```

Expected: new worktree at `.codex/worktrees/eval-mcp-rag-corpus-provisioning`.

- [ ] **Step 2: Switch all work into the worktree**

Run:

```bash
cd .codex/worktrees/eval-mcp-rag-corpus-provisioning
pwd
git branch --show-current
git rev-parse --show-toplevel
```

Expected:

```text
/Users/kylebradshaw/repos/gen_ai_engineer/.codex/worktrees/eval-mcp-rag-corpus-provisioning
eval-mcp-rag-corpus-provisioning
/Users/kylebradshaw/repos/gen_ai_engineer/.codex/worktrees/eval-mcp-rag-corpus-provisioning
```

- [ ] **Step 3: Commit checkpoint**

No commit is required for the worktree setup task.

## Task 2: Add Ingestion Corpus Manifest Storage

**Files:**
- Modify: `services/ingestion/app/collection_meta.py`
- Test: `services/ingestion/tests/test_collection_meta.py`

- [ ] **Step 1: Write failing manifest persistence tests**

Append these tests to `services/ingestion/tests/test_collection_meta.py`:

```python
@pytest.mark.asyncio
async def test_manifest_round_trip(tmp_path):
    db = CollectionMetaDB(str(tmp_path / "meta.db"))
    await db.init()
    manifest = {
        "collection": "eval_product_catalog_v1_a1b2c3d4",
        "fixture_id": "product_catalog_v1",
        "fixture_hash": "a1b2c3d4",
        "documents": [
            {
                "path": "docs/product-catalog/laptop-pro-15-specs.pdf",
                "sha256": "abc123",
            }
        ],
        "provisioned_at": "2026-05-20T00:00:00Z",
        "provisioned_by": "eval-mcp-service",
    }

    await db.upsert_manifest("eval_product_catalog_v1_a1b2c3d4", manifest)

    assert await db.get_manifest("eval_product_catalog_v1_a1b2c3d4") == manifest
    await db.close()


@pytest.mark.asyncio
async def test_get_missing_manifest_returns_none(tmp_path):
    db = CollectionMetaDB(str(tmp_path / "meta.db"))
    await db.init()
    assert await db.get_manifest("missing") is None
    await db.close()
```

- [ ] **Step 2: Run tests to verify failure**

Run:

```bash
PYTHONPATH=services pytest services/ingestion/tests/test_collection_meta.py -v
```

Expected: failures mention missing `upsert_manifest` or `get_manifest`.

- [ ] **Step 3: Implement manifest table and methods**

Modify `services/ingestion/app/collection_meta.py`:

```python
import json
```

Add this table creation in `CollectionMetaDB.init()` after the existing
`collection_meta` migration block:

```python
        await self._db.execute(
            """
            CREATE TABLE IF NOT EXISTS collection_manifests (
                collection TEXT PRIMARY KEY,
                manifest_json TEXT NOT NULL
            )
            """
        )
```

Add these methods to `CollectionMetaDB`:

```python
    async def upsert_manifest(self, collection: str, manifest: dict) -> None:
        await self._db.execute(
            "INSERT INTO collection_manifests (collection, manifest_json) "
            "VALUES (?, ?) "
            "ON CONFLICT(collection) DO UPDATE SET "
            "manifest_json=excluded.manifest_json",
            (collection, json.dumps(manifest, sort_keys=True)),
        )
        await self._db.commit()

    async def get_manifest(self, collection: str) -> dict | None:
        cursor = await self._db.execute(
            "SELECT manifest_json FROM collection_manifests WHERE collection = ?",
            (collection,),
        )
        row = await cursor.fetchone()
        if not row:
            return None
        return json.loads(row["manifest_json"])
```

- [ ] **Step 4: Run tests to verify pass**

Run:

```bash
PYTHONPATH=services pytest services/ingestion/tests/test_collection_meta.py -v
```

Expected: all tests in `test_collection_meta.py` pass.

- [ ] **Step 5: Commit**

Run:

```bash
git add services/ingestion/app/collection_meta.py services/ingestion/tests/test_collection_meta.py
git commit -m "feat: store ingestion collection manifests"
```

## Task 3: Add Ingestion Manifest Endpoints

**Files:**
- Modify: `services/ingestion/app/main.py`
- Test: `services/ingestion/tests/test_main.py`

- [ ] **Step 1: Write failing endpoint tests**

Append tests to `services/ingestion/tests/test_main.py` near the existing
collection config tests:

```python
@patch("app.main.get_meta_db")
def test_put_collection_manifest_stores_metadata(mock_get_meta_db):
    mock_db = AsyncMock()
    mock_get_meta_db.return_value = mock_db
    payload = {
        "collection": "eval_product_catalog_v1_a1b2c3d4",
        "fixture_id": "product_catalog_v1",
        "fixture_hash": "a1b2c3d4",
        "documents": [],
        "provisioned_at": "2026-05-20T00:00:00Z",
        "provisioned_by": "eval-mcp-service",
    }

    response = client.put(
        "/collections/eval_product_catalog_v1_a1b2c3d4/manifest",
        json=payload,
    )

    assert response.status_code == 200
    assert response.json() == {"status": "stored", "collection": "eval_product_catalog_v1_a1b2c3d4"}
    mock_db.upsert_manifest.assert_awaited_once_with(
        "eval_product_catalog_v1_a1b2c3d4", payload
    )


@patch("app.main.get_meta_db")
def test_get_collection_manifest_returns_metadata(mock_get_meta_db):
    payload = {
        "collection": "eval_product_catalog_v1_a1b2c3d4",
        "fixture_id": "product_catalog_v1",
        "fixture_hash": "a1b2c3d4",
        "documents": [],
        "provisioned_at": "2026-05-20T00:00:00Z",
        "provisioned_by": "eval-mcp-service",
    }
    mock_db = AsyncMock()
    mock_db.get_manifest.return_value = payload
    mock_get_meta_db.return_value = mock_db

    response = client.get("/collections/eval_product_catalog_v1_a1b2c3d4/manifest")

    assert response.status_code == 200
    assert response.json() == payload


@patch("app.main.get_meta_db")
def test_get_collection_manifest_404_when_unknown(mock_get_meta_db):
    mock_db = AsyncMock()
    mock_db.get_manifest.return_value = None
    mock_get_meta_db.return_value = mock_db

    response = client.get("/collections/eval_product_catalog_v1_a1b2c3d4/manifest")

    assert response.status_code == 404
```

If `AsyncMock`, `patch`, or `client` are already imported, reuse existing
imports instead of adding duplicates.

- [ ] **Step 2: Run tests to verify failure**

Run:

```bash
PYTHONPATH=services pytest services/ingestion/tests/test_main.py -k "collection_manifest" -v
```

Expected: 404 or route-not-found failures.

- [ ] **Step 3: Implement endpoints**

Add `Body` to the FastAPI imports in `services/ingestion/app/main.py`:

```python
from fastapi import Body, Depends, FastAPI, File, HTTPException, Query, UploadFile
```

Add endpoints after `get_collection_config`:

```python
@app.put("/collections/{name}/manifest")
@limiter.limit("30/minute")
async def put_collection_manifest(
    request: Request,
    name: str,
    manifest: dict = Body(...),
    user_id: str = Depends(require_auth),
):
    if not _COLLECTION_NAME_RE.match(name):
        raise HTTPException(status_code=422, detail="Invalid collection name")
    if manifest.get("collection") != name:
        raise HTTPException(
            status_code=422,
            detail="manifest collection must match path collection",
        )
    db = await get_meta_db()
    await db.upsert_manifest(name, manifest)
    return {"status": "stored", "collection": name}


@app.get("/collections/{name}/manifest")
@limiter.limit("30/minute")
async def get_collection_manifest(
    request: Request, name: str, user_id: str = Depends(require_auth)
):
    if not _COLLECTION_NAME_RE.match(name):
        raise HTTPException(status_code=422, detail="Invalid collection name")
    db = await get_meta_db()
    manifest = await db.get_manifest(name)
    if manifest is None:
        raise HTTPException(status_code=404, detail="manifest not found")
    return manifest
```

- [ ] **Step 4: Run targeted ingestion tests**

Run:

```bash
PYTHONPATH=services pytest services/ingestion/tests/test_main.py -k "collection_manifest or collection_config" -v
```

Expected: selected tests pass.

- [ ] **Step 5: Commit**

Run:

```bash
git add services/ingestion/app/main.py services/ingestion/tests/test_main.py
git commit -m "feat: expose ingestion collection manifests"
```

## Task 4: Add Go Corpus Fixture Catalog

**Files:**
- Create: `go/eval-mcp-service/internal/corpusfixture/catalog.go`
- Create: `go/eval-mcp-service/internal/corpusfixture/catalog_test.go`
- Modify: `go/eval-mcp-service/internal/config/config.go`
- Test: `go/eval-mcp-service/internal/config/config_test.go`

- [ ] **Step 1: Write catalog tests**

Create `go/eval-mcp-service/internal/corpusfixture/catalog_test.go`:

```go
package corpusfixture

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCatalogListsFixtureWithDeterministicCollection(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "laptop.pdf", "%PDF-1.4\nlaptop")
	writeFile(t, root, "product_catalog_v1.json", `{
		"id": "product_catalog_v1",
		"name": "Product Catalog v1",
		"description": "Product catalog PDFs",
		"documents": ["laptop.pdf"],
		"expected_collection_prefix": "eval_product_catalog_v1"
	}`)

	fixtures, err := New([]string{root}).List()
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(fixtures) != 1 {
		t.Fatalf("fixture count = %d", len(fixtures))
	}
	got := fixtures[0]
	if got.ID != "product_catalog_v1" || got.DocumentCount != 1 || got.SourceHash == "" {
		t.Fatalf("unexpected fixture: %#v", got)
	}
	if !strings.HasPrefix(got.DefaultCollection, "eval_product_catalog_v1_") {
		t.Fatalf("default collection = %q", got.DefaultCollection)
	}
}

func TestLoadRejectsEscapingDocument(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "bad.json", `{
		"id": "bad",
		"name": "Bad",
		"description": "Bad fixture",
		"documents": ["../secret.pdf"],
		"expected_collection_prefix": "eval_bad"
	}`)

	_, err := New([]string{root}).Load("bad")
	if err == nil || !strings.Contains(err.Error(), "must stay under fixture root") {
		t.Fatalf("error = %v", err)
	}
}

func writeFile(t *testing.T, root, name, content string) {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}
```

- [ ] **Step 2: Run tests to verify failure**

Run:

```bash
cd go/eval-mcp-service && go test ./internal/corpusfixture -run Test -count=1
```

Expected: package does not exist or tests fail before implementation.

- [ ] **Step 3: Implement catalog**

Create `go/eval-mcp-service/internal/corpusfixture/catalog.go`:

```go
package corpusfixture

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var (
	fixtureIDPattern  = regexp.MustCompile(`^[a-z0-9_]{1,80}$`)
	collectionPattern = regexp.MustCompile(`^eval_[a-z0-9_]+$`)
)

type Catalog struct {
	roots []string
}

type Fixture struct {
	ID                       string     `json:"id"`
	Name                     string     `json:"name"`
	Description              string     `json:"description"`
	Path                     string     `json:"path"`
	Root                     string     `json:"root"`
	Documents                []Document `json:"documents"`
	DocumentCount            int        `json:"document_count"`
	SourceHash               string     `json:"source_hash"`
	DefaultCollection         string     `json:"default_collection"`
	ExpectedCollectionPrefix string     `json:"expected_collection_prefix"`
	Notes                    string     `json:"notes,omitempty"`
}

type Document struct {
	Path   string `json:"path"`
	Abs    string `json:"-"`
	SHA256 string `json:"sha256"`
}

type fixtureFile struct {
	ID                       string   `json:"id"`
	Name                     string   `json:"name"`
	Description              string   `json:"description"`
	Documents                []string `json:"documents"`
	ExpectedCollectionPrefix string   `json:"expected_collection_prefix"`
	Notes                    string   `json:"notes"`
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
	var out []Fixture
	for _, root := range c.roots {
		matches, err := filepath.Glob(filepath.Join(root, "*.json"))
		if err != nil {
			return nil, err
		}
		sort.Strings(matches)
		for _, path := range matches {
			fixture, err := c.Load(path)
			if err != nil {
				return nil, err
			}
			out = append(out, fixture)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (c *Catalog) Load(idOrPath string) (Fixture, error) {
	path, root, err := c.resolve(idOrPath)
	if err != nil {
		return Fixture{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Fixture{}, err
	}
	var raw fixtureFile
	if err := json.Unmarshal(data, &raw); err != nil {
		return Fixture{}, err
	}
	if !fixtureIDPattern.MatchString(raw.ID) {
		return Fixture{}, fmt.Errorf("fixture id must match %s", fixtureIDPattern.String())
	}
	if raw.Name == "" || raw.Description == "" {
		return Fixture{}, fmt.Errorf("name and description are required")
	}
	if len(raw.Documents) == 0 {
		return Fixture{}, fmt.Errorf("documents are required")
	}
	if !collectionPattern.MatchString(raw.ExpectedCollectionPrefix) {
		return Fixture{}, fmt.Errorf("expected_collection_prefix must match %s", collectionPattern.String())
	}
	docs := make([]Document, 0, len(raw.Documents))
	for _, docPath := range raw.Documents {
		doc, err := resolveDocument(root, docPath)
		if err != nil {
			return Fixture{}, err
		}
		docs = append(docs, doc)
	}
	sourceHash, err := computeHash(raw, docs)
	if err != nil {
		return Fixture{}, err
	}
	return Fixture{
		ID:                       raw.ID,
		Name:                     raw.Name,
		Description:              raw.Description,
		Path:                     path,
		Root:                     root,
		Documents:                docs,
		DocumentCount:            len(docs),
		SourceHash:               sourceHash,
		DefaultCollection:         raw.ExpectedCollectionPrefix + "_" + sourceHash[:8],
		ExpectedCollectionPrefix: raw.ExpectedCollectionPrefix,
		Notes:                    raw.Notes,
	}, nil
}

func (c *Catalog) resolve(idOrPath string) (string, string, error) {
	for _, root := range c.roots {
		candidate := idOrPath
		if !filepath.IsAbs(candidate) && filepath.Ext(candidate) == "" {
			candidate = filepath.Join(root, idOrPath+".json")
		} else if !filepath.IsAbs(candidate) {
			candidate = filepath.Join(root, candidate)
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
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
			continue
		}
		if _, err := os.Stat(absCandidate); err == nil {
			return absCandidate, absRoot, nil
		}
	}
	return "", "", fmt.Errorf("corpus fixture %q not found", idOrPath)
}

func resolveDocument(root, docPath string) (Document, error) {
	if docPath == "" || filepath.IsAbs(docPath) {
		return Document{}, fmt.Errorf("document %q must be relative", docPath)
	}
	if strings.ToLower(filepath.Ext(docPath)) != ".pdf" {
		return Document{}, fmt.Errorf("document %q must be a PDF", docPath)
	}
	absDoc, err := filepath.Abs(filepath.Join(root, docPath))
	if err != nil {
		return Document{}, err
	}
	rel, err := filepath.Rel(root, absDoc)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return Document{}, fmt.Errorf("document %q must stay under fixture root", docPath)
	}
	data, err := os.ReadFile(absDoc)
	if err != nil {
		return Document{}, err
	}
	sum := sha256.Sum256(data)
	return Document{Path: filepath.ToSlash(docPath), Abs: absDoc, SHA256: hex.EncodeToString(sum[:])}, nil
}

func computeHash(raw fixtureFile, docs []Document) (string, error) {
	h := sha256.New()
	h.Write([]byte(raw.ID + "\n" + raw.Name + "\n" + raw.Description + "\n" + raw.ExpectedCollectionPrefix + "\n"))
	for _, doc := range docs {
		h.Write([]byte(doc.Path + "\n" + doc.SHA256 + "\n"))
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
```

- [ ] **Step 4: Add config for corpus fixture roots**

In `go/eval-mcp-service/internal/config/config.go`, add a field:

```go
CorpusFixtureRoots []string
```

Set it in `FromEnv()`:

```go
CorpusFixtureRoots: corpusFixtureRoots(),
```

Add helper:

```go
func corpusFixtureRoots() []string {
	value := os.Getenv("EVAL_MCP_CORPUS_FIXTURE_ROOTS")
	if value != "" {
		return splitPathList(value)
	}
	return []string{filepath.Clean(filepath.Join("..", "..", "docs", "product-catalog"))}
}
```

Refactor `datasetFixtureRoots()` to reuse:

```go
func datasetFixtureRoots() []string {
	value := os.Getenv("EVAL_MCP_DATASET_FIXTURE_ROOTS")
	if value != "" {
		return splitPathList(value)
	}
	return []string{filepath.Clean(filepath.Join("..", "..", "docs", "product-catalog"))}
}

func splitPathList(value string) []string {
	parts := strings.Split(value, string(os.PathListSeparator))
	roots := make([]string, 0, len(parts))
	for _, part := range parts {
		if strings.TrimSpace(part) != "" {
			roots = append(roots, part)
		}
	}
	return roots
}
```

Add a config test in `go/eval-mcp-service/internal/config/config_test.go`:

```go
func TestFromEnvReadsCorpusFixtureRoots(t *testing.T) {
	t.Setenv("EVAL_API_TOKEN", "token")
	t.Setenv("EVAL_MCP_CORPUS_FIXTURE_ROOTS", "/tmp/a"+string(os.PathListSeparator)+"/tmp/b")

	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv returned error: %v", err)
	}
	if len(cfg.CorpusFixtureRoots) != 2 || cfg.CorpusFixtureRoots[0] != "/tmp/a" || cfg.CorpusFixtureRoots[1] != "/tmp/b" {
		t.Fatalf("CorpusFixtureRoots = %#v", cfg.CorpusFixtureRoots)
	}
}
```

- [ ] **Step 5: Add initial corpus fixture manifest**

Create `docs/product-catalog/product_catalog_v1.corpus.json`:

```json
{
  "id": "product_catalog_v1",
  "name": "Product Catalog v1",
  "description": "Curated product catalog PDFs used by product-spec RAG eval datasets.",
  "documents": [
    "27-4k-monitor-specs.pdf",
    "laptop-pro-15-specs.pdf",
    "robot-vacuum-specs.pdf",
    "smartwatch-sport-specs.pdf",
    "stand-mixer-5qt-specs.pdf"
  ],
  "expected_collection_prefix": "eval_product_catalog_v1",
  "notes": "Use this corpus with product catalog golden QA datasets."
}
```

- [ ] **Step 6: Run Go tests**

Run:

```bash
cd go/eval-mcp-service && go test ./internal/corpusfixture ./internal/config -count=1
```

Expected: tests pass.

- [ ] **Step 7: Commit**

Run:

```bash
git add go/eval-mcp-service/internal/corpusfixture go/eval-mcp-service/internal/config docs/product-catalog/product_catalog_v1.corpus.json
git commit -m "feat: add eval rag corpus fixtures"
```

## Task 5: Extend Go Ingestion Client

**Files:**
- Modify: `go/eval-mcp-service/internal/ingestionapi/client.go`
- Test: `go/eval-mcp-service/internal/ingestionapi/client_test.go`

- [ ] **Step 1: Write failing client tests**

Append tests to `go/eval-mcp-service/internal/ingestionapi/client_test.go`:

```go
func TestClientUploadPDF(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/ingest" || r.URL.Query().Get("collection") != "eval_product_catalog_v1_a1b2c3d4" {
			t.Fatalf("unexpected request: %s %s?%s", r.Method, r.URL.Path, r.URL.RawQuery)
		}
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			t.Fatalf("ParseMultipartForm: %v", err)
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			t.Fatalf("FormFile: %v", err)
		}
		defer file.Close()
		if header.Filename != "laptop.pdf" {
			t.Fatalf("filename = %q", header.Filename)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "chunks_created": 3})
	}))
	defer server.Close()

	got, err := New(server.URL, "token", server.Client()).UploadPDF(context.Background(), "eval_product_catalog_v1_a1b2c3d4", "laptop.pdf", []byte("%PDF-1.4"))
	if err != nil {
		t.Fatalf("UploadPDF returned error: %v", err)
	}
	if got.ChunksCreated != 3 {
		t.Fatalf("ChunksCreated = %d", got.ChunksCreated)
	}
}

func TestClientManifestRoundTripMethods(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/collections/eval_product_catalog_v1_a1b2c3d4/manifest" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		switch r.Method {
		case http.MethodPut:
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "stored"})
		case http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{"fixture_id": "product_catalog_v1"})
		default:
			t.Fatalf("unexpected method: %s", r.Method)
		}
	}))
	defer server.Close()

	client := New(server.URL, "", server.Client())
	err := client.PutCollectionManifest(context.Background(), "eval_product_catalog_v1_a1b2c3d4", map[string]any{"fixture_id": "product_catalog_v1"})
	if err != nil {
		t.Fatalf("PutCollectionManifest returned error: %v", err)
	}
	got, err := client.GetCollectionManifest(context.Background(), "eval_product_catalog_v1_a1b2c3d4")
	if err != nil {
		t.Fatalf("GetCollectionManifest returned error: %v", err)
	}
	if got["fixture_id"] != "product_catalog_v1" {
		t.Fatalf("manifest = %#v", got)
	}
}
```

- [ ] **Step 2: Run tests to verify failure**

Run:

```bash
cd go/eval-mcp-service && go test ./internal/ingestionapi -run "UploadPDF|Manifest" -count=1
```

Expected: missing methods/types failures.

- [ ] **Step 3: Implement client methods**

In `go/eval-mcp-service/internal/ingestionapi/client.go`, add imports:

```go
import (
	"mime/multipart"
	"path/filepath"
)
```

Add result type:

```go
type IngestResponse struct {
	Status        string `json:"status"`
	DocumentID    string `json:"document_id"`
	ChunksCreated int    `json:"chunks_created"`
	Filename      string `json:"filename"`
}
```

Add methods:

```go
func (c *Client) UploadPDF(ctx context.Context, collection, filename string, data []byte) (IngestResponse, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", filepath.Base(filename))
	if err != nil {
		return IngestResponse{}, err
	}
	if _, err := part.Write(data); err != nil {
		return IngestResponse{}, err
	}
	if err := writer.Close(); err != nil {
		return IngestResponse{}, err
	}
	path := "/ingest?collection=" + url.QueryEscape(collection)
	var out IngestResponse
	if err := c.doRaw(ctx, http.MethodPost, path, writer.FormDataContentType(), &body, &out); err != nil {
		return IngestResponse{}, err
	}
	return out, nil
}

func (c *Client) PutCollectionManifest(ctx context.Context, name string, manifest map[string]any) error {
	path := "/collections/" + url.PathEscape(name) + "/manifest"
	return c.do(ctx, http.MethodPut, path, manifest, nil)
}

func (c *Client) GetCollectionManifest(ctx context.Context, name string) (map[string]any, error) {
	var response map[string]any
	path := "/collections/" + url.PathEscape(name) + "/manifest"
	if err := c.do(ctx, http.MethodGet, path, nil, &response); err != nil {
		return nil, err
	}
	return response, nil
}

func (c *Client) DeleteCollection(ctx context.Context, name string) error {
	path := "/collections/" + url.PathEscape(name)
	return c.do(ctx, http.MethodDelete, path, nil, nil)
}
```

Add helper:

```go
func (c *Client) doRaw(ctx context.Context, method, path, contentType string, reader io.Reader, out any) error {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
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

Then make `do()` call `doRaw()` after JSON marshaling to avoid duplicate HTTP
response handling.

- [ ] **Step 4: Run client tests**

Run:

```bash
cd go/eval-mcp-service && go test ./internal/ingestionapi -count=1
```

Expected: ingestion client tests pass.

- [ ] **Step 5: Commit**

Run:

```bash
git add go/eval-mcp-service/internal/ingestionapi
git commit -m "feat: extend eval mcp ingestion client"
```

## Task 6: Add Eval Workflow Corpus Provisioning

**Files:**
- Modify: `go/eval-mcp-service/internal/evalworkflow/service.go`
- Test: `go/eval-mcp-service/internal/evalworkflow/service_test.go`
- Modify: `go/eval-mcp-service/cmd/eval-mcp/main.go`

- [ ] **Step 1: Write workflow tests**

Add fake corpus fixtures and ingestion methods to `service_test.go`, then add
tests:

```go
func TestProvisionRAGCorpusUploadsFixtureAndWritesManifest(t *testing.T) {
	root := t.TempDir()
	pdfPath := filepath.Join(root, "laptop.pdf")
	if err := os.WriteFile(pdfPath, []byte("%PDF-1.4\nlaptop"), 0o644); err != nil {
		t.Fatalf("write pdf: %v", err)
	}
	ingestion := &recordingCorpusIngestion{
		collections: []ingestionapi.Collection{},
		uploadResponse: ingestionapi.IngestResponse{
			Status:        "success",
			DocumentID:    "doc-1",
			ChunksCreated: 4,
			Filename:      "laptop.pdf",
		},
	}
	fixtures := fakeCorpusFixtures{fixture: corpusfixture.Fixture{
		ID:                "product_catalog_v1",
		Name:              "Product Catalog v1",
		SourceHash:        "a1b2c3d4e5f6",
		DefaultCollection: "eval_product_catalog_v1_a1b2c3d4",
		Documents: []corpusfixture.Document{{
			Path:   "laptop.pdf",
			Abs:    pdfPath,
			SHA256: "abc123",
		}},
		DocumentCount: 1,
	}}
	service := New(&fakeAPI{}, ingestion, fakeFixtures{}, time.Second, time.Minute).
		WithCorpusFixtures(fixtures)

	got, err := service.ProvisionRAGCorpus(context.Background(), ProvisionCorpusInput{
		Fixture: "product_catalog_v1",
	})
	if err != nil {
		t.Fatalf("ProvisionRAGCorpus returned error: %v", err)
	}
	if got.Collection != "eval_product_catalog_v1_a1b2c3d4" || got.ChunksCreated != 4 || got.FixtureHash != "a1b2c3d4e5f6" {
		t.Fatalf("result = %#v", got)
	}
	if ingestion.uploadCollection != "eval_product_catalog_v1_a1b2c3d4" || ingestion.uploadFilename != "laptop.pdf" {
		t.Fatalf("upload collection=%q filename=%q", ingestion.uploadCollection, ingestion.uploadFilename)
	}
	if ingestion.manifest["fixture_hash"] != "a1b2c3d4e5f6" {
		t.Fatalf("manifest = %#v", ingestion.manifest)
	}
}

func TestProvisionRAGCorpusRejectsUnsafeCollection(t *testing.T) {
	service := New(fakeAPI{}, fakeIngestion{}, fakeFixtures{}, time.Second, time.Minute)
	_, err := service.ProvisionRAGCorpus(context.Background(), ProvisionCorpusInput{
		Fixture:    "product_catalog_v1",
		Collection: "documents",
	})
	if err == nil || !strings.Contains(err.Error(), "managed eval collection") {
		t.Fatalf("error = %v", err)
	}
}

type fakeCorpusFixtures struct {
	fixtures []corpusfixture.Fixture
	fixture  corpusfixture.Fixture
}

func (f fakeCorpusFixtures) List() ([]corpusfixture.Fixture, error) {
	return f.fixtures, nil
}

func (f fakeCorpusFixtures) Load(string) (corpusfixture.Fixture, error) {
	return f.fixture, nil
}

type recordingCorpusIngestion struct {
	collections      []ingestionapi.Collection
	manifest         map[string]any
	uploadResponse   ingestionapi.IngestResponse
	uploadCollection string
	uploadFilename   string
}

func (r *recordingCorpusIngestion) ListCollections(context.Context) ([]ingestionapi.Collection, error) {
	return r.collections, nil
}

func (r *recordingCorpusIngestion) GetCollectionConfig(context.Context, string) (map[string]any, error) {
	return map[string]any{}, nil
}

func (r *recordingCorpusIngestion) UploadPDF(_ context.Context, collection, filename string, _ []byte) (ingestionapi.IngestResponse, error) {
	r.uploadCollection = collection
	r.uploadFilename = filename
	return r.uploadResponse, nil
}

func (r *recordingCorpusIngestion) PutCollectionManifest(_ context.Context, _ string, manifest map[string]any) error {
	r.manifest = manifest
	return nil
}

func (r *recordingCorpusIngestion) GetCollectionManifest(context.Context, string) (map[string]any, error) {
	return nil, &ingestionapi.HTTPError{Method: http.MethodGet, Path: "/manifest", StatusCode: http.StatusNotFound}
}

func (r *recordingCorpusIngestion) DeleteCollection(context.Context, string) error {
	return nil
}
```

Add `os`, `path/filepath`, `net/http`, and
`github.com/kabradshaw1/portfolio/go/eval-mcp-service/internal/corpusfixture`
to the test imports.

Use concrete fake types rather than mocks. The first test should create a temp
PDF and return a `corpusfixture.Fixture` from a fake catalog.

- [ ] **Step 2: Run tests to verify failure**

Run:

```bash
cd go/eval-mcp-service && go test ./internal/evalworkflow -run "ProvisionRAGCorpus|RAGCorpus" -count=1
```

Expected: missing methods/types failures.

- [ ] **Step 3: Extend workflow interfaces and types**

In `service.go`, add import:

```go
"github.com/kabradshaw1/portfolio/go/eval-mcp-service/internal/corpusfixture"
```

Extend `Ingestion`:

```go
	UploadPDF(context.Context, string, string, []byte) (ingestionapi.IngestResponse, error)
	PutCollectionManifest(context.Context, string, map[string]any) error
	GetCollectionManifest(context.Context, string) (map[string]any, error)
	DeleteCollection(context.Context, string) error
```

Add interface:

```go
type CorpusFixtures interface {
	List() ([]corpusfixture.Fixture, error)
	Load(string) (corpusfixture.Fixture, error)
}
```

Add field to `Service`:

```go
	corpusFixtures CorpusFixtures
```

Add constructor option:

```go
func (s *Service) WithCorpusFixtures(corpusFixtures CorpusFixtures) *Service {
	s.corpusFixtures = corpusFixtures
	return s
}
```

Add input/result types:

```go
type ProvisionCorpusInput struct {
	Fixture       string `json:"fixture"`
	Collection    string `json:"collection,omitempty"`
	ForceRecreate bool   `json:"force_recreate,omitempty"`
}

type ProvisionCorpusResult struct {
	Collection    string         `json:"collection"`
	FixtureID     string         `json:"fixture_id"`
	FixtureHash   string         `json:"fixture_hash"`
	DocumentCount int            `json:"document_count"`
	ChunksCreated int            `json:"chunks_created"`
	Documents     []CorpusUpload `json:"documents"`
	Idempotent     bool           `json:"idempotent"`
}

type CorpusUpload struct {
	Path          string `json:"path"`
	SHA256        string `json:"sha256"`
	ChunksCreated int    `json:"chunks_created"`
}
```

- [ ] **Step 4: Implement guardrails and provisioning**

Add helpers and methods to `service.go`:

```go
var managedEvalCollectionPattern = regexp.MustCompile(`^eval_[a-z0-9_]+_[a-f0-9]{8,16}$`)

func isManagedEvalCollection(name string) bool {
	return managedEvalCollectionPattern.MatchString(name)
}
```

Implement:

```go
func (s *Service) ListRAGCorpusFixtures(context.Context) ([]corpusfixture.Fixture, error) {
	if s.corpusFixtures == nil {
		return nil, errors.New("corpus fixture catalog is not configured")
	}
	return s.corpusFixtures.List()
}

func (s *Service) GetRAGCorpusManifest(ctx context.Context, collection string) (map[string]any, error) {
	if !isManagedEvalCollection(collection) {
		return nil, fmt.Errorf("collection %q is not a managed eval collection", collection)
	}
	return s.ingestion.GetCollectionManifest(ctx, collection)
}

func (s *Service) DeleteRAGCorpus(ctx context.Context, collection string) error {
	if !isManagedEvalCollection(collection) {
		return fmt.Errorf("collection %q is not a managed eval collection", collection)
	}
	return s.ingestion.DeleteCollection(ctx, collection)
}
```

Implement `ProvisionRAGCorpus` to load the fixture, choose
`fixture.DefaultCollection` unless input collection is set, enforce
`isManagedEvalCollection`, compare existing manifest `fixture_hash`, delete on
`ForceRecreate`, read each PDF from `Document.Abs`, upload through ingestion,
write manifest, and return totals.

- [ ] **Step 5: Wire catalog in main**

In `go/eval-mcp-service/cmd/eval-mcp/main.go`, import `corpusfixture`, then:

```go
corpusFixtures := corpusfixture.New(cfg.CorpusFixtureRoots)
service := evalworkflow.New(api, ingestion, fixtures, cfg.PollInterval, cfg.WaitTimeout, cfg.MaxBackoff).
	WithCorpusFixtures(corpusFixtures).
	WithTriageAPI(triageAdapter{client: triageClient})
```

- [ ] **Step 6: Run workflow tests**

Run:

```bash
cd go/eval-mcp-service && go test ./internal/evalworkflow ./cmd/eval-mcp -count=1
```

Expected: tests pass.

- [ ] **Step 7: Commit**

Run:

```bash
git add go/eval-mcp-service/internal/evalworkflow go/eval-mcp-service/cmd/eval-mcp
git commit -m "feat: provision rag corpora through eval workflow"
```

## Task 7: Expose MCP Tools And Docs

**Files:**
- Modify: `go/eval-mcp-service/internal/mcpserver/server.go`
- Test: `go/eval-mcp-service/internal/mcpserver/server_test.go`
- Modify: `go/eval-mcp-service/README.md`

- [ ] **Step 1: Write MCP exposure tests**

Update `fakeEvalService` in `server_test.go` to implement:

```go
func (f *fakeEvalService) ListRAGCorpusFixtures(context.Context) ([]corpusfixture.Fixture, error) {
	return []corpusfixture.Fixture{{ID: "product_catalog_v1", DefaultCollection: "eval_product_catalog_v1_a1b2c3d4"}}, nil
}

func (f *fakeEvalService) ProvisionRAGCorpus(context.Context, evalworkflow.ProvisionCorpusInput) (evalworkflow.ProvisionCorpusResult, error) {
	return evalworkflow.ProvisionCorpusResult{Collection: "eval_product_catalog_v1_a1b2c3d4", FixtureID: "product_catalog_v1"}, nil
}

func (f *fakeEvalService) GetRAGCorpusManifest(context.Context, string) (map[string]any, error) {
	return map[string]any{"fixture_id": "product_catalog_v1"}, nil
}

func (f *fakeEvalService) DeleteRAGCorpus(context.Context, string) error {
	return nil
}
```

Add assertions that tool list/prompt text includes:

```text
list_rag_corpus_fixtures
provision_rag_corpus
get_rag_corpus_manifest
delete_rag_corpus
Never mutate a collection after baseline starts
```

- [ ] **Step 2: Run tests to verify failure**

Run:

```bash
cd go/eval-mcp-service && go test ./internal/mcpserver -run "Tools|Prompt|Workflow" -count=1
```

Expected: missing methods or missing tool-name assertions fail.

- [ ] **Step 3: Extend server interface and register tools**

In `server.go`, import `corpusfixture` and extend `EvalService`:

```go
	ListRAGCorpusFixtures(context.Context) ([]corpusfixture.Fixture, error)
	ProvisionRAGCorpus(context.Context, evalworkflow.ProvisionCorpusInput) (evalworkflow.ProvisionCorpusResult, error)
	GetRAGCorpusManifest(context.Context, string) (map[string]any, error)
	DeleteRAGCorpus(context.Context, string) error
```

Register tools after existing RAG collection tools:

```go
addTool(srv, "list_rag_corpus_fixtures", "List curated RAG corpus fixtures available in the repo.", emptySchema(), listRAGCorpusFixturesHandler(service))
addTool(srv, "provision_rag_corpus", "Provision a managed eval RAG collection from a curated fixture through ingestion.", provisionRAGCorpusSchema(), provisionRAGCorpusHandler(service))
addTool(srv, "get_rag_corpus_manifest", "Fetch corpus manifest metadata for one managed eval collection.", ragCorpusManifestSchema(), getRAGCorpusManifestHandler(service))
addTool(srv, "delete_rag_corpus", "Delete one managed eval RAG corpus collection. Mutating cleanup-only tool.", ragCorpusManifestSchema(), deleteRAGCorpusHandler(service))
```

Add schemas:

```go
func provisionRAGCorpusSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"fixture":{"type":"string","minLength":1},"collection":{"type":"string","pattern":"^eval_[a-z0-9_]+_[a-f0-9]{8,16}$"},"force_recreate":{"type":"boolean"}},"required":["fixture"],"additionalProperties":false}`)
}

func ragCorpusManifestSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"collection":{"type":"string","pattern":"^eval_[a-z0-9_]+_[a-f0-9]{8,16}$"}},"required":["collection"],"additionalProperties":false}`)
}
```

Add handlers using the same `decodeArgs` and `resultOrError` pattern as the
existing handlers.

- [ ] **Step 4: Update prompt and README**

In `evalPromptHandler()` and `evalWorkflowInstructions()`, add guidance:

```text
Use list_rag_corpus_fixtures and provision_rag_corpus only before starting a baseline run when a curated corpus collection is missing. Never mutate a collection after baseline starts, and use the same collection for all candidates in one experiment.
```

In `go/eval-mcp-service/README.md`, add:

```markdown
- `EVAL_MCP_CORPUS_FIXTURE_ROOTS`: path-list of curated RAG corpus fixture roots,
  defaults to the repo `docs/product-catalog` directory.
```

Add the four new tools to the Tools list.

- [ ] **Step 5: Run MCP tests**

Run:

```bash
cd go/eval-mcp-service && go test ./internal/mcpserver -count=1
```

Expected: MCP server tests pass.

- [ ] **Step 6: Commit**

Run:

```bash
git add go/eval-mcp-service/internal/mcpserver go/eval-mcp-service/README.md
git commit -m "feat: expose rag corpus mcp tools"
```

## Task 8: Verification And PR

**Files:**
- No new source changes expected unless verification finds issues.

- [ ] **Step 1: Run Python preflight**

Run:

```bash
make preflight-python
```

Expected: ingestion/chat/debug/eval Python tests pass.

- [ ] **Step 2: Run Go preflight**

Run:

```bash
make preflight-go
```

Expected: Go lint and tests pass, including `go/eval-mcp-service`.

- [ ] **Step 3: Run security preflight**

Run:

```bash
make preflight-security
```

Expected: security checks pass or report only known accepted findings.

- [ ] **Step 4: Inspect final diff**

Run:

```bash
git status --short
git log --oneline --max-count=8
git diff main...HEAD --stat
```

Expected: only intended ingestion, eval MCP, README, and corpus fixture files
changed.

- [ ] **Step 5: Push branch**

Run:

```bash
git push -u origin eval-mcp-rag-corpus-provisioning
```

Expected: branch pushed successfully.

- [ ] **Step 6: Create PR to `qa`**

Run:

```bash
gh pr create \
  --base qa \
  --head eval-mcp-rag-corpus-provisioning \
  --title "Add eval MCP RAG corpus provisioning" \
  --body "Adds controlled eval MCP tools for provisioning curated RAG corpus fixtures through ingestion into managed eval collections. Keeps eval runs immutable and records corpus manifest metadata."
```

Expected: PR created. Do not watch CI unless Kyle asks.

## Self-Review

- Spec coverage: ingestion manifest storage, curated fixtures, deterministic
  eval collection names, MCP provisioning/list/get/delete tools, safety
  guardrails, docs, and verification are covered.
- Scope: no direct Qdrant access, no inline eval-run ingestion, no frontend UI.
- Placeholder scan: no open-ended implementation placeholders remain; any
  intentionally flexible code is bounded by concrete method names, files, and
  tests.
- Type consistency: Go result/input names are introduced before use by MCP
  handlers; Python manifest methods are introduced before endpoint tests pass.
