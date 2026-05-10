# Cross-Encoder Re-Ranking Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add opt-in local cross-encoder re-ranking to the Python RAG chat and eval paths for issue #81.

**Architecture:** `services/chat` keeps Qdrant hybrid retrieval as the candidate generator, then optionally applies an in-process `sentence-transformers` `CrossEncoder` to reorder candidates. `/search`, `/chat`, and eval requests default to `rerank=false`, so existing behavior remains unchanged until explicitly requested.

**Tech Stack:** Python 3.11, FastAPI, Pydantic settings, Qdrant client, Prometheus client, `sentence-transformers==5.4.1`, `cross-encoder/ms-marco-MiniLM-L6-v2`.

---

## File Structure

- Create `services/chat/app/reranker.py`: lazy local cross-encoder loader, scoring, stable sorting, metrics/error helpers.
- Create `services/chat/tests/test_reranker.py`: isolated tests for scoring order, stable ties, empty inputs, and model load behavior.
- Modify `services/chat/app/config.py`: add re-ranker settings and validation.
- Modify `services/chat/app/metrics.py`: add re-ranker duration, candidate-count, and fallback metrics.
- Modify `services/chat/app/chain.py`: compute candidate count, call re-ranker when requested, preserve fallback behavior.
- Modify `services/chat/app/main.py`: add `rerank` request fields, expose config, thread request flags.
- Modify `services/chat/requirements.txt`: pin `sentence-transformers==5.4.1`.
- Modify `services/chat/tests/test_config.py`: cover re-ranker config validation.
- Modify `services/chat/tests/test_chain.py`: cover chain orchestration and fallback metadata.
- Modify `services/chat/tests/test_main.py`: cover `/config`, `/search`, `/chat` request threading.
- Modify `services/eval/app/models.py`: add `rerank: bool = False` to `StartEvaluationRequest`.
- Modify `services/eval/app/rag_client.py`: pass `rerank` to `/search` and `/chat`.
- Modify `services/eval/app/evaluator.py`: accept and pass `rerank` through dataset construction.
- Modify `services/eval/app/main.py`: pass request-level `rerank` into background evaluation task.
- Modify `services/eval/tests/test_models.py`, `services/eval/tests/test_rag_client.py`, `services/eval/tests/test_evaluator.py`, and `services/eval/tests/test_main.py`: cover eval request threading.

## External References

- Sentence Transformers documents `CrossEncoder` as the API for pairwise scoring and re-ranking. It jointly processes sentence pairs and outputs scores, which matches this feature's design.
- Hugging Face model card for `cross-encoder/ms-marco-MiniLM-L6-v2` lists Apache-2.0 licensing, Sentence Transformers usage, MS MARCO passage ranking training, and 22.7M parameters.
- PyPI currently publishes `sentence-transformers==5.4.1`; use an exact pin and let `make preflight-security` validate the dependency graph.

## Task 1: Dependency And Model Preflight

**Files:**
- Modify: `services/chat/requirements.txt`

- [ ] **Step 1: Confirm current package metadata**

Run:

```bash
python -m pip index versions sentence-transformers
```

Expected: output includes `5.4.1` in the available versions list.

- [ ] **Step 2: Add the pinned dependency**

Append this line to `services/chat/requirements.txt`:

```text
sentence-transformers==5.4.1
```

- [ ] **Step 3: Run the chat dependency install check**

Run:

```bash
python -m venv /tmp/chat-rerank-venv
/tmp/chat-rerank-venv/bin/python -m pip install --upgrade pip
/tmp/chat-rerank-venv/bin/pip install -r services/chat/requirements.txt
```

Expected: install succeeds. If it fails because of platform wheel issues, remove the dependency line and stop for design review.

- [ ] **Step 4: Run security preflight early**

Run:

```bash
make preflight-security
```

Expected: exits 0. If new advisories appear from `sentence-transformers` or transitive dependencies, remove the dependency line and stop for model/package reassessment.

- [ ] **Step 5: Commit the dependency pin**

Run:

```bash
git add services/chat/requirements.txt
git commit -m "build: add chat reranker dependency"
```

Expected: commit succeeds.

## Task 2: Re-Ranker Unit

**Files:**
- Create: `services/chat/app/reranker.py`
- Create: `services/chat/tests/test_reranker.py`

- [ ] **Step 1: Write failing tests for scoring, stable ties, and empty input**

Create `services/chat/tests/test_reranker.py` with:

```python
from unittest.mock import MagicMock

from app.reranker import CrossEncoderReranker, rerank_chunks


def _chunk(text: str, score: float = 0.1) -> dict:
    return {
        "text": text,
        "page_number": 1,
        "filename": "doc.pdf",
        "document_id": "doc",
        "score": score,
    }


def test_rerank_chunks_sorts_by_cross_encoder_score_descending():
    model = MagicMock()
    model.predict.return_value = [0.2, 0.9, 0.4]
    reranker = CrossEncoderReranker(model_loader=lambda: model)

    result = reranker.rerank(
        query="where is the answer?",
        chunks=[_chunk("weak"), _chunk("best"), _chunk("middle")],
        top_k=2,
        model_name="test-model",
    )

    assert [c["text"] for c in result.chunks] == ["best", "middle"]
    assert [c["rerank_score"] for c in result.chunks] == [0.9, 0.4]
    model.predict.assert_called_once_with(
        [
            ("where is the answer?", "weak"),
            ("where is the answer?", "best"),
            ("where is the answer?", "middle"),
        ],
        show_progress_bar=False,
    )


def test_rerank_chunks_preserves_original_order_for_equal_scores():
    model = MagicMock()
    model.predict.return_value = [0.5, 0.5, 0.5]
    reranker = CrossEncoderReranker(model_loader=lambda: model)

    result = reranker.rerank(
        query="same score",
        chunks=[_chunk("first"), _chunk("second"), _chunk("third")],
        top_k=3,
        model_name="test-model",
    )

    assert [c["text"] for c in result.chunks] == ["first", "second", "third"]


def test_rerank_chunks_empty_input_does_not_load_model():
    model_loader = MagicMock()
    reranker = CrossEncoderReranker(model_loader=model_loader)

    result = reranker.rerank(
        query="anything",
        chunks=[],
        top_k=5,
        model_name="test-model",
    )

    assert result.chunks == []
    assert result.metadata == {
        "rerank_applied": False,
        "rerank_model": "test-model",
        "rerank_candidate_count": 0,
        "rerank_returned_count": 0,
    }
    model_loader.assert_not_called()


def test_rerank_chunks_uses_default_singleton_loader(monkeypatch):
    model = MagicMock()
    model.predict.return_value = [0.8]
    loader = MagicMock(return_value=model)
    monkeypatch.setattr("app.reranker.get_cross_encoder", loader)

    result = rerank_chunks(
        query="q",
        chunks=[_chunk("answer")],
        top_k=1,
        model_name="test-model",
    )

    assert result.chunks[0]["text"] == "answer"
    loader.assert_called_once_with("test-model", "cpu")
```

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
cd services/chat && python -m pytest tests/test_reranker.py -v
```

Expected: fails with `ModuleNotFoundError: No module named 'app.reranker'`.

- [ ] **Step 3: Implement `reranker.py`**

Create `services/chat/app/reranker.py` with:

```python
from __future__ import annotations

import time
from dataclasses import dataclass
from typing import Callable

from sentence_transformers import CrossEncoder

from app.metrics import RERANK_CANDIDATES, RERANK_DURATION


@dataclass(frozen=True)
class RerankResult:
    chunks: list[dict]
    metadata: dict


_models: dict[tuple[str, str], CrossEncoder] = {}


def get_cross_encoder(model_name: str, device: str) -> CrossEncoder:
    key = (model_name, device)
    if key not in _models:
        _models[key] = CrossEncoder(model_name, device=device)
    return _models[key]


class CrossEncoderReranker:
    def __init__(self, model_loader: Callable[[], object]):
        self._model_loader = model_loader

    def rerank(
        self,
        query: str,
        chunks: list[dict],
        top_k: int,
        model_name: str,
    ) -> RerankResult:
        if not chunks:
            return RerankResult(
                chunks=[],
                metadata={
                    "rerank_applied": False,
                    "rerank_model": model_name,
                    "rerank_candidate_count": 0,
                    "rerank_returned_count": 0,
                },
            )

        start = time.perf_counter()
        model = self._model_loader()
        pairs = [(query, chunk["text"]) for chunk in chunks]
        scores = [float(score) for score in model.predict(pairs, show_progress_bar=False)]
        ranked = sorted(
            enumerate(zip(chunks, scores, strict=True)),
            key=lambda item: (-item[1][1], item[0]),
        )
        reranked = []
        for _, (chunk, score) in ranked[:top_k]:
            updated = dict(chunk)
            updated["rerank_score"] = score
            reranked.append(updated)

        RERANK_DURATION.labels(model=model_name, outcome="applied").observe(
            time.perf_counter() - start
        )
        RERANK_CANDIDATES.labels(model=model_name).observe(len(chunks))

        return RerankResult(
            chunks=reranked,
            metadata={
                "rerank_applied": True,
                "rerank_model": model_name,
                "rerank_candidate_count": len(chunks),
                "rerank_returned_count": len(reranked),
            },
        )


