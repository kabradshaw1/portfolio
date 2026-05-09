# Hybrid Search Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add Qdrant-native hybrid retrieval so newly ingested RAG collections use dense semantic vectors plus BM25 sparse vectors, while legacy dense-only collections keep working.

**Architecture:** Ingestion creates named `dense` and `sparse` vectors for new collections and upserts both vectors per chunk. Chat defaults to Qdrant Query API hybrid search with RRF, then falls back to legacy semantic search when a collection is dense-only. Eval captures retrieval metadata so semantic-only and hybrid runs can be compared.

**Tech Stack:** Python 3.11, FastAPI, qdrant-client with FastEmbed extra, Qdrant 1.16.3+, pytest, Docker Compose, Kubernetes manifests.

---

## References

- Design spec: `docs/superpowers/specs/2026-05-08-hybrid-search-design.md`
- Qdrant hybrid Query API: https://qdrant.tech/documentation/search/hybrid-queries/
- Qdrant text search and BM25 sparse vectors: https://qdrant.tech/documentation/search/text-search/
- qdrant-client package: https://pypi.org/project/qdrant-client/

## File Structure

- Modify `services/shared/pyproject.toml`: include a new shared `rag` package and install `qdrant-client[fastembed]==1.17.1`.
- Create `services/shared/rag/__init__.py`: export sparse vector helpers.
- Create `services/shared/rag/sparse.py`: normalize FastEmbed BM25 sparse output into Qdrant `SparseVector` models.
- Create `services/shared/tests/test_sparse.py`: unit tests for sparse vector normalization without loading real ONNX models.
- Modify `services/ingestion/requirements.txt`: use `qdrant-client[fastembed]==1.17.1`.
- Modify `services/chat/requirements.txt`: use `qdrant-client[fastembed]==1.17.1`.
- Modify `docker-compose.yml`: pin Qdrant to `qdrant/qdrant:v1.16.3`.
- Modify `k8s/ai-services/deployments/qdrant.yml`: pin Qdrant to `qdrant/qdrant:v1.16.3`.
- Modify `services/ingestion/app/store.py`: create hybrid collection configs and upsert named vectors.
- Modify `services/ingestion/app/config.py`: add sparse model config.
- Modify `services/ingestion/app/collection_meta.py`: persist hybrid metadata.
- Modify `services/ingestion/app/main.py`: generate sparse vectors before Qdrant upsert and record metadata.
- Modify `services/ingestion/tests/test_store.py`: cover hybrid collection creation and named-vector upserts.
- Modify `services/ingestion/tests/test_main.py`: cover sparse generation and metadata writes.
- Modify `services/chat/app/config.py`: add retrieval mode, vector names, sparse model, and hybrid prefetch config.
- Modify `services/chat/app/retriever.py`: add semantic and hybrid retrieval methods with fallback metadata.
- Modify `services/chat/app/chain.py`: return chunks plus retrieval metadata to `/chat` callers.
- Modify `services/chat/app/main.py`: expose retrieval metadata from `/search`, `/chat` JSON, `/chat` SSE, and `/config`.
- Modify `services/chat/tests/test_retriever.py`: cover Query API hybrid, named semantic, and legacy fallback.
- Modify `services/chat/tests/test_chain.py`: cover metadata propagation.
- Modify `services/chat/tests/test_main.py`: cover API response metadata compatibility.
- Modify `services/eval/app/config_capture.py`: capture chat retrieval config already exposed by `/config`.
- Modify `services/eval/tests/test_config_capture.py`: assert hybrid retrieval settings are preserved.

## Task 1: Pin Runtime And Client Versions

**Files:**
- Modify: `services/ingestion/requirements.txt`
- Modify: `services/chat/requirements.txt`
- Modify: `services/shared/pyproject.toml`
- Modify: `docker-compose.yml`
- Modify: `k8s/ai-services/deployments/qdrant.yml`

- [ ] **Step 1: Update qdrant-client dependencies**

In `services/ingestion/requirements.txt`, replace:

```txt
qdrant-client==1.9.0
```

with:

```txt
qdrant-client[fastembed]==1.17.1
```

In `services/chat/requirements.txt`, make the same replacement.

In `services/shared/pyproject.toml`, update the package include and dependencies to:

```toml
dependencies = [
    "httpx>=0.27",
    "openai>=1.0",
    "anthropic>=0.30",
    "qdrant-client[fastembed]==1.17.1",
]

[tool.setuptools.packages.find]
include = ["llm*", "rag*"]
```

