# System Architecture Documentation — Design Spec

**Issue:** #76 (Phase 1 of AI Platform Roadmap #75)
**Scope:** Documentation only — no code changes
**Deliverables:** `docs/architecture.md` + CLAUDE.md update

## Context

The portfolio has extensive per-service ADRs (21+ standalone markdown, 17 Jupyter notebooks) and a README with a tech stack overview, but no single document explaining how the whole system works end-to-end. This is a problem for three audiences:

1. **Interview prep** — Kyle needs to articulate the AI platform architecture clearly for Gen AI Engineer interviews
2. **Agents** — Claude Code and other agents working on the repo need system-level context beyond what CLAUDE.md currently provides
3. **Portfolio visitors** — deferred to a later phase; the frontend architecture page will be built after the AI platform work (Phases 2-4) is complete

This spec covers deliverables 1 and 2. The frontend page is out of scope.

## Deliverable 1: `docs/architecture.md`

### Structure

```
# System Architecture

## 1. System Overview
## 2. AI Platform Deep Dive
  ### 2.1 The MCP Tool Gateway (Go ai-service)
  ### 2.2 RAG Pipeline (Python services)
  ### 2.3 MCP-RAG Bridge
  ### 2.4 End-to-End Data Flow
  ### 2.5 Observability
## 3. Infrastructure
```

### Section Details

#### 1. System Overview

One paragraph explaining what this project is: a polyglot portfolio demonstrating RAG architecture, agentic AI, and production microservices through three backend stacks (Go, Java, Python) with a Next.js frontend.

**Service map Mermaid diagram** showing all services grouped by stack:
- Go: auth-service, ecommerce-service, ai-service
- Java: task-service, activity-service, notification-service, gateway-service
- Python: ingestion, chat, debug, eval
- Databases: PostgreSQL, MongoDB, Redis, RabbitMQ, Qdrant
- External: Ollama (Qwen 2.5 14B, nomic-embed-text)

Brief description of how the frontend connects to each backend:
- REST + JWT cookies → Go auth/ecommerce
- GraphQL via Apollo → Java gateway
- SSE → Go ai-service (unified AI assistant)

#### 2. AI Platform Deep Dive

##### 2.1 The MCP Tool Gateway (Go ai-service)

**Tool registry pattern:**
- Interface: `Name()`, `Description()`, `Schema()`, `Call(ctx, args, userID)`
- In-memory `MemRegistry` in `go/ai-service/internal/tools/registry.go`
- 12 built-in tools registered at startup in `main.go`:
  - Ecommerce (9): search_products, get_product, check_inventory, list_orders, get_order, summarize_orders, view_cart, add_to_cart, initiate_return
  - RAG (3): search_documents, ask_document, list_collections
- MCP external tool discovery via `DiscoverTools()` with namespace prefixing

**Agent loop (ReAct pattern):**
- Implementation: `Agent.Run()` in `go/ai-service/internal/agent/agent.go`
- Max 8 steps, 90-second timeout
- Flow: call Ollama with tool schemas → detect tool calls → execute tools → feed results back → repeat until final response or step limit
- Refusal detection via guardrails
- `safeCall()` wrapper catches panics from tool execution

**Resilience:**
- Redis caching via `Cached()` decorator — TTLs: 60s (products, collections), 30s (search_documents), 10s (inventory, orders)
- Cache key: SHA256 of `toolName + args + userID`
- Circuit breakers on all external HTTP calls
- Rate limiting: 20 requests/min per IP on POST /chat (Redis-backed fixed-window)

##### 2.2 RAG Pipeline (Python services)

**Ingestion service** (`services/ingestion/`):
- PDF upload → `pdf_parser.py` extracts text per page → `chunker.py` splits with `RecursiveCharacterTextSplitter` (1000 tokens, 200 overlap) → `embedder.py` embeds with nomic-embed-text (768-dim) → `store.py` upserts to Qdrant with COSINE distance
- Multi-collection support, per-document UUID tracking for deletion
- Endpoints: POST /ingest, GET /collections, GET /documents, DELETE /documents/{id}, DELETE /collections/{name}

**Chat service** (`services/chat/`):
- Question → embed → Qdrant top-k vector search → collect source metadata (filename, page) → assemble RAG prompt with retrieved chunks → stream response from Ollama via SSE
- Endpoints: POST /chat (RAG generation), POST /search (retrieval only, no generation)
- Content negotiation: SSE if `Accept: text/event-stream`, JSON otherwise

**Debug service** (`services/debug/`):
- Code-aware indexing: walks Python files, chunks with `RecursiveCharacterTextSplitter.from_language(Language.PYTHON)` (1500 chars, 200 overlap), stores file paths + line numbers
- Agent loop: up to 10 iterations with tools (file search, grep, test run)
- Security: path allowlist via `settings.allowed_project_paths`
- Endpoints: POST /index, POST /debug

**Eval service** (`services/eval/`):
- RAGAS evaluation: builds dataset by querying chat service, runs AnswerRelevancy, ContextPrecision, ContextRecall, Faithfulness metrics
- SQLite storage for datasets and evaluation results
- Endpoints: POST /datasets, POST /evaluations, GET /evaluations/{id}

**Shared LLM module** (`services/shared/llm/`):
- Provider factory supporting Ollama, OpenAI, Anthropic
- Protocol-based interfaces: `EmbeddingProvider`, `LLMProvider`
- All providers implement `embed()`, `generate()`, `chat()`, `check_health()`

##### 2.3 MCP-RAG Bridge

