# Stage C1 — Python Service Instrumentation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add real Prometheus metrics to all three Python FastAPI services (ingestion, chat, debug) — RED via `prometheus-fastapi-instrumentator`, plus custom Ollama, Qdrant, and RAG pipeline histograms/counters. Add pod annotations for Kubernetes SD discovery. Update compose + k8s manifests to stay in sync.

**Architecture:** Each service gets `prometheus-fastapi-instrumentator` wired into its `main.py` to expose `/metrics` with automatic HTTP RED metrics. Custom metrics (Ollama call latency/tokens, Qdrant search latency, embedding latency, RAG pipeline stages) are defined in a shared pattern — a `metrics.py` module per service — and recorded at the call site by wrapping existing `httpx` / Qdrant calls with timing + counter logic. No new abstractions or wrapper classes; just add `time.perf_counter()` calls and `observe()`/`inc()` at the existing call sites.

**Tech Stack:** `prometheus-fastapi-instrumentator`, `prometheus_client` (transitive), Python `time.perf_counter()`.

**Parent spec:** `docs/superpowers/specs/2026-04-09-grafana-overhaul-design.md`

---

## Pre-flight context

- All three services share identical FastAPI app patterns: `app = FastAPI(...)`, CORS middleware, health endpoint, endpoints.
- No middleware beyond CORS currently.
- Ollama calls use `httpx.AsyncClient()` — embed at `/api/embed`, generate at `/api/generate` (chat, streaming), chat at `/api/chat` (debug, non-streaming). Response JSON includes `prompt_eval_count`, `eval_count`, `eval_duration` but only when `stream: false`. The chat service streams, so we can only measure wall-clock latency there, not token counts from the generate call. Embedding calls are non-streaming and return no token fields. The debug service's `call_ollama()` is non-streaming and returns the full response including eval fields.
- Qdrant calls use synchronous `QdrantClient` methods: `search()`, `upsert()`, `scroll()`, `delete()`.
- `docker-compose.yml` and k8s manifests must stay in sync per CLAUDE.md compose-smoke rule.
- Preflight: `make preflight-python` runs ruff + pytest for all three services.
- Tests use `unittest.mock` patches heavily. Adding `prometheus-fastapi-instrumentator` to the app shouldn't break existing tests since it's just middleware.

### Streaming limitation

The chat service's `stream_ollama_response()` uses `client.stream("POST", ...)` with `stream: True`. Ollama only returns `prompt_eval_count`, `eval_count`, `eval_duration` in the **final** chunk when `done: true`. Currently the code breaks on `done` but doesn't capture those fields. We'll extract them from the final chunk to record token metrics for chat too.

---

## File structure

### New files (per service pattern)

```
services/ingestion/app/metrics.py    # metric definitions + instrumentator setup
services/chat/app/metrics.py         # metric definitions + instrumentator setup
services/debug/app/metrics.py        # metric definitions + instrumentator setup
```

### Modified files

```
services/ingestion/requirements.txt  # add prometheus-fastapi-instrumentator
services/chat/requirements.txt       # add prometheus-fastapi-instrumentator
services/debug/requirements.txt      # add prometheus-fastapi-instrumentator
services/ingestion/app/main.py       # wire instrumentator
services/chat/app/main.py            # wire instrumentator
services/debug/app/main.py           # wire instrumentator
services/ingestion/app/embedder.py   # add timing + metrics recording
services/chat/app/chain.py           # add timing + metrics recording
services/chat/app/retriever.py       # add timing + metrics recording
services/debug/app/agent.py          # add timing + metrics recording
services/ingestion/app/store.py      # add timing + metrics recording
k8s/ai-services/deployments/ingestion.yml  # add prometheus.io annotations
k8s/ai-services/deployments/chat.yml       # add prometheus.io annotations
k8s/ai-services/deployments/debug.yml      # add prometheus.io annotations
monitoring/prometheus.yml             # add k8s-pods SD job (compose parity)
```

### New test files

```
services/ingestion/tests/test_metrics.py
services/chat/tests/test_metrics.py
services/debug/tests/test_metrics.py
```

---

## Task 1: Add prometheus-fastapi-instrumentator to all services