- [ ] **Step 2: Pin Qdrant images**

In `docker-compose.yml`, replace:

```yaml
image: qdrant/qdrant:latest
```

with:

```yaml
image: qdrant/qdrant:v1.16.3
```

In `k8s/ai-services/deployments/qdrant.yml`, make the same image replacement.

- [ ] **Step 3: Verify dependency text changes**

Run:

```bash
rg -n "qdrant-client|qdrant/qdrant" services/ingestion/requirements.txt services/chat/requirements.txt services/shared/pyproject.toml docker-compose.yml k8s/ai-services/deployments/qdrant.yml
```

Expected: chat and ingestion requirements show `qdrant-client[fastembed]==1.17.1`, shared pyproject includes the same dependency, and both Qdrant image references are `qdrant/qdrant:v1.16.3`.

- [ ] **Step 4: Commit**

```bash
git add services/ingestion/requirements.txt services/chat/requirements.txt services/shared/pyproject.toml docker-compose.yml k8s/ai-services/deployments/qdrant.yml
git commit -m "chore: pin qdrant hybrid search dependencies"
```

## Task 2: Add Shared BM25 Sparse Vector Encoder

**Files:**
- Create: `services/shared/rag/__init__.py`
- Create: `services/shared/rag/sparse.py`
- Create: `services/shared/tests/test_sparse.py`

- [ ] **Step 1: Write failing sparse encoder tests**

Create `services/shared/tests/test_sparse.py`:

```python
from types import SimpleNamespace

from qdrant_client.models import SparseVector
from rag.sparse import SparseVectorEncoder, normalize_sparse_vector


def test_normalize_sparse_vector_converts_numpy_like_values():
    raw = SimpleNamespace(
        indices=SimpleNamespace(tolist=lambda: [2, 7]),
        values=SimpleNamespace(tolist=lambda: [0.5, 1.25]),
    )

    vector = normalize_sparse_vector(raw)

    assert vector == SparseVector(indices=[2, 7], values=[0.5, 1.25])


def test_sparse_vector_encoder_returns_one_vector_per_text(monkeypatch):
    class FakeSparseTextEmbedding:
        def __init__(self, model_name: str, batch_size: int):
            self.model_name = model_name
            self.batch_size = batch_size

        def embed(self, texts):
            assert texts == ["RFC 7231", "section 4.2"]
            return [
                SimpleNamespace(indices=[1], values=[0.7]),
                SimpleNamespace(indices=[3, 4], values=[0.2, 0.9]),
            ]

    monkeypatch.setattr("rag.sparse.SparseTextEmbedding", FakeSparseTextEmbedding)

    encoder = SparseVectorEncoder(model_name="Qdrant/bm25", batch_size=16)
    vectors = encoder.embed(["RFC 7231", "section 4.2"])

    assert vectors == [
        SparseVector(indices=[1], values=[0.7]),
        SparseVector(indices=[3, 4], values=[0.2, 0.9]),
    ]


def test_sparse_vector_encoder_returns_empty_list_for_empty_input(monkeypatch):
    class FailingSparseTextEmbedding:
        def __init__(self, model_name: str, batch_size: int):
            raise AssertionError("model should not load for empty input")

    monkeypatch.setattr("rag.sparse.SparseTextEmbedding", FailingSparseTextEmbedding)

    encoder = SparseVectorEncoder(model_name="Qdrant/bm25", batch_size=16)

    assert encoder.embed([]) == []
```

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
PYTHONPATH=services/shared pytest services/shared/tests/test_sparse.py -v
```

Expected: FAIL with `ModuleNotFoundError: No module named 'rag'`.

- [ ] **Step 3: Implement sparse encoder**

Create `services/shared/rag/__init__.py`:

```python
from rag.sparse import SparseVectorEncoder, normalize_sparse_vector

__all__ = ["SparseVectorEncoder", "normalize_sparse_vector"]
```

Create `services/shared/rag/sparse.py`:

```python
from __future__ import annotations

from collections.abc import Sequence
from typing import Any

from fastembed import SparseTextEmbedding
from qdrant_client.models import SparseVector


def _as_list(value: Any) -> list:
    if hasattr(value, "tolist"):
        return value.tolist()
    return list(value)


