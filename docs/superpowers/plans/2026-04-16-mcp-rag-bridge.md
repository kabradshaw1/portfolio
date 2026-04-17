# MCP–RAG Bridge Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Expose the Python RAG pipeline as MCP tools through the Go ai-service, add supporting Python endpoints, and write a RAG learning ADR.

**Architecture:** The Go ai-service gets a new RAG HTTP client (like the ecommerce client) and 4 new tools that proxy to the Python chat and ingestion services. The Python services get 3 small endpoint additions: `/search` (retrieval-only), `/chat` JSON mode, and `/collections` (discovery). All tools register into the existing MCP server automatically.

**Tech Stack:** Go (ai-service tools + HTTP client), Python/FastAPI (new endpoints), Qdrant (collection listing), existing shared packages (resilience, tracing)

---

### Task 1: Python chat service — `POST /search` endpoint

Add a retrieval-only endpoint that returns ranked chunks without LLM generation.

**Files:**
- Modify: `services/chat/app/main.py` (add endpoint after line 123)
- Modify: `services/chat/app/chain.py` (extract `retrieve_chunks` function from `rag_query`)
- Test: `services/chat/tests/test_main.py` (add search endpoint tests)

- [ ] **Step 1: Write failing tests for `/search`**

Add to `services/chat/tests/test_main.py`:

```python
@patch("app.main.retrieve_chunks", new_callable=AsyncMock)
def test_search_returns_chunks(mock_retrieve):
    mock_retrieve.return_value = [
        {"text": "Hello world", "filename": "test.pdf", "page_number": 1, "document_id": "abc", "score": 0.92},
        {"text": "Goodbye world", "filename": "test.pdf", "page_number": 2, "document_id": "abc", "score": 0.85},
    ]

    response = client.post("/search", json={"query": "hello", "limit": 5})
    assert response.status_code == 200
    data = response.json()
    assert len(data["results"]) == 2
    assert data["results"][0]["text"] == "Hello world"
    assert data["results"][0]["score"] == 0.92


def test_search_requires_query():
    response = client.post("/search", json={})
    assert response.status_code == 422


def test_search_rejects_invalid_collection():
    response = client.post(
        "/search", json={"query": "hello", "collection": "DROP TABLE users"}
    )
    assert response.status_code == 422
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd services/chat && python -m pytest tests/test_main.py::test_search_returns_chunks tests/test_main.py::test_search_requires_query tests/test_main.py::test_search_rejects_invalid_collection -v`

Expected: FAIL — `retrieve_chunks` not found, no `/search` route

- [ ] **Step 3: Extract `retrieve_chunks` from `rag_query` in `chain.py`**

Add this function at line 33 in `services/chat/app/chain.py` (before `stream_response`):

```python
async def retrieve_chunks(
    question: str,
    embedding_provider: EmbeddingProvider,
    embedding_model: str,
    qdrant_host: str,
    qdrant_port: int,
    collection_name: str,
    top_k: int = 5,
) -> list[dict]:
    """Embed question and retrieve ranked chunks from Qdrant."""
    retrieve_start = time.perf_counter()
    vectors = await embed_texts(
        texts=[question],
        provider=embedding_provider,
        model=embedding_model,
    )
    query_vector = vectors[0]

    retriever = QdrantRetriever(
        host=qdrant_host, port=qdrant_port, collection_name=collection_name
    )
    chunks = retriever.search(query_vector=query_vector, top_k=top_k)
    RAG_PIPELINE_DURATION.labels(stage="retrieve").observe(
        time.perf_counter() - retrieve_start
    )
    return chunks
```

Then refactor `rag_query` (starting at its current line 66) to use it:

```python
async def rag_query(
    question: str,
    llm_provider: LLMProvider,
    embedding_provider: EmbeddingProvider,
    chat_model: str,
    embedding_model: str,
    qdrant_host: str,
    qdrant_port: int,
    collection_name: str,
    top_k: int = 5,
) -> AsyncGenerator[dict, None]:
    chunks = await retrieve_chunks(
        question=question,
        embedding_provider=embedding_provider,
        embedding_model=embedding_model,
        qdrant_host=qdrant_host,
        qdrant_port=qdrant_port,
        collection_name=collection_name,
        top_k=top_k,
    )

    # Build prompt
    build_start = time.perf_counter()
    prompt = build_rag_prompt(question=question, chunks=chunks)
    RAG_PIPELINE_DURATION.labels(stage="build_prompt").observe(
        time.perf_counter() - build_start
    )

    # Collect unique sources
    seen = set()
    sources = []
    for chunk in chunks:
        key = (chunk["filename"], chunk["page_number"])
        if key not in seen:
            seen.add(key)
            sources.append({"file": chunk["filename"], "page": chunk["page_number"]})

    # Stream response
    generate_start = time.perf_counter()
    async for event in stream_response(
        prompt=prompt, model=chat_model, provider=llm_provider
    ):
        yield event
    RAG_PIPELINE_DURATION.labels(stage="generate").observe(
        time.perf_counter() - generate_start
    )

    yield {"done": True, "sources": sources}
```