**Files:**
- Modify: `services/ingestion/requirements.txt`
- Modify: `services/chat/requirements.txt`
- Modify: `services/debug/requirements.txt`

- [ ] **Step 1: Add the dependency to all three requirements.txt files**

Append to each `requirements.txt`:

```
prometheus-fastapi-instrumentator==7.0.2
```

- [ ] **Step 2: Install locally to verify resolution**

```bash
pip install prometheus-fastapi-instrumentator==7.0.2
```

Expected: installs cleanly, pulls in `prometheus_client` as transitive dep.

- [ ] **Step 3: Commit**

```bash
git add services/ingestion/requirements.txt services/chat/requirements.txt services/debug/requirements.txt
git commit -m "deps(python): add prometheus-fastapi-instrumentator to all services

Co-Authored-By: Claude Opus 4.6 (1M context) <noreply@anthropic.com>"
```

---

## Task 2: Create metrics.py for the ingestion service

**Files:**
- Create: `services/ingestion/app/metrics.py`

- [ ] **Step 1: Write the metrics module**

Create `services/ingestion/app/metrics.py`:

```python
"""Prometheus metrics for the ingestion service."""

from prometheus_client import Counter, Histogram
from prometheus_fastapi_instrumentator import Instrumentator

SERVICE = "ingestion"

# --- Instrumentator (auto RED metrics on /metrics) ---
instrumentator = Instrumentator(
    should_group_status_codes=False,
    excluded_handlers=["/health", "/metrics"],
)

# --- Ollama embedding metrics ---
EMBEDDING_DURATION = Histogram(
    "embedding_duration_seconds",
    "Time spent calling Ollama /api/embed",
    ["service", "model"],
    buckets=(0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0, 30.0, 60.0, 120.0),
)

# --- Qdrant metrics ---
QDRANT_OPERATION_DURATION = Histogram(
    "qdrant_operation_duration_seconds",
    "Time spent on Qdrant operations",
    ["service", "operation"],
    buckets=(0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 5.0),
)

# --- Pipeline metrics ---
CHUNKS_CREATED = Counter(
    "ingestion_chunks_created_total",
    "Total number of chunks created during ingestion",
    ["service"],
)
```

- [ ] **Step 2: Commit**

```bash
git add services/ingestion/app/metrics.py
git commit -m "feat(ingestion): add Prometheus metrics definitions

Embedding duration histogram, Qdrant operation histogram, and chunk counter.

Co-Authored-By: Claude Opus 4.6 (1M context) <noreply@anthropic.com>"
```

---

## Task 3: Wire instrumentator into ingestion main.py and add timing

**Files:**
- Modify: `services/ingestion/app/main.py`
- Modify: `services/ingestion/app/embedder.py`
- Modify: `services/ingestion/app/store.py`

- [ ] **Step 1: Wire instrumentator in main.py**

In `services/ingestion/app/main.py`, add import after existing imports:

```python
from app.metrics import CHUNKS_CREATED, instrumentator
```

After the CORS middleware block (after line 28), add:

```python
instrumentator.instrument(app).expose(app, include_in_schema=False)
```

In the `ingest()` endpoint, after the `store.upsert(...)` call (around line 141), add:

```python
CHUNKS_CREATED.labels(service="ingestion").inc(len(chunks))
```

- [ ] **Step 2: Add timing to embedder.py**

In `services/ingestion/app/embedder.py`, add imports at top:

```python
import time

from app.metrics import EMBEDDING_DURATION
```

Wrap the Ollama call with timing. Replace the body of `embed_texts` (lines 13-25) with:

```python
    if not texts:
        return []

    start = time.perf_counter()
    async with httpx.AsyncClient() as client:
        response = await client.post(
            f"{ollama_base_url}/api/embed",
            json={"model": model, "input": texts},
            timeout=120.0,
        )
        response.raise_for_status()
        data = response.json()
    duration = time.perf_counter() - start
    EMBEDDING_DURATION.labels(service="ingestion", model=model).observe(duration)

    return data["embeddings"]
```

- [ ] **Step 3: Add timing to store.py**

In `services/ingestion/app/store.py`, add imports at top:

```python
import time

from app.metrics import QDRANT_OPERATION_DURATION
```

In `upsert()`, wrap the `self.client.upsert(...)` call (lines 51-54):

```python
        start = time.perf_counter()
        self.client.upsert(
            collection_name=self.collection_name,
            points=points,
        )
        QDRANT_OPERATION_DURATION.labels(
            service="ingestion", operation="upsert"
        ).observe(time.perf_counter() - start)
```

In `search()` within `list_documents()` (the `scroll` call, lines 57-62):

```python
        start = time.perf_counter()
        records, _ = self.client.scroll(
            collection_name=self.collection_name,
            limit=10000,
            with_payload=True,
            with_vectors=False,
        )
        QDRANT_OPERATION_DURATION.labels(
            service="ingestion", operation="scroll"
        ).observe(time.perf_counter() - start)
```

In `delete_document()`, wrap the `self.client.delete(...)` call (lines 96-106):

```python
        start = time.perf_counter()
        self.client.delete(
            collection_name=self.collection_name,
            points_selector=Filter(
                must=[
                    FieldCondition(
                        key="document_id",
                        match=MatchValue(value=document_id),
                    )
                ]
            ),
        )
        QDRANT_OPERATION_DURATION.labels(
            service="ingestion", operation="delete"
        ).observe(time.perf_counter() - start)
```

- [ ] **Step 4: Run ruff and fix any issues**

```bash
ruff check services/ingestion/ && ruff format --check services/ingestion/
```

- [ ] **Step 5: Run tests**

```bash
pytest services/ingestion/tests/ -v
```

Expected: all pass. The `prometheus-fastapi-instrumentator` middleware doesn't affect existing tests; mock patches target `httpx` and `QdrantClient` directly.

- [ ] **Step 6: Commit**

```bash
git add services/ingestion/app/main.py services/ingestion/app/embedder.py services/ingestion/app/store.py
git commit -m "feat(ingestion): wire Prometheus metrics into endpoints and clients

Instrumentator exposes /metrics with automatic RED. Embedding and Qdrant
calls record latency histograms. Chunk counter tracks ingestion volume.

Co-Authored-By: Claude Opus 4.6 (1M context) <noreply@anthropic.com>"
```

---

## Task 4: Create metrics.py for the chat service

**Files:**
- Create: `services/chat/app/metrics.py`

- [ ] **Step 1: Write the metrics module**

Create `services/chat/app/metrics.py`:

```python
"""Prometheus metrics for the chat service."""

from prometheus_client import Counter, Histogram
from prometheus_fastapi_instrumentator import Instrumentator

SERVICE = "chat"

instrumentator = Instrumentator(
    should_group_status_codes=False,
    excluded_handlers=["/health", "/metrics"],
)

# --- Ollama metrics ---
OLLAMA_REQUEST_DURATION = Histogram(
    "ollama_request_duration_seconds",
    "Wall-clock time for Ollama API calls",
    ["service", "model", "operation"],
    buckets=(0.1, 0.5, 1.0, 2.5, 5.0, 10.0, 30.0, 60.0, 120.0, 300.0),
)

OLLAMA_TOKENS = Counter(
    "ollama_tokens_total",
    "Total tokens processed by Ollama",
    ["service", "model", "kind"],
)

OLLAMA_EVAL_DURATION = Histogram(
    "ollama_eval_duration_seconds",
    "Ollama model evaluation duration (from response metadata)",
    ["service", "model"],
    buckets=(0.1, 0.5, 1.0, 2.5, 5.0, 10.0, 30.0, 60.0, 120.0),
)

EMBEDDING_DURATION = Histogram(
    "embedding_duration_seconds",
    "Time spent calling Ollama /api/embed",
    ["service", "model"],
    buckets=(0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0, 30.0, 60.0, 120.0),
)

# --- Qdrant metrics ---
QDRANT_SEARCH_DURATION = Histogram(
    "qdrant_search_duration_seconds",
    "Time spent on Qdrant search operations",
    ["collection"],
    buckets=(0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0),
)

QDRANT_SEARCH_RESULTS = Histogram(
    "qdrant_search_results",
    "Number of results returned by Qdrant search",
    ["collection"],
    buckets=(0, 1, 2, 3, 5, 10, 20),
)

# --- RAG pipeline metrics ---
RAG_PIPELINE_DURATION = Histogram(
    "rag_pipeline_duration_seconds",
    "RAG pipeline stage durations",
    ["stage"],
    buckets=(0.01, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0, 30.0, 60.0),
)

RAG_PIPELINE_ERRORS = Counter(
    "rag_pipeline_errors_total",
    "RAG pipeline errors by stage",
    ["stage"],
)
```

