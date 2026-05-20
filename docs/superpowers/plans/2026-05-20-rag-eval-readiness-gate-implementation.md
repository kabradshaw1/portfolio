# RAG Eval Readiness Gate Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a blocking/warning readiness gate for RAG eval experiments so empty, mismatched, or under-evidenced retrieval collections cannot produce misleading eval runs.

**Architecture:** Implement the readiness policy in the Python eval service, backed by a narrow ingestion source-inventory endpoint and exposed through the Go eval MCP. The eval API is the policy source of truth; the MCP calls it explicitly and also guards `start_eval_experiment` and `start_eval_run` so agents cannot skip the gate.

**Tech Stack:** Python 3/FastAPI/Pydantic/httpx/pytest, Qdrant Python client, Go 1.x/modelcontextprotocol go-sdk/net/http/httptest, SQLite-backed eval metadata.

---

## Implementation Notes

This plan changes application/runtime behavior and Go/Python services. Execute it from a feature worktree, not directly on `qa`:

```bash
git worktree add .codex/worktrees/rag-eval-readiness-gate -b feat/rag-eval-readiness-gate qa
cd .codex/worktrees/rag-eval-readiness-gate
pwd
git branch --show-current
git rev-parse --show-toplevel
```

Expected:

- `pwd` ends with `.codex/worktrees/rag-eval-readiness-gate`
- branch is `feat/rag-eval-readiness-gate`
- top level is the same worktree path

Before every edit, confirm the working directory is inside the worktree.

## File Structure

Create:

- `services/eval/app/readiness.py` - readiness evidence collection, policy classification, and helper functions.
- `services/eval/tests/test_readiness.py` - unit tests for readiness policy without FastAPI routing.

Modify:

- `services/ingestion/app/store.py` - add `list_sources(collection_name)` using Qdrant payload scroll.
- `services/ingestion/app/main.py` - add `GET /collections/{name}/sources`.
- `services/ingestion/tests/test_store.py` - store tests for source inventory.
- `services/ingestion/tests/test_main.py` - API tests for source inventory.
- `services/eval/app/models.py` - add typed readiness request/response models.
- `services/eval/app/main.py` - add `POST /readiness/rag`, call readiness before starting evals, persist readiness in run config, and expose readiness in existing run evidence via config.
- `services/eval/app/config_capture.py` - preserve existing config capture shape; no readiness logic belongs here.
- `services/eval/tests/test_main.py` - API and start-run persistence tests.
- `go/eval-mcp-service/internal/evalapi/client.go` - add readiness request/response types and `CheckRAGReadiness`.
- `go/eval-mcp-service/internal/evalapi/client_test.go` - eval API client readiness test.
- `go/eval-mcp-service/internal/evalworkflow/service.go` - add readiness input/result, explicit `CheckReadiness`, and guard `StartExperiment`/`StartRun`.
- `go/eval-mcp-service/internal/evalworkflow/service_test.go` - workflow readiness tests.
- `go/eval-mcp-service/internal/mcpserver/server.go` - register `check_rag_eval_readiness`, add schema/handler, update prompt/resource instructions.
- `go/eval-mcp-service/internal/mcpserver/server_test.go` - MCP registration/schema/handler tests.

Do not modify frontend files in this first implementation.

---

### Task 1: Add Ingestion Source Inventory

**Files:**
- Modify: `services/ingestion/app/store.py`
- Modify: `services/ingestion/app/main.py`
- Test: `services/ingestion/tests/test_store.py`
- Test: `services/ingestion/tests/test_main.py`

- [ ] **Step 1: Write the failing store test**

Append this test to `services/ingestion/tests/test_store.py`:

```python
def test_list_sources_counts_distinct_filenames(mock_qdrant_client):
    mock_qdrant_client.collection_exists.return_value = True
    store = QdrantStore(host="localhost", port=6333, collection_name="default")

    mock_qdrant_client.collection_exists.return_value = True
    mock_qdrant_client.scroll.return_value = (
        [
            MagicMock(payload={"filename": "laptop.pdf"}),
            MagicMock(payload={"filename": "laptop.pdf"}),
            MagicMock(payload={"filename": "monitor.pdf"}),
            MagicMock(payload={"filename": ""}),
            MagicMock(payload={}),
        ],
        None,
    )

    got = store.list_sources("documents")

    assert got == [
        {"filename": "laptop.pdf", "chunks": 2},
        {"filename": "monitor.pdf", "chunks": 1},
    ]
    mock_qdrant_client.scroll.assert_called_once()
    assert mock_qdrant_client.scroll.call_args.kwargs["collection_name"] == "documents"
    assert mock_qdrant_client.scroll.call_args.kwargs["with_payload"] is True
    assert mock_qdrant_client.scroll.call_args.kwargs["with_vectors"] is False
```

- [ ] **Step 2: Run the store test and verify it fails**

Run:

```bash
cd services/ingestion
pytest tests/test_store.py::test_list_sources_counts_distinct_filenames -q
```

Expected: FAIL with `AttributeError: 'QdrantStore' object has no attribute 'list_sources'`.

- [ ] **Step 3: Implement `QdrantStore.list_sources`**

In `services/ingestion/app/store.py`, add this method after `list_documents`:

```python
    def list_sources(self, collection_name: str) -> list[dict]:
        """Return distinct source filenames and chunk counts for a collection."""
        if not self.client.collection_exists(collection_name):
            raise ValueError(f"Collection {collection_name} not found")

        start = time.perf_counter()
        records, _ = self.client.scroll(
            collection_name=collection_name,
            limit=10000,
            with_payload=True,
            with_vectors=False,
        )
        QDRANT_OPERATION_DURATION.labels(
            service="ingestion", operation="scroll_sources"
        ).observe(time.perf_counter() - start)

        counts: dict[str, int] = {}
        for record in records:
            filename = record.payload.get("filename") if record.payload else None
            if not filename:
                continue
            counts[filename] = counts.get(filename, 0) + 1

        return [
            {"filename": filename, "chunks": count}
            for filename, count in sorted(counts.items())
        ]
```

- [ ] **Step 4: Run the store test and verify it passes**

Run:

```bash
cd services/ingestion
pytest tests/test_store.py::test_list_sources_counts_distinct_filenames -q
```

Expected: PASS.

- [ ] **Step 5: Write the failing API tests**

Append these tests to `services/ingestion/tests/test_main.py`:

```python
@patch("app.main.get_store")
def test_list_collection_sources(mock_get_store):
    mock_store = MagicMock()
    mock_store.list_sources.return_value = [
        {"filename": "laptop.pdf", "chunks": 2},
        {"filename": "monitor.pdf", "chunks": 1},
    ]
    mock_get_store.return_value = mock_store

    response = client.get("/collections/documents/sources")

    assert response.status_code == 200
    assert response.json() == {
        "collection": "documents",
        "sources": [
            {"filename": "laptop.pdf", "chunks": 2},
            {"filename": "monitor.pdf", "chunks": 1},
        ],
    }
    mock_store.list_sources.assert_called_once_with("documents")


@patch("app.main.get_store")
def test_list_collection_sources_rejects_invalid_collection_name(mock_get_store):
    response = client.get("/collections/bad name/sources")

    assert response.status_code == 422
    assert response.json()["detail"] == "Invalid collection name"
    mock_get_store.assert_not_called()


@patch("app.main.get_store")
def test_list_collection_sources_not_found(mock_get_store):
    mock_store = MagicMock()
    mock_store.list_sources.side_effect = ValueError("Collection missing not found")
    mock_get_store.return_value = mock_store

    response = client.get("/collections/missing/sources")

    assert response.status_code == 404
    assert response.json()["detail"] == "Collection missing not found"
```