def normalize_sparse_vector(raw: Any) -> SparseVector:
    return SparseVector(
        indices=[int(index) for index in _as_list(raw.indices)],
        values=[float(value) for value in _as_list(raw.values)],
    )


class SparseVectorEncoder:
    def __init__(self, model_name: str = "Qdrant/bm25", batch_size: int = 256):
        self.model_name = model_name
        self.batch_size = batch_size
        self._model: SparseTextEmbedding | None = None

    def _get_model(self) -> SparseTextEmbedding:
        if self._model is None:
            self._model = SparseTextEmbedding(
                model_name=self.model_name,
                batch_size=self.batch_size,
            )
        return self._model

    def embed(self, texts: Sequence[str]) -> list[SparseVector]:
        if not texts:
            return []
        return [
            normalize_sparse_vector(vector)
            for vector in self._get_model().embed(list(texts))
        ]
```

- [ ] **Step 4: Run tests to verify they pass**

Run:

```bash
PYTHONPATH=services/shared pytest services/shared/tests/test_sparse.py -v
```

Expected: PASS for 3 tests.

- [ ] **Step 5: Commit**

```bash
git add services/shared/rag/__init__.py services/shared/rag/sparse.py services/shared/tests/test_sparse.py
git commit -m "feat: add bm25 sparse vector encoder"
```

## Task 3: Store Hybrid Vectors During Ingestion

**Files:**
- Modify: `services/ingestion/app/store.py`
- Modify: `services/ingestion/tests/test_store.py`

- [ ] **Step 1: Write failing store tests**

Append these tests to `services/ingestion/tests/test_store.py`:

```python
from qdrant_client.models import SparseVector


def test_store_init_creates_hybrid_collection(mock_qdrant_client):
    mock_qdrant_client.collection_exists.return_value = False

    QdrantStore(host="localhost", port=6333, collection_name="test")

    call_args = mock_qdrant_client.create_collection.call_args
    assert call_args.kwargs["collection_name"] == "test"
    assert sorted(call_args.kwargs["vectors_config"].keys()) == ["dense"]
    assert sorted(call_args.kwargs["sparse_vectors_config"].keys()) == ["sparse"]


def test_upsert_writes_named_dense_and_sparse_vectors(mock_qdrant_client):
    mock_qdrant_client.collection_exists.return_value = True
    store = QdrantStore(host="localhost", port=6333, collection_name="test")

    chunks = [{"text": "RFC 7231", "page_number": 1, "chunk_index": 0}]
    dense_vectors = [[0.1] * 768]
    sparse_vectors = [SparseVector(indices=[10, 11], values=[0.8, 0.4])]

    store.upsert(
        chunks=chunks,
        vectors=dense_vectors,
        sparse_vectors=sparse_vectors,
        document_id="doc-123",
        filename="http.pdf",
    )

    point = mock_qdrant_client.upsert.call_args.kwargs["points"][0]
    assert point.vector["dense"] == dense_vectors[0]
    assert point.vector["sparse"] == sparse_vectors[0]
    assert point.payload["text"] == "RFC 7231"
```

Update the existing `test_upsert_vectors` call to include:

```python
sparse_vectors=[
    SparseVector(indices=[1], values=[0.1]),
    SparseVector(indices=[2], values=[0.2]),
],
```

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
PYTHONPATH=services/ingestion/app:services/shared pytest services/ingestion/tests/test_store.py -v
```

Expected: FAIL because `QdrantStore.upsert()` does not accept `sparse_vectors` and collection creation has no sparse config.

- [ ] **Step 3: Implement hybrid collection and upsert support**

In `services/ingestion/app/store.py`, update imports:

```python
from qdrant_client.models import (
    Distance,
    FieldCondition,
    Filter,
    MatchValue,
    PointStruct,
    SparseVector,
    SparseVectorParams,
    VectorParams,
)
```

Add constants near imports:

```python
DENSE_VECTOR_NAME = "dense"
SPARSE_VECTOR_NAME = "sparse"
```

Update `_ensure_collection()`:

```python
    def _ensure_collection(self):
        if not self.client.collection_exists(self.collection_name):
            self.client.create_collection(
                collection_name=self.collection_name,
                vectors_config={
                    DENSE_VECTOR_NAME: VectorParams(
                        size=768,
                        distance=Distance.COSINE,
                    )
                },
                sparse_vectors_config={
                    SPARSE_VECTOR_NAME: SparseVectorParams(),
                },
            )
```