- [ ] **Step 4: Add request model and `/search` endpoint to `main.py`**

Add after the `ChatRequest` model (line 63) in `services/chat/app/main.py`:

```python
class SearchRequest(BaseModel):
    query: str = Field(max_length=2000)
    collection: str | None = Field(default=None, pattern=r"^[a-zA-Z0-9_-]{1,100}$")
    limit: int = Field(default=5, ge=1, le=20)
```

Update the import from `app.chain` (line 18) to include `retrieve_chunks`:

```python
from app.chain import rag_query, retrieve_chunks
```

Add the endpoint after the `/chat` endpoint (after line 123):

```python
@app.post("/search")
@limiter.limit("30/minute")
async def search(
    request: Request, body: SearchRequest, user_id: str = Depends(require_auth)
):
    try:
        chunks = await retrieve_chunks(
            question=body.query,
            embedding_provider=_embedding_provider,
            embedding_model=settings.embedding_model,
            qdrant_host=settings.qdrant_host,
            qdrant_port=settings.qdrant_port,
            collection_name=body.collection or settings.collection_name,
            top_k=body.limit,
        )
    except (httpx.ConnectError, httpx.TimeoutException) as e:
        logger.error("Embedding service error: %s", e)
        raise HTTPException(status_code=503, detail="Embedding service unavailable")

    return {
        "results": [
            {
                "text": c["text"],
                "filename": c["filename"],
                "page_number": c["page_number"],
                "score": c["score"],
            }
            for c in chunks
        ]
    }
```

Add `HTTPException` to the FastAPI import (line 5):

```python
from fastapi import Depends, FastAPI, HTTPException
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd services/chat && python -m pytest tests/ -v`

Expected: All tests pass (existing + new)

- [ ] **Step 6: Run preflight**

Run: `make preflight-python`

Expected: ruff lint + format + pytest all pass

- [ ] **Step 7: Commit**

```bash
git add services/chat/app/main.py services/chat/app/chain.py services/chat/tests/test_main.py
git commit -m "feat(chat): add /search endpoint for retrieval-only queries"
```

---

### Task 2: Python chat service — JSON response mode for `/chat`

Add `Accept: application/json` support to return a single JSON response instead of SSE.

**Files:**
- Modify: `services/chat/app/main.py` (update `/chat` endpoint)
- Test: `services/chat/tests/test_main.py` (add JSON mode test)

- [ ] **Step 1: Write failing test**

Add to `services/chat/tests/test_main.py`:

```python
@patch("app.main.rag_query")
def test_chat_json_mode(mock_rag_query):
    async def fake_rag_query(**kwargs):
        yield {"token": "Hello"}
        yield {"token": " world"}
        yield {"done": True, "sources": [{"file": "test.pdf", "page": 1}]}

    mock_rag_query.return_value = fake_rag_query()

    response = client.post(
        "/chat",
        json={"question": "What is this?"},
        headers={"Accept": "application/json"},
    )
    assert response.status_code == 200
    data = response.json()
    assert data["answer"] == "Hello world"
    assert len(data["sources"]) == 1
    assert data["sources"][0]["file"] == "test.pdf"
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd services/chat && python -m pytest tests/test_main.py::test_chat_json_mode -v`

Expected: FAIL — returns SSE stream, not JSON

- [ ] **Step 3: Update `/chat` endpoint to support JSON mode**

Replace the `/chat` endpoint in `services/chat/app/main.py` (lines 98-123):

```python
@app.post("/chat")
@limiter.limit("20/minute")
async def chat(
    request: Request, body: ChatRequest, user_id: str = Depends(require_auth)
):
    wants_json = request.headers.get("accept", "").startswith("application/json")

    if wants_json:
        try:
            tokens = []
            sources = []
            async for event in rag_query(
                question=body.question,
                llm_provider=_llm_provider,
                embedding_provider=_embedding_provider,
                chat_model=settings.get_llm_model(),
                embedding_model=settings.embedding_model,
                qdrant_host=settings.qdrant_host,
                qdrant_port=settings.qdrant_port,
                collection_name=body.collection or settings.collection_name,
            ):
                if "token" in event:
                    tokens.append(event["token"])
                if event.get("done"):
                    sources = event.get("sources", [])
            return {"answer": "".join(tokens), "sources": sources}
        except (httpx.ConnectError, httpx.TimeoutException) as e:
            logger.error("Backend service error: %s", e)
            raise HTTPException(status_code=503, detail="Service unavailable")
        except Exception as e:
            logger.error("Internal error: %s", e, exc_info=True)
            raise HTTPException(status_code=500, detail="Internal error")

    async def event_generator():
        try:
            async for event in rag_query(
                question=body.question,
                llm_provider=_llm_provider,
                embedding_provider=_embedding_provider,
                chat_model=settings.get_llm_model(),
                embedding_model=settings.embedding_model,
                qdrant_host=settings.qdrant_host,
                qdrant_port=settings.qdrant_port,
                collection_name=body.collection or settings.collection_name,
            ):
                yield {"data": json.dumps(event)}
        except (httpx.ConnectError, httpx.TimeoutException) as e:
            logger.error("Backend service error: %s", e)
            yield {"data": json.dumps({"error": "Service unavailable"})}
        except Exception as e:
            logger.error("Internal error: %s", e, exc_info=True)
            yield {"data": json.dumps({"error": "Internal error"})}

    return EventSourceResponse(event_generator())
```