- [ ] **Step 6: Run the API tests and verify they fail**

Run:

```bash
cd services/ingestion
pytest tests/test_main.py::test_list_collection_sources tests/test_main.py::test_list_collection_sources_rejects_invalid_collection_name tests/test_main.py::test_list_collection_sources_not_found -q
```

Expected: FAIL with 404 because `/collections/{name}/sources` is not defined.

- [ ] **Step 7: Add the source inventory route**

In `services/ingestion/app/main.py`, insert this route after `get_collection_config` and before `/ingest`:

```python
@app.get("/collections/{name}/sources")
@limiter.limit("30/minute")
async def list_collection_sources(
    request: Request, name: str, user_id: str = Depends(require_auth)
):
    if not _COLLECTION_NAME_RE.match(name):
        raise HTTPException(status_code=422, detail="Invalid collection name")
    store = get_store()
    try:
        sources = store.list_sources(name)
    except ValueError as e:
        raise HTTPException(status_code=404, detail=str(e))
    except Exception as e:
        logger.error("Qdrant error listing collection sources: %s", e, exc_info=True)
        raise HTTPException(status_code=503, detail="Vector store unavailable")
    return {"collection": name, "sources": sources}
```

- [ ] **Step 8: Run ingestion tests for the new endpoint**

Run:

```bash
cd services/ingestion
pytest tests/test_store.py::test_list_sources_counts_distinct_filenames tests/test_main.py::test_list_collection_sources tests/test_main.py::test_list_collection_sources_rejects_invalid_collection_name tests/test_main.py::test_list_collection_sources_not_found -q
```

Expected: PASS.

- [ ] **Step 9: Commit Task 1**

```bash
git add services/ingestion/app/store.py services/ingestion/app/main.py services/ingestion/tests/test_store.py services/ingestion/tests/test_main.py
git commit -m "feat(ingestion): expose collection source inventory"
```

---

### Task 2: Add Python Readiness Models And Policy

**Files:**
- Modify: `services/eval/app/models.py`
- Create: `services/eval/app/readiness.py`
- Test: `services/eval/tests/test_readiness.py`

- [ ] **Step 1: Add failing readiness policy tests**

Create `services/eval/tests/test_readiness.py` with:

```python
import pytest
from app.models import RetrievalConfig
from app.readiness import RAGReadinessChecker


class FakeDB:
    def __init__(self):
        self.dataset = {
            "id": "ds-1",
            "name": "product-docs-rag-v1",
            "items": [
                {
                    "query": "q1",
                    "expected_answer": "a1",
                    "expected_sources": ["laptop.pdf"],
                },
                {
                    "query": "q2",
                    "expected_answer": "a2",
                    "expected_sources": ["monitor.pdf"],
                },
            ],
            "created_at": "2026-05-20T00:00:00+00:00",
        }
        self.evaluations = {}
        self.experiments = {}

    async def get_dataset(self, dataset_id):
        return self.dataset if dataset_id == self.dataset["id"] else None

    async def get_evaluation(self, eval_id):
        return self.evaluations.get(eval_id)

    async def get_experiment(self, experiment_id):
        return self.experiments.get(experiment_id)


class FakeUpstream:
    def __init__(self):
        self.collections = [{"name": "documents", "points_count": 10}]
        self.collection_config = {
            "chunk_size": 1000,
            "chunk_overlap": 200,
            "embedding_model": "nomic-embed-text",
            "hybrid_enabled": True,
            "dense_vector_name": "dense",
            "sparse_vector_name": "sparse",
            "sparse_model": "Qdrant/bm25",
        }
        self.sources = [
            {"filename": "laptop.pdf", "chunks": 4},
            {"filename": "monitor.pdf", "chunks": 3},
        ]
        self.chat_config = {
            "retrieval_mode": "hybrid",
            "dense_vector_name": "dense",
            "sparse_vector_name": "sparse",
            "rerank_enabled": True,
            "top_k": 5,
        }

    async def list_collections(self):
        return self.collections

    async def get_collection_config(self, collection):
        return self.collection_config

    async def list_collection_sources(self, collection):
        return self.sources

    async def get_chat_config(self):
        return self.chat_config


@pytest.mark.asyncio
async def test_readiness_ready_when_dataset_collection_and_configs_match():
    result = await RAGReadinessChecker(FakeDB(), FakeUpstream()).check(
        dataset_id="ds-1",
        collection="documents",
        rerank=False,
        retrieval_config=None,
    )

    assert result.status == "ready"
    assert result.blocking_failures == []
    assert result.warnings == []
    assert result.evidence["dataset"]["item_count"] == 2
    assert result.evidence["collection"]["points_count"] == 10


@pytest.mark.asyncio
async def test_readiness_blocks_empty_collection():
    upstream = FakeUpstream()
    upstream.collections = [{"name": "documents", "points_count": 0}]

    result = await RAGReadinessChecker(FakeDB(), upstream).check(
        dataset_id="ds-1",
        collection="documents",
        rerank=False,
        retrieval_config=None,
    )

    assert result.status == "blocked"
    assert [finding.code for finding in result.blocking_failures] == [
        "collection_empty"
    ]


@pytest.mark.asyncio
async def test_readiness_blocks_missing_collection_config():
    class MissingConfigUpstream(FakeUpstream):
        async def get_collection_config(self, collection):
            raise RuntimeError("status 404")

    result = await RAGReadinessChecker(FakeDB(), MissingConfigUpstream()).check(
        dataset_id="ds-1",
        collection="documents",
        rerank=False,
        retrieval_config=None,
    )

    assert result.status == "blocked"
    assert [finding.code for finding in result.blocking_failures] == [
        "collection_config_unavailable"
    ]


@pytest.mark.asyncio
async def test_readiness_blocks_zero_expected_source_matches():
    upstream = FakeUpstream()
    upstream.sources = [{"filename": "other.pdf", "chunks": 2}]

    result = await RAGReadinessChecker(FakeDB(), upstream).check(
        dataset_id="ds-1",
        collection="documents",
        rerank=False,
        retrieval_config=None,
    )

    assert result.status == "blocked"
    assert [finding.code for finding in result.blocking_failures] == [
        "expected_sources_missing"
    ]


@pytest.mark.asyncio
async def test_readiness_warns_partial_expected_source_coverage():
    upstream = FakeUpstream()
    upstream.sources = [{"filename": "laptop.pdf", "chunks": 4}]

    result = await RAGReadinessChecker(FakeDB(), upstream).check(
        dataset_id="ds-1",
        collection="documents",
        rerank=False,
        retrieval_config=None,
    )

    assert result.status == "warning"
    assert result.blocking_failures == []
    assert [finding.code for finding in result.warnings] == [
        "partial_expected_source_coverage"
    ]


@pytest.mark.asyncio
async def test_readiness_blocks_chat_collection_vector_mismatch():
    upstream = FakeUpstream()
    upstream.collection_config["dense_vector_name"] = "content"

    result = await RAGReadinessChecker(FakeDB(), upstream).check(
        dataset_id="ds-1",
        collection="documents",
        rerank=False,
        retrieval_config=None,
    )

    assert result.status == "blocked"
    assert [finding.code for finding in result.blocking_failures] == [
        "dense_vector_mismatch"
    ]


@pytest.mark.asyncio
async def test_readiness_warns_rerank_requested_when_runtime_disabled():
    upstream = FakeUpstream()
    upstream.chat_config["rerank_enabled"] = False

    result = await RAGReadinessChecker(FakeDB(), upstream).check(
        dataset_id="ds-1",
        collection="documents",
        rerank=True,
        retrieval_config=None,
    )

    assert result.status == "warning"
    assert [finding.code for finding in result.warnings] == [
        "rerank_requested_but_disabled"
    ]


@pytest.mark.asyncio
async def test_readiness_warns_top_k_override_differs_from_chat_default():
    result = await RAGReadinessChecker(FakeDB(), FakeUpstream()).check(
        dataset_id="ds-1",
        collection="documents",
        rerank=False,
        retrieval_config=RetrievalConfig(top_k=3),
    )

    assert result.status == "warning"
    assert [finding.code for finding in result.warnings] == [
        "top_k_override"
    ]
```