Update `upsert()` signature and point creation:

```python
    def upsert(
        self,
        chunks: list[dict],
        vectors: list[list[float]],
        sparse_vectors: list[SparseVector],
        document_id: str,
        filename: str,
    ) -> None:
        points = [
            PointStruct(
                id=str(uuid.uuid4()),
                vector={
                    DENSE_VECTOR_NAME: vector,
                    SPARSE_VECTOR_NAME: sparse_vector,
                },
                payload={
                    "text": chunk["text"],
                    "page_number": chunk["page_number"],
                    "chunk_index": chunk["chunk_index"],
                    "document_id": document_id,
                    "filename": filename,
                },
            )
            for chunk, vector, sparse_vector in zip(chunks, vectors, sparse_vectors)
        ]
```

- [ ] **Step 4: Run tests to verify they pass**

Run:

```bash
PYTHONPATH=services/ingestion/app:services/shared pytest services/ingestion/tests/test_store.py -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add services/ingestion/app/store.py services/ingestion/tests/test_store.py
git commit -m "feat: store hybrid qdrant vectors"
```

## Task 4: Generate Sparse Vectors And Metadata In Ingestion API

**Files:**
- Modify: `services/ingestion/app/config.py`
- Modify: `services/ingestion/app/collection_meta.py`
- Modify: `services/ingestion/app/main.py`
- Modify: `services/ingestion/tests/test_main.py`

- [ ] **Step 1: Write failing ingestion API test**

In `services/ingestion/tests/test_main.py`, add imports:

```python
from qdrant_client.models import SparseVector
```

Add this test near the existing ingest tests:

```python
@patch("app.main.SparseVectorEncoder")
@patch("app.main.QdrantStore")
@patch("app.main.embed_texts", new_callable=AsyncMock)
@patch("app.main.extract_pages")
def test_ingest_generates_sparse_vectors_and_records_hybrid_metadata(
    mock_extract,
    mock_embed,
    mock_qdrant_store_cls,
    mock_sparse_encoder_cls,
    client,
):
    mock_extract.return_value = [{"page_number": 1, "text": "RFC 7231 section 4.2"}]
    mock_embed.return_value = [[0.1] * 768]

    mock_sparse_encoder = MagicMock()
    sparse_vectors = [SparseVector(indices=[1, 2], values=[0.7, 0.4])]
    mock_sparse_encoder.embed.return_value = sparse_vectors
    mock_sparse_encoder_cls.return_value = mock_sparse_encoder

    mock_store = MagicMock()
    mock_qdrant_store_cls.return_value = mock_store

    response = client.post(
        "/ingest",
        files={"file": ("http.pdf", b"%PDF-1.4 fake", "application/pdf")},
    )

    assert response.status_code == 200
    mock_sparse_encoder.embed.assert_called_once_with(["RFC 7231 section 4.2"])
    mock_store.upsert.assert_called_once()
    assert mock_store.upsert.call_args.kwargs["sparse_vectors"] == sparse_vectors
```

- [ ] **Step 2: Run test to verify it fails**

Run:

```bash
PYTHONPATH=services/ingestion/app:services/shared pytest services/ingestion/tests/test_main.py::test_ingest_generates_sparse_vectors_and_records_hybrid_metadata -v
```

Expected: FAIL because `app.main.SparseVectorEncoder` does not exist and `store.upsert()` is not called with sparse vectors.

- [ ] **Step 3: Add sparse ingestion config**

In `services/ingestion/app/config.py`, add settings:

```python
    sparse_model: str = "Qdrant/bm25"
    sparse_batch_size: int = 256
    hybrid_enabled: bool = True
```

- [ ] **Step 4: Extend metadata persistence**

In `services/ingestion/app/collection_meta.py`, extend the stored config dict in `upsert()` so records include:

```python
"hybrid_enabled": True,
"dense_vector_name": "dense",
"sparse_vector_name": "sparse",
"sparse_model": sparse_model,
```

Update the `upsert()` method signature to accept:

```python
sparse_model: str | None = None,
hybrid_enabled: bool = True,
```

When serializing, use `sparse_model` as passed from settings.

- [ ] **Step 5: Generate sparse vectors in ingest**

In `services/ingestion/app/main.py`, add imports:

```python
from rag.sparse import SparseVectorEncoder
```

Add module global after `_embedding_provider`:

```python
_sparse_encoder = SparseVectorEncoder(
    model_name=settings.sparse_model,
    batch_size=settings.sparse_batch_size,
)
```

After dense `vectors = await embed_texts(...)`, add:

```python
    try:
        sparse_vectors = _sparse_encoder.embed(texts)
    except Exception as e:
        logger.error("sparse_vector_error", error=str(e), exc_info=True)
        raise HTTPException(status_code=503, detail="Sparse vector generation failed")
```

Update `store.upsert()`:

```python
        store.upsert(
            chunks=chunks,
            vectors=vectors,
            sparse_vectors=sparse_vectors,
            document_id=document_id,
            filename=file.filename,
        )
```

Update metadata write:

```python
        await meta_db.upsert(
            collection=target_collection,
            chunk_size=settings.chunk_size,
            chunk_overlap=settings.chunk_overlap,
            embedding_model=settings.embedding_model,
            sparse_model=settings.sparse_model,
            hybrid_enabled=settings.hybrid_enabled,
        )
```

- [ ] **Step 6: Run targeted ingestion tests**

Run:

```bash
PYTHONPATH=services/ingestion/app:services/shared pytest services/ingestion/tests/test_main.py services/ingestion/tests/test_store.py -v
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add services/ingestion/app/config.py services/ingestion/app/collection_meta.py services/ingestion/app/main.py services/ingestion/tests/test_main.py
git commit -m "feat: generate sparse vectors during ingestion"
```

## Task 5: Add Hybrid And Fallback Retrieval In Chat

**Files:**
- Modify: `services/chat/app/config.py`
- Modify: `services/chat/app/retriever.py`
- Modify: `services/chat/tests/test_retriever.py`

- [ ] **Step 1: Write failing retriever tests**

Replace `services/chat/tests/test_retriever.py` with tests that cover all retrieval modes while preserving old assertions:

```python
from unittest.mock import MagicMock, patch

import pytest
from app.retriever import QdrantRetriever
from qdrant_client.models import SparseVector


@pytest.fixture
def mock_qdrant_client():
    with patch("app.retriever.QdrantClient") as MockClient:
        client = MagicMock()
        MockClient.return_value = client
        yield client


def _hit(score=0.95, text="relevant chunk"):
    return MagicMock(
        score=score,
        payload={
            "text": text,
            "page_number": 1,
            "filename": "doc.pdf",
            "document_id": "abc",
        },
    )


def test_hybrid_search_uses_prefetch_and_rrf(mock_qdrant_client):
    mock_qdrant_client.query_points.return_value = MagicMock(points=[_hit()])
    retriever = QdrantRetriever(host="localhost", port=6333, collection_name="test")

    result = retriever.search_hybrid(
        query_vector=[0.1] * 768,
        sparse_vector=SparseVector(indices=[1], values=[0.7]),
        top_k=5,
        prefetch_limit=20,
    )

    assert result.metadata == {
        "retrieval_mode": "hybrid",
        "retrieval_fallback": False,
        "fusion": "rrf",
    }
    assert result.chunks[0]["text"] == "relevant chunk"
    call_args = mock_qdrant_client.query_points.call_args
    assert call_args.kwargs["collection_name"] == "test"
    assert call_args.kwargs["limit"] == 5
    assert len(call_args.kwargs["prefetch"]) == 2


def test_semantic_search_uses_named_dense_vector(mock_qdrant_client):
    mock_qdrant_client.search.return_value = [_hit()]
    retriever = QdrantRetriever(host="localhost", port=6333, collection_name="test")

    result = retriever.search_semantic(query_vector=[0.1] * 768, top_k=3)

    assert result.metadata["retrieval_mode"] == "semantic"
    assert result.metadata["retrieval_fallback"] is False
    assert mock_qdrant_client.search.call_args.kwargs["query_vector"][0] == "dense"


def test_legacy_semantic_search_uses_unnamed_vector(mock_qdrant_client):
    mock_qdrant_client.search.return_value = [_hit()]
    retriever = QdrantRetriever(host="localhost", port=6333, collection_name="test")

    result = retriever.search_semantic(
        query_vector=[0.1] * 768,
        top_k=3,
        legacy_vector=True,
        fallback=True,
    )

    assert result.metadata["retrieval_mode"] == "semantic"
    assert result.metadata["retrieval_fallback"] is True
    assert mock_qdrant_client.search.call_args.kwargs["query_vector"] == [0.1] * 768
```

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
PYTHONPATH=services/chat/app:services/shared pytest services/chat/tests/test_retriever.py -v
```

Expected: FAIL because `search_hybrid`, `search_semantic`, and metadata result objects do not exist.

- [ ] **Step 3: Add chat retrieval settings**

In `services/chat/app/config.py`, add:

```python
    retrieval_mode: str = "hybrid"
    dense_vector_name: str = "dense"
    sparse_vector_name: str = "sparse"
    sparse_model: str = "Qdrant/bm25"
    sparse_batch_size: int = 256
    hybrid_prefetch_limit: int = 20