def rerank_chunks(
    query: str,
    chunks: list[dict],
    top_k: int,
    model_name: str,
    device: str = "cpu",
) -> RerankResult:
    reranker = CrossEncoderReranker(
        model_loader=lambda: get_cross_encoder(model_name, device)
    )
    return reranker.rerank(
        query=query,
        chunks=chunks,
        top_k=top_k,
        model_name=model_name,
    )
```

- [ ] **Step 4: Run tests and observe metrics import failure**

Run:

```bash
cd services/chat && python -m pytest tests/test_reranker.py -v
```

Expected: fails because `RERANK_CANDIDATES` and `RERANK_DURATION` are not defined yet.

- [ ] **Step 5: Commit the failing re-ranker slice only if your workflow allows red commits**

Do not commit this task yet if following green-only commits. Continue directly to Task 3 and commit Tasks 2-3 together after tests pass.

## Task 3: Re-Ranker Config And Metrics

**Files:**
- Modify: `services/chat/app/config.py`
- Modify: `services/chat/app/metrics.py`
- Modify: `services/chat/tests/test_config.py`

- [ ] **Step 1: Add failing config validation tests**

Append to `services/chat/tests/test_config.py`:

```python
def test_settings_validate_rejects_rerank_candidate_limit_below_top_k():
    settings = Settings(top_k=10, rerank_candidate_limit=9)

    with pytest.raises(ValueError, match="rerank_candidate_limit"):
        settings.validate()


def test_settings_validate_rejects_rerank_max_candidates_below_candidate_limit():
    settings = Settings(rerank_candidate_limit=20, rerank_max_candidates=19)

    with pytest.raises(ValueError, match="rerank_max_candidates"):
        settings.validate()
```

- [ ] **Step 2: Run config tests to verify failure**

Run:

```bash
cd services/chat && python -m pytest tests/test_config.py -v
```

Expected: fails because `Settings` does not accept or validate re-ranker fields.

- [ ] **Step 3: Add config fields and validation**

In `services/chat/app/config.py`, after `hybrid_prefetch_limit`, add:

```python
    rerank_enabled: bool = True
    rerank_model: str = "cross-encoder/ms-marco-MiniLM-L6-v2"
    rerank_candidate_limit: int = 20
    rerank_max_candidates: int = 50
    rerank_device: str = "cpu"
```

In `validate()`, after the hybrid prefetch validation, add:

```python
        if self.rerank_candidate_limit < self.top_k:
            raise ValueError("rerank_candidate_limit must be >= top_k")
        if self.rerank_max_candidates < self.rerank_candidate_limit:
            raise ValueError(
                "rerank_max_candidates must be >= rerank_candidate_limit"
            )
```

- [ ] **Step 4: Add metrics**

In `services/chat/app/metrics.py`, after `RAG_PIPELINE_ERRORS`, add:

```python
RERANK_DURATION = Histogram(
    "rag_rerank_duration_seconds",
    "Time spent scoring retrieval candidates with the cross-encoder re-ranker",
    ["model", "outcome"],
    buckets=(0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0),
)

RERANK_CANDIDATES = Histogram(
    "rag_rerank_candidates",
    "Number of retrieval candidates sent to the cross-encoder re-ranker",
    ["model"],
    buckets=(0, 1, 2, 3, 5, 10, 20, 50),
)