- [ ] **Step 2: Run the readiness tests and verify they fail**

Run:

```bash
cd services/eval
pytest tests/test_readiness.py -q
```

Expected: FAIL with `ModuleNotFoundError: No module named 'app.readiness'`.

- [ ] **Step 3: Add readiness models**

In `services/eval/app/models.py`, add these models after `RetrievalConfig`:

```python
ReadinessStatus = Literal["ready", "warning", "blocked"]


class ReadinessFinding(BaseModel):
    code: str
    message: str
    remediation: str


class RAGReadinessRequest(BaseModel):
    dataset_id: str
    collection: str = Field(pattern=r"^[a-zA-Z0-9_-]{1,100}$")
    rerank: bool = False
    retrieval_config: RetrievalConfig | None = None
    baseline_eval_id: str | None = None
    experiment_id: str | None = None


class RAGReadinessResponse(BaseModel):
    status: ReadinessStatus
    blocking_failures: list[ReadinessFinding] = Field(default_factory=list)
    warnings: list[ReadinessFinding] = Field(default_factory=list)
    evidence: dict[str, Any] = Field(default_factory=dict)
    next_steps: list[str] = Field(default_factory=list)
```

- [ ] **Step 4: Implement the readiness policy**

Create `services/eval/app/readiness.py`:

```python
from __future__ import annotations

from collections.abc import Iterable
from datetime import datetime, timezone
from typing import Protocol

import httpx

from app.models import (
    RAGReadinessResponse,
    ReadinessFinding,
    RetrievalConfig,
)

_UTC = timezone.utc  # noqa: UP017


class EvalDBLike(Protocol):
    async def get_dataset(self, ds_id: str) -> dict | None: ...
    async def get_evaluation(self, eval_id: str) -> dict | None: ...
    async def get_experiment(self, experiment_id: str) -> dict | None: ...


class RAGReadinessUpstream:
    def __init__(self, chat_url: str, ingestion_url: str):
        self.chat_url = chat_url.rstrip("/")
        self.ingestion_url = ingestion_url.rstrip("/")

    async def _get_json(self, url: str) -> dict:
        async with httpx.AsyncClient(timeout=5.0) as client:
            resp = await client.get(url)
            resp.raise_for_status()
            return resp.json()

    async def list_collections(self) -> list[dict]:
        payload = await self._get_json(f"{self.ingestion_url}/collections")
        return payload.get("collections", [])

    async def get_collection_config(self, collection: str) -> dict:
        return await self._get_json(
            f"{self.ingestion_url}/collections/{collection}/config"
        )

    async def list_collection_sources(self, collection: str) -> list[dict]:
        payload = await self._get_json(
            f"{self.ingestion_url}/collections/{collection}/sources"
        )
        return payload.get("sources", [])

    async def get_chat_config(self) -> dict:
        return await self._get_json(f"{self.chat_url}/config")


class RAGReadinessChecker:
    def __init__(self, db: EvalDBLike, upstream):
        self.db = db
        self.upstream = upstream

    async def check(
        self,
        *,
        dataset_id: str,
        collection: str,
        rerank: bool,
        retrieval_config: RetrievalConfig | None,
        baseline_eval_id: str | None = None,
        experiment_id: str | None = None,
    ) -> RAGReadinessResponse:
        blocking: list[ReadinessFinding] = []
        warnings: list[ReadinessFinding] = []
        evidence: dict = {
            "checked_at": datetime.now(_UTC).isoformat(),
            "requested": {
                "rerank": rerank,
                "retrieval_config": (
                    retrieval_config.model_dump(exclude_none=True)
                    if retrieval_config
                    else {}
                ),
            },
        }

        dataset = await self.db.get_dataset(dataset_id)
        if dataset is None:
            blocking.append(_finding("dataset_not_found", f"Dataset {dataset_id} was not found.", "Choose an existing eval dataset before starting the run."))
            return _response(blocking, warnings, evidence)
        items = dataset.get("items", [])
        evidence["dataset"] = {
            "id": dataset["id"],
            "name": dataset.get("name"),
            "item_count": len(items),
            "expected_sources": sorted(_expected_sources(items)),
        }
        if not items:
            blocking.append(_finding("dataset_empty", f"Dataset {dataset_id} has no items.", "Create a dataset with at least one golden question."))

        await self._validate_baseline(baseline_eval_id, dataset_id, collection, blocking, evidence)
        await self._validate_experiment(experiment_id, dataset_id, collection, blocking, evidence)
        await self._collect_upstream(collection, rerank, retrieval_config, items, blocking, warnings, evidence)
        return _response(blocking, warnings, evidence)

    async def _validate_baseline(self, baseline_eval_id, dataset_id, collection, blocking, evidence) -> None:
        if not baseline_eval_id:
            return
        baseline = await self.db.get_evaluation(baseline_eval_id)
        evidence["baseline"] = {"id": baseline_eval_id}
        if baseline is None:
            blocking.append(_finding("baseline_not_found", f"Baseline evaluation {baseline_eval_id} was not found.", "Choose an existing completed baseline for this dataset and collection."))
            return
        evidence["baseline"].update({"status": baseline.get("status"), "dataset_id": baseline.get("dataset_id"), "collection": baseline.get("collection")})
        if baseline.get("dataset_id") != dataset_id or baseline.get("collection") != collection:
            blocking.append(_finding("baseline_scope_mismatch", "Baseline evaluation uses a different dataset or collection.", "Use a baseline from the same dataset and collection."))

    async def _validate_experiment(self, experiment_id, dataset_id, collection, blocking, evidence) -> None:
        if not experiment_id:
            return
        experiment = await self.db.get_experiment(experiment_id)
        evidence["experiment"] = {"id": experiment_id}
        if experiment is None:
            blocking.append(_finding("experiment_not_found", f"Experiment {experiment_id} was not found.", "Choose an existing experiment or create a new one."))
            return
        evidence["experiment"].update({"status": experiment.get("status"), "dataset_id": experiment.get("dataset_id"), "collection": experiment.get("collection")})
        if experiment.get("dataset_id") != dataset_id or experiment.get("collection") != collection:
            blocking.append(_finding("experiment_scope_mismatch", "Experiment uses a different dataset or collection.", "Use an experiment with the same dataset and collection."))

    async def _collect_upstream(self, collection, rerank, retrieval_config, items, blocking, warnings, evidence) -> None:
        try:
            collections = await self.upstream.list_collections()
        except Exception as exc:
            blocking.append(_finding("collections_unavailable", f"Unable to list retrieval collections: {exc}", "Restore ingestion collection discovery before starting evals."))
            return

        selected = next((item for item in collections if item.get("name") == collection), None)
        if selected is None:
            blocking.append(_finding("collection_missing", f"Collection {collection} does not exist.", "Choose an existing retrieval collection."))
            return
        points_count = int(selected.get("points_count") or selected.get("point_count") or 0)
        evidence["collection"] = {"name": collection, "points_count": points_count}
        if points_count == 0:
            blocking.append(_finding("collection_empty", f"Collection {collection} has 0 points.", "Re-run ingestion or choose a populated collection before starting the eval."))

        collection_config = await self._required_collection_config(collection, blocking)
        chat_config = await self._required_chat_config(blocking)
        sources = await self._required_sources(collection, blocking)
        if collection_config is not None:
            evidence["collection"]["config"] = collection_config
        if chat_config is not None:
            evidence["chat"] = chat_config
        if sources is not None:
            evidence["collection"]["sources"] = sources

        if collection_config and chat_config:
            self._check_vector_config(collection_config, chat_config, blocking)
            self._check_rerank_and_top_k(rerank, retrieval_config, chat_config, warnings)
        if sources is not None:
            self._check_source_coverage(items, sources, blocking, warnings)

    async def _required_collection_config(self, collection, blocking):
        try:
            return await self.upstream.get_collection_config(collection)
        except Exception as exc:
            blocking.append(_finding("collection_config_unavailable", f"Collection config for {collection} is unavailable: {exc}", "Re-ingest the collection so metadata is recorded."))
            return None

    async def _required_chat_config(self, blocking):
        try:
            return await self.upstream.get_chat_config()
        except Exception as exc:
            blocking.append(_finding("chat_config_unavailable", f"Chat config is unavailable: {exc}", "Restore chat /config before starting evals."))
            return None

    async def _required_sources(self, collection, blocking):
        try:
            return await self.upstream.list_collection_sources(collection)
        except Exception as exc:
            blocking.append(_finding("source_inventory_unavailable", f"Source inventory for {collection} is unavailable: {exc}", "Restore ingestion source inventory before starting source-sensitive evals."))
            return None

    def _check_vector_config(self, collection_config, chat_config, blocking) -> None:
        if collection_config.get("dense_vector_name") != chat_config.get("dense_vector_name"):
            blocking.append(_finding("dense_vector_mismatch", "Chat and collection dense vector names do not match.", "Reconfigure chat or re-ingest the collection with matching dense vector metadata."))
        if chat_config.get("retrieval_mode") == "hybrid":
            if collection_config.get("hybrid_enabled") is False:
                blocking.append(_finding("hybrid_collection_disabled", "Chat is configured for hybrid retrieval but collection metadata has hybrid disabled.", "Use a hybrid collection or switch chat retrieval mode."))
            if collection_config.get("sparse_vector_name") != chat_config.get("sparse_vector_name"):
                blocking.append(_finding("sparse_vector_mismatch", "Chat and collection sparse vector names do not match.", "Reconfigure chat or re-ingest the collection with matching sparse vector metadata."))

    def _check_rerank_and_top_k(self, rerank, retrieval_config, chat_config, warnings) -> None:
        if rerank and not bool(chat_config.get("rerank_enabled")):
            warnings.append(_finding("rerank_requested_but_disabled", "Rerank was requested but chat runtime rerank support is disabled.", "Enable rerank in chat or run without rerank and record the caveat."))
        if retrieval_config and retrieval_config.top_k is not None and retrieval_config.top_k != chat_config.get("top_k"):
            warnings.append(_finding("top_k_override", "Requested top_k differs from the chat runtime default.", "Keep this caveat with the run because it is an intentional retrieval override."))

    def _check_source_coverage(self, items, sources, blocking, warnings) -> None:
        expected = _expected_sources(items)
        if not expected:
            return
        indexed = {source.get("filename") for source in sources if source.get("filename")}
        matched = sorted(expected & indexed)
        missing = sorted(expected - indexed)
        if not matched:
            blocking.append(_finding("expected_sources_missing", "None of the dataset expected sources are present in the selected collection.", "Re-ingest the expected documents or choose the collection that contains them."))
        elif missing:
            warnings.append(_finding("partial_expected_source_coverage", f"{len(matched)} of {len(expected)} expected source names were found in the collection.", "Review missing sources before treating source-sensitive regressions as retrieval failures."))


def _expected_sources(items: Iterable[dict]) -> set[str]:
    return {
        source
        for item in items
        for source in item.get("expected_sources", [])
        if source
    }


def _finding(code: str, message: str, remediation: str) -> ReadinessFinding:
    return ReadinessFinding(code=code, message=message, remediation=remediation)


def _response(blocking, warnings, evidence) -> RAGReadinessResponse:
    status = "blocked" if blocking else "warning" if warnings else "ready"
    if blocking:
        next_steps = [finding.remediation for finding in blocking]
    elif warnings:
        next_steps = ["Proceed only if the warning caveats are acceptable for this experiment."]
    else:
        next_steps = ["Proceed with the eval run."]
    return RAGReadinessResponse(
        status=status,
        blocking_failures=blocking,
        warnings=warnings,
        evidence=evidence,
        next_steps=next_steps,
    )
```