```

In `validate()`, add:

```python
        if self.retrieval_mode not in {"semantic", "hybrid"}:
            raise ValueError("retrieval_mode must be 'semantic' or 'hybrid'")
        if self.hybrid_prefetch_limit < self.top_k:
            raise ValueError("hybrid_prefetch_limit must be >= top_k")
```

- [ ] **Step 4: Implement retriever result and search methods**

Replace `services/chat/app/retriever.py` with:

```python
import time
from dataclasses import dataclass

from qdrant_client import QdrantClient
from qdrant_client.models import Fusion, FusionQuery, Prefetch, SparseVector

from app.config import settings
from app.metrics import QDRANT_SEARCH_DURATION, QDRANT_SEARCH_RESULTS


@dataclass(frozen=True)
class RetrievalResult:
    chunks: list[dict]
    metadata: dict


class QdrantRetriever:
    def __init__(self, host: str, port: int, collection_name: str):
        self.client = QdrantClient(host=host, port=port)
        self.collection_name = collection_name

    def _format_results(self, results) -> list[dict]:
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

    def search_semantic(
        self,
        query_vector: list[float],
        top_k: int = 5,
        legacy_vector: bool = False,
        fallback: bool = False,
    ) -> RetrievalResult:
        start = time.perf_counter()
        vector_selector = (
            query_vector if legacy_vector else (settings.dense_vector_name, query_vector)
        )
        results = self.client.search(
            collection_name=self.collection_name,
            query_vector=vector_selector,
            limit=top_k,
        )
        QDRANT_SEARCH_DURATION.labels(collection=self.collection_name).observe(
            time.perf_counter() - start
        )
        QDRANT_SEARCH_RESULTS.labels(collection=self.collection_name).observe(
            len(results)
        )
        return RetrievalResult(
            chunks=self._format_results(results),
            metadata={
                "retrieval_mode": "semantic",
                "retrieval_fallback": fallback,
                "fusion": None,
            },
        )

    def search_hybrid(
        self,
        query_vector: list[float],
        sparse_vector: SparseVector,
        top_k: int = 5,
        prefetch_limit: int = 20,
    ) -> RetrievalResult:
        start = time.perf_counter()
        response = self.client.query_points(
            collection_name=self.collection_name,
            prefetch=[
                Prefetch(
                    query=query_vector,
                    using=settings.dense_vector_name,
                    limit=prefetch_limit,
                ),
                Prefetch(
                    query=sparse_vector,
                    using=settings.sparse_vector_name,
                    limit=prefetch_limit,
                ),
            ],
            query=FusionQuery(fusion=Fusion.RRF),
            limit=top_k,
            with_payload=True,
        )
        results = response.points
        QDRANT_SEARCH_DURATION.labels(collection=self.collection_name).observe(
            time.perf_counter() - start
        )
        QDRANT_SEARCH_RESULTS.labels(collection=self.collection_name).observe(
            len(results)
        )
        return RetrievalResult(
            chunks=self._format_results(results),
            metadata={
                "retrieval_mode": "hybrid",
                "retrieval_fallback": False,
                "fusion": "rrf",
            },
        )

    def search(self, query_vector: list[float], top_k: int = 5) -> list[dict]:
        return self.search_semantic(query_vector=query_vector, top_k=top_k).chunks
