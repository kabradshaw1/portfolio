# Eval Per-Run Retrieval Config Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a typed per-run `retrieval_config.top_k` override that flows from MCP to eval to chat and is captured in eval run metadata.

**Architecture:** `retrieval_config` is an optional typed object with one v1 field, `top_k`. The eval API resolves one effective final context budget per run, uses it for both `/search.limit` and `/chat.retrieval_config.top_k`, and records requested plus effective retrieval config in metadata. Chat remains backwards compatible by falling back to `settings.top_k` when no override is supplied.

**Tech Stack:** Python/FastAPI/Pydantic/httpx/pytest for `services/chat` and `services/eval`; Go MCP service with standard `encoding/json`, `net/http`, and `testing`.

---

## Execution Setup

This is runtime behavior work. Do not implement from `main`.

- [ ] **Step 1: Create a feature worktree**

Run from repo root:

```bash
git branch --show-current
git worktree add .codex/worktrees/eval-per-run-retrieval-config -b feat/eval-per-run-retrieval-config
cd .codex/worktrees/eval-per-run-retrieval-config
pwd
git branch --show-current
git rev-parse --show-toplevel
```

Expected:

```text
main
/Users/kylebradshaw/repos/gen_ai_engineer/.codex/worktrees/eval-per-run-retrieval-config
feat/eval-per-run-retrieval-config
/Users/kylebradshaw/repos/gen_ai_engineer/.codex/worktrees/eval-per-run-retrieval-config
```

- [ ] **Step 2: Confirm scoped instructions in the worktree**

Run:

```bash
sed -n '1,220p' AGENTS.md
sed -n '1,220p' services/AGENTS.md
sed -n '1,220p' go/AGENTS.md
```

Expected: instructions load successfully. Follow them for all subsequent reads,
edits, tests, commits, and pushes.

## File Structure

Modify:

- `services/chat/app/main.py`: define chat-side `RetrievalConfig`, accept it on `ChatRequest`, resolve effective top-k, thread it into `rag_query`.
- `services/chat/app/chain.py`: add final context budget to retrieval metadata returned by `retrieve_chunks`.
- `services/chat/tests/test_main.py`: request validation and top-k threading tests.
- `services/chat/tests/test_chain.py`: retrieval metadata test for final top-k.
- `services/eval/app/models.py`: define eval-side `RetrievalConfig`, add it to `StartEvaluationRequest`, reject unknown fields.
- `services/eval/app/rag_client.py`: forward `retrieval_config` to `/chat`.
- `services/eval/app/config_capture.py`: accept requested/effective retrieval config and persist both in captured metadata.
- `services/eval/app/evaluator.py`: accept `top_k`, use it for `/search.limit` and `/chat.retrieval_config.top_k`.
- `services/eval/app/main.py`: capture config, resolve effective top-k, pass effective config to evaluation.
- `services/eval/tests/test_models.py`: model validation tests.
- `services/eval/tests/test_rag_client.py`: request forwarding tests.
- `services/eval/tests/test_evaluator.py`: aligned `/search` and `/chat` top-k tests.
- `services/eval/tests/test_config_capture.py`: metadata capture tests.
- `services/eval/tests/test_main.py`: endpoint/background task tests.
- `go/eval-mcp-service/internal/evalapi/client.go`: typed `RetrievalConfig` on `StartEvaluationRequest`.
- `go/eval-mcp-service/internal/evalworkflow/service.go`: typed `RetrievalConfig` on `StartRunInput`, forward to eval API.
- `go/eval-mcp-service/internal/mcpserver/server.go`: MCP handler decode and JSON schema for `retrieval_config`.
- `go/eval-mcp-service/internal/evalapi/client_test.go`: serialization and retry preservation tests.
- `go/eval-mcp-service/internal/evalworkflow/service_test.go`: workflow forwarding test.
- `go/eval-mcp-service/internal/mcpserver/server_test.go`: MCP schema/handler forwarding tests.

Do not modify `k8s/ai-services/configmaps/chat-config.yml`; runtime `TOP_K`
changes are not part of this solution.

## Task 1: Chat API Top-K Override

**Files:**
- Modify: `services/chat/app/main.py`
- Modify: `services/chat/app/chain.py`
- Test: `services/chat/tests/test_main.py`
- Test: `services/chat/tests/test_chain.py`

- [ ] **Step 1: Add failing chat request tests**

Append these tests to `services/chat/tests/test_main.py` near
`test_chat_threads_settings_top_k_into_rag_query`:

```python
@patch("app.main.rag_query")
def test_chat_threads_retrieval_config_top_k_into_rag_query(mock_rag_query):
    captured = {}

    async def fake(**kwargs):
        captured.update(kwargs)
        yield {"done": True, "sources": [], "retrieval": {}}

    mock_rag_query.side_effect = fake

    response = client.post(
        "/chat",
        json={"question": "hi", "retrieval_config": {"top_k": 3}},
        headers={"Accept": "application/json"},
    )

    assert response.status_code == 200
    assert captured["top_k"] == 3


@patch("app.main.rag_query")
def test_chat_stream_threads_retrieval_config_top_k_into_rag_query(mock_rag_query):
    captured = {}

    async def fake(**kwargs):
        captured.update(kwargs)
        yield {"done": True, "sources": [], "retrieval": {}}

    mock_rag_query.side_effect = fake

    response = client.post(
        "/chat",
        json={"question": "hi", "retrieval_config": {"top_k": 4}},
    )

    assert response.status_code == 200
    assert captured["top_k"] == 4


def test_chat_rejects_invalid_retrieval_config_top_k():
    response = client.post(
        "/chat",
        json={"question": "hi", "retrieval_config": {"top_k": 0}},
        headers={"Accept": "application/json"},
    )

    assert response.status_code == 422


def test_chat_rejects_unknown_retrieval_config_fields():
    response = client.post(
        "/chat",
        json={"question": "hi", "retrieval_config": {"top_k": 3, "score": 0.8}},
        headers={"Accept": "application/json"},
    )

    assert response.status_code == 422
```

Append this test to `services/chat/tests/test_chain.py` near other
`retrieve_chunks` metadata tests:

```python
@pytest.mark.asyncio
@patch("app.chain.QdrantRetriever")
@patch("app.chain.get_embedding")
async def test_retrieve_chunks_metadata_includes_final_top_k(
    mock_get_embedding, mock_retriever_cls
):
    mock_get_embedding.return_value = [0.1] * 768
    retriever = mock_retriever_cls.return_value
    retriever.search_hybrid.return_value = RetrievalResult(
        chunks=[],
        metadata={
            "retrieval_mode": "hybrid",
            "retrieval_fallback": False,
            "fusion": "rrf",
        },
    )

    result = await retrieve_chunks(
        question="hi",
        embedding_provider=AsyncMock(),
        embedding_model="nomic-embed-text",
        qdrant_host="qdrant",
        qdrant_port=6333,
        collection_name="documents",
        top_k=3,
        rerank=False,
    )

    assert result.metadata["top_k"] == 3
```

- [ ] **Step 2: Run chat tests to verify failure**

Run:

```bash
pytest services/chat/tests/test_main.py::test_chat_threads_retrieval_config_top_k_into_rag_query services/chat/tests/test_main.py::test_chat_stream_threads_retrieval_config_top_k_into_rag_query services/chat/tests/test_main.py::test_chat_rejects_invalid_retrieval_config_top_k services/chat/tests/test_main.py::test_chat_rejects_unknown_retrieval_config_fields services/chat/tests/test_chain.py::test_retrieve_chunks_metadata_includes_final_top_k -q
```

Expected: at least the top-k threading tests fail because `ChatRequest` does
not yet accept or use `retrieval_config`.

- [ ] **Step 3: Implement chat request model and top-k resolution**

In `services/chat/app/main.py`, update imports and models:

```python
from pydantic import BaseModel, ConfigDict, Field


class RetrievalConfig(BaseModel):
    model_config = ConfigDict(extra="forbid")

    top_k: int | None = Field(default=None, ge=1, le=20)
```

Update `ChatRequest`:

```python
class ChatRequest(BaseModel):
    question: str = Field(max_length=2000)
    collection: str | None = Field(default=None, pattern=r"^[a-zA-Z0-9_-]{1,100}$")
    rerank: bool = False
    retrieval_config: RetrievalConfig | None = None
```

Add helper near the request models:

```python
def _effective_top_k(config: RetrievalConfig | None) -> int:
    if config is not None and config.top_k is not None:
        return config.top_k
    return settings.top_k
```

In both `/chat` branches, compute once and pass it to `rag_query`:

```python
effective_top_k = _effective_top_k(body.retrieval_config)
```

Replace:

```python
top_k=settings.top_k,
```

with:

```python
top_k=effective_top_k,
```

- [ ] **Step 4: Implement chat retrieval metadata**

In `services/chat/app/chain.py`, add `top_k` to returned metadata before
rerank-specific metadata branches can return:

```python
result = _with_rerank_metadata(
    result,
    {
        "top_k": top_k,
    },
)
```

Place it immediately after the initial retrieval result is created and before
the `if rerank and not settings.rerank_enabled:` branch. Preserve all existing
rerank metadata keys.

- [ ] **Step 5: Run chat tests to verify pass**

Run:

```bash
pytest services/chat/tests/test_main.py services/chat/tests/test_chain.py -q
```

Expected: all selected chat tests pass.

- [ ] **Step 6: Commit chat slice**

Run:

```bash
git add services/chat/app/main.py services/chat/app/chain.py services/chat/tests/test_main.py services/chat/tests/test_chain.py
git commit -m "feat: support chat retrieval top-k override"
```

Expected: commit succeeds.

## Task 2: Eval API, Forwarding, and Metadata

**Files:**
- Modify: `services/eval/app/models.py`
- Modify: `services/eval/app/rag_client.py`
- Modify: `services/eval/app/evaluator.py`
- Modify: `services/eval/app/config_capture.py`
- Modify: `services/eval/app/main.py`
- Test: `services/eval/tests/test_models.py`
- Test: `services/eval/tests/test_rag_client.py`
- Test: `services/eval/tests/test_evaluator.py`
- Test: `services/eval/tests/test_config_capture.py`
- Test: `services/eval/tests/test_main.py`

- [ ] **Step 1: Add failing eval model tests**

Append to `services/eval/tests/test_models.py` near existing start request
tests:

```python
def test_start_request_accepts_retrieval_config_top_k():
    req = StartEvaluationRequest(
        dataset_id="ds-1",
        retrieval_config={"top_k": 3},
    )

    assert req.retrieval_config is not None
    assert req.retrieval_config.top_k == 3


def test_start_request_rejects_invalid_retrieval_config_top_k():
    with pytest.raises(ValidationError):
        StartEvaluationRequest(dataset_id="ds-1", retrieval_config={"top_k": 0})


def test_start_request_rejects_unknown_retrieval_config_fields():
    with pytest.raises(ValidationError):
        StartEvaluationRequest(
            dataset_id="ds-1",
            retrieval_config={"top_k": 3, "score_threshold": 0.8},
        )
```

- [ ] **Step 2: Run eval model tests to verify failure**

Run:

```bash
pytest services/eval/tests/test_models.py::test_start_request_accepts_retrieval_config_top_k services/eval/tests/test_models.py::test_start_request_rejects_invalid_retrieval_config_top_k services/eval/tests/test_models.py::test_start_request_rejects_unknown_retrieval_config_fields -q
```

Expected: tests fail because `retrieval_config` is not modeled yet.

- [ ] **Step 3: Implement eval request model**

In `services/eval/app/models.py`, update the Pydantic imports:

```python
from pydantic import BaseModel, ConfigDict, Field
```

Add near `StartEvaluationRequest`:

```python
class RetrievalConfig(BaseModel):
    model_config = ConfigDict(extra="forbid")

    top_k: int | None = Field(default=None, ge=1, le=20)
```

Update `StartEvaluationRequest`:

```python
class StartEvaluationRequest(BaseModel):
    dataset_id: str
    collection: str | None = Field(default=None, pattern=r"^[a-zA-Z0-9_-]{1,100}$")
    notes: str | None = Field(default=None, max_length=500)
    baseline_eval_id: str | None = None
    rerank: bool = False
    experiment_id: str | None = None
    experiment_label: str | None = Field(
        default=None, min_length=1, max_length=100, pattern=r"^[a-zA-Z0-9_-]+$"
    )
    retrieval_config: RetrievalConfig | None = None
```

- [ ] **Step 4: Run eval model tests to verify pass**

Run:

```bash
pytest services/eval/tests/test_models.py::test_start_request_accepts_retrieval_config_top_k services/eval/tests/test_models.py::test_start_request_rejects_invalid_retrieval_config_top_k services/eval/tests/test_models.py::test_start_request_rejects_unknown_retrieval_config_fields -q
```

Expected: tests pass.

- [ ] **Step 5: Add failing RAG client forwarding test**

Append to `services/eval/tests/test_rag_client.py`:

```python
@pytest.mark.asyncio
async def test_ask_forwards_retrieval_config(mock_chat_response):
    async def mock_handler(request: httpx.Request) -> httpx.Response:
        assert request.url.path == "/chat"
        body = json.loads(request.content)
        assert body["retrieval_config"] == {"top_k": 3}
        return httpx.Response(200, json=mock_chat_response)

    transport = httpx.MockTransport(mock_handler)
    client = RAGClient(base_url="http://chat:8000", transport=transport)

    response = await client.ask(
        "what is kubernetes",
        collection=None,
        rerank=False,
        retrieval_config={"top_k": 3},
    )

    assert response["answer"] == mock_chat_response["answer"]
```

- [ ] **Step 6: Run RAG client test to verify failure**

Run:

```bash
pytest services/eval/tests/test_rag_client.py::test_ask_forwards_retrieval_config -q
```

Expected: fails because `RAGClient.ask()` does not accept `retrieval_config`.

- [ ] **Step 7: Implement RAG client forwarding**

In `services/eval/app/rag_client.py`, update `ask`:

```python
async def ask(
    self,
    question: str,
    collection: str | None,
    rerank: bool = False,
    retrieval_config: dict | None = None,
) -> dict:
    """Call POST /chat with Accept: application/json for a full RAG response."""
    body: dict = {"question": question, "rerank": rerank}
    if collection:
        body["collection"] = collection
    if retrieval_config:
        body["retrieval_config"] = retrieval_config

    resp = await self._client.post(
        "/chat", json=body, headers={"Accept": "application/json"}
    )
    resp.raise_for_status()
    return resp.json()
```

- [ ] **Step 8: Add failing evaluator alignment test**

Append to `services/eval/tests/test_evaluator.py`:

```python
@pytest.mark.asyncio
async def test_build_evaluation_dataset_uses_effective_top_k_for_search_and_chat(
    golden_items, mock_search_results, mock_chat_answer
):
    rag_client = AsyncMock()
    rag_client.search.return_value = mock_search_results
    rag_client.ask.return_value = mock_chat_answer

    await build_evaluation_dataset(
        items=golden_items,
        rag_client=rag_client,
        collection="documents",
        rerank=False,
        top_k=3,
    )

    assert rag_client.search.call_args_list[0].kwargs["limit"] == 3
    assert rag_client.ask.call_args_list[0].kwargs["retrieval_config"] == {"top_k": 3}
```

- [ ] **Step 9: Run evaluator alignment test to verify failure**

Run:

```bash
pytest services/eval/tests/test_evaluator.py::test_build_evaluation_dataset_uses_effective_top_k_for_search_and_chat -q
```

Expected: fails because `build_evaluation_dataset()` does not accept `top_k`.

- [ ] **Step 10: Implement evaluator top-k plumbing**

In `services/eval/app/evaluator.py`, update signatures:

```python
async def build_evaluation_dataset(
    items: list[dict],
    rag_client: RAGClient,
    collection: str | None,
    rerank: bool = False,
    top_k: int = 5,
) -> list[dict]:
```

Replace the hardcoded search limit and ask call:

```python
search_results = await rag_client.search(
    query, collection=collection, limit=top_k, rerank=rerank
)
chat_response = await rag_client.ask(
    query,
    collection=collection,
    rerank=rerank,
    retrieval_config={"top_k": top_k},
)
```

Update `run_evaluation`:

```python
async def run_evaluation(
    items: list[dict],
    rag_client: RAGClient,
    collection: str | None,
    llm_provider: str,
    llm_base_url: str,
    llm_model: str,
    llm_api_key: str,
    rerank: bool = False,
    top_k: int = 5,
    judge: JudgeFn | None = None,
) -> tuple[dict, list[dict]]:
    """Run a full first-party RAG evaluation."""
    raw_dataset = await build_evaluation_dataset(
        items, rag_client, collection, rerank=rerank, top_k=top_k
    )
```

- [ ] **Step 11: Add failing config capture tests**

Append to `services/eval/tests/test_config_capture.py`:

```python
@pytest.mark.asyncio
@respx.mock
async def test_capture_records_requested_and_effective_retrieval_config():
    respx.get("http://chat/config").mock(
        return_value=httpx.Response(200, json={"top_k": 5})
    )
    respx.get("http://ingestion/collections/documents/config").mock(
        return_value=httpx.Response(200, json={"chunk_size": 1000})
    )

    cfg = await capture_run_config(
        chat_url="http://chat",
        ingestion_url="http://ingestion",
        collection="documents",
        requested_rerank=False,
        requested_retrieval_config={"top_k": 3},
    )

    assert cfg["requested_retrieval_config"] == {"top_k": 3}
    assert cfg["effective_retrieval_config"] == {"top_k": 3}
    assert cfg["chat"]["top_k"] == 5


@pytest.mark.asyncio
@respx.mock
async def test_capture_records_empty_requested_retrieval_config_for_default_run():
    respx.get("http://chat/config").mock(
        return_value=httpx.Response(200, json={"top_k": 5})
    )
    respx.get("http://ingestion/collections/documents/config").mock(
        return_value=httpx.Response(200, json={"chunk_size": 1000})
    )

    cfg = await capture_run_config(
        chat_url="http://chat",
        ingestion_url="http://ingestion",
        collection="documents",
        requested_rerank=False,
        requested_retrieval_config={},
    )

    assert cfg["requested_retrieval_config"] == {}
    assert cfg["effective_retrieval_config"] == {"top_k": 5}
```

- [ ] **Step 12: Run config capture tests to verify failure**

Run:

```bash
pytest services/eval/tests/test_config_capture.py::test_capture_records_requested_and_effective_retrieval_config services/eval/tests/test_config_capture.py::test_capture_records_empty_requested_retrieval_config_for_default_run -q
```

Expected: fails because `capture_run_config()` does not accept retrieval config
arguments.