How Go wraps Python services as MCP tools:
- `RAGClient` in `go/ai-service/internal/tools/clients/rag.go`
- Hits chat service at POST `/search` and POST `/chat`
- Hits ingestion service at GET `/collections`
- 30-second HTTP timeout (vs 5s for ecommerce) to accommodate LLM generation
- Circuit breaker via `resilience.Call` wrapper
- OTel trace propagation via `otelhttp.NewTransport()`
- Three tool wrappers in `tools/rag.go`: `SearchDocumentsTool` (cached 30s), `AskDocumentTool` (not cached), `ListCollectionsTool` (cached 60s)

##### 2.4 End-to-End Data Flow

**Mermaid sequence diagram** showing the full tool-call flow:
1. User types question in frontend
2. Frontend opens SSE connection: POST /chat → Go ai-service
3. Go agent calls Ollama with tool schemas
4. Ollama returns tool_call (e.g., search_documents)
5. Agent dispatches to tool → tool calls Python chat service POST /search
6. Python embeds query, searches Qdrant, returns chunks
7. Agent feeds result back to Ollama
8. Ollama may call another tool or return final response
9. Each step emits SSE events: tool_call, tool_result, tool_error, final

**SSE event format:**
- `event: tool_call\ndata: {"name": "...", "args": {...}}\n\n`
- `event: tool_result\ndata: {"name": "...", "display": {...}}\n\n`
- `event: final\ndata: {"text": "..."}\n\n`

##### 2.5 Observability

**Structured logging:** Go uses `slog.Info` with structured fields — turn_id, user_id, steps, tools_called, duration_ms, outcome (final/refused/error/max_steps). Python uses structlog with request middleware.

**OpenTelemetry tracing:** Spans hierarchy:
- `agent.turn` (parent) → `agent.llm_call` (per step) → `agent.tool_call` (per tool)
- `ollama.chat` spans with model, prompt/eval token counts
- HTTP client spans via `otelhttp` propagate traces to Python services
- Redis spans via `tracing.RedisSpan()`

**Prometheus metrics:**
- Agent: turns_total (by outcome), steps_per_turn, turn_duration_seconds
- Tools: tool_calls_total (by name/outcome), tool_duration_seconds
- Cache: cache_events_total (hit/miss)
- LLM: ollama_request_duration, ollama_tokens_total, ollama_eval_duration

#### 3. Infrastructure

Brief section pointing to the deployment-architecture ADR for full details. Covers:
- K8s namespaces: ai-services, java-tasks, go-ecommerce, monitoring (+ QA mirrors)
- NGINX Ingress: path-based routing (`/go-api/*` → ecommerce, `/ai-api/*` → ai-service, `/ingestion/*` → Python, etc.)
- Cloudflare Tunnel: `api.kylebradshaw.dev` → Minikube Ingress
- Minikube on Debian 13 with RTX 3090 for Ollama

### Mermaid Diagrams

Three inline Mermaid diagrams:

1. **System service map** — flowchart showing all services grouped by namespace, with connections to databases and inter-service dependencies. Go ai-service at the center connecting to Python services and ecommerce.

2. **MCP tool-call sequence** — sequence diagram: Frontend → Go ai-service → Ollama → Tool dispatch → (Python RAG services | Ecommerce API) → response chain back through SSE.

3. **RAG pipeline flow** — two-path flowchart:
   - Ingestion path: PDF → parse → chunk → embed → Qdrant
   - Query path: question → embed → search → prompt → generate → stream

## Deliverable 2: CLAUDE.md Update

Add an "AI Platform Architecture" section after the existing "Project Structure" section. Content:

```markdown
## AI Platform Architecture

The Go ai-service (`go/ai-service/`) is the MCP gateway for all AI functionality. It fronts 9 ecommerce tools and 3 RAG tools through a unified agent loop.

- **Tool registry:** 12 built-in tools in `go/ai-service/internal/tools/`, registered in `main.go`. Interface: Name/Description/Schema/Call. Cached via Redis wrapper (`tools/cached.go`).
- **Agent loop:** ReAct pattern in `go/ai-service/internal/agent/agent.go`. 8 steps max, 90s timeout. Streams SSE events (tool_call, tool_result, tool_error, final, error) from `internal/http/chat.go`.
- **RAG bridge:** Go calls Python chat service at `/search` and `/chat`, ingestion service at `/collections`. Client in `go/ai-service/internal/tools/clients/rag.go`. 30s timeout, circuit breaker, OTel trace propagation.
- **Python services:** ingestion (PDF→chunk→embed→Qdrant), chat (embed→search→RAG prompt→stream), debug (code indexing + agent loop), eval (RAGAS metrics). Shared LLM factory in `services/shared/llm/`.
- **Key env vars:** `RAG_CHAT_URL`, `RAG_INGESTION_URL`, `OLLAMA_URL`, `REDIS_URL`, `ECOMMERCE_URL`.
- **Frontend integration:** POST /chat with SSE streaming. Frontend client in `frontend/src/lib/ai-service.ts` parses event types.
- **Roadmap (Q2 2026, issue #75):** Phase 1: architecture doc → Phase 2: unified AI assistant UI (#77) → Phase 3: Loki log aggregation (#78) → Phase 4a-c: RAG eval harness, hybrid search, cross-encoder re-ranking (#79-#81).
```

## What's NOT in Scope

- Frontend architecture page (deferred until Phases 2-4 complete)
- Code changes of any kind
- Changes to existing ADRs
- Updating the root README.md (the issue mentions this, but the README already links to docs/ and the architecture doc will be discoverable there)

## Success Criteria

- `docs/architecture.md` exists with all three Mermaid diagrams rendering on GitHub
- Kyle can walk through the doc and explain the AI platform architecture in an interview setting
- CLAUDE.md has enough context for agents to understand service relationships without reading source code