- [ ] **Step 5: Run readiness tests**

Run:

```bash
cd services/eval
pytest tests/test_readiness.py -q
```

Expected: PASS.

- [ ] **Step 6: Commit Task 2**

```bash
git add services/eval/app/models.py services/eval/app/readiness.py services/eval/tests/test_readiness.py
git commit -m "feat(eval): add rag readiness policy"
```

---

### Task 3: Add Eval Readiness API And Persist Readiness

**Files:**
- Modify: `services/eval/app/main.py`
- Modify: `services/eval/tests/test_main.py`

- [ ] **Step 1: Write failing API and start-run tests**

Append these tests to `services/eval/tests/test_main.py`:

```python
@patch("app.main.RAGReadinessChecker")
@patch("app.main.RAGReadinessUpstream")
@patch("app.main.get_db")
def test_check_rag_eval_readiness_endpoint(mock_get_db, mock_upstream_cls, mock_checker_cls):
    mock_db = AsyncMock()
    mock_get_db.return_value = mock_db
    result = MagicMock()
    result.model_dump.return_value = {
        "status": "ready",
        "blocking_failures": [],
        "warnings": [],
        "evidence": {"dataset": {"id": "ds-123"}},
        "next_steps": ["Proceed with the eval run."],
    }
    checker = MagicMock()
    checker.check = AsyncMock(return_value=result)
    mock_checker_cls.return_value = checker

    response = client.post(
        "/readiness/rag",
        json={"dataset_id": "ds-123", "collection": "documents", "rerank": False},
    )

    assert response.status_code == 200
    assert response.json()["status"] == "ready"
    checker.check.assert_awaited_once()
    assert checker.check.await_args.kwargs["dataset_id"] == "ds-123"
    assert checker.check.await_args.kwargs["collection"] == "documents"


@patch("app.main.RAGReadinessChecker")
@patch("app.main.RAGReadinessUpstream")
@patch("app.main.get_db")
def test_start_evaluation_blocks_when_readiness_blocked(
    mock_get_db, mock_upstream_cls, mock_checker_cls
):
    mock_db = AsyncMock()
    mock_db.get_dataset.return_value = {
        "id": "ds-123",
        "name": "test",
        "items": [{"query": "q", "expected_answer": "a", "expected_sources": []}],
        "created_at": "2026-04-16T00:00:00Z",
    }
    mock_get_db.return_value = mock_db
    result = MagicMock()
    result.status = "blocked"
    result.model_dump.return_value = {
        "status": "blocked",
        "blocking_failures": [
            {
                "code": "collection_empty",
                "message": "Collection documents has 0 points.",
                "remediation": "Re-run ingestion.",
            }
        ],
        "warnings": [],
        "evidence": {},
        "next_steps": ["Re-run ingestion."],
    }
    checker = MagicMock()
    checker.check = AsyncMock(return_value=result)
    mock_checker_cls.return_value = checker

    response = client.post(
        "/evaluations",
        json={"dataset_id": "ds-123", "collection": "documents"},
    )

    assert response.status_code == 422
    assert response.json()["detail"]["status"] == "blocked"
    mock_db.create_evaluation.assert_not_awaited()


@patch("app.main.capture_run_config", new_callable=AsyncMock)
@patch("app.main.RAGReadinessChecker")
@patch("app.main.RAGReadinessUpstream")
@patch("app.main.get_db")
def test_start_evaluation_persists_readiness_in_config(
    mock_get_db, mock_upstream_cls, mock_checker_cls, mock_capture_run_config
):
    mock_db = AsyncMock()
    mock_db.get_dataset.return_value = {
        "id": "ds-123",
        "name": "test",
        "items": [{"query": "q", "expected_answer": "a", "expected_sources": []}],
        "created_at": "2026-04-16T00:00:00Z",
    }
    mock_db.create_evaluation.return_value = "eval-456"
    mock_db.create_evaluation_items.return_value = [{"id": "item-1", "item_index": 0}]
    mock_get_db.return_value = mock_db
    mock_capture_run_config.return_value = {"chat": {"top_k": 5}}
    result = MagicMock()
    result.status = "warning"
    result.model_dump.return_value = {
        "status": "warning",
        "blocking_failures": [],
        "warnings": [
            {
                "code": "top_k_override",
                "message": "Requested top_k differs.",
                "remediation": "Record caveat.",
            }
        ],
        "evidence": {"requested": {"retrieval_config": {"top_k": 3}}},
        "next_steps": ["Proceed only if caveats are acceptable."],
    }
    checker = MagicMock()
    checker.check = AsyncMock(return_value=result)
    mock_checker_cls.return_value = checker

    response = client.post(
        "/evaluations",
        json={
            "dataset_id": "ds-123",
            "collection": "documents",
            "retrieval_config": {"top_k": 3},
        },
    )

    assert response.status_code == 202
    stored_config = mock_db.set_evaluation_config.await_args.args[1]
    assert stored_config["readiness"]["status"] == "warning"
    assert stored_config["readiness"]["warnings"][0]["code"] == "top_k_override"
```