```

- [ ] **Step 5: Run retriever tests**

Run:

```bash
PYTHONPATH=services/chat/app:services/shared pytest services/chat/tests/test_retriever.py -v
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add services/chat/app/config.py services/chat/app/retriever.py services/chat/tests/test_retriever.py
git commit -m "feat: add qdrant hybrid retriever"
```

## Task 6: Wire Hybrid Retrieval Through Chat API

**Files:**
- Modify: `services/chat/app/chain.py`
- Modify: `services/chat/app/main.py`
- Modify: `services/chat/tests/test_chain.py`
- Modify: `services/chat/tests/test_main.py`

- [ ] **Step 1: Write failing chain metadata test**

In `services/chat/tests/test_chain.py`, add:

```python
from app.retriever import RetrievalResult
```

Add this test:

```python
@patch("app.chain.SparseVectorEncoder")
@patch("app.chain.QdrantRetriever")
@pytest.mark.asyncio
async def test_retrieve_chunks_returns_hybrid_metadata(
    mock_retriever_cls,
    mock_sparse_encoder_cls,
):
    embedding_provider = AsyncMock()
    embedding_provider.embed.return_value = [[0.1] * 768]

    sparse_vector = MagicMock()
    mock_sparse_encoder = MagicMock()
    mock_sparse_encoder.embed.return_value = [sparse_vector]
    mock_sparse_encoder_cls.return_value = mock_sparse_encoder

    retriever = MagicMock()
    retriever.search_hybrid.return_value = RetrievalResult(
        chunks=[{"text": "RFC 7231", "page_number": 1, "filename": "http.pdf", "document_id": "doc", "score": 0.9}],
        metadata={"retrieval_mode": "hybrid", "retrieval_fallback": False, "fusion": "rrf"},
    )
    mock_retriever_cls.return_value = retriever

    result = await retrieve_chunks(
        question="What is RFC 7231?",
        embedding_provider=embedding_provider,
        embedding_model="nomic-embed-text",
        qdrant_host="localhost",
        qdrant_port=6333,
        collection_name="documents",
        top_k=5,
    )

    assert result.metadata["retrieval_mode"] == "hybrid"
    assert result.chunks[0]["text"] == "RFC 7231"
```

- [ ] **Step 2: Run chain test to verify it fails**

Run:

```bash
PYTHONPATH=services/chat/app:services/shared pytest services/chat/tests/test_chain.py::test_retrieve_chunks_returns_hybrid_metadata -v
```

Expected: FAIL because `retrieve_chunks()` returns `list[dict]`.

- [ ] **Step 3: Update chain retrieval**

In `services/chat/app/chain.py`, import:

```python
from rag.sparse import SparseVectorEncoder
from app.config import settings
from app.retriever import QdrantRetriever, RetrievalResult
```

Add module global:

```python
_sparse_encoder = SparseVectorEncoder(
    model_name=settings.sparse_model,
    batch_size=settings.sparse_batch_size,
)
```

Update `retrieve_chunks()` return type to `RetrievalResult` and replace retriever call:

```python
    retriever = QdrantRetriever(
        host=qdrant_host, port=qdrant_port, collection_name=collection_name
    )
    if settings.retrieval_mode == "hybrid":
        try:
            sparse_vector = _sparse_encoder.embed([question])[0]
            result = retriever.search_hybrid(
                query_vector=query_vector,
                sparse_vector=sparse_vector,
                top_k=top_k,
                prefetch_limit=settings.hybrid_prefetch_limit,
            )
        except Exception:
            result = retriever.search_semantic(
                query_vector=query_vector,
                top_k=top_k,
                legacy_vector=True,
                fallback=True,
            )
    else:
        result = retriever.search_semantic(query_vector=query_vector, top_k=top_k)
```

Return `result`.

In `rag_query()`, change:

```python
    retrieval = await retrieve_chunks(...)
    chunks = retrieval.chunks
```

and final event:

```python
    yield {"done": True, "sources": sources, "retrieval": retrieval.metadata}
```

- [ ] **Step 4: Update `/search`, `/chat`, and `/config` metadata**

In `services/chat/app/main.py`, update `/config` response:

```python
        "retrieval_mode": settings.retrieval_mode,
        "hybrid_prefetch_limit": settings.hybrid_prefetch_limit,
        "dense_vector_name": settings.dense_vector_name,
        "sparse_vector_name": settings.sparse_vector_name,
        "sparse_model": settings.sparse_model,
        "fusion": "rrf" if settings.retrieval_mode == "hybrid" else None,
```

In JSON `/chat`, collect final retrieval metadata:

```python
            retrieval = {}
            ...
                if event.get("done"):
                    sources = event.get("sources", [])
                    retrieval = event.get("retrieval", {})
            return {"answer": "".join(tokens), "sources": sources, "retrieval": retrieval}
```

In `/search`, use the new result object:

```python
        retrieval = await retrieve_chunks(...)