- [ ] **Step 4: Run all chat tests**

Run: `cd services/chat && python -m pytest tests/ -v`

Expected: All tests pass (existing SSE tests unchanged + new JSON test)

- [ ] **Step 5: Commit**

```bash
git add services/chat/app/main.py services/chat/tests/test_main.py
git commit -m "feat(chat): add JSON response mode via Accept header"
```

---

### Task 3: Python ingestion service — `GET /collections` endpoint

**Files:**
- Modify: `services/ingestion/app/main.py` (add endpoint)
- Modify: `services/ingestion/app/store.py` (add `list_collections` method)
- Test: `services/ingestion/tests/test_store.py` (test store method)
- Test: `services/ingestion/tests/test_main.py` (test endpoint)

- [ ] **Step 1: Write failing test for store method**

Add to `services/ingestion/tests/test_store.py`:

```python
def test_list_collections(mock_qdrant_client):
    store = QdrantStore(host="localhost", port=6333, collection_name="default")

    collection1 = MagicMock()
    collection1.name = "documents"
    collection2 = MagicMock()
    collection2.name = "debug-myproject"

    mock_qdrant_client.return_value.get_collections.return_value = MagicMock(
        collections=[collection1, collection2]
    )

    info1 = MagicMock()
    info1.points_count = 150
    info2 = MagicMock()
    info2.points_count = 42

    mock_qdrant_client.return_value.get_collection.side_effect = [info1, info2]

    result = store.list_collections()
    assert len(result) == 2
    assert result[0]["name"] == "documents"
    assert result[0]["point_count"] == 150
    assert result[1]["name"] == "debug-myproject"
    assert result[1]["point_count"] == 42
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd services/ingestion && python -m pytest tests/test_store.py::test_list_collections -v`

Expected: FAIL — `list_collections` not found on QdrantStore

- [ ] **Step 3: Add `list_collections` to QdrantStore**

Add to `services/ingestion/app/store.py` after the `__init__` and `_ensure_collection` methods (after line 31):

```python
def list_collections(self) -> list[dict]:
    """List all Qdrant collections with point counts."""
    response = self.client.get_collections()
    result = []
    for col in response.collections:
        info = self.client.get_collection(col.name)
        result.append({
            "name": col.name,
            "point_count": info.points_count,
        })
    return result
```

- [ ] **Step 4: Run store test to verify it passes**

Run: `cd services/ingestion && python -m pytest tests/test_store.py::test_list_collections -v`

Expected: PASS

- [ ] **Step 5: Write failing test for `/collections` endpoint**

Add to `services/ingestion/tests/test_main.py`:

```python
@patch("app.main.get_store")
def test_list_collections(mock_get_store):
    mock_store = MagicMock()
    mock_store.list_collections.return_value = [
        {"name": "documents", "point_count": 150},
        {"name": "debug-myproject", "point_count": 42},
    ]
    mock_get_store.return_value = mock_store

    response = client.get("/collections")
    assert response.status_code == 200
    data = response.json()
    assert len(data["collections"]) == 2
    assert data["collections"][0]["name"] == "documents"
```

- [ ] **Step 6: Add `/collections` endpoint to ingestion main.py**

Add after the `/health` endpoint (after line 101) in `services/ingestion/app/main.py`:

```python
@app.get("/collections")
@limiter.limit("30/minute")
async def list_collections(request: Request, user_id: str = Depends(require_auth)):
    store = get_store()
    try:
        collections = store.list_collections()
    except Exception as e:
        logger.error("Qdrant error listing collections: %s", e, exc_info=True)
        raise HTTPException(status_code=503, detail="Vector store unavailable")
    return {"collections": collections}
```

- [ ] **Step 7: Run all ingestion tests**

Run: `cd services/ingestion && python -m pytest tests/ -v`

Expected: All tests pass

- [ ] **Step 8: Run preflight**

Run: `make preflight-python`

Expected: All checks pass

- [ ] **Step 9: Commit**

```bash
git add services/ingestion/app/main.py services/ingestion/app/store.py services/ingestion/tests/test_main.py services/ingestion/tests/test_store.py
git commit -m "feat(ingestion): add GET /collections endpoint"
```

