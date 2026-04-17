# MCP–RAG Bridge: Expose Document Search via Go AI Service

**Date:** 2026-04-16
**Status:** Draft
**Author:** Kyle Bradshaw + Claude

## Context

The Go ai-service already supports MCP (server + client) with 9 ecommerce tools. The Python RAG services (ingestion, chat) are complete and deployed but only accessible via their own HTTP endpoints. There's no way for an MCP client (Claude Desktop, Cursor, the Go agent itself) to search uploaded documents or ask questions against the RAG pipeline.

Bridging these — exposing the RAG pipeline as MCP tools through the Go ai-service — creates a single MCP gateway for all AI capabilities and provides a vehicle for learning RAG concepts in depth.

## Decision

Add RAG-proxy tools to the Go ai-service that call the existing Python services over HTTP. Add two small Python endpoints to support discovery and raw retrieval. Write an ADR covering the RAG concepts encountered during the build.

## Scope

### Go ai-service: New tools

All tools live in `go/ai-service/internal/tools/` following the existing `Tool` interface pattern. Registered in `main.go` alongside ecommerce tools.

#### `search_documents`
- **Purpose:** Raw semantic search — returns retrieved chunks without LLM generation. This exposes the "R" in RAG so you can inspect retrieval quality independently.
- **Params:** `query` (string, required), `collection` (string, optional), `limit` (int, optional, default 5)
- **Calls:** `POST /search` on the Python chat service (new endpoint, see below)
- **Returns:** Array of `{text, filename, page_number, score}`
- **Auth:** None required (public read)

#### `ask_document`
- **Purpose:** Full RAG Q&A — embed, search, build prompt, generate answer with citations.
- **Params:** `question` (string, required), `collection` (string, optional)
- **Calls:** `POST /chat` on the Python chat service (existing endpoint, non-streaming mode)
- **Returns:** `{answer, sources: [{filename, page_number}]}`
- **Auth:** None required (public read)
- **Note:** The Python `/chat` endpoint currently streams SSE. We need a non-streaming response path for tool calls. Options: (a) add `Accept: application/json` header support to return a single JSON response, or (b) consume the SSE stream in the Go client and aggregate. Option (a) is cleaner.

#### `list_collections`
- **Purpose:** Discovery — what document sets are available to search.
- **Params:** None
- **Calls:** `GET /collections` on the Python ingestion service (new endpoint)
- **Returns:** Array of `{name, document_count, point_count}`
- **Auth:** None required

#### `index_document` (v1: URL-based only)
- **Purpose:** Index a new document into the RAG pipeline.
- **Params:** `url` (string, required — publicly accessible PDF URL), `collection` (string, optional)
- **Calls:** `POST /ingest` on the Python ingestion service
- **Returns:** `{collection, chunks_created, document_id}`
- **Auth:** JWT required (writes to the vector store)
- **Limitation:** MCP tool calls can't easily pass file bytes. V1 accepts a URL; the Python service fetches and processes it. Direct file upload stays on the existing frontend flow.

### Go ai-service: RAG client

New file: `go/ai-service/internal/tools/clients/rag.go`

An HTTP client for the Python RAG services, following the same pattern as `ecommerce.go`:
- Circuit breaker integration
- Retry with exponential backoff
- OpenTelemetry instrumentation
- Configurable base URL via `RAG_SERVICE_URL` env var (defaults to `http://chat-service:8001` for K8s)

Needs two base URLs:
- `RAG_CHAT_URL` — Python chat service (search + ask)
- `RAG_INGESTION_URL` — Python ingestion service (collections + ingest)

### Python chat service: New endpoints

#### `POST /search`
- **Location:** `services/chat/app/main.py`
- **Purpose:** Retrieval-only endpoint — embed the query and return ranked chunks without LLM generation.
- **Request:** `{"query": "...", "collection": "...", "limit": 5}`
- **Response:** `{"results": [{"text": "...", "filename": "...", "page_number": 1, "score": 0.87}]}`
- **Implementation:** Reuses the existing `QdrantRetriever.search()` from `retriever.py` and `EmbeddingProvider.embed()` from shared. No new logic — just a new endpoint wiring existing pieces.
- **Why this matters for learning:** This endpoint isolates the retrieval step. You can compare what the retriever finds (`/search`) vs what the LLM produces (`/chat`) to understand how RAG prompt engineering transforms raw results into answers.

#### `POST /chat` — JSON response mode
- Add `Accept: application/json` header support to the existing `/chat` endpoint.
- When requested, run the full RAG pipeline but return a single JSON response instead of SSE stream: `{"answer": "...", "sources": [{"filename": "...", "page_number": 1}]}`
- Streaming SSE remains the default (no breaking changes).

### Python ingestion service: New endpoint

#### `GET /collections`
- **Location:** `services/ingestion/app/main.py`
- **Purpose:** List all Qdrant collections with metadata.
- **Response:** `{"collections": [{"name": "...", "document_count": 12, "point_count": 340}]}`
- **Implementation:** Calls Qdrant client's `get_collections()` + `get_collection()` for counts.