- [ ] **Step 2: Run the new tests and verify they fail**

Run:

```bash
cd services/eval
pytest tests/test_main.py::test_check_rag_eval_readiness_endpoint tests/test_main.py::test_start_evaluation_blocks_when_readiness_blocked tests/test_main.py::test_start_evaluation_persists_readiness_in_config -q
```

Expected: FAIL because the route/imports/readiness integration do not exist.

- [ ] **Step 3: Wire readiness imports and helper**

In `services/eval/app/main.py`, add imports:

```python
from app.models import (
    ...
    RAGReadinessRequest,
    RAGReadinessResponse,
    ...
)
from app.readiness import RAGReadinessChecker, RAGReadinessUpstream
```

Add this helper near `_effective_top_k`:

```python
async def _check_rag_readiness(
    db: EvalDB,
    *,
    dataset_id: str,
    collection: str,
    rerank: bool,
    retrieval_config: RetrievalConfig | None,
    baseline_eval_id: str | None = None,
    experiment_id: str | None = None,
) -> RAGReadinessResponse:
    upstream = RAGReadinessUpstream(
        chat_url=settings.chat_service_url,
        ingestion_url=settings.ingestion_service_url,
    )
    return await RAGReadinessChecker(db, upstream).check(
        dataset_id=dataset_id,
        collection=collection,
        rerank=rerank,
        retrieval_config=retrieval_config,
        baseline_eval_id=baseline_eval_id,
        experiment_id=experiment_id,
    )
```

- [ ] **Step 4: Add the readiness endpoint**

In `services/eval/app/main.py`, add before `# --- Evaluations ---`:

```python
@app.post(
    "/readiness/rag",
    response_model=RAGReadinessResponse,
    dependencies=[Depends(enforce_eval_read)],
)
async def check_rag_eval_readiness(request: Request, body: RAGReadinessRequest):
    db = await get_db()
    return await _check_rag_readiness(
        db,
        dataset_id=body.dataset_id,
        collection=body.collection,
        rerank=body.rerank,
        retrieval_config=body.retrieval_config,
        baseline_eval_id=body.baseline_eval_id,
        experiment_id=body.experiment_id,
    )
```

- [ ] **Step 5: Replace the old collection-only gate in `start_evaluation`**

In `start_evaluation`, replace:

```python
    await validate_collection_exists(settings.ingestion_service_url, collection)
```

with:

```python
    readiness = await _check_rag_readiness(
        db,
        dataset_id=body.dataset_id,
        collection=collection,
        rerank=body.rerank,
        retrieval_config=body.retrieval_config,
        baseline_eval_id=body.baseline_eval_id,
        experiment_id=body.experiment_id,
    )
    readiness_payload = readiness.model_dump()
    if readiness.status == "blocked":
        raise HTTPException(status_code=422, detail=readiness_payload)
```

Then after `config = await capture_run_config(...)`, add:

```python
    config["readiness"] = readiness_payload
```

Do not remove `_validate_baseline` or `_validate_experiment_for_run`; keep those direct DB checks because they return existing API-compatible errors.

- [ ] **Step 6: Run the new eval API tests**

Run:

```bash
cd services/eval
pytest tests/test_main.py::test_check_rag_eval_readiness_endpoint tests/test_main.py::test_start_evaluation_blocks_when_readiness_blocked tests/test_main.py::test_start_evaluation_persists_readiness_in_config -q
```

Expected: PASS.

- [ ] **Step 7: Run eval model/config tests touched by imports**

Run:

```bash
cd services/eval
pytest tests/test_models.py tests/test_config_capture.py tests/test_main.py -q
```

Expected: PASS.

- [ ] **Step 8: Commit Task 3**

```bash
git add services/eval/app/main.py services/eval/tests/test_main.py
git commit -m "feat(eval): expose rag readiness preflight"
```

---

### Task 4: Add Go Eval API And Ingestion Client Support

**Files:**
- Modify: `go/eval-mcp-service/internal/evalapi/client.go`
- Modify: `go/eval-mcp-service/internal/evalapi/client_test.go`
- Modify: `go/eval-mcp-service/internal/ingestionapi/client.go`
- Modify: `go/eval-mcp-service/internal/ingestionapi/client_test.go`

- [ ] **Step 1: Write failing eval API client test**

Append to `go/eval-mcp-service/internal/evalapi/client_test.go`:

```go
func TestClientCheckRAGReadiness(t *testing.T) {
	topK := 3
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/readiness/rag" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var body RAGReadinessRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body.DatasetID != "ds-1" || body.Collection != "documents" || !body.Rerank {
			t.Fatalf("body = %#v", body)
		}
		if body.RetrievalConfig == nil || body.RetrievalConfig.TopK == nil || *body.RetrievalConfig.TopK != 3 {
			t.Fatalf("retrieval config = %#v", body.RetrievalConfig)
		}
		_ = json.NewEncoder(w).Encode(RAGReadinessResponse{
			Status: "warning",
			Warnings: []ReadinessFinding{{
				Code:        "top_k_override",
				Message:     "Requested top_k differs.",
				Remediation: "Record caveat.",
			}},
			Evidence: map[string]any{"collection": map[string]any{"name": "documents"}},
			NextSteps: []string{"Proceed only if caveats are acceptable."},
		})
	}))
	defer server.Close()

	got, err := New(server.URL, "", server.Client()).CheckRAGReadiness(context.Background(), RAGReadinessRequest{
		DatasetID:       "ds-1",
		Collection:      "documents",
		Rerank:          true,
		RetrievalConfig: &RetrievalConfig{TopK: &topK},
	})
	if err != nil {
		t.Fatalf("CheckRAGReadiness error: %v", err)
	}
	if got.Status != "warning" || got.Warnings[0].Code != "top_k_override" {
		t.Fatalf("readiness = %#v", got)
	}
}
```