RERANK_FALLBACKS = Counter(
    "rag_rerank_fallbacks_total",
    "Re-ranker fallback count by reason",
    ["reason"],
)
```

- [ ] **Step 5: Run re-ranker and config tests**

Run:

```bash
cd services/chat && python -m pytest tests/test_reranker.py tests/test_config.py -v
```

Expected: all tests pass.

- [ ] **Step 6: Commit re-ranker, config, and metrics**

Run:

```bash
git add services/chat/app/reranker.py services/chat/tests/test_reranker.py services/chat/app/config.py services/chat/app/metrics.py services/chat/tests/test_config.py
git commit -m "feat: add local cross-encoder reranker"
```

Expected: commit succeeds.

## Task 4: Chat Chain Integration

**Files:**
- Modify: `services/chat/app/chain.py`
- Modify: `services/chat/tests/test_chain.py`

- [ ] **Step 1: Add failing tests for opt-in candidate expansion and re-ranking**

Append to `services/chat/tests/test_chain.py`:

```python
@patch("app.chain.rerank_chunks")
@patch("app.chain.get_sparse_encoder", create=True)
@patch("app.chain.QdrantRetriever")
@pytest.mark.asyncio
async def test_retrieve_chunks_rerank_true_uses_larger_candidate_pool(
    mock_retriever_cls,
    mock_sparse_encoder,
    mock_rerank,
    monkeypatch,
):
    monkeypatch.setattr("app.chain.settings.retrieval_mode", "hybrid")
    monkeypatch.setattr("app.chain.settings.rerank_enabled", True)
    monkeypatch.setattr("app.chain.settings.rerank_candidate_limit", 20)
    monkeypatch.setattr("app.chain.settings.rerank_max_candidates", 50)
    monkeypatch.setattr(
        "app.chain.settings.rerank_model", "cross-encoder/ms-marco-MiniLM-L6-v2"
    )
    monkeypatch.setattr("app.chain.settings.rerank_device", "cpu")

    embedding_provider = AsyncMock()
    embedding_provider.embed.return_value = [[0.1] * 768]
    sparse_vector = MagicMock()
    mock_sparse_encoder.return_value.embed.return_value = [sparse_vector]
    candidates = [
        {
            "text": f"chunk {i}",
            "page_number": i,
            "filename": "doc.pdf",
            "document_id": "doc",
            "score": 1.0 - (i / 100),
        }
        for i in range(20)
    ]
    retriever = MagicMock()
    retriever.search_hybrid.return_value = hybrid_result(candidates)
    mock_retriever_cls.return_value = retriever
    mock_rerank.return_value.chunks = [candidates[3], candidates[1], candidates[0]]
    mock_rerank.return_value.metadata = {
        "rerank_applied": True,
        "rerank_model": "cross-encoder/ms-marco-MiniLM-L6-v2",
        "rerank_candidate_count": 20,
        "rerank_returned_count": 3,
    }

    result = await retrieve_chunks(
        question="best chunk?",
        embedding_provider=embedding_provider,
        embedding_model="nomic-embed-text",
        qdrant_host="localhost",
        qdrant_port=6333,
        collection_name="documents",
        top_k=3,
        rerank=True,
    )

    retriever.search_hybrid.assert_called_once_with(
        query_vector=[0.1] * 768,
        sparse_vector=sparse_vector,
        top_k=20,
        prefetch_limit=20,
    )
    mock_rerank.assert_called_once_with(
        query="best chunk?",
        chunks=candidates,
        top_k=3,
        model_name="cross-encoder/ms-marco-MiniLM-L6-v2",
        device="cpu",
    )
    assert [c["text"] for c in result.chunks] == ["chunk 3", "chunk 1", "chunk 0"]
    assert result.metadata["rerank_requested"] is True
    assert result.metadata["rerank_applied"] is True
    assert result.metadata["rerank_fallback"] is False


@patch("app.chain.rerank_chunks")
@patch("app.chain.get_sparse_encoder", create=True)
@patch("app.chain.QdrantRetriever")
@pytest.mark.asyncio
async def test_retrieve_chunks_rerank_failure_falls_back_to_original_order(
    mock_retriever_cls,
    mock_sparse_encoder,
    mock_rerank,
    monkeypatch,
):
    monkeypatch.setattr("app.chain.settings.retrieval_mode", "hybrid")
    monkeypatch.setattr("app.chain.settings.rerank_enabled", True)
    monkeypatch.setattr("app.chain.settings.rerank_candidate_limit", 20)
    monkeypatch.setattr("app.chain.settings.rerank_max_candidates", 50)
    monkeypatch.setattr(
        "app.chain.settings.rerank_model", "cross-encoder/ms-marco-MiniLM-L6-v2"
    )
    monkeypatch.setattr("app.chain.settings.rerank_device", "cpu")

    embedding_provider = AsyncMock()
    embedding_provider.embed.return_value = [[0.1] * 768]
    mock_sparse_encoder.return_value.embed.return_value = [MagicMock()]
    candidates = [
        {
            "text": f"chunk {i}",
            "page_number": i,
            "filename": "doc.pdf",
            "document_id": "doc",
            "score": 1.0,
        }
        for i in range(5)
    ]
    retriever = MagicMock()
    retriever.search_hybrid.return_value = hybrid_result(candidates)
    mock_retriever_cls.return_value = retriever
    mock_rerank.side_effect = RuntimeError("model unavailable")

    result = await retrieve_chunks(
        question="q",
        embedding_provider=embedding_provider,
        embedding_model="nomic-embed-text",
        qdrant_host="localhost",
        qdrant_port=6333,
        collection_name="documents",
        top_k=2,
        rerank=True,
    )

    assert [c["text"] for c in result.chunks] == ["chunk 0", "chunk 1"]
    assert result.metadata["rerank_requested"] is True
    assert result.metadata["rerank_applied"] is False
    assert result.metadata["rerank_fallback"] is True
    assert result.metadata["rerank_error"] == "RuntimeError"