---

### Task 4: Go RAG HTTP client

Create the HTTP client for calling the Python RAG services, following the ecommerce client pattern.

**Files:**
- Create: `go/ai-service/internal/tools/clients/rag.go`
- Create: `go/ai-service/internal/tools/clients/rag_test.go`

- [ ] **Step 1: Write failing tests for the RAG client**

Create `go/ai-service/internal/tools/clients/rag_test.go`:

```go
package clients

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kabradshaw1/portfolio/go/pkg/resilience"
)

func TestRAGClient_Search(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body["query"] != "what is kubernetes" {
			t.Fatalf("expected query 'what is kubernetes', got %q", body["query"])
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[
			{"text":"Kubernetes is...","filename":"k8s.pdf","page_number":1,"score":0.95},
			{"text":"Pods are...","filename":"k8s.pdf","page_number":3,"score":0.82}
		]}`))
	}))
	defer server.Close()

	c := NewRAGClient(server.URL, "", resilience.NewBreaker(resilience.BreakerConfig{Name: "test"}))
	results, err := c.Search(context.Background(), "what is kubernetes", "", 5)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].Text != "Kubernetes is..." {
		t.Errorf("unexpected text: %s", results[0].Text)
	}
	if results[0].Score != 0.95 {
		t.Errorf("unexpected score: %f", results[0].Score)
	}
}

func TestRAGClient_Ask(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Accept") != "application/json" {
			t.Fatalf("expected Accept: application/json, got %q", r.Header.Get("Accept"))
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"answer": "Kubernetes is a container orchestration platform.",
			"sources": [{"file": "k8s.pdf", "page": 1}]
		}`))
	}))
	defer server.Close()

	c := NewRAGClient(server.URL, "", resilience.NewBreaker(resilience.BreakerConfig{Name: "test"}))
	answer, err := c.Ask(context.Background(), "what is kubernetes", "")
	if err != nil {
		t.Fatalf("Ask: %v", err)
	}
	if answer.Answer != "Kubernetes is a container orchestration platform." {
		t.Errorf("unexpected answer: %s", answer.Answer)
	}
	if len(answer.Sources) != 1 || answer.Sources[0].File != "k8s.pdf" {
		t.Errorf("unexpected sources: %+v", answer.Sources)
	}
}

func TestRAGClient_ListCollections(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/collections" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodGet {
			t.Fatalf("expected GET, got %s", r.Method)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"collections":[
			{"name":"documents","point_count":150},
			{"name":"debug-myproject","point_count":42}
		]}`))
	}))
	defer server.Close()

	c := NewRAGClient("", server.URL, resilience.NewBreaker(resilience.BreakerConfig{Name: "test"}))
	collections, err := c.ListCollections(context.Background())
	if err != nil {
		t.Fatalf("ListCollections: %v", err)
	}
	if len(collections) != 2 {
		t.Fatalf("expected 2 collections, got %d", len(collections))
	}
	if collections[0].Name != "documents" || collections[0].PointCount != 150 {
		t.Errorf("unexpected collection: %+v", collections[0])
	}
}

func TestRAGClient_Search_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer server.Close()

	c := NewRAGClient(server.URL, "", resilience.NewBreaker(resilience.BreakerConfig{Name: "test"}))
	_, err := c.Search(context.Background(), "test", "", 5)
	if err == nil {
		t.Fatal("expected error on 500")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd go/ai-service && go test ./internal/tools/clients/ -run TestRAGClient -v`

Expected: FAIL — `NewRAGClient` not found

- [ ] **Step 3: Implement the RAG client**

Create `go/ai-service/internal/tools/clients/rag.go`:

```go
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

// SearchResult represents a single chunk returned by the RAG retriever.
type SearchResult struct {
	Text       string  `json:"text"`
	Filename   string  `json:"filename"`
	PageNumber int     `json:"page_number"`
	Score      float64 `json:"score"`
}

// AskAnswer represents the full RAG response with citations.
type AskAnswer struct {
	Answer  string      `json:"answer"`
	Sources []AskSource `json:"sources"`
}

// AskSource is a source citation from the RAG pipeline.
type AskSource struct {
	File string `json:"file"`
	Page int    `json:"page"`
}

// Collection represents a Qdrant collection with metadata.
type Collection struct {
	Name       string `json:"name"`
	PointCount int    `json:"point_count"`
}

// RAGClient calls the Python chat and ingestion services.
type RAGClient struct {
	chatURL      string
	ingestionURL string
	http         *http.Client
	breaker      *gobreaker.CircuitBreaker[any]
	retryCfg     resilience.RetryConfig
}

// NewRAGClient creates a client for the Python RAG services.
func NewRAGClient(chatURL, ingestionURL string, breaker *gobreaker.CircuitBreaker[any]) *RAGClient {
	cfg := resilience.DefaultRetryConfig()
	cfg.IsRetryable = func(err error) bool {
		if err == nil {
			return false
		}
		msg := err.Error()
		return !strings.Contains(msg, "status 4")
	}
	return &RAGClient{
		chatURL:      chatURL,
		ingestionURL: ingestionURL,
		http:         &http.Client{Timeout: 30 * time.Second, Transport: otelhttp.NewTransport(http.DefaultTransport)},
		breaker:      breaker,
		retryCfg:     cfg,
	}
}

// Search calls POST /search on the chat service for retrieval-only results.
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

// Ask calls POST /chat on the chat service with Accept: application/json for a full RAG response.
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

// ListCollections calls GET /collections on the ingestion service.
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
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd go/ai-service && go test ./internal/tools/clients/ -run TestRAGClient -v`

Expected: All 4 tests pass

- [ ] **Step 5: Commit**

```bash
git add go/ai-service/internal/tools/clients/rag.go go/ai-service/internal/tools/clients/rag_test.go
git commit -m "feat(ai-service): add RAG HTTP client for Python services"
```

---

### Task 5: Go tools — `search_documents`, `ask_document`, `list_collections`

Create the three RAG tools following the existing tool pattern.

**Files:**
- Create: `go/ai-service/internal/tools/rag.go`
- Create: `go/ai-service/internal/tools/rag_test.go`

- [ ] **Step 1: Write failing tests**

Create `go/ai-service/internal/tools/rag_test.go`:

```go
package tools

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/kabradshaw1/portfolio/go/ai-service/internal/tools/clients"
)