- [ ] **Step 2: Commit**

```bash
git add services/chat/app/metrics.py
git commit -m "feat(chat): add Prometheus metrics definitions

Ollama request/token/eval metrics, Qdrant search histograms,
RAG pipeline stage duration and error counters.

Co-Authored-By: Claude Opus 4.6 (1M context) <noreply@anthropic.com>"
```

---

## Task 5: Wire instrumentator into chat service and add timing

**Files:**
- Modify: `services/chat/app/main.py`
- Modify: `services/chat/app/chain.py`
- Modify: `services/chat/app/retriever.py`

- [ ] **Step 1: Wire instrumentator in main.py**

In `services/chat/app/main.py`, add import:

```python
from app.metrics import instrumentator
```

After the CORS middleware block (after line 23), add:

```python
instrumentator.instrument(app).expose(app, include_in_schema=False)
```

- [ ] **Step 2: Add timing to chain.py**

In `services/chat/app/chain.py`, add imports at top:

```python
import time

from app.metrics import (
    EMBEDDING_DURATION,
    OLLAMA_EVAL_DURATION,
    OLLAMA_REQUEST_DURATION,
    OLLAMA_TOKENS,
    RAG_PIPELINE_DURATION,
)
```

Replace `embed_texts` function body (lines 14-23) with:

```python
    if not texts:
        return []

    start = time.perf_counter()
    async with httpx.AsyncClient() as client:
        response = await client.post(
            f"{ollama_base_url}/api/embed",
            json={"model": model, "input": texts},
            timeout=120.0,
        )
        response.raise_for_status()
    duration = time.perf_counter() - start
    EMBEDDING_DURATION.labels(service="chat", model=model).observe(duration)

    return response.json()["embeddings"]
```

Replace `stream_ollama_response` function (lines 26-52) with:

```python
async def stream_ollama_response(
    prompt: str,
    model: str,
    base_url: str,
) -> AsyncGenerator[dict, None]:
    start = time.perf_counter()
    async with httpx.AsyncClient() as client:
        async with client.stream(
            "POST",
            f"{base_url}/api/generate",
            json={
                "model": model,
                "prompt": prompt,
                "system": SYSTEM_PROMPT,
                "stream": True,
            },
            timeout=300.0,
        ) as response:
            response.raise_for_status()
            import json

            async for line in response.aiter_lines():
                if line.strip():
                    data = json.loads(line)
                    if data.get("response"):
                        yield {"token": data["response"]}
                    if data.get("done"):
                        # Extract token metrics from final chunk
                        duration = time.perf_counter() - start
                        OLLAMA_REQUEST_DURATION.labels(
                            service="chat", model=model, operation="generate"
                        ).observe(duration)
                        prompt_tokens = data.get("prompt_eval_count", 0)
                        completion_tokens = data.get("eval_count", 0)
                        if prompt_tokens:
                            OLLAMA_TOKENS.labels(
                                service="chat", model=model, kind="prompt"
                            ).inc(prompt_tokens)
                        if completion_tokens:
                            OLLAMA_TOKENS.labels(
                                service="chat", model=model, kind="completion"
                            ).inc(completion_tokens)
                        eval_ns = data.get("eval_duration", 0)
                        if eval_ns:
                            OLLAMA_EVAL_DURATION.labels(
                                service="chat", model=model
                            ).observe(eval_ns / 1e9)
                        break
```

Replace `rag_query` function (lines 55-97) with:

```python
async def rag_query(
    question: str,
    ollama_base_url: str,
    chat_model: str,
    embedding_model: str,
    qdrant_host: str,
    qdrant_port: int,
    collection_name: str,
    top_k: int = 5,
) -> AsyncGenerator[dict, None]:
    # Embed the question
    retrieve_start = time.perf_counter()
    vectors = await embed_texts(
        texts=[question],
        ollama_base_url=ollama_base_url,
        model=embedding_model,
    )
    query_vector = vectors[0]

    # Retrieve relevant chunks
    retriever = QdrantRetriever(
        host=qdrant_host, port=qdrant_port, collection_name=collection_name
    )
    chunks = retriever.search(query_vector=query_vector, top_k=top_k)
    RAG_PIPELINE_DURATION.labels(stage="retrieve").observe(
        time.perf_counter() - retrieve_start
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

    # Stream response (generate stage timing is inside stream_ollama_response)
    generate_start = time.perf_counter()
    async for event in stream_ollama_response(
        prompt=prompt, model=chat_model, base_url=ollama_base_url
    ):
        yield event
    RAG_PIPELINE_DURATION.labels(stage="generate").observe(
        time.perf_counter() - generate_start
    )

    yield {"done": True, "sources": sources}
```

- [ ] **Step 3: Add timing to retriever.py**

In `services/chat/app/retriever.py`, add imports:

```python
import time

from app.metrics import QDRANT_SEARCH_DURATION, QDRANT_SEARCH_RESULTS
```

Replace the `search` method body (lines 10-25) with:

```python
    def search(self, query_vector: list[float], top_k: int = 5) -> list[dict]:
        start = time.perf_counter()
        results = self.client.search(
            collection_name=self.collection_name,
            query_vector=query_vector,
            limit=top_k,
        )
        QDRANT_SEARCH_DURATION.labels(
            collection=self.collection_name
        ).observe(time.perf_counter() - start)
        QDRANT_SEARCH_RESULTS.labels(
            collection=self.collection_name
        ).observe(len(results))

        return [
            {
                "text": hit.payload["text"],
                "page_number": hit.payload["page_number"],
                "filename": hit.payload["filename"],
                "document_id": hit.payload["document_id"],
                "score": hit.score,
            }
            for hit in results
        ]
```

- [ ] **Step 4: Run ruff and fix any issues**

```bash
ruff check services/chat/ && ruff format --check services/chat/
```

- [ ] **Step 5: Run tests**

```bash
pytest services/chat/tests/ -v
```

- [ ] **Step 6: Commit**

```bash
git add services/chat/app/main.py services/chat/app/chain.py services/chat/app/retriever.py
git commit -m "feat(chat): wire Prometheus metrics into RAG pipeline

Instrumentator exposes /metrics. Embedding, Qdrant search, and Ollama
generate calls record latency. Token counts captured from streaming
final chunk. RAG pipeline stages (retrieve, build_prompt, generate)
tracked separately.

Co-Authored-By: Claude Opus 4.6 (1M context) <noreply@anthropic.com>"
```

---

## Task 6: Create metrics.py for the debug service

**Files:**
- Create: `services/debug/app/metrics.py`

- [ ] **Step 1: Write the metrics module**

Create `services/debug/app/metrics.py`:

```python
"""Prometheus metrics for the debug service."""

from prometheus_client import Counter, Histogram
from prometheus_fastapi_instrumentator import Instrumentator

SERVICE = "debug"

instrumentator = Instrumentator(
    should_group_status_codes=False,
    excluded_handlers=["/health", "/metrics"],
)

# --- Ollama metrics ---
OLLAMA_REQUEST_DURATION = Histogram(
    "ollama_request_duration_seconds",
    "Wall-clock time for Ollama API calls",
    ["service", "model", "operation"],
    buckets=(0.1, 0.5, 1.0, 2.5, 5.0, 10.0, 30.0, 60.0, 120.0, 300.0),
)

OLLAMA_TOKENS = Counter(
    "ollama_tokens_total",
    "Total tokens processed by Ollama",
    ["service", "model", "kind"],
)

OLLAMA_EVAL_DURATION = Histogram(
    "ollama_eval_duration_seconds",
    "Ollama model evaluation duration (from response metadata)",
    ["service", "model"],
    buckets=(0.1, 0.5, 1.0, 2.5, 5.0, 10.0, 30.0, 60.0, 120.0),
)

EMBEDDING_DURATION = Histogram(
    "embedding_duration_seconds",
    "Time spent calling Ollama /api/embed",
    ["service", "model"],
    buckets=(0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0, 30.0, 60.0, 120.0),
)

# --- Agent loop metrics ---
AGENT_LOOP_ITERATIONS = Histogram(
    "agent_loop_iterations",
    "Number of agent loop iterations per debug session",
    ["service"],
    buckets=(1, 2, 3, 4, 5, 6, 7, 8, 9, 10),
)

AGENT_TOOL_CALLS = Counter(
    "agent_tool_calls_total",
    "Total tool calls made by the debug agent",
    ["tool", "result"],
)

AGENT_TOOL_DURATION = Histogram(
    "agent_tool_duration_seconds",
    "Time spent executing agent tools",
    ["tool"],
    buckets=(0.01, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0, 30.0),
)
```