- [ ] **Step 2: Run the eval API client test and verify it fails**

Run:

```bash
cd go/eval-mcp-service
go test ./internal/evalapi -run TestClientCheckRAGReadiness -count=1
```

Expected: FAIL with undefined readiness types/method.

- [ ] **Step 3: Add eval API readiness types and method**

In `go/eval-mcp-service/internal/evalapi/client.go`, add after `StartEvaluationResponse`:

```go
type RAGReadinessRequest struct {
	DatasetID        string           `json:"dataset_id"`
	Collection       string           `json:"collection"`
	Rerank           bool             `json:"rerank"`
	RetrievalConfig  *RetrievalConfig `json:"retrieval_config,omitempty"`
	BaselineEvalID   string           `json:"baseline_eval_id,omitempty"`
	ExperimentID     string           `json:"experiment_id,omitempty"`
}

type ReadinessFinding struct {
	Code        string `json:"code"`
	Message     string `json:"message"`
	Remediation string `json:"remediation"`
}

type RAGReadinessResponse struct {
	Status           string             `json:"status"`
	BlockingFailures []ReadinessFinding `json:"blocking_failures"`
	Warnings         []ReadinessFinding `json:"warnings"`
	Evidence         map[string]any     `json:"evidence"`
	NextSteps        []string           `json:"next_steps"`
}
```

Add this method near `StartEvaluation`:

```go
func (c *Client) CheckRAGReadiness(ctx context.Context, in RAGReadinessRequest) (RAGReadinessResponse, error) {
	var response RAGReadinessResponse
	if err := c.do(ctx, http.MethodPost, "/readiness/rag", in, &response); err != nil {
		return RAGReadinessResponse{}, err
	}
	return response, nil
}
```

- [ ] **Step 4: Write failing ingestion client source test**

Append to `go/eval-mcp-service/internal/ingestionapi/client_test.go`:

```go
func TestClientListCollectionSources(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/collections/documents/sources" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"collection": "documents",
			"sources": []map[string]any{
				{"filename": "laptop.pdf", "chunks": 2},
			},
		})
	}))
	defer server.Close()

	got, err := New(server.URL, "", server.Client()).ListCollectionSources(context.Background(), "documents")
	if err != nil {
		t.Fatalf("ListCollectionSources returned error: %v", err)
	}
	if len(got) != 1 || got[0].Filename != "laptop.pdf" || got[0].Chunks != 2 {
		t.Fatalf("unexpected sources: %#v", got)
	}
}
```

- [ ] **Step 5: Run the ingestion client source test and verify it fails**

Run:

```bash
cd go/eval-mcp-service
go test ./internal/ingestionapi -run TestClientListCollectionSources -count=1
```

Expected: FAIL with undefined source type/method.

- [ ] **Step 6: Add ingestion source client support**

In `go/eval-mcp-service/internal/ingestionapi/client.go`, add:

```go
type Source struct {
	Filename string `json:"filename"`
	Chunks   int    `json:"chunks"`
}
```

Then add:

```go
func (c *Client) ListCollectionSources(ctx context.Context, name string) ([]Source, error) {
	var response struct {
		Sources []Source `json:"sources"`
	}
	path := "/collections/" + url.PathEscape(name) + "/sources"
	if err := c.do(ctx, http.MethodGet, path, nil, &response); err != nil {
		return nil, err
	}
	return response.Sources, nil
}
```

- [ ] **Step 7: Run Go client package tests**

Run:

```bash
cd go/eval-mcp-service
go test ./internal/evalapi ./internal/ingestionapi -count=1
```

Expected: PASS.

- [ ] **Step 8: Commit Task 4**

```bash
git add go/eval-mcp-service/internal/evalapi/client.go go/eval-mcp-service/internal/evalapi/client_test.go go/eval-mcp-service/internal/ingestionapi/client.go go/eval-mcp-service/internal/ingestionapi/client_test.go
git commit -m "feat(eval-mcp): add readiness api clients"
```

---

### Task 5: Guard Go Eval Workflow With Readiness

**Files:**
- Modify: `go/eval-mcp-service/internal/evalworkflow/service.go`
- Modify: `go/eval-mcp-service/internal/evalworkflow/service_test.go`

- [ ] **Step 1: Write failing workflow tests**

Add these tests near existing start-run tests in `go/eval-mcp-service/internal/evalworkflow/service_test.go`:

```go
func TestCheckReadinessDelegatesToEvalAPI(t *testing.T) {
	api := &fakeAPI{readinessResponse: evalapi.RAGReadinessResponse{Status: "ready"}}
	svc := newTestService(api)

	got, err := svc.CheckReadiness(context.Background(), ReadinessInput{
		DatasetID:  "ds-1",
		Collection: "documents",
		Rerank:     true,
	})
	if err != nil {
		t.Fatalf("CheckReadiness error: %v", err)
	}
	if got.Status != "ready" {
		t.Fatalf("readiness = %#v", got)
	}
	if api.readinessRequest.DatasetID != "ds-1" || api.readinessRequest.Collection != "documents" || !api.readinessRequest.Rerank {
		t.Fatalf("request = %#v", api.readinessRequest)
	}
}

func TestStartRunBlocksWhenReadinessBlocked(t *testing.T) {
	api := &fakeAPI{readinessResponse: evalapi.RAGReadinessResponse{
		Status: "blocked",
		BlockingFailures: []evalapi.ReadinessFinding{{
			Code: "collection_empty",
		}},
	}}
	svc := newTestService(api)

	_, err := svc.StartRun(context.Background(), StartRunInput{
		DatasetID:  "ds-1",
		Collection: "documents",
	})
	if err == nil || !strings.Contains(err.Error(), "readiness blocked") || !strings.Contains(err.Error(), "collection_empty") {
		t.Fatalf("error = %v", err)
	}
	if len(api.startRequests) != 0 {
		t.Fatalf("StartEvaluation calls = %d, want 0", len(api.startRequests))
	}
}

func TestStartRunAllowsReadinessWarning(t *testing.T) {
	api := &fakeAPI{
		readinessResponse: evalapi.RAGReadinessResponse{
			Status:   "warning",
			Warnings: []evalapi.ReadinessFinding{{Code: "top_k_override"}},
		},
		startResponse: evalapi.StartEvaluationResponse{ID: "eval-1", Status: "queued"},
	}
	svc := newTestService(api)

	got, err := svc.StartRun(context.Background(), StartRunInput{
		DatasetID:  "ds-1",
		Collection: "documents",
	})
	if err != nil {
		t.Fatalf("StartRun error: %v", err)
	}
	if got.EvalID != "eval-1" || len(api.startRequests) != 1 {
		t.Fatalf("result=%#v startRequests=%d", got, len(api.startRequests))
	}
}

func TestStartExperimentBlocksWhenReadinessBlocked(t *testing.T) {
	api := &fakeAPI{readinessResponse: evalapi.RAGReadinessResponse{
		Status: "blocked",
		BlockingFailures: []evalapi.ReadinessFinding{{Code: "collection_empty"}},
	}}
	svc := newTestService(api)

	_, err := svc.StartExperiment(context.Background(), StartExperimentInput{
		Name:      "experiment",
		DatasetID: "ds-1",
		Collection: "documents",
	})
	if err == nil || !strings.Contains(err.Error(), "readiness blocked") {
		t.Fatalf("error = %v", err)
	}
	if len(api.createExperimentRequests) != 0 {
		t.Fatalf("CreateExperiment calls = %d, want 0", len(api.createExperimentRequests))
	}
}
```