// fakeRAG is a test double for the ragAPI interface.
type fakeRAG struct {
	searchResults  []clients.SearchResult
	searchErr      error
	askAnswer      clients.AskAnswer
	askErr         error
	collections    []clients.Collection
	collectionsErr error
}

func (f *fakeRAG) Search(ctx context.Context, query, collection string, limit int) ([]clients.SearchResult, error) {
	return f.searchResults, f.searchErr
}

func (f *fakeRAG) Ask(ctx context.Context, question, collection string) (clients.AskAnswer, error) {
	return f.askAnswer, f.askErr
}

func (f *fakeRAG) ListCollections(ctx context.Context) ([]clients.Collection, error) {
	return f.collections, f.collectionsErr
}

func TestSearchDocumentsTool_Success(t *testing.T) {
	fake := &fakeRAG{searchResults: []clients.SearchResult{
		{Text: "Kubernetes is...", Filename: "k8s.pdf", PageNumber: 1, Score: 0.95},
		{Text: "Pods are...", Filename: "k8s.pdf", PageNumber: 3, Score: 0.82},
	}}
	tool := NewSearchDocumentsTool(fake)

	if tool.Name() != "search_documents" {
		t.Fatalf("expected name search_documents, got %s", tool.Name())
	}

	res, err := tool.Call(context.Background(), json.RawMessage(`{"query":"kubernetes"}`), "")
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	items, ok := res.Content.([]map[string]any)
	if !ok {
		t.Fatalf("expected []map content, got %T", res.Content)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 results, got %d", len(items))
	}
	if items[0]["text"] != "Kubernetes is..." {
		t.Errorf("unexpected text: %v", items[0]["text"])
	}
}

func TestSearchDocumentsTool_MissingQuery(t *testing.T) {
	tool := NewSearchDocumentsTool(&fakeRAG{})
	_, err := tool.Call(context.Background(), json.RawMessage(`{}`), "")
	if err == nil {
		t.Fatal("expected error for missing query")
	}
}