- [ ] **Step 13: Implement config capture metadata**

In `services/eval/app/config_capture.py`, update the signature:

```python
async def capture_run_config(
    chat_url: str,
    ingestion_url: str,
    collection: str,
    requested_rerank: bool,
    requested_retrieval_config: dict | None = None,
) -> dict:
```

Add a helper above `capture_run_config`:

```python
def _effective_retrieval_config(
    requested_retrieval_config: dict | None, chat_config: dict | None
) -> dict:
    requested_top_k = (requested_retrieval_config or {}).get("top_k")
    if isinstance(requested_top_k, int):
        return {"top_k": requested_top_k}
    chat_top_k = (chat_config or {}).get("top_k")
    if isinstance(chat_top_k, int):
        return {"top_k": chat_top_k}
    return {"top_k": 5}
```

After the chat and collection responses are processed, set the retrieval config
metadata:

```python
out: dict = {
    "captured_at": captured_at,
    "effective_collection": collection,
    "requested_rerank": requested_rerank,
    "requested_retrieval_config": requested_retrieval_config or {},
}
```

Then, after `out["chat"] = chat_res` when chat capture succeeds and before the
final return:

```python
chat_config = out.get("chat") if isinstance(out.get("chat"), dict) else None
out["effective_retrieval_config"] = _effective_retrieval_config(
    requested_retrieval_config, chat_config
)
```

Keep existing error handling unchanged.

- [ ] **Step 14: Add failing eval main test**

Append to `services/eval/tests/test_main.py` near
`test_start_evaluation_passes_rerank_to_background_run`:

```python
@patch("app.main.run_evaluation", new_callable=AsyncMock)
@patch("app.main.capture_run_config", new_callable=AsyncMock)
@patch("app.main.validate_collection_exists", new_callable=AsyncMock)
@patch("app.main.get_db")
def test_start_evaluation_passes_retrieval_config_to_background_run(
    mock_get_db, mock_validate_collection, mock_capture, mock_run_evaluation
):
    mock_db = AsyncMock()
    mock_db.get_dataset.return_value = {
        "id": "ds-top-k",
        "name": "test",
        "items": [{"query": "q", "expected_answer": "a", "expected_sources": []}],
        "created_at": "2026-04-16T00:00:00Z",
    }
    mock_db.create_evaluation.return_value = "eval-top-k"
    mock_get_db.return_value = mock_db
    mock_capture.return_value = {
        "captured_at": "x",
        "chat": {"top_k": 5},
        "effective_retrieval_config": {"top_k": 3},
    }
    mock_run_evaluation.return_value = ({"faithfulness": 0.8}, [])

    response = client.post(
        "/evaluations",
        json={"dataset_id": "ds-top-k", "retrieval_config": {"top_k": 3}},
    )

    assert response.status_code == 202
    assert mock_capture.await_args.kwargs["requested_retrieval_config"] == {"top_k": 3}
    assert mock_run_evaluation.await_args.kwargs["top_k"] == 3
```

- [ ] **Step 15: Run eval main test to verify failure**

Run:

```bash
pytest services/eval/tests/test_main.py::test_start_evaluation_passes_retrieval_config_to_background_run -q
```

Expected: fails because `start_evaluation` and `_run_evaluation_task` do not
carry `retrieval_config`.

- [ ] **Step 16: Implement eval main effective top-k resolution**

In `services/eval/app/main.py`, import `RetrievalConfig` from `app.models`.

Add helpers near `_run_evaluation_task`:

```python
def _requested_retrieval_config(config: RetrievalConfig | None) -> dict:
    if config is None:
        return {}
    return config.model_dump(exclude_none=True)


def _effective_top_k(config: RetrievalConfig | None, captured_config: dict) -> int:
    if config is not None and config.top_k is not None:
        return config.top_k
    effective_top_k = captured_config.get("effective_retrieval_config", {}).get("top_k")
    if isinstance(effective_top_k, int):
        return effective_top_k
    return 5
```

Update `_run_evaluation_task` signature:

```python
async def _run_evaluation_task(
    eval_id: str,
    items: list[dict],
    collection: str | None,
    rerank: bool = False,
    retrieval_config: RetrievalConfig | None = None,
):
```

Inside `_run_evaluation_task`, replace the capture/evaluation block with:

```python
requested_retrieval_config = _requested_retrieval_config(retrieval_config)
config = await capture_run_config(
    chat_url=settings.chat_service_url,
    ingestion_url=settings.ingestion_service_url,
    collection=coll_name,
    requested_rerank=rerank,
    requested_retrieval_config=requested_retrieval_config,
)
effective_top_k = _effective_top_k(retrieval_config, config)
await db.set_evaluation_config(eval_id, config)

aggregate, results = await run_evaluation(
    items=items,
    rag_client=rag_client,
    collection=collection,
    llm_provider=settings.llm_provider,
    llm_base_url=settings.llm_base_url,
    llm_model=settings.llm_model,
    llm_api_key=settings.llm_api_key,
    rerank=rerank,
    top_k=effective_top_k,
)
```