Update the fake API in the same file:

```go
readinessRequest  evalapi.RAGReadinessRequest
readinessResponse evalapi.RAGReadinessResponse
```

and add method:

```go
func (f *fakeAPI) CheckRAGReadiness(_ context.Context, in evalapi.RAGReadinessRequest) (evalapi.RAGReadinessResponse, error) {
	f.readinessRequest = in
	if f.readinessResponse.Status == "" {
		return evalapi.RAGReadinessResponse{Status: "ready"}, nil
	}
	return f.readinessResponse, nil
}
```

- [ ] **Step 2: Run workflow tests and verify they fail**

Run:

```bash
cd go/eval-mcp-service
go test ./internal/evalworkflow -run 'Test(CheckReadinessDelegatesToEvalAPI|StartRunBlocksWhenReadinessBlocked|StartRunAllowsReadinessWarning|StartExperimentBlocksWhenReadinessBlocked)' -count=1
```

Expected: FAIL with undefined `CheckReadiness`, `ReadinessInput`, or fake API interface mismatch.

- [ ] **Step 3: Add workflow readiness types and API method**

In `go/eval-mcp-service/internal/evalworkflow/service.go`, add `CheckRAGReadiness` to `API`:

```go
	CheckRAGReadiness(context.Context, evalapi.RAGReadinessRequest) (evalapi.RAGReadinessResponse, error)
```

Add input type near `StartRunInput`:

```go
type ReadinessInput struct {
	DatasetID       string
	Collection      string
	Rerank          bool
	RetrievalConfig *evalapi.RetrievalConfig
	BaselineEvalID  string
	ExperimentID    string
}
```

Add methods:

```go
func (s *Service) CheckReadiness(ctx context.Context, in ReadinessInput) (evalapi.RAGReadinessResponse, error) {
	collection := in.Collection
	if collection == "" {
		collection = DefaultCollection
	}
	return s.api.CheckRAGReadiness(ctx, evalapi.RAGReadinessRequest{
		DatasetID:       in.DatasetID,
		Collection:      collection,
		Rerank:          in.Rerank,
		RetrievalConfig: in.RetrievalConfig,
		BaselineEvalID:  in.BaselineEvalID,
		ExperimentID:    in.ExperimentID,
	})
}

func ensureReadinessAllowed(readiness evalapi.RAGReadinessResponse) error {
	if readiness.Status != "blocked" {
		return nil
	}
	codes := make([]string, 0, len(readiness.BlockingFailures))
	for _, finding := range readiness.BlockingFailures {
		codes = append(codes, finding.Code)
	}
	if len(codes) == 0 {
		codes = append(codes, "unknown")
	}
	return fmt.Errorf("readiness blocked: %s", strings.Join(codes, ", "))
}
```

- [ ] **Step 4: Guard `StartExperiment` and `StartRun`**

In `StartExperiment`, after collection/focus metric validation and before `CreateExperiment`, call:

```go
	readiness, err := s.CheckReadiness(ctx, ReadinessInput{
		DatasetID:      in.DatasetID,
		Collection:     collection,
		BaselineEvalID: in.BaselineEvalID,
	})
	if err != nil {
		return evalapi.Experiment{}, err
	}
	if err := ensureReadinessAllowed(readiness); err != nil {
		return evalapi.Experiment{}, err
	}
```

Remove the older `validateCollectionExists` call from `StartExperiment`.

In `StartRun`, replace the old `validateCollectionExists` call with:

```go
	readiness, err := s.CheckReadiness(ctx, ReadinessInput{
		DatasetID:       in.DatasetID,
		Collection:      in.Collection,
		Rerank:          in.Rerank,
		RetrievalConfig: in.RetrievalConfig,
		BaselineEvalID:  in.BaselineEvalID,
		ExperimentID:    in.ExperimentID,
	})
	if err != nil {
		return StartRunResult{}, err
	}
	if err := ensureReadinessAllowed(readiness); err != nil {
		return StartRunResult{}, err
	}
```

Keep `validateCollectionExists` if other code still uses it; otherwise remove it after tests confirm it is unused.

- [ ] **Step 5: Run workflow tests**

Run:

```bash
cd go/eval-mcp-service
go test ./internal/evalworkflow -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit Task 5**

```bash
git add go/eval-mcp-service/internal/evalworkflow/service.go go/eval-mcp-service/internal/evalworkflow/service_test.go
git commit -m "feat(eval-mcp): guard workflows with readiness"
```

---

### Task 6: Add MCP Readiness Tool And Workflow Instructions

**Files:**
- Modify: `go/eval-mcp-service/internal/mcpserver/server.go`
- Modify: `go/eval-mcp-service/internal/mcpserver/server_test.go`

- [ ] **Step 1: Write failing MCP tests**

In `go/eval-mcp-service/internal/mcpserver/server_test.go`, add `readinessInput evalworkflow.ReadinessInput` to `fakeEvalService` and method:

```go
func (f *fakeEvalService) CheckReadiness(_ context.Context, in evalworkflow.ReadinessInput) (evalapi.RAGReadinessResponse, error) {
	f.readinessInput = in
	return evalapi.RAGReadinessResponse{Status: "ready", NextSteps: []string{"Proceed with the eval run."}}, nil
}
```

Add tests:

```go
func TestCheckRAGReadinessHandler(t *testing.T) {
	fake := &fakeEvalService{}
	result, err := checkRAGReadinessHandler(fake)(context.Background(), callReq(map[string]any{
		"dataset_id": "ds-1",
		"collection": "documents",
		"rerank": true,
		"retrieval_config": map[string]any{"top_k": float64(3)},
	}))
	if err != nil {
		t.Fatalf("handler returned transport error: %v", err)
	}
	if result.IsError {
		t.Fatalf("handler returned MCP error: %#v", result)
	}
	if fake.readinessInput.DatasetID != "ds-1" || fake.readinessInput.Collection != "documents" || !fake.readinessInput.Rerank {
		t.Fatalf("readiness input = %#v", fake.readinessInput)
	}
	if fake.readinessInput.RetrievalConfig == nil || fake.readinessInput.RetrievalConfig.TopK == nil || *fake.readinessInput.RetrievalConfig.TopK != 3 {
		t.Fatalf("retrieval config = %#v", fake.readinessInput.RetrievalConfig)
	}
}

func TestCheckRAGReadinessHandlerRequiresDatasetAndCollection(t *testing.T) {
	fake := &fakeEvalService{}
	result, err := checkRAGReadinessHandler(fake)(context.Background(), callReq(map[string]any{
		"dataset_id": "ds-1",
	}))
	if err != nil {
		t.Fatalf("handler returned transport error: %v", err)
	}
	if !result.IsError || !strings.Contains(textResult(t, result), "collection is required") {
		t.Fatalf("result = %#v", result)
	}
}