func TestSearchDocumentsTool_APIError(t *testing.T) {
	fake := &fakeRAG{searchErr: errors.New("connection refused")}
	tool := NewSearchDocumentsTool(fake)
	_, err := tool.Call(context.Background(), json.RawMessage(`{"query":"test"}`), "")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestAskDocumentTool_Success(t *testing.T) {
	fake := &fakeRAG{askAnswer: clients.AskAnswer{
		Answer:  "Kubernetes is a container orchestration platform.",
		Sources: []clients.AskSource{{File: "k8s.pdf", Page: 1}},
	}}
	tool := NewAskDocumentTool(fake)

	if tool.Name() != "ask_document" {
		t.Fatalf("expected name ask_document, got %s", tool.Name())
	}

	res, err := tool.Call(context.Background(), json.RawMessage(`{"question":"what is kubernetes"}`), "")
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	m, ok := res.Content.(map[string]any)
	if !ok {
		t.Fatalf("expected map content, got %T", res.Content)
	}
	if m["answer"] != "Kubernetes is a container orchestration platform." {
		t.Errorf("unexpected answer: %v", m["answer"])
	}
}

func TestAskDocumentTool_MissingQuestion(t *testing.T) {
	tool := NewAskDocumentTool(&fakeRAG{})
	_, err := tool.Call(context.Background(), json.RawMessage(`{}`), "")
	if err == nil {
		t.Fatal("expected error for missing question")
	}
}

func TestListCollectionsTool_Success(t *testing.T) {
	fake := &fakeRAG{collections: []clients.Collection{
		{Name: "documents", PointCount: 150},
		{Name: "debug-myproject", PointCount: 42},
	}}
	tool := NewListCollectionsTool(fake)

	if tool.Name() != "list_collections" {
		t.Fatalf("expected name list_collections, got %s", tool.Name())
	}

	res, err := tool.Call(context.Background(), json.RawMessage(`{}`), "")
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	items, ok := res.Content.([]map[string]any)
	if !ok {
		t.Fatalf("expected []map content, got %T", res.Content)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 collections, got %d", len(items))
	}
	if items[0]["name"] != "documents" {
		t.Errorf("unexpected name: %v", items[0]["name"])
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd go/ai-service && go test ./internal/tools/ -run "TestSearchDocuments|TestAskDocument|TestListCollections" -v`

Expected: FAIL — types and constructors not found

- [ ] **Step 3: Implement the RAG tools**

Create `go/ai-service/internal/tools/rag.go`:

```go
package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/kabradshaw1/portfolio/go/ai-service/internal/tools/clients"
)

// ragAPI is the interface the RAG tools depend on.
type ragAPI interface {
	Search(ctx context.Context, query, collection string, limit int) ([]clients.SearchResult, error)
	Ask(ctx context.Context, question, collection string) (clients.AskAnswer, error)
	ListCollections(ctx context.Context) ([]clients.Collection, error)
}

// --- search_documents ---

type searchDocumentsTool struct {
	api ragAPI
}

func NewSearchDocumentsTool(api ragAPI) Tool { return &searchDocumentsTool{api: api} }

func (t *searchDocumentsTool) Name() string { return "search_documents" }
func (t *searchDocumentsTool) Description() string {
	return "Search uploaded documents using semantic similarity. Returns matching text chunks with relevance scores. Use this to find specific information across documents without generating an answer."
}

func (t *searchDocumentsTool) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type":"object",
		"properties":{
			"query":{"type":"string","description":"The search query to find relevant document chunks."},
			"collection":{"type":"string","description":"Optional collection name to search within. Omit to search the default collection."},
			"limit":{"type":"integer","description":"Maximum number of results to return (1-20, default 5)."}
		},
		"required":["query"]
	}`)
}

type searchDocumentsArgs struct {
	Query      string `json:"query"`
	Collection string `json:"collection"`
	Limit      int    `json:"limit"`
}

func (t *searchDocumentsTool) Call(ctx context.Context, args json.RawMessage, userID string) (Result, error) {
	var a searchDocumentsArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return Result{}, fmt.Errorf("search_documents: bad args: %w", err)
	}
	if a.Query == "" {
		return Result{}, errors.New("search_documents: query is required")
	}
	if a.Limit <= 0 {
		a.Limit = 5
	}
	if a.Limit > 20 {
		a.Limit = 20
	}

	results, err := t.api.Search(ctx, a.Query, a.Collection, a.Limit)
	if err != nil {
		return Result{}, fmt.Errorf("search_documents: %w", err)
	}

	items := make([]map[string]any, len(results))
	for i, r := range results {
		items[i] = map[string]any{
			"text":        r.Text,
			"filename":    r.Filename,
			"page_number": r.PageNumber,
			"score":       r.Score,
		}
	}
	return Result{
		Content: items,
		Display: map[string]any{"kind": "search_results", "results": results},
	}, nil
}

// --- ask_document ---

type askDocumentTool struct {
	api ragAPI
}

func NewAskDocumentTool(api ragAPI) Tool { return &askDocumentTool{api: api} }

func (t *askDocumentTool) Name() string { return "ask_document" }
func (t *askDocumentTool) Description() string {
	return "Ask a question about uploaded documents. Uses RAG to retrieve relevant context and generate an answer with source citations. Use this for questions that need a synthesized answer, not just raw search results."
}

func (t *askDocumentTool) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type":"object",
		"properties":{
			"question":{"type":"string","description":"The question to answer based on the uploaded documents."},
			"collection":{"type":"string","description":"Optional collection name to search within. Omit for the default collection."}
		},
		"required":["question"]
	}`)
}

type askDocumentArgs struct {
	Question   string `json:"question"`
	Collection string `json:"collection"`
}

func (t *askDocumentTool) Call(ctx context.Context, args json.RawMessage, userID string) (Result, error) {
	var a askDocumentArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return Result{}, fmt.Errorf("ask_document: bad args: %w", err)
	}
	if a.Question == "" {
		return Result{}, errors.New("ask_document: question is required")
	}

	answer, err := t.api.Ask(ctx, a.Question, a.Collection)
	if err != nil {
		return Result{}, fmt.Errorf("ask_document: %w", err)
	}

	sources := make([]map[string]any, len(answer.Sources))
	for i, s := range answer.Sources {
		sources[i] = map[string]any{"file": s.File, "page": s.Page}
	}
	return Result{
		Content: map[string]any{"answer": answer.Answer, "sources": sources},
		Display: map[string]any{"kind": "rag_answer", "answer": answer.Answer, "sources": answer.Sources},
	}, nil
}