@patch("app.chain.rerank_chunks")
@patch("app.chain.QdrantRetriever")
@pytest.mark.asyncio
async def test_retrieve_chunks_rerank_disabled_does_not_call_reranker(
    mock_retriever_cls,
    mock_rerank,
    monkeypatch,
):
    monkeypatch.setattr("app.chain.settings.retrieval_mode", "semantic")
    monkeypatch.setattr("app.chain.settings.rerank_enabled", False)
    embedding_provider = AsyncMock()
    embedding_provider.embed.return_value = [[0.1] * 768]
    retriever = MagicMock()
    retriever.search_semantic.return_value = semantic_fallback_result(
        [{"text": "a", "page_number": 1, "filename": "doc.pdf", "document_id": "d", "score": 0.1}]
    )
    mock_retriever_cls.return_value = retriever

    result = await retrieve_chunks(
        question="q",
        embedding_provider=embedding_provider,
        embedding_model="nomic-embed-text",
        qdrant_host="localhost",
        qdrant_port=6333,
        collection_name="documents",
        top_k=1,
        rerank=True,
    )

    mock_rerank.assert_not_called()
    assert result.metadata["rerank_requested"] is True
    assert result.metadata["rerank_applied"] is False
    assert result.metadata["rerank_enabled"] is False
```

- [ ] **Step 2: Run the new chain tests to verify failure**

Run:

```bash
cd services/chat && python -m pytest tests/test_chain.py -k rerank -v
```

Expected: fails because `retrieve_chunks` has no `rerank` argument and `app.chain.rerank_chunks` is missing.

- [ ] **Step 3: Import re-ranker and fallback metrics**

In `services/chat/app/chain.py`, add imports:

```python
from app.metrics import RERANK_FALLBACKS
from app.reranker import rerank_chunks
```

Keep the existing multi-line `from app.metrics import (...)` and include `RERANK_FALLBACKS` in that block instead of creating a second metrics import.

- [ ] **Step 4: Add helper functions above `retrieve_chunks`**

Add:

```python
def _rerank_candidate_limit(top_k: int) -> int:
    return min(
        max(top_k, settings.rerank_candidate_limit),
        settings.rerank_max_candidates,
    )


def _with_rerank_metadata(
    retrieval: RetrievalResult,
    metadata: dict,
) -> RetrievalResult:
    merged = dict(retrieval.metadata)
    merged.update(metadata)
    return RetrievalResult(chunks=retrieval.chunks, metadata=merged)
```

- [ ] **Step 5: Update `retrieve_chunks` signature and candidate count**

Change the signature to:

```python
async def retrieve_chunks(
    question: str,
    embedding_provider: EmbeddingProvider,
    embedding_model: str,
    qdrant_host: str,
    qdrant_port: int,
    collection_name: str,
    top_k: int = 5,
    rerank: bool = False,
) -> RetrievalResult:
```

After creating the retriever, add:

```python
    retrieval_top_k = _rerank_candidate_limit(top_k) if rerank else top_k
```

Replace all retrieval calls in this function that currently pass `top_k=top_k`
with `top_k=retrieval_top_k`.

- [ ] **Step 6: Apply re-ranking before returning**

Before recording `RAG_PIPELINE_DURATION`, add:

```python
    if rerank and not settings.rerank_enabled:
        result = _with_rerank_metadata(
            result,
            {
                "rerank_requested": True,
                "rerank_enabled": False,
                "rerank_applied": False,
                "rerank_model": settings.rerank_model,
                "rerank_candidate_count": len(result.chunks),
                "rerank_returned_count": min(len(result.chunks), top_k),
                "rerank_fallback": False,
            },
        )
        result = RetrievalResult(chunks=result.chunks[:top_k], metadata=result.metadata)
    elif rerank:
        try:
            rerank_result = rerank_chunks(
                query=question,
                chunks=result.chunks,
                top_k=top_k,
                model_name=settings.rerank_model,
                device=settings.rerank_device,
            )
            metadata = {
                "rerank_requested": True,
                "rerank_enabled": True,
                "rerank_fallback": False,
            }
            metadata.update(rerank_result.metadata)
            result = RetrievalResult(
                chunks=rerank_result.chunks,
                metadata={**result.metadata, **metadata},
            )
            RAG_PIPELINE_DURATION.labels(stage="rerank").observe(
                time.perf_counter() - retrieve_start
            )
        except Exception as e:
            logger.warning(
                "rerank_fallback",
                error=str(e),
                error_type=e.__class__.__name__,
                collection=collection_name,
                candidate_count=len(result.chunks),
                exc_info=True,
            )
            RERANK_FALLBACKS.labels(reason=e.__class__.__name__).inc()
            result = RetrievalResult(
                chunks=result.chunks[:top_k],
                metadata={
                    **result.metadata,
                    "rerank_requested": True,
                    "rerank_enabled": True,
                    "rerank_applied": False,
                    "rerank_model": settings.rerank_model,
                    "rerank_candidate_count": len(result.chunks),
                    "rerank_returned_count": min(len(result.chunks), top_k),
                    "rerank_fallback": True,
                    "rerank_error": e.__class__.__name__,
                },
            )
    else:
        result = _with_rerank_metadata(
            result,
            {
                "rerank_requested": False,
                "rerank_enabled": settings.rerank_enabled,
                "rerank_applied": False,
                "rerank_fallback": False,
            },
        )