```

and return:

```python
    return {
        "results": [
            {
                "text": c["text"],
                "filename": c["filename"],
                "page_number": c["page_number"],
                "score": c["score"],
            }
            for c in retrieval.chunks
        ],
        "retrieval": retrieval.metadata,
    }
```

- [ ] **Step 5: Run targeted chat tests**

Run:

```bash
PYTHONPATH=services/chat/app:services/shared pytest services/chat/tests/test_chain.py services/chat/tests/test_main.py -v
```

Expected: PASS after updating existing tests that mock `retrieve_chunks()` to return `RetrievalResult(chunks=[...], metadata={...})`.

- [ ] **Step 6: Commit**

```bash
git add services/chat/app/chain.py services/chat/app/main.py services/chat/tests/test_chain.py services/chat/tests/test_main.py
git commit -m "feat: expose hybrid retrieval metadata"
```

## Task 7: Capture Hybrid Retrieval Settings In Eval

**Files:**
- Modify: `services/eval/tests/test_config_capture.py`
- Modify: `services/eval/app/config_capture.py`

- [ ] **Step 1: Write failing config capture assertion**

In `services/eval/tests/test_config_capture.py`, update the fake chat config response to include:

```python
{
    "retrieval_mode": "hybrid",
    "hybrid_prefetch_limit": 20,
    "dense_vector_name": "dense",
    "sparse_vector_name": "sparse",
    "sparse_model": "Qdrant/bm25",
    "fusion": "rrf",
}
```

Add assertion:

```python
assert result["chat"]["retrieval_mode"] == "hybrid"
assert result["chat"]["fusion"] == "rrf"
assert result["chat"]["sparse_model"] == "Qdrant/bm25"
```

- [ ] **Step 2: Run eval config tests**

Run:

```bash
PYTHONPATH=services/eval/app pytest services/eval/tests/test_config_capture.py -v
```

Expected: PASS if config capture already preserves arbitrary chat config keys. If it fails because the test fixture or response model filters keys, remove that filtering so `capture_run_config()` stores the full chat config dict.

- [ ] **Step 3: Commit if code changed**

If only tests changed and they pass:

```bash
git add services/eval/tests/test_config_capture.py
git commit -m "test: cover hybrid retrieval config capture"
```

If `services/eval/app/config_capture.py` also changed:

```bash
git add services/eval/app/config_capture.py services/eval/tests/test_config_capture.py
git commit -m "feat: capture hybrid retrieval config"
```

## Task 8: Final Verification

**Files:**
- Verify all touched Python, Docker, and Kubernetes files.

- [ ] **Step 1: Run Python preflight**

Run:

```bash
make preflight-python
```

Expected: exit 0.

- [ ] **Step 2: Run security preflight**

Run:

```bash
make preflight-security
```

Expected: exit 0.

- [ ] **Step 3: Optional local compose smoke**

Run only if Docker resources are available locally:

```bash
docker compose up -d qdrant mock-ollama ingestion chat
```

Then ingest a small PDF into a fresh collection and call `/search`. Expected:
the response includes `"retrieval": {"retrieval_mode": "hybrid", ...}`.

- [ ] **Step 4: Inspect final diff**

Run:

```bash
git status --short --branch
git log --oneline -8
```

Expected: branch is `qa`, all intended commits are present, and there are no unstaged files unless generated local artifacts are intentionally left untracked.

- [ ] **Step 5: Do not push doc-only or implementation commits unless branch rules allow the current work type**

On `qa`, implementation commits may be pushed autonomously. Doc-only commits stay local until a later code change or explicit request.

## Self-Review

Spec coverage:

- Dense+sparse indexing: Tasks 2, 3, and 4.
- Qdrant native hybrid search with RRF: Task 5.
- Legacy semantic fallback: Tasks 5 and 6.
- Existing `/chat` and `/search` compatibility: Task 6.
- Eval config capture: Task 7.
- Version pinning: Task 1.
- Verification: Task 8.

Placeholder scan: no unresolved markers or vague implementation steps are present.

Type consistency:

- `SparseVectorEncoder.embed()` returns `list[qdrant_client.models.SparseVector]`.
- `QdrantRetriever.search_hybrid()` and `search_semantic()` both return `RetrievalResult`.
- API metadata uses one shape everywhere: `retrieval_mode`, `retrieval_fallback`, and `fusion`.