// --- list_collections ---

type listCollectionsTool struct {
	api ragAPI
}

func NewListCollectionsTool(api ragAPI) Tool { return &listCollectionsTool{api: api} }

func (t *listCollectionsTool) Name() string { return "list_collections" }
func (t *listCollectionsTool) Description() string {
	return "List all available document collections. Returns collection names and how many document chunks each contains. Use this to discover what documents have been uploaded."
}

func (t *listCollectionsTool) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type":"object",
		"properties":{}
	}`)
}

func (t *listCollectionsTool) Call(ctx context.Context, args json.RawMessage, userID string) (Result, error) {
	collections, err := t.api.ListCollections(ctx)
	if err != nil {
		return Result{}, fmt.Errorf("list_collections: %w", err)
	}

	items := make([]map[string]any, len(collections))
	for i, c := range collections {
		items[i] = map[string]any{
			"name":        c.Name,
			"point_count": c.PointCount,
		}
	}
	return Result{
		Content: items,
		Display: map[string]any{"kind": "collections_list", "collections": collections},
	}, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd go/ai-service && go test ./internal/tools/ -run "TestSearchDocuments|TestAskDocument|TestListCollections" -v`

Expected: All 6 tests pass

- [ ] **Step 5: Commit**

```bash
git add go/ai-service/internal/tools/rag.go go/ai-service/internal/tools/rag_test.go
git commit -m "feat(ai-service): add search_documents, ask_document, list_collections tools"
```

---

### Task 6: Register RAG tools in main.go

Wire the RAG client and tools into the ai-service startup.

**Files:**
- Modify: `go/ai-service/cmd/server/main.go`

- [ ] **Step 1: Add RAG env vars and client creation**

In `go/ai-service/cmd/server/main.go`, add after the ecommerce URL env var read (after line 57):

```go
ragChatURL := getenv("RAG_CHAT_URL", "http://chat-service:8001")
ragIngestionURL := getenv("RAG_INGESTION_URL", "http://ingestion-service:8002")
```

Add a circuit breaker after the ecommerce breaker (after line 81):

```go
ragBreaker := resilience.NewBreaker(resilience.BreakerConfig{
	Name:          "ai-rag",
	OnStateChange: resilience.ObserveStateChange,
})
```

Add RAG client creation after the ecommerce client (after line 102):

```go
ragClient := clients.NewRAGClient(ragChatURL, ragIngestionURL, ragBreaker)
```

- [ ] **Step 2: Register RAG tools**

Add after the existing tool registrations (after line 132):

```go
// RAG document tools
registry.Register(tools.Cached(tools.NewSearchDocumentsTool(ragClient), toolCache, 30*time.Second))
registry.Register(tools.NewAskDocumentTool(ragClient))
registry.Register(tools.Cached(tools.NewListCollectionsTool(ragClient), toolCache, 60*time.Second))
```

Note: `ask_document` is not cached (answers depend on LLM generation), but `search_documents` and `list_collections` are cached with appropriate TTLs.

- [ ] **Step 3: Run all Go tests**

Run: `cd go/ai-service && go test ./... -v`

Expected: All tests pass

- [ ] **Step 4: Run preflight**

Run: `make preflight-go`

Expected: golangci-lint + tests pass

- [ ] **Step 5: Commit**

```bash
git add go/ai-service/cmd/server/main.go
git commit -m "feat(ai-service): register RAG tools in main.go"
```

---

### Task 7: K8s configuration

Add the RAG service URLs to the Go ai-service deployment.

**Files:**
- Modify: `go/k8s/` — ai-service ConfigMap or deployment env vars

- [ ] **Step 1: Find the ai-service K8s config**

Check which file contains the ai-service environment variables:

```bash
grep -r "ECOMMERCE_URL" go/k8s/ --include="*.yml" --include="*.yaml" -l
```

- [ ] **Step 2: Add RAG env vars to the ConfigMap/deployment**

Add alongside the existing `ECOMMERCE_URL`:

```yaml
RAG_CHAT_URL: "http://chat-service.ai-services.svc.cluster.local:8001"
RAG_INGESTION_URL: "http://ingestion-service.ai-services.svc.cluster.local:8002"
```

The Python services run in the `ai-services` namespace while the Go ai-service runs in `go-ecommerce`. Cross-namespace service references need the full `<service>.<namespace>.svc.cluster.local` DNS name.

- [ ] **Step 3: Also add to QA overlay if separate**

Check for QA-specific config and add the same env vars pointing to the QA namespace services if they exist.

- [ ] **Step 4: Update docker-compose if needed**