Update the background task call:

```python
background_tasks.add_task(
    _run_evaluation_task,
    eval_id,
    dataset["items"],
    collection,
    body.rerank,
    body.retrieval_config,
)
```

- [ ] **Step 17: Run eval tests to verify pass**

Run:

```bash
pytest services/eval/tests/test_models.py services/eval/tests/test_rag_client.py services/eval/tests/test_evaluator.py services/eval/tests/test_config_capture.py services/eval/tests/test_main.py -q
```

Expected: selected eval tests pass.

- [ ] **Step 18: Commit eval slice**

Run:

```bash
git add services/eval/app/models.py services/eval/app/rag_client.py services/eval/app/evaluator.py services/eval/app/config_capture.py services/eval/app/main.py services/eval/tests/test_models.py services/eval/tests/test_rag_client.py services/eval/tests/test_evaluator.py services/eval/tests/test_config_capture.py services/eval/tests/test_main.py
git commit -m "feat: support eval per-run retrieval config"
```

Expected: commit succeeds.

## Task 3: Go MCP and Eval API Plumbing

**Files:**
- Modify: `go/eval-mcp-service/internal/evalapi/client.go`
- Modify: `go/eval-mcp-service/internal/evalworkflow/service.go`
- Modify: `go/eval-mcp-service/internal/mcpserver/server.go`
- Test: `go/eval-mcp-service/internal/evalapi/client_test.go`
- Test: `go/eval-mcp-service/internal/evalworkflow/service_test.go`
- Test: `go/eval-mcp-service/internal/mcpserver/server_test.go`

- [ ] **Step 1: Add failing Go eval API serialization test**

In `go/eval-mcp-service/internal/evalapi/client_test.go`, update
`TestStartEvaluationSendsOptionalFields` body assertion:

```go
if body.DatasetID != "ds-1" || body.Collection != "documents" || body.Notes != "candidate" || body.BaselineEvalID != "eval-base" || body.ExperimentID != "exp-1" || body.ExperimentLabel != "candidate" || !body.Rerank || body.RetrievalConfig == nil || body.RetrievalConfig.TopK == nil || *body.RetrievalConfig.TopK != 3 {
	t.Fatalf("body = %#v", body)
}
```

Update the request in that test:

```go
topK := 3
got, err := client.StartEvaluation(context.Background(), StartEvaluationRequest{
	DatasetID:       "ds-1",
	Collection:      "documents",
	Notes:           "candidate",
	BaselineEvalID:  "eval-base",
	ExperimentID:    "exp-1",
	ExperimentLabel: "candidate",
	Rerank:          true,
	RetrievalConfig: &RetrievalConfig{TopK: &topK},
})
```

In `TestStartEvaluationRetriesUnauthorizedWithOriginalBody`, add:

```go
topK := 3
wantBody := StartEvaluationRequest{
	DatasetID:       "ds-1",
	Collection:      "documents",
	Notes:           "candidate",
	ExperimentID:    "exp-1",
	ExperimentLabel: "candidate",
	Rerank:          true,
	RetrievalConfig: &RetrievalConfig{TopK: &topK},
}
```

Replace the struct equality check in that test with field assertions that
handle pointer fields:

```go
if gotBody.DatasetID != wantBody.DatasetID || gotBody.Collection != wantBody.Collection || gotBody.Notes != wantBody.Notes || gotBody.BaselineEvalID != wantBody.BaselineEvalID || gotBody.Rerank != wantBody.Rerank {
	t.Fatalf("body request %d = %#v", requests, gotBody)
}
if gotBody.RetrievalConfig == nil || gotBody.RetrievalConfig.TopK == nil || *gotBody.RetrievalConfig.TopK != topK {
	t.Fatalf("body retrieval config request %d = %#v", requests, gotBody.RetrievalConfig)
}
```

- [ ] **Step 2: Run Go eval API tests to verify failure**

Run:

```bash
go test ./go/eval-mcp-service/internal/evalapi -run 'TestStartEvaluationSendsOptionalFields|TestStartEvaluationRetriesUnauthorizedWithOriginalBody' -count=1
```

Expected: fails because `RetrievalConfig` is not defined.

- [ ] **Step 3: Implement Go eval API type**

In `go/eval-mcp-service/internal/evalapi/client.go`, add:

```go
type RetrievalConfig struct {
	TopK *int `json:"top_k,omitempty"`
}
```

Update `StartEvaluationRequest`:

```go
type StartEvaluationRequest struct {
	DatasetID        string           `json:"dataset_id"`
	Collection       string           `json:"collection,omitempty"`
	Notes            string           `json:"notes,omitempty"`
	BaselineEvalID   string           `json:"baseline_eval_id,omitempty"`
	ExperimentID     string           `json:"experiment_id,omitempty"`
	ExperimentLabel  string           `json:"experiment_label,omitempty"`
	Rerank           bool             `json:"rerank"`
	RetrievalConfig  *RetrievalConfig `json:"retrieval_config,omitempty"`
}
```

- [ ] **Step 4: Add failing workflow forwarding test**

In `go/eval-mcp-service/internal/evalworkflow/service_test.go`, update
`TestStartRunSendsExperimentAttachmentToEvalAPI`:

```go
topK := 3
got, err := svc.StartRun(ctx, StartRunInput{
	DatasetID:       "dataset-1",
	Collection:      "kb",
	Notes:           "candidate notes",
	ExperimentID:    "exp-7",
	Label:           "candidate",
	BaselineEvalID:  "eval-base",
	Rerank:          true,
	RetrievalConfig: &evalapi.RetrievalConfig{TopK: &topK},
})
```

Extend assertions:

```go
if req.RetrievalConfig == nil || req.RetrievalConfig.TopK == nil || *req.RetrievalConfig.TopK != 3 {
	t.Fatalf("StartEvaluation retrieval config = %#v", req.RetrievalConfig)
}
```

- [ ] **Step 5: Run workflow test to verify failure**

Run:

```bash
go test ./go/eval-mcp-service/internal/evalworkflow -run TestStartRunSendsExperimentAttachmentToEvalAPI -count=1
```

Expected: fails because `StartRunInput` does not include `RetrievalConfig`.

- [ ] **Step 6: Implement workflow forwarding**

In `go/eval-mcp-service/internal/evalworkflow/service.go`, update
`StartRunInput`:

```go
type StartRunInput struct {
	DatasetID       string
	Collection      string
	Notes           string
	BaselineEvalID  string
	Rerank          bool
	RetrievalConfig *evalapi.RetrievalConfig
	ExperimentID    string
	Label           string
}
```

Update `StartRun` request:

```go
resp, err := s.api.StartEvaluation(ctx, evalapi.StartEvaluationRequest{
	DatasetID:        in.DatasetID,
	Collection:       in.Collection,
	Notes:            in.Notes,
	BaselineEvalID:   in.BaselineEvalID,
	Rerank:           in.Rerank,
	RetrievalConfig:  in.RetrievalConfig,
	ExperimentID:     in.ExperimentID,
	ExperimentLabel:  in.Label,
})
```

- [ ] **Step 7: Add failing MCP handler/schema test**

In `go/eval-mcp-service/internal/mcpserver/server_test.go`, add:

```go
func TestStartEvalRunForwardsRetrievalConfig(t *testing.T) {
	fake := &fakeEvalService{}
	result, err := startEvalRunHandler(fake)(context.Background(), callReq(map[string]any{
		"dataset_id": "ds-1",
		"collection": "documents",
		"rerank": true,
		"retrieval_config": map[string]any{
			"top_k": float64(3),
		},
	}))
	if err != nil {
		t.Fatalf("handler returned transport error: %v", err)
	}
	if result.IsError {
		t.Fatalf("handler returned MCP error: %#v", result)
	}
	if fake.startRunInput.RetrievalConfig == nil || fake.startRunInput.RetrievalConfig.TopK == nil || *fake.startRunInput.RetrievalConfig.TopK != 3 {
		t.Fatalf("retrieval config = %#v", fake.startRunInput.RetrievalConfig)
	}
}
```

In the schema test that checks tool descriptions, add `"retrieval_config"` to
the wanted strings if a schema-string assertion exists for `start_eval_run`.

- [ ] **Step 8: Run MCP handler test to verify failure**

Run:

```bash
go test ./go/eval-mcp-service/internal/mcpserver -run TestStartEvalRunForwardsRetrievalConfig -count=1
```

Expected: fails because handler decode and schema do not include
`retrieval_config`.

- [ ] **Step 9: Implement MCP decode and schema**

In `go/eval-mcp-service/internal/mcpserver/server.go`, update the
`startEvalRunHandler` args:

```go
var args struct {
	DatasetID       string                   `json:"dataset_id"`
	Collection      string                   `json:"collection"`
	Notes           string                   `json:"notes,omitempty"`
	BaselineEvalID  string                   `json:"baseline_eval_id,omitempty"`
	Rerank          bool                     `json:"rerank,omitempty"`
	RetrievalConfig *evalapi.RetrievalConfig `json:"retrieval_config,omitempty"`
	ExperimentID    string                   `json:"experiment_id,omitempty"`
	Label           string                   `json:"label,omitempty"`
}
```