#### `POST /ingest` — URL mode
- Extend the existing `/ingest` endpoint to accept a JSON body with `{"url": "...", "collection": "..."}` as an alternative to multipart file upload.
- The service fetches the PDF from the URL, then runs the same pipeline (parse → chunk → embed → store).
- Validate: URL must end in `.pdf`, response content-type must be `application/pdf`, enforce same size limits.

### Configuration

New env vars for Go ai-service:

| Variable | Default | Purpose |
|----------|---------|---------|
| `RAG_CHAT_URL` | `http://chat-service:8001` | Python chat service base URL |
| `RAG_INGESTION_URL` | `http://ingestion-service:8002` | Python ingestion service base URL |

K8s: Add these to the Go ai-service deployment ConfigMap. For local dev, they point to `localhost` via SSH tunnel or Docker Compose networking.

### RAG Learning ADR

File: `docs/adr/mcp-rag-bridge.md`

Structured as a design record grounded in the actual codebase:

1. **Decision** — Why bridge RAG via MCP through Go, architectural trade-offs
2. **Chunking strategies** — Current settings (RecursiveCharacterTextSplitter, chunk_size=1000, overlap=200), why those numbers, how chunk size affects retrieval quality, when to use language-aware splitting (as the debug service does for Python)
3. **Embeddings & similarity** — nomic-embed-text, 768 dimensions, cosine distance, what these mean practically, how embedding model choice affects search quality
4. **Retrieval** — Top-k search, score interpretation, precision vs recall, when re-ranking helps, the `/search` endpoint as a diagnostic tool
5. **RAG prompt engineering** — XML context wrapping (prevents prompt injection from document content), "answer only from context" constraints, citation strategies, the chat service's prompt template as a worked example
6. **Evaluation** — How to measure RAG quality: faithfulness (does the answer match the context?), relevance (did we retrieve the right chunks?), frameworks like RAGAS, manual evaluation approaches
7. **Production considerations** — Hybrid search (BM25 + semantic), metadata filtering, caching, chunk hierarchies, what you'd add to this pipeline for scale

Each section references specific code files and line numbers.

## Files modified

### Go (new files)
- `go/ai-service/internal/tools/clients/rag.go` — RAG service HTTP client
- `go/ai-service/internal/tools/search_documents.go` — search_documents tool
- `go/ai-service/internal/tools/ask_document.go` — ask_document tool
- `go/ai-service/internal/tools/list_collections.go` — list_collections tool
- `go/ai-service/internal/tools/index_document.go` — index_document tool

### Go (modified files)
- `go/ai-service/cmd/server/main.go` — register new tools, add RAG client init, new env vars
- `go/ai-service/internal/mcp/server.go` — no changes expected (tools auto-register via registry)

### Python (modified files)
- `services/chat/app/main.py` — add `POST /search` endpoint, add JSON response mode to `/chat`
- `services/chat/app/retriever.py` — possibly extract a `search_only()` method if needed
- `services/ingestion/app/main.py` — add `GET /collections` endpoint, add URL mode to `/ingest`
- `services/ingestion/app/store.py` — add `list_collections()` method if not already present

### Documentation
- `docs/adr/mcp-rag-bridge.md` — RAG learning ADR

### K8s / Config
- `go/k8s/` — add `RAG_CHAT_URL` and `RAG_INGESTION_URL` to ai-service ConfigMap
- `docker-compose.yml` — add RAG env vars for local dev (if not already reachable)

## Verification

1. **Preflight checks:** `make preflight-python` (for Python changes), `make preflight-go` (for Go changes)
2. **Unit tests (Go):** Mock RAG client responses, verify tool parameter validation and response shaping
3. **Unit tests (Python):** Test `/search` endpoint returns correct format, test `/collections` lists correctly, test URL-based ingest
4. **Local integration test:**
   - Start Python services + Qdrant via Docker Compose
   - Upload a test PDF via the existing frontend or curl
   - Start Go ai-service with `RAG_CHAT_URL=http://localhost:8001` and `RAG_INGESTION_URL=http://localhost:8002`
   - Test via MCP stdio: `echo '...' | ./ai-service mcp` → list tools, call search_documents, call ask_document
5. **Claude Desktop smoke test:**
   - Update `claude_desktop_config.json` to point at the ai-service
   - Verify all tools appear (ecommerce + RAG)
   - Ask Claude to search your documents, then ask a question about them
6. **RAG quality inspection:**
   - Call `/search` directly to see raw retrieval results with scores
   - Call `/chat` (or `ask_document` tool) with the same query
   - Compare: did the LLM use the retrieved context faithfully? Did it hallucinate?
   - Document findings in the ADR

## Out of scope

- Frontend changes (Shopping Assistant drawer expansion — natural follow-up)
- Re-ranking or hybrid search implementation (discussed in ADR as "what we'd add")
- Python MCP server (Go is the single MCP gateway)
- Multi-modal RAG (images, tables)
- Authentication on RAG read endpoints (public for now, matching existing `/chat` behavior)