Check `docker-compose.yml` for the ai-service and add env vars for local dev:

```yaml
RAG_CHAT_URL: "http://chat:8001"
RAG_INGESTION_URL: "http://ingestion:8002"
```

- [ ] **Step 5: Commit**

```bash
git add go/k8s/ docker-compose.yml
git commit -m "chore(k8s): add RAG service URLs to ai-service config"
```

---

### Task 8: RAG learning ADR

Write the ADR documenting RAG concepts grounded in the codebase.

**Files:**
- Create: `docs/adr/mcp-rag-bridge.md`

- [ ] **Step 1: Write the ADR**

Create `docs/adr/mcp-rag-bridge.md` with the following structure. Each section should reference actual file paths and explain the concept in interview-ready terms:

1. **Decision** — Why bridge RAG via MCP through Go rather than extending Python. Reference `docs/adr/rag-reevaluation-2026-04.md` for strategic context.

2. **Chunking strategies** — Explain `RecursiveCharacterTextSplitter` (used in `services/ingestion/app/chunker.py`), chunk_size=1000 / overlap=200 defaults (from `services/ingestion/app/config.py`), trade-offs: smaller chunks = more precise retrieval but less context per chunk; larger chunks = more context but noisier matches. The debug service uses `from_language(Language.PYTHON)` with chunk_size=1500 (`services/debug/app/indexer.py`) — explain why code needs different splitting.

3. **Embeddings & similarity** — nomic-embed-text produces 768-dimensional vectors. Cosine similarity (configured in `services/ingestion/app/store.py` line 28-29). Explain: embeddings capture semantic meaning, cosine measures angle between vectors (1.0 = identical direction). Why cosine over dot product or Euclidean.

4. **Retrieval** — Top-k search returns the k most similar chunks (`services/chat/app/retriever.py`). Score interpretation: 0.9+ is strong match, 0.7-0.9 is relevant, <0.7 is usually noise. The new `/search` endpoint lets you inspect retrieval quality directly. Precision (are the returned chunks relevant?) vs recall (did we miss relevant chunks?).

5. **RAG prompt engineering** — Walk through `services/chat/app/prompt.py`: XML-wrapped context prevents prompt injection from document content, "answer only from context" instruction reduces hallucination, source citations enable verification. Explain the prompt template structure.

6. **Evaluation** — How to measure RAG quality: faithfulness (does the answer reflect the retrieved context?), answer relevance (does it address the question?), context relevance (did we retrieve the right chunks?). Mention RAGAS framework. Manual evaluation approach: compare `/search` results vs `/chat` answers for the same query.

7. **Production considerations** — Hybrid search (BM25 keyword + semantic), metadata filtering (filter by filename/date before vector search), re-ranking (cross-encoder to re-score top-k results), caching (embedding cache for repeated queries), chunk hierarchies (parent-child chunks for context expansion).

- [ ] **Step 2: Commit**

```bash
git add docs/adr/mcp-rag-bridge.md
git commit -m "docs: add MCP-RAG bridge ADR with RAG learning notes"
```

---

### Task 9: End-to-end verification

Run all preflight checks and verify the full integration.

**Files:** None (verification only)

- [ ] **Step 1: Run full preflight**

Run: `make preflight`

Expected: All checks pass (Python + Go + frontend + security)

- [ ] **Step 2: Local smoke test (if services are running)**

If Python services + Go service are accessible via SSH tunnel:

```bash
# List collections via Go ai-service MCP
echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"test","version":"1.0.0"}}}
{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}' | \
  RAG_CHAT_URL=http://localhost:8001 RAG_INGESTION_URL=http://localhost:8002 \
  ECOMMERCE_URL=http://localhost:8092 JWT_SECRET=test-secret \
  go run ./go/ai-service/cmd/server/ mcp
```

Verify: tool list includes `search_documents`, `ask_document`, `list_collections` alongside the 9 ecommerce tools.

- [ ] **Step 3: Push and watch CI**

```bash
git push origin HEAD
```

Monitor GitHub Actions. Fix any CI failures before creating PR.

- [ ] **Step 4: Create PR to qa**

```bash
gh pr create --base qa --title "feat: MCP-RAG bridge" --body "Expose Python RAG pipeline as MCP tools through Go ai-service.

## Changes
- Python chat: add POST /search (retrieval-only) and JSON response mode for /chat
- Python ingestion: add GET /collections
- Go ai-service: add RAG HTTP client + 3 new tools (search_documents, ask_document, list_collections)
- K8s config: add RAG_CHAT_URL and RAG_INGESTION_URL
- ADR: RAG learning notes grounded in codebase

## Test plan
- [ ] Unit tests for all new Python endpoints
- [ ] Unit tests for Go RAG client and tools
- [ ] golangci-lint + ruff pass
- [ ] MCP stdio smoke test shows all 12 tools
- [ ] Claude Desktop integration test (post-deploy)"
```