- [ ] **Step 2: Commit**

```bash
git add services/debug/app/metrics.py
git commit -m "feat(debug): add Prometheus metrics definitions

Ollama request/token/eval metrics, embedding duration, agent loop
iteration histogram, tool call counter and latency histogram.

Co-Authored-By: Claude Opus 4.6 (1M context) <noreply@anthropic.com>"
```

---

## Task 7: Wire instrumentator into debug service and add timing

**Files:**
- Modify: `services/debug/app/main.py`
- Modify: `services/debug/app/agent.py`

- [ ] **Step 1: Wire instrumentator in main.py**

In `services/debug/app/main.py`, add import:

```python
from app.metrics import instrumentator
```

After the CORS middleware block (after line 26), add:

```python
instrumentator.instrument(app).expose(app, include_in_schema=False)
```

- [ ] **Step 2: Add timing to agent.py**

In `services/debug/app/agent.py`, add imports at top:

```python
import time

from app.metrics import (
    AGENT_LOOP_ITERATIONS,
    AGENT_TOOL_CALLS,
    AGENT_TOOL_DURATION,
    OLLAMA_EVAL_DURATION,
    OLLAMA_REQUEST_DURATION,
    OLLAMA_TOKENS,
)
```

In `call_ollama()`, wrap the HTTP call (lines 35-38) with timing and record Ollama response metrics:

Replace the function body with:

```python
async def call_ollama(
    messages: list[dict],
    model: str,
    base_url: str,
    tools: list[dict] | None = None,
) -> dict:
    """POST to Ollama /api/chat and return the parsed JSON response."""
    payload: dict = {
        "model": model,
        "messages": messages,
        "stream": False,
    }
    if tools is not None:
        payload["tools"] = tools

    start = time.perf_counter()
    async with httpx.AsyncClient(timeout=300.0) as client:
        response = await client.post(f"{base_url}/api/chat", json=payload)
        response.raise_for_status()
        data = response.json()
    duration = time.perf_counter() - start

    operation = "chat" if tools else "chat_final"
    OLLAMA_REQUEST_DURATION.labels(
        service="debug", model=model, operation=operation
    ).observe(duration)

    # Record token metrics from response
    prompt_tokens = data.get("prompt_eval_count", 0)
    completion_tokens = data.get("eval_count", 0)
    if prompt_tokens:
        OLLAMA_TOKENS.labels(service="debug", model=model, kind="prompt").inc(
            prompt_tokens
        )
    if completion_tokens:
        OLLAMA_TOKENS.labels(service="debug", model=model, kind="completion").inc(
            completion_tokens
        )
    eval_ns = data.get("eval_duration", 0)
    if eval_ns:
        OLLAMA_EVAL_DURATION.labels(service="debug", model=model).observe(
            eval_ns / 1e9
        )

    return data
```

In `run_agent_loop()`, add tool timing around `execute_tool` (around line 132). Replace lines 132-141:

```python
        # Execute the tool
        tool_start = time.perf_counter()
        result: str = await execute_tool(
            tool_name=tool_name,
            arguments=arguments,
            project_path=project_path,
            collection=collection,
            ollama_base_url=ollama_base_url,
            embedding_model=embedding_model,
            qdrant_host=qdrant_host,
            qdrant_port=qdrant_port,
        )
        tool_duration = time.perf_counter() - tool_start
        AGENT_TOOL_DURATION.labels(tool=tool_name).observe(tool_duration)
        AGENT_TOOL_CALLS.labels(tool=tool_name, result="success").inc()
```