```

- [ ] **Step 7: Thread `rerank` through `rag_query`**

Change `rag_query` signature to include:

```python
    rerank: bool = False,
```

Pass it into `retrieve_chunks`:

```python
        rerank=rerank,
```

- [ ] **Step 8: Run chain tests**

Run:

```bash
cd services/chat && python -m pytest tests/test_chain.py -v
```

Expected: all `test_chain.py` tests pass.

- [ ] **Step 9: Commit chain integration**

Run:

```bash
git add services/chat/app/chain.py services/chat/tests/test_chain.py
git commit -m "feat: apply optional reranking in chat retrieval"
```

Expected: commit succeeds.

## Task 5: Chat API Threading And Config Exposure

**Files:**
- Modify: `services/chat/app/main.py`
- Modify: `services/chat/tests/test_main.py`

- [ ] **Step 1: Add failing tests for `/config`, `/search`, and `/chat` threading**

Append to `services/chat/tests/test_main.py`:

```python
@patch("app.main.retrieve_chunks", new_callable=AsyncMock)
def test_search_threads_rerank_flag_into_retrieve_chunks(mock_retrieve):
    mock_retrieve.return_value = retrieval_result()

    response = client.post("/search", json={"query": "hello", "limit": 5, "rerank": True})

    assert response.status_code == 200
    assert mock_retrieve.await_args.kwargs["rerank"] is True


@patch("app.main.rag_query")
def test_chat_json_threads_rerank_flag_into_rag_query(mock_rag_query):
    captured = {}

    async def fake(**kwargs):
        captured.update(kwargs)
        yield {"done": True, "sources": [], "retrieval": {}}

    mock_rag_query.side_effect = fake

    response = client.post(
        "/chat",
        json={"question": "hi", "rerank": True},
        headers={"Accept": "application/json"},
    )

    assert response.status_code == 200
    assert captured["rerank"] is True


def test_config_endpoint_returns_rerank_settings():
    from app.config import settings

    response = client.get("/config")

    assert response.status_code == 200
    body = response.json()
    assert body["rerank_enabled"] == settings.rerank_enabled
    assert body["rerank_model"] == settings.rerank_model
    assert body["rerank_candidate_limit"] == settings.rerank_candidate_limit
    assert body["rerank_max_candidates"] == settings.rerank_max_candidates
    assert body["rerank_device"] == settings.rerank_device
```

- [ ] **Step 2: Run API tests to verify failure**

Run:

```bash
cd services/chat && python -m pytest tests/test_main.py -k rerank -v
```

Expected: fails because request models and `/config` do not include re-ranker fields.

- [ ] **Step 3: Add request fields**

In `ChatRequest`, add:

```python
    rerank: bool = False
```

In `SearchRequest`, add:

```python
    rerank: bool = False
```

- [ ] **Step 4: Expose config fields**

In `get_config()`, add:

```python
        "rerank_enabled": settings.rerank_enabled,
        "rerank_model": settings.rerank_model,
        "rerank_candidate_limit": settings.rerank_candidate_limit,
        "rerank_max_candidates": settings.rerank_max_candidates,
        "rerank_device": settings.rerank_device,
```

- [ ] **Step 5: Thread `rerank` into both chat paths and search**

In both `rag_query(...)` calls in `chat()`, add:

```python
                rerank=body.rerank,
```

In the `retrieve_chunks(...)` call in `search()`, add:

```python
            rerank=body.rerank,