func TestCheckRAGReadinessSchemaIncludesRetrievalConfig(t *testing.T) {
	var schema struct {
		Properties map[string]map[string]any `json:"properties"`
		Required   []string                  `json:"required"`
	}
	if err := json.Unmarshal(checkRAGReadinessSchema(), &schema); err != nil {
		t.Fatalf("schema is invalid JSON: %v", err)
	}
	if !reflect.DeepEqual(schema.Required, []string{"dataset_id", "collection"}) {
		t.Fatalf("required = %#v", schema.Required)
	}
	if _, ok := schema.Properties["retrieval_config"]; !ok {
		t.Fatalf("missing retrieval_config property")
	}
}
```

Update `TestServerRegistersPromptResourceAndTools` expected tool list to include `check_rag_eval_readiness` in alphabetical order.

- [ ] **Step 2: Run MCP tests and verify they fail**

Run:

```bash
cd go/eval-mcp-service
go test ./internal/mcpserver -run 'Test(CheckRAGReadiness|ServerRegistersPromptResourceAndTools)' -count=1
```

Expected: FAIL with undefined handler/schema and missing tool.

- [ ] **Step 3: Add MCP interface method, tool registration, handler, and schema**

In `EvalService`, add:

```go
	CheckReadiness(context.Context, evalworkflow.ReadinessInput) (evalapi.RAGReadinessResponse, error)
```

In `New`, add:

```go
	addTool(srv, "check_rag_eval_readiness", "Check whether a dataset and retrieval collection are ready for a RAG eval run.", checkRAGReadinessSchema(), checkRAGReadinessHandler(service))
```

Add handler near `getRAGCollectionConfigHandler`:

```go
func checkRAGReadinessHandler(service EvalService) sdkmcp.ToolHandler {
	return func(ctx context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		var args struct {
			DatasetID       string          `json:"dataset_id"`
			Collection      string          `json:"collection"`
			Rerank          bool            `json:"rerank,omitempty"`
			RetrievalConfig json.RawMessage `json:"retrieval_config,omitempty"`
			BaselineEvalID  string          `json:"baseline_eval_id,omitempty"`
			ExperimentID    string          `json:"experiment_id,omitempty"`
		}
		if err := decodeArgs(req, &args); err != nil {
			return toolError(err.Error()), nil
		}
		if strings.TrimSpace(args.DatasetID) == "" {
			return toolError("dataset_id is required"), nil
		}
		if strings.TrimSpace(args.Collection) == "" {
			return toolError("collection is required"), nil
		}
		retrievalConfig, err := parseRetrievalConfig(args.RetrievalConfig)
		if err != nil {
			return toolError(err.Error()), nil
		}
		result, err := service.CheckReadiness(ctx, evalworkflow.ReadinessInput{
			DatasetID:       args.DatasetID,
			Collection:      args.Collection,
			Rerank:          args.Rerank,
			RetrievalConfig: retrievalConfig,
			BaselineEvalID:  args.BaselineEvalID,
			ExperimentID:    args.ExperimentID,
		})
		return resultOrError(result, err), nil
	}
}
```

Add schema near `startEvalRunSchema`:

```go
func checkRAGReadinessSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"dataset_id":{"type":"string"},"collection":{"type":"string"},"rerank":{"type":"boolean"},"retrieval_config":{"type":"object","properties":{"top_k":{"type":"integer","minimum":1,"maximum":20}},"additionalProperties":false},"baseline_eval_id":{"type":"string"},"experiment_id":{"type":"string"}},"required":["dataset_id","collection"],"additionalProperties":false}`)
}
```

- [ ] **Step 4: Update MCP prompt and workflow resource**

In `evalPromptHandler`, replace the sentence that says:

```text
Use list_eval_dataset_fixtures and create_eval_dataset for curated repo fixtures, list_eval_datasets to choose existing API data, then use list_rag_collections and get_rag_collection_config before start_eval_experiment or start_eval_run.
```

with:

```text
Use list_eval_dataset_fixtures and create_eval_dataset for curated repo fixtures, list_eval_datasets to choose existing API data, then call check_rag_eval_readiness before start_eval_experiment or start_eval_run. Treat blocked readiness as a stop condition and warning readiness as a caveated run condition.
```

In `evalWorkflowInstructions`, replace steps 4-6 with:

```text
4. Call check_rag_eval_readiness before start_eval_experiment or start_eval_run. Treat blocked readiness as a stop condition and warning readiness as a caveated run condition.
5. Start baseline with start_eval_run only after readiness is ready or warning, then call wait_for_eval_run. Run baseline to completion before starting rerank while runtime hardening is pending.
6. Start candidate runs with start_eval_run only after readiness is ready or warning, then call wait_for_eval_run until each run completes or fails.
```

- [ ] **Step 5: Run MCP tests**

Run:

```bash
cd go/eval-mcp-service
go test ./internal/mcpserver -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit Task 6**

```bash
git add go/eval-mcp-service/internal/mcpserver/server.go go/eval-mcp-service/internal/mcpserver/server_test.go
git commit -m "feat(eval-mcp): expose rag readiness tool"
```

---

### Task 7: Verification And PR Prep

**Files:**
- Verify all changed files.

- [ ] **Step 1: Run targeted Python tests**

Run:

```bash
cd services/ingestion
pytest tests/test_store.py tests/test_main.py -q
cd ../eval
pytest tests/test_readiness.py tests/test_main.py tests/test_models.py tests/test_config_capture.py -q
```

Expected: PASS.

- [ ] **Step 2: Run targeted Go tests**

Run:

```bash
cd go/eval-mcp-service
go test ./internal/evalapi ./internal/ingestionapi ./internal/evalworkflow ./internal/mcpserver -count=1
```

Expected: PASS.

- [ ] **Step 3: Run required preflights**

From repo root:

```bash
make preflight-python
make preflight-security
make preflight-go
```

Expected: PASS. If a tool is missing locally, capture the exact missing-tool error and leave remaining verification to CI.

- [ ] **Step 4: Inspect final diff**

Run:

```bash
git status --short
git diff --stat qa...HEAD
git diff --check
```

Expected:

- `git status --short` shows only intentional tracked changes before final commit, then clean after commit.
- `git diff --check` exits 0 with no whitespace errors.

- [ ] **Step 5: Final commit if any changes remain**

If verification required fixes after Task 6:

```bash
git add services go
git commit -m "test: verify rag eval readiness gate"
```

If there are no remaining changes, skip this commit.

- [ ] **Step 6: Push and open PR to `qa`**

```bash
git push -u origin feat/rag-eval-readiness-gate
gh pr create --base qa --head feat/rag-eval-readiness-gate --title "Add RAG eval readiness gate" --body "## Summary
- add ingestion source inventory for collection readiness
- add eval API readiness policy and blocking/warning gate
- expose readiness through eval MCP and guard eval workflows

## Verification
- pytest ingestion/eval readiness targets
- go test eval-mcp readiness packages
- make preflight-python
- make preflight-security
- make preflight-go"
```

Per repo workflow, do not watch CI unless Kyle asks.

---

## Self-Review

Spec coverage:

- Blocking vs warning readiness policy: Tasks 2, 3, 5.
- Eval API source of truth: Tasks 2 and 3.
- Ingestion source inventory: Task 1.
- MCP explicit readiness tool: Task 6.
- MCP internal guard on experiment/run start: Task 5.
- Persist readiness in run config: Task 3.
- Evidence in run evidence and summaries through config: Task 3 keeps readiness in existing config returned by eval API and Go `RunEvidence`.
- No frontend work: explicitly deferred.

Red-flag scan:

- No unresolved markers remain in the implementation steps.
- Each code-changing task includes concrete test code, implementation code, commands, and expected result.

Type consistency:

- Python request/response models use `RAGReadinessRequest`, `RAGReadinessResponse`, and `ReadinessFinding`.
- Go client mirrors those names as `RAGReadinessRequest`, `RAGReadinessResponse`, and `ReadinessFinding`.
- MCP workflow uses `ReadinessInput` and returns `evalapi.RAGReadinessResponse`.