At the end of `run_agent_loop()`, before `yield {"event": "done", ...}` in both the normal exit (line 95-96) and the max-steps exit (line 198-199), record the loop iteration count. The simplest approach: add a counter variable at the top of the loop and record at each exit point.

After `for step in range(1, max_steps + 1):` (line 71), the `step` variable already tracks this. At each `yield {"event": "done", ...}` line, add before it:

```python
        AGENT_LOOP_ITERATIONS.labels(service="debug").observe(step)
```

There are three exit points:
1. Line 85-86 (error during Ollama call) — add `AGENT_LOOP_ITERATIONS.labels(service="debug").observe(step)` before the yield
2. Line 95-96 (normal diagnosis) — add same
3. Line 198-199 (max steps exhausted) — add `AGENT_LOOP_ITERATIONS.labels(service="debug").observe(max_steps)` before the yield

- [ ] **Step 3: Run ruff and fix any issues**

```bash
ruff check services/debug/ && ruff format --check services/debug/
```

- [ ] **Step 4: Run tests**

```bash
pytest services/debug/tests/ -v
```

- [ ] **Step 5: Commit**

```bash
git add services/debug/app/main.py services/debug/app/agent.py
git commit -m "feat(debug): wire Prometheus metrics into agent loop

Instrumentator exposes /metrics. Ollama calls record latency, tokens,
eval duration. Tool executions record per-tool latency and call counts.
Agent loop iterations tracked per session.

Co-Authored-By: Claude Opus 4.6 (1M context) <noreply@anthropic.com>"
```

---

## Task 8: Add metrics endpoint tests

**Files:**
- Create: `services/ingestion/tests/test_metrics.py`
- Create: `services/chat/tests/test_metrics.py`
- Create: `services/debug/tests/test_metrics.py`

- [ ] **Step 1: Write ingestion metrics test**

Create `services/ingestion/tests/test_metrics.py`:

```python
from app.main import app
from fastapi.testclient import TestClient

client = TestClient(app)


def test_metrics_endpoint_returns_200():
    response = client.get("/metrics")
    assert response.status_code == 200
    assert "text/plain" in response.headers["content-type"] or "text/plain" in response.headers.get("content-type", "")


def test_metrics_contains_http_request_metrics():
    # Hit an endpoint first to generate metrics
    client.get("/health")
    response = client.get("/metrics")
    body = response.text
    # instrumentator should NOT include /health (excluded), but process metrics should exist
    assert "python_info" in body or "process_" in body


def test_metrics_contains_custom_metrics():
    response = client.get("/metrics")
    body = response.text
    # Custom metrics should be registered (may have 0 observations)
    assert "embedding_duration_seconds" in body
    assert "qdrant_operation_duration_seconds" in body
    assert "ingestion_chunks_created_total" in body
```

- [ ] **Step 2: Write chat metrics test**

Create `services/chat/tests/test_metrics.py`:

```python
from app.main import app
from fastapi.testclient import TestClient

client = TestClient(app)


def test_metrics_endpoint_returns_200():
    response = client.get("/metrics")
    assert response.status_code == 200


def test_metrics_contains_custom_metrics():
    response = client.get("/metrics")
    body = response.text
    assert "ollama_request_duration_seconds" in body
    assert "ollama_tokens_total" in body
    assert "embedding_duration_seconds" in body
    assert "qdrant_search_duration_seconds" in body
    assert "rag_pipeline_duration_seconds" in body
```

- [ ] **Step 3: Write debug metrics test**

Create `services/debug/tests/test_metrics.py`:

```python
from app.main import app
from fastapi.testclient import TestClient

client = TestClient(app)


def test_metrics_endpoint_returns_200():
    response = client.get("/metrics")
    assert response.status_code == 200


def test_metrics_contains_custom_metrics():
    response = client.get("/metrics")
    body = response.text
    assert "ollama_request_duration_seconds" in body
    assert "agent_loop_iterations" in body
    assert "agent_tool_calls_total" in body
    assert "agent_tool_duration_seconds" in body
```