```

- [ ] **Step 6: Run chat API tests**

Run:

```bash
cd services/chat && python -m pytest tests/test_main.py -v
```

Expected: all `test_main.py` tests pass.

- [ ] **Step 7: Commit API threading**

Run:

```bash
git add services/chat/app/main.py services/chat/tests/test_main.py
git commit -m "feat: expose rerank flag on chat api"
```

Expected: commit succeeds.

## Task 6: Eval Request Threading

**Files:**
- Modify: `services/eval/app/models.py`
- Modify: `services/eval/app/rag_client.py`
- Modify: `services/eval/app/evaluator.py`
- Modify: `services/eval/app/main.py`
- Modify: `services/eval/tests/test_models.py`
- Modify: `services/eval/tests/test_rag_client.py`
- Modify: `services/eval/tests/test_evaluator.py`
- Modify: `services/eval/tests/test_main.py`

- [ ] **Step 1: Add failing RAG client tests**

Append to `services/eval/tests/test_rag_client.py`:

```python
@pytest.mark.asyncio
async def test_search_passes_rerank_when_true(mock_search_response):
    async def mock_handler(request: httpx.Request) -> httpx.Response:
        body = json.loads(request.content)
        assert body["rerank"] is True
        return httpx.Response(200, json=mock_search_response)

    transport = httpx.MockTransport(mock_handler)
    client = RAGClient(base_url="http://chat:8000", transport=transport)

    await client.search("test", collection=None, limit=5, rerank=True)


@pytest.mark.asyncio
async def test_ask_passes_rerank_when_true(mock_chat_response):
    async def mock_handler(request: httpx.Request) -> httpx.Response:
        body = json.loads(request.content)
        assert body["rerank"] is True
        return httpx.Response(200, json=mock_chat_response)

    transport = httpx.MockTransport(mock_handler)
    client = RAGClient(base_url="http://chat:8000", transport=transport)

    await client.ask("test", collection=None, rerank=True)
```

- [ ] **Step 2: Add failing model test**

Append to `services/eval/tests/test_models.py`:

```python
def test_start_request_accepts_rerank_flag():
    req = StartEvaluationRequest(dataset_id="ds-1", rerank=True)

    assert req.rerank is True
```

- [ ] **Step 3: Add failing evaluator pass-through test**

In `services/eval/tests/test_evaluator.py`, add a test next to the existing dataset construction tests:

```python
@pytest.mark.asyncio
async def test_build_evaluation_dataset_passes_rerank(golden_items, mock_search_results, mock_chat_answer):
    rag_client = AsyncMock()
    rag_client.search.return_value = mock_search_results
    rag_client.ask.return_value = mock_chat_answer

    await build_evaluation_dataset(
        items=golden_items,
        rag_client=rag_client,
        collection="documents",
        rerank=True,
    )

    assert rag_client.search.call_args_list[0].kwargs["rerank"] is True
    assert rag_client.ask.call_args_list[0].kwargs["rerank"] is True
```

- [ ] **Step 4: Add failing eval API threading test**

Append to `services/eval/tests/test_main.py`:

```python
@patch("app.main.run_evaluation", new_callable=AsyncMock)
@patch("app.main.capture_run_config", new_callable=AsyncMock)
@patch("app.main.get_db")
def test_start_evaluation_passes_rerank_to_background_run(
    mock_get_db, mock_capture, mock_run_evaluation
):
    mock_db = AsyncMock()
    mock_db.get_dataset.return_value = {
        "id": "ds-rerank",
        "name": "test",
        "items": [{"query": "q", "expected_answer": "a", "expected_sources": []}],
        "created_at": "2026-04-16T00:00:00Z",
    }
    mock_db.create_evaluation.return_value = "eval-rerank"
    mock_get_db.return_value = mock_db
    mock_capture.return_value = {"captured_at": "x"}
    mock_run_evaluation.return_value = ({"faithfulness": 0.8}, [])

    response = client.post(
        "/evaluations",
        json={"dataset_id": "ds-rerank", "rerank": True},
    )

    assert response.status_code == 202
    assert mock_run_evaluation.await_args.kwargs["rerank"] is True
```

- [ ] **Step 5: Run eval rerank tests to verify failure**

Run:

```bash
cd services/eval && python -m pytest tests/test_rag_client.py tests/test_models.py tests/test_evaluator.py tests/test_main.py -k rerank -v
```

Expected: fails because eval models and client methods do not yet accept `rerank`.

- [ ] **Step 6: Add `rerank` to eval model**

In `services/eval/app/models.py`, add to `StartEvaluationRequest`:

```python
    rerank: bool = False
```

- [ ] **Step 7: Update eval RAG client**

Change `RAGClient.search` signature in `services/eval/app/rag_client.py` to:

```python
    async def search(
        self, query: str, collection: str | None, limit: int, rerank: bool = False
    ) -> list[dict]:
```

Before posting, add:

```python
        if rerank:
            body["rerank"] = True
```

Change `RAGClient.ask` signature to:

```python
    async def ask(
        self, question: str, collection: str | None, rerank: bool = False
    ) -> dict:
```

Before posting, add:

```python
        if rerank:
            body["rerank"] = True
```

- [ ] **Step 8: Thread rerank through evaluator**

In `services/eval/app/evaluator.py`, change `build_evaluation_dataset` signature to:

```python
async def build_evaluation_dataset(
    items: list[dict],
    rag_client: RAGClient,
    collection: str | None,
    rerank: bool = False,
) -> list[dict]:
```

Change calls inside it to:

```python
        search_results = await rag_client.search(
            query, collection=collection, limit=5, rerank=rerank
        )
        chat_response = await rag_client.ask(
            query, collection=collection, rerank=rerank
        )
```

Change `run_evaluation` signature to include:

```python
    rerank: bool = False,
```

Change the dataset call to:

```python
    raw_dataset = await build_evaluation_dataset(
        items, rag_client, collection, rerank=rerank
    )
```

- [ ] **Step 9: Thread rerank through eval main**

Change `_run_evaluation_task` signature in `services/eval/app/main.py` to:

```python
async def _run_evaluation_task(
    eval_id: str, items: list[dict], collection: str | None, rerank: bool
):
```

Pass `rerank=rerank` into `run_evaluation(...)`.

Change `background_tasks.add_task(...)` to:

```python
    background_tasks.add_task(
        _run_evaluation_task, eval_id, dataset["items"], body.collection, body.rerank
    )
```

- [ ] **Step 10: Run eval tests**

Run:

```bash
cd services/eval && python -m pytest tests/test_rag_client.py tests/test_models.py tests/test_evaluator.py tests/test_main.py -v
```

Expected: all listed eval tests pass.

- [ ] **Step 11: Commit eval threading**

Run:

```bash
git add services/eval/app/models.py services/eval/app/rag_client.py services/eval/app/evaluator.py services/eval/app/main.py services/eval/tests/test_models.py services/eval/tests/test_rag_client.py services/eval/tests/test_evaluator.py services/eval/tests/test_main.py
git commit -m "feat: thread rerank through eval runs"
```

Expected: commit succeeds.

## Task 7: Final Verification And PR

**Files:**
- Verify all modified Python files

- [ ] **Step 1: Run focused chat tests**

Run:

```bash
cd services/chat && python -m pytest tests/test_reranker.py tests/test_config.py tests/test_chain.py tests/test_main.py -v
```

Expected: all tests pass.

- [ ] **Step 2: Run focused eval tests**

Run:

```bash
cd services/eval && python -m pytest tests/test_rag_client.py tests/test_models.py tests/test_evaluator.py tests/test_main.py -v
```

Expected: all tests pass.

- [ ] **Step 3: Run required Python preflight**

Run:

```bash
make preflight-python
```

Expected: exits 0.

- [ ] **Step 4: Run required security preflight**

Run:

```bash
make preflight-security
```

Expected: exits 0.

- [ ] **Step 5: Inspect final diff**

Run:

```bash
git status --short
git diff --stat
git diff --check
```

Expected: only intended files are modified, and `git diff --check` exits 0.

- [ ] **Step 6: Commit any final fixes**

If Step 5 shows uncommitted intended fixes, run:

```bash
git add services/chat services/eval
git commit -m "test: complete rerank verification fixes"
```

Expected: commit succeeds, or there are no remaining changes to commit.

- [ ] **Step 7: Push feature branch and create PR to `qa`**

Run from the feature worktree branch:

```bash
git branch --show-current
git push -u origin HEAD
gh pr create --base qa --head "$(git branch --show-current)" --title "Phase 4c: Cross-encoder re-ranking" --body "Implements issue #81 with opt-in local cross-encoder re-ranking for chat/search and eval comparison support."
```

Expected: PR is created against `qa`. Do not watch CI unless Kyle asks.

## Self-Review

- Spec coverage: tasks cover the local cross-encoder module, opt-in API flags,
  config, metadata, fallback behavior, metrics, eval pass-through, and required
  Python/security preflights.
- Scope check: the plan does not add a separate service, UI work, production
  data mutation, or default-on rollout.
- Type consistency: `rerank` is a boolean on chat and eval request models;
  `retrieve_chunks` and `rag_query` both accept `rerank: bool = False`;
  eval client methods accept `rerank: bool = False`.
- Dependency risk: the plan includes an early install and security gate before
  broader implementation.