Add validation after required-field checks:

```go
if args.RetrievalConfig != nil && args.RetrievalConfig.TopK != nil {
	if *args.RetrievalConfig.TopK < 1 || *args.RetrievalConfig.TopK > 20 {
		return toolError("retrieval_config.top_k must be between 1 and 20"), nil
	}
}
```

Forward it:

```go
in := evalworkflow.StartRunInput{
	DatasetID:       args.DatasetID,
	Collection:      args.Collection,
	Notes:           args.Notes,
	BaselineEvalID:  args.BaselineEvalID,
	Rerank:          args.Rerank,
	RetrievalConfig: args.RetrievalConfig,
	ExperimentID:    args.ExperimentID,
	Label:           args.Label,
}
```

Update `startEvalRunSchema()`:

```go
func startEvalRunSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"dataset_id":{"type":"string"},"collection":{"type":"string"},"notes":{"type":"string"},"baseline_eval_id":{"type":"string"},"rerank":{"type":"boolean"},"retrieval_config":{"type":"object","properties":{"top_k":{"type":"integer","minimum":1,"maximum":20}},"additionalProperties":false},"experiment_id":{"type":"string","minLength":1},"label":{"type":"string"}},"required":["dataset_id","collection"],"additionalProperties":false}`)
}
```

- [ ] **Step 10: Run Go MCP tests to verify pass**

Run:

```bash
go test ./go/eval-mcp-service/internal/evalapi ./go/eval-mcp-service/internal/evalworkflow ./go/eval-mcp-service/internal/mcpserver -count=1
```

Expected: all selected Go tests pass.

- [ ] **Step 11: Commit Go slice**

Run:

```bash
git add go/eval-mcp-service/internal/evalapi/client.go go/eval-mcp-service/internal/evalapi/client_test.go go/eval-mcp-service/internal/evalworkflow/service.go go/eval-mcp-service/internal/evalworkflow/service_test.go go/eval-mcp-service/internal/mcpserver/server.go go/eval-mcp-service/internal/mcpserver/server_test.go
git commit -m "feat: add retrieval config to eval MCP runs"
```

Expected: commit succeeds.

## Task 4: Full Verification and PR

**Files:**
- Verify all modified files.
- No code edits unless verification exposes failures.

- [ ] **Step 1: Run Python preflight**

Run:

```bash
make preflight-python
```

Expected: Python tests, lint, and formatting checks pass.

- [ ] **Step 2: Run Go preflight**

Run:

```bash
make preflight-go
```

Expected: Go tests and lint checks pass.

- [ ] **Step 3: Run security preflight**

Run:

```bash
make preflight-security
```

Expected: security checks pass.

- [ ] **Step 4: Inspect final diff**

Run:

```bash
git status --short
git log --oneline -5
git diff --stat qa...HEAD
git diff --check qa...HEAD
```

Expected:

- Working tree is clean.
- The branch contains the three feature commits.
- Diff touches only the planned chat, eval, MCP, and test files.
- `git diff --check` exits 0.

- [ ] **Step 5: Push and create PR to qa**

Run:

```bash
git push -u origin feat/eval-per-run-retrieval-config
gh pr create --base qa --head feat/eval-per-run-retrieval-config --title "Add per-run retrieval config to eval workflow" --body "$(cat <<'PR_BODY'
## Summary
- add typed `retrieval_config.top_k` support to eval runs
- align eval `/search.limit` and chat `/chat` final context budget
- capture requested and effective retrieval config in eval run metadata
- expose the option through the eval MCP `start_eval_run` tool

## Verification
- make preflight-python
- make preflight-go
- make preflight-security
PR_BODY
)"
```

Expected: branch pushes and PR is created against `qa`. Do not watch CI unless
Kyle asks.

## Post-Deployment Experiment

After QA deployment, run the controlled experiment through MCP:

```json
{
  "dataset_id": "19d3ee69-6544-44b8-94bd-0fac110f06f5",
  "collection": "documents",
  "experiment_id": "EXP_ID",
  "label": "top_k_5_baseline",
  "rerank": false,
  "retrieval_config": {
    "top_k": 5
  },
  "notes": "Baseline final context budget 5, rerank off."
}
```

```json
{
  "dataset_id": "19d3ee69-6544-44b8-94bd-0fac110f06f5",
  "collection": "documents",
  "experiment_id": "EXP_ID",
  "label": "top_k_3_candidate",
  "rerank": false,
  "retrieval_config": {
    "top_k": 3
  },
  "notes": "Candidate final context budget 3, rerank off."
}
```

Compare completed runs and inspect worst cases for `context_precision` and
`context_recall`. Keep the candidate only if `context_precision` improves
meaningfully while `answer_relevancy` and `faithfulness` remain effectively
stable, with no unacceptable loss in `context_recall`.