- [ ] **Step 4: Run all tests**

```bash
pytest services/ingestion/tests/ services/chat/tests/ services/debug/tests/ -v
```

Expected: all pass.

- [ ] **Step 5: Commit**

```bash
git add services/ingestion/tests/test_metrics.py services/chat/tests/test_metrics.py services/debug/tests/test_metrics.py
git commit -m "test(python): add /metrics endpoint tests for all services

Verify /metrics returns 200, contains process metrics, and exposes
all custom metric names.

Co-Authored-By: Claude Opus 4.6 (1M context) <noreply@anthropic.com>"
```

---

## Task 9: Add Prometheus pod annotations to k8s deployments

**Files:**
- Modify: `k8s/ai-services/deployments/ingestion.yml`
- Modify: `k8s/ai-services/deployments/chat.yml`
- Modify: `k8s/ai-services/deployments/debug.yml`

- [ ] **Step 1: Add annotations to all three deployments**

In each file, add annotations under `spec.template.metadata` (after `labels:`):

For `k8s/ai-services/deployments/ingestion.yml`, `chat.yml`, and `debug.yml`, add:

```yaml
      annotations:
        prometheus.io/scrape: "true"
        prometheus.io/port: "8000"
        prometheus.io/path: "/metrics"
```

This goes under `spec.template.metadata`, alongside the existing `labels:` block.

For example, `chat.yml` becomes:

```yaml
  template:
    metadata:
      labels:
        app: chat
      annotations:
        prometheus.io/scrape: "true"
        prometheus.io/port: "8000"
        prometheus.io/path: "/metrics"
```

Apply the same pattern to `ingestion.yml` and `debug.yml`.

- [ ] **Step 2: Commit**

```bash
git add k8s/ai-services/deployments/ingestion.yml k8s/ai-services/deployments/chat.yml k8s/ai-services/deployments/debug.yml
git commit -m "feat(k8s): add Prometheus scrape annotations to Python service pods

Kubernetes SD will discover these pods and scrape /metrics on :8000.

Co-Authored-By: Claude Opus 4.6 (1M context) <noreply@anthropic.com>"
```

---

## Task 10: Run full preflight and verify

- [ ] **Step 1: Run preflight-python**

```bash
make preflight-python
```

Expected: ruff lint + format pass, all three pytest suites pass.

- [ ] **Step 2: Run preflight-security**

```bash
make preflight-security
```

Expected: pass (prometheus-fastapi-instrumentator has no known CVEs).

- [ ] **Step 3: Verify /metrics locally**

Start the ingestion service locally (requires qdrant + ollama, but we can test without them):

```bash
cd services/ingestion && python -c "
from app.main import app
from fastapi.testclient import TestClient
c = TestClient(app)
r = c.get('/metrics')
print('Status:', r.status_code)
for line in r.text.split('\n')[:5]:
    print(line)
"
```

Expected: 200, Prometheus exposition format with metric names visible.

---

## Acceptance criteria

1. All three services expose `/metrics` returning 200 with Prometheus exposition format.
2. `make preflight-python` passes (ruff + all tests green).
3. Custom metric names present in `/metrics` output: `embedding_duration_seconds`, `ollama_request_duration_seconds`, `ollama_tokens_total`, `qdrant_search_duration_seconds`, `rag_pipeline_duration_seconds`, `agent_loop_iterations`, `agent_tool_calls_total`.
4. k8s deployment YAMLs have `prometheus.io/scrape: "true"` annotations.
5. No changes to `/health` endpoint behavior — existing health checks still work.

---

## Not in this stage

- Updating `docker-compose.yml` / `monitoring/prometheus.yml` for compose parity — the compose stack still uses the old static scrape config. The `k8s-pods` SD job was added in Stage B's k8s config. Compose parity will be addressed when the compose-smoke CI job is updated (can be a follow-up or part of Stage D).
- Deploying to Minikube — requires rebuilding Docker images with the new `prometheus-fastapi-instrumentator` dependency. Kyle will push, CI builds the images, then redeploy. The k8s annotations take effect on the next pod rollout.
