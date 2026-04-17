# RAG Evaluation Service Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a Python FastAPI microservice that evaluates RAG pipeline quality using RAGAS metrics, with golden dataset management and stored evaluation results.

**Architecture:** New `services/eval` service calls the existing chat service's `/search` and `/chat` endpoints for each golden dataset query, scores results with RAGAS (faithfulness, answer relevancy, context precision, context recall), and stores scorecards in SQLite. Follows the same patterns as `services/chat` (Pydantic Settings, shared auth, Prometheus, slowapi).

**Tech Stack:** Python 3.11, FastAPI, RAGAS, aiosqlite, httpx, Pydantic Settings, Prometheus, shared auth/LLM modules

**Prerequisite:** The MCP-RAG bridge must be merged first (provides `POST /search` and `POST /chat` JSON mode on the chat service).

---

### Task 1: Service scaffold — Dockerfile, requirements.txt, config

Create the basic service structure with no endpoints yet. Just enough to start and pass a health check.

**Files:**
- Create: `services/eval/Dockerfile`
- Create: `services/eval/requirements.txt`
- Create: `services/eval/app/__init__.py`
- Create: `services/eval/app/config.py`
- Create: `services/eval/app/main.py`
- Create: `services/eval/tests/__init__.py`
- Create: `services/eval/tests/conftest.py`

- [ ] **Step 1: Create the Dockerfile**

Create `services/eval/Dockerfile`:

```dockerfile
FROM python:3.11-slim

ENV PYTHONDONTWRITEBYTECODE=1
ENV PYTHONUNBUFFERED=1

WORKDIR /app

# Install shared LLM package first (changes less frequently)
COPY shared/ /shared/
RUN pip install --no-cache-dir /shared

COPY eval/requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

COPY shared/ ./shared/
COPY eval/app/ ./app/

RUN useradd --create-home appuser
USER appuser

CMD ["uvicorn", "app.main:app", "--host", "0.0.0.0", "--port", "8000"]
```

- [ ] **Step 2: Create requirements.txt**

Create `services/eval/requirements.txt`:

```
fastapi==0.135.3
uvicorn[standard]==0.44.0
httpx==0.27.0
pydantic-settings==2.3.0
aiosqlite==0.21.0
ragas==0.2.15
datasets>=3.0.0
openai>=1.0
anthropic>=0.30
pytest==8.4.2
pytest-asyncio==0.26.0
pytest-cov==5.0.0
prometheus-fastapi-instrumentator==7.0.2
slowapi==0.1.9
pyjwt==2.12.0
```

- [ ] **Step 3: Create config.py**

Create `services/eval/app/config.py`:

```python
from pydantic_settings import BaseSettings


class Settings(BaseSettings):
    # Chat service URL (for calling /search and /chat)
    chat_service_url: str = "http://chat:8000"

    # LLM config for RAGAS judge calls
    llm_provider: str = "ollama"
    llm_base_url: str = "http://host.docker.internal:11434"
    llm_api_key: str = ""
    llm_model: str = "qwen2.5:14b"

    # SQLite database path
    db_path: str = "data/eval.db"

    # Auth
    jwt_secret: str = ""

    # CORS
    allowed_origins: str = "https://kylebradshaw.dev"


settings = Settings()
```

- [ ] **Step 4: Create the empty `__init__.py` files**

Create `services/eval/app/__init__.py` (empty file).

Create `services/eval/tests/__init__.py` (empty file).

- [ ] **Step 5: Create conftest.py**

Create `services/eval/tests/conftest.py`:

```python
import pytest


@pytest.fixture(autouse=True)
def disable_rate_limiting():
    """Disable rate limiting in tests to prevent 429 interference."""
    from app.main import limiter

    limiter.enabled = False
    yield
    limiter.enabled = True
```

- [ ] **Step 6: Create main.py with health check only**

Create `services/eval/app/main.py`:

```python
import logging

import httpx
from fastapi import FastAPI
from fastapi.middleware.cors import CORSMiddleware
from prometheus_fastapi_instrumentator import Instrumentator
from slowapi import Limiter, _rate_limit_exceeded_handler
from slowapi.errors import RateLimitExceeded
from slowapi.util import get_remote_address
from starlette.requests import Request
from starlette.responses import JSONResponse

from app.config import settings
from shared.auth import create_auth_dependency

logger = logging.getLogger(__name__)

app = FastAPI(title="Eval API")

app.add_middleware(
    CORSMiddleware,
    allow_origins=settings.allowed_origins.split(","),
    allow_methods=["GET", "POST"],
    allow_headers=["Authorization", "Content-Type"],
)

instrumentator = Instrumentator()
instrumentator.instrument(app).expose(app, include_in_schema=False)

limiter = Limiter(key_func=get_remote_address)
app.state.limiter = limiter
app.add_exception_handler(RateLimitExceeded, _rate_limit_exceeded_handler)

require_auth = create_auth_dependency(settings.jwt_secret)


@app.get("/health")
async def health():
    """Health check — verifies chat service is reachable."""
    chat_ok = True
    try:
        async with httpx.AsyncClient(timeout=5.0) as client:
            resp = await client.get(f"{settings.chat_service_url}/health")
            if resp.status_code != 200:
                chat_ok = False
    except Exception:
        chat_ok = False

    status = "healthy" if chat_ok else "degraded"
    code = 200 if chat_ok else 503
    return JSONResponse(
        status_code=code,
        content={"status": status, "chat_service": "ok" if chat_ok else "unreachable"},
    )
```

- [ ] **Step 7: Run the service locally to verify it starts**

Run: `cd services/eval && pip install -e ../shared && pip install -r requirements.txt && python -m uvicorn app.main:app --port 8003 &`

Then: `curl http://localhost:8003/health`

Expected: JSON response with `"status": "degraded"` (chat service not running locally). Kill the server after verifying.

- [ ] **Step 8: Commit**

```bash
git add services/eval/
git commit -m "feat(eval): scaffold eval service with Dockerfile, config, and health check"
```

---

### Task 2: SQLite database layer

Create the async SQLite database module with tables for datasets and evaluations.

**Files:**
- Create: `services/eval/app/db.py`
- Create: `services/eval/tests/test_db.py`

- [ ] **Step 1: Write failing tests for the database layer**

Create `services/eval/tests/test_db.py`:

```python
import pytest
import pytest_asyncio

from app.db import EvalDB


@pytest_asyncio.fixture
async def db(tmp_path):
    """Create a test database in a temp directory."""
    db_path = str(tmp_path / "test.db")
    eval_db = EvalDB(db_path)
    await eval_db.init()
    yield eval_db
    await eval_db.close()


@pytest.mark.asyncio
async def test_create_and_get_dataset(db):
    ds_id = await db.create_dataset(
        name="test-dataset",
        items=[
            {
                "query": "What is chunking?",
                "expected_answer": "Splitting text into smaller pieces",
                "expected_sources": ["ingestion.pdf"],
            }
        ],
    )
    assert ds_id is not None

    dataset = await db.get_dataset(ds_id)
    assert dataset["name"] == "test-dataset"
    assert len(dataset["items"]) == 1
    assert dataset["items"][0]["query"] == "What is chunking?"


@pytest.mark.asyncio
async def test_create_dataset_duplicate_name(db):
    await db.create_dataset(name="dup", items=[{"query": "q", "expected_answer": "a", "expected_sources": []}])
    with pytest.raises(ValueError, match="already exists"):
        await db.create_dataset(name="dup", items=[{"query": "q2", "expected_answer": "a2", "expected_sources": []}])


@pytest.mark.asyncio
async def test_list_datasets(db):
    await db.create_dataset(name="ds1", items=[{"query": "q", "expected_answer": "a", "expected_sources": []}])
    await db.create_dataset(name="ds2", items=[{"query": "q", "expected_answer": "a", "expected_sources": []}])

    datasets = await db.list_datasets()
    assert len(datasets) == 2
    names = {d["name"] for d in datasets}
    assert names == {"ds1", "ds2"}


@pytest.mark.asyncio
async def test_create_and_get_evaluation(db):
    ds_id = await db.create_dataset(name="ds", items=[{"query": "q", "expected_answer": "a", "expected_sources": []}])
    eval_id = await db.create_evaluation(dataset_id=ds_id, collection="documents")

    evaluation = await db.get_evaluation(eval_id)
    assert evaluation["status"] == "running"
    assert evaluation["dataset_id"] == ds_id
    assert evaluation["collection"] == "documents"


@pytest.mark.asyncio
async def test_complete_evaluation(db):
    ds_id = await db.create_dataset(name="ds", items=[{"query": "q", "expected_answer": "a", "expected_sources": []}])
    eval_id = await db.create_evaluation(dataset_id=ds_id, collection="documents")

    aggregate = {"faithfulness": 0.87, "answer_relevancy": 0.92}
    results = [{"query": "q", "answer": "a", "contexts": [], "scores": {"faithfulness": 0.87}}]

    await db.complete_evaluation(eval_id, aggregate_scores=aggregate, results=results)

    evaluation = await db.get_evaluation(eval_id)
    assert evaluation["status"] == "completed"
    assert evaluation["aggregate_scores"]["faithfulness"] == 0.87
    assert len(evaluation["results"]) == 1


@pytest.mark.asyncio
async def test_fail_evaluation(db):
    ds_id = await db.create_dataset(name="ds", items=[{"query": "q", "expected_answer": "a", "expected_sources": []}])
    eval_id = await db.create_evaluation(dataset_id=ds_id, collection="documents")

    await db.fail_evaluation(eval_id, error="LLM timeout")

    evaluation = await db.get_evaluation(eval_id)
    assert evaluation["status"] == "failed"
    assert evaluation["error"] == "LLM timeout"


@pytest.mark.asyncio
async def test_list_evaluations(db):
    ds_id = await db.create_dataset(name="ds", items=[{"query": "q", "expected_answer": "a", "expected_sources": []}])
    await db.create_evaluation(dataset_id=ds_id, collection="documents")
    await db.create_evaluation(dataset_id=ds_id, collection="documents")

    evaluations = await db.list_evaluations(limit=10, offset=0)
    assert len(evaluations) == 2


@pytest.mark.asyncio
async def test_get_dataset_not_found(db):
    result = await db.get_dataset("nonexistent")
    assert result is None


@pytest.mark.asyncio
async def test_get_evaluation_not_found(db):
    result = await db.get_evaluation("nonexistent")
    assert result is None
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd services/eval && python -m pytest tests/test_db.py -v`

Expected: FAIL — `app.db` module not found

- [ ] **Step 3: Implement the database module**

Create `services/eval/app/db.py`:

```python
import json
import uuid
from datetime import datetime, timezone

import aiosqlite


class EvalDB:
    def __init__(self, db_path: str):
        self.db_path = db_path
        self._db: aiosqlite.Connection | None = None

    async def init(self):
        """Initialize the database and create tables."""
        self._db = await aiosqlite.connect(self.db_path)
        self._db.row_factory = aiosqlite.Row
        await self._db.executescript(
            """
            CREATE TABLE IF NOT EXISTS datasets (
                id TEXT PRIMARY KEY,
                name TEXT UNIQUE NOT NULL,
                items TEXT NOT NULL,
                created_at TEXT NOT NULL
            );
            CREATE TABLE IF NOT EXISTS evaluations (
                id TEXT PRIMARY KEY,
                dataset_id TEXT NOT NULL REFERENCES datasets(id),
                status TEXT NOT NULL DEFAULT 'running',
                collection TEXT,
                aggregate_scores TEXT,
                results TEXT,
                error TEXT,
                created_at TEXT NOT NULL,
                completed_at TEXT
            );
            """
        )
        await self._db.commit()

    async def close(self):
        if self._db:
            await self._db.close()

    async def create_dataset(self, name: str, items: list[dict]) -> str:
        """Create a golden dataset. Raises ValueError if name already exists."""
        existing = await self._db.execute(
            "SELECT id FROM datasets WHERE name = ?", (name,)
        )
        if await existing.fetchone():
            raise ValueError(f"Dataset '{name}' already exists")

        ds_id = str(uuid.uuid4())
        now = datetime.now(timezone.utc).isoformat()
        await self._db.execute(
            "INSERT INTO datasets (id, name, items, created_at) VALUES (?, ?, ?, ?)",
            (ds_id, name, json.dumps(items), now),
        )
        await self._db.commit()
        return ds_id

    async def get_dataset(self, ds_id: str) -> dict | None:
        cursor = await self._db.execute(
            "SELECT * FROM datasets WHERE id = ?", (ds_id,)
        )
        row = await cursor.fetchone()
        if not row:
            return None
        return {
            "id": row["id"],
            "name": row["name"],
            "items": json.loads(row["items"]),
            "created_at": row["created_at"],
        }

    async def list_datasets(self) -> list[dict]:
        cursor = await self._db.execute(
            "SELECT id, name, created_at FROM datasets ORDER BY created_at DESC"
        )
        rows = await cursor.fetchall()
        return [{"id": r["id"], "name": r["name"], "created_at": r["created_at"]} for r in rows]

    async def create_evaluation(self, dataset_id: str, collection: str) -> str:
        eval_id = str(uuid.uuid4())
        now = datetime.now(timezone.utc).isoformat()
        await self._db.execute(
            "INSERT INTO evaluations (id, dataset_id, status, collection, created_at) VALUES (?, ?, 'running', ?, ?)",
            (eval_id, dataset_id, collection, now),
        )
        await self._db.commit()
        return eval_id

    async def get_evaluation(self, eval_id: str) -> dict | None:
        cursor = await self._db.execute(
            "SELECT * FROM evaluations WHERE id = ?", (eval_id,)
        )
        row = await cursor.fetchone()
        if not row:
            return None
        return {
            "id": row["id"],
            "dataset_id": row["dataset_id"],
            "status": row["status"],
            "collection": row["collection"],
            "aggregate_scores": json.loads(row["aggregate_scores"]) if row["aggregate_scores"] else None,
            "results": json.loads(row["results"]) if row["results"] else None,
            "error": row["error"],
            "created_at": row["created_at"],
            "completed_at": row["completed_at"],
        }

    async def list_evaluations(self, limit: int = 20, offset: int = 0) -> list[dict]:
        cursor = await self._db.execute(
            "SELECT id, dataset_id, status, collection, aggregate_scores, created_at, completed_at "
            "FROM evaluations ORDER BY created_at DESC LIMIT ? OFFSET ?",
            (limit, offset),
        )
        rows = await cursor.fetchall()
        return [
            {
                "id": r["id"],
                "dataset_id": r["dataset_id"],
                "status": r["status"],
                "collection": r["collection"],
                "aggregate_scores": json.loads(r["aggregate_scores"]) if r["aggregate_scores"] else None,
                "created_at": r["created_at"],
                "completed_at": r["completed_at"],
            }
            for r in rows
        ]

    async def complete_evaluation(
        self, eval_id: str, aggregate_scores: dict, results: list[dict]
    ):
        now = datetime.now(timezone.utc).isoformat()
        await self._db.execute(
            "UPDATE evaluations SET status = 'completed', aggregate_scores = ?, results = ?, completed_at = ? WHERE id = ?",
            (json.dumps(aggregate_scores), json.dumps(results), now, eval_id),
        )
        await self._db.commit()

    async def fail_evaluation(self, eval_id: str, error: str):
        now = datetime.now(timezone.utc).isoformat()
        await self._db.execute(
            "UPDATE evaluations SET status = 'failed', error = ?, completed_at = ? WHERE id = ?",
            (error, now, eval_id),
        )
        await self._db.commit()
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd services/eval && python -m pytest tests/test_db.py -v`

Expected: All 9 tests pass

- [ ] **Step 5: Commit**

```bash
git add services/eval/app/db.py services/eval/tests/test_db.py
git commit -m "feat(eval): add async SQLite database layer for datasets and evaluations"
```

---

### Task 3: RAG client — httpx wrapper for chat service

Create the HTTP client that calls `/search` and `/chat` on the existing chat service.

**Files:**
- Create: `services/eval/app/rag_client.py`
- Create: `services/eval/tests/test_rag_client.py`

- [ ] **Step 1: Write failing tests for the RAG client**

Create `services/eval/tests/test_rag_client.py`:

```python
import json

import httpx
import pytest

from app.rag_client import RAGClient


@pytest.fixture
def mock_search_response():
    return {
        "results": [
            {"text": "Kubernetes is a container orchestration platform.", "filename": "k8s.pdf", "page_number": 1, "score": 0.95},
            {"text": "Pods are the smallest deployable units.", "filename": "k8s.pdf", "page_number": 3, "score": 0.82},
        ]
    }


@pytest.fixture
def mock_chat_response():
    return {
        "answer": "Kubernetes is a container orchestration platform for automating deployment.",
        "sources": [{"file": "k8s.pdf", "page": 1}],
    }


@pytest.mark.asyncio
async def test_search(mock_search_response):
    async def mock_handler(request: httpx.Request) -> httpx.Response:
        assert request.url.path == "/search"
        body = json.loads(request.content)
        assert body["query"] == "what is kubernetes"
        assert body["limit"] == 5
        return httpx.Response(200, json=mock_search_response)

    transport = httpx.MockTransport(mock_handler)
    client = RAGClient(base_url="http://chat:8000", transport=transport)

    results = await client.search("what is kubernetes", collection=None, limit=5)
    assert len(results) == 2
    assert results[0]["text"] == "Kubernetes is a container orchestration platform."
    assert results[0]["score"] == 0.95


@pytest.mark.asyncio
async def test_search_with_collection(mock_search_response):
    async def mock_handler(request: httpx.Request) -> httpx.Response:
        body = json.loads(request.content)
        assert body["collection"] == "my-docs"
        return httpx.Response(200, json=mock_search_response)

    transport = httpx.MockTransport(mock_handler)
    client = RAGClient(base_url="http://chat:8000", transport=transport)

    results = await client.search("test", collection="my-docs", limit=5)
    assert len(results) == 2


@pytest.mark.asyncio
async def test_ask(mock_chat_response):
    async def mock_handler(request: httpx.Request) -> httpx.Response:
        assert request.url.path == "/chat"
        assert request.headers["accept"] == "application/json"
        body = json.loads(request.content)
        assert body["question"] == "what is kubernetes"
        return httpx.Response(200, json=mock_chat_response)

    transport = httpx.MockTransport(mock_handler)
    client = RAGClient(base_url="http://chat:8000", transport=transport)

    answer = await client.ask("what is kubernetes", collection=None)
    assert answer["answer"] == "Kubernetes is a container orchestration platform for automating deployment."
    assert len(answer["sources"]) == 1


@pytest.mark.asyncio
async def test_search_server_error():
    async def mock_handler(request: httpx.Request) -> httpx.Response:
        return httpx.Response(500, json={"detail": "internal error"})

    transport = httpx.MockTransport(mock_handler)
    client = RAGClient(base_url="http://chat:8000", transport=transport)

    with pytest.raises(httpx.HTTPStatusError):
        await client.search("test", collection=None, limit=5)


@pytest.mark.asyncio
async def test_ask_timeout():
    async def mock_handler(request: httpx.Request) -> httpx.Response:
        raise httpx.ConnectTimeout("connection timed out")

    transport = httpx.MockTransport(mock_handler)
    client = RAGClient(base_url="http://chat:8000", transport=transport)

    with pytest.raises(httpx.ConnectTimeout):
        await client.ask("test", collection=None)
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd services/eval && python -m pytest tests/test_rag_client.py -v`

Expected: FAIL — `app.rag_client` module not found

- [ ] **Step 3: Implement the RAG client**

Create `services/eval/app/rag_client.py`:

```python
import httpx


class RAGClient:
    """HTTP client for the chat service's /search and /chat endpoints."""

    def __init__(self, base_url: str, transport: httpx.AsyncBaseTransport | None = None):
        client_kwargs = {"base_url": base_url, "timeout": 60.0}
        if transport:
            client_kwargs["transport"] = transport
        self._client = httpx.AsyncClient(**client_kwargs)

    async def search(
        self, query: str, collection: str | None, limit: int
    ) -> list[dict]:
        """Call POST /search for retrieval-only results."""
        body: dict = {"query": query, "limit": limit}
        if collection:
            body["collection"] = collection

        resp = await self._client.post("/search", json=body)
        resp.raise_for_status()
        return resp.json()["results"]

    async def ask(self, question: str, collection: str | None) -> dict:
        """Call POST /chat with Accept: application/json for a full RAG response."""
        body: dict = {"question": question}
        if collection:
            body["collection"] = collection

        resp = await self._client.post(
            "/chat", json=body, headers={"Accept": "application/json"}
        )
        resp.raise_for_status()
        return resp.json()

    async def close(self):
        await self._client.aclose()
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd services/eval && python -m pytest tests/test_rag_client.py -v`

Expected: All 5 tests pass

- [ ] **Step 5: Commit**

```bash
git add services/eval/app/rag_client.py services/eval/tests/test_rag_client.py
git commit -m "feat(eval): add RAG client for chat service /search and /chat endpoints"
```

---

### Task 4: Pydantic models

Create the request/response models for the API.

**Files:**
- Create: `services/eval/app/models.py`

- [ ] **Step 1: Create the models module**

Create `services/eval/app/models.py`:

```python
from pydantic import BaseModel, Field


class GoldenItem(BaseModel):
    query: str = Field(max_length=2000)
    expected_answer: str = Field(max_length=5000)
    expected_sources: list[str] = Field(default_factory=list)


class CreateDatasetRequest(BaseModel):
    name: str = Field(min_length=1, max_length=100, pattern=r"^[a-zA-Z0-9_-]+$")
    items: list[GoldenItem] = Field(min_length=1, max_length=100)


class DatasetSummary(BaseModel):
    id: str
    name: str
    created_at: str


class DatasetDetail(BaseModel):
    id: str
    name: str
    items: list[GoldenItem]
    created_at: str


class StartEvaluationRequest(BaseModel):
    dataset_id: str
    collection: str | None = Field(default=None, pattern=r"^[a-zA-Z0-9_-]{1,100}$")


class QueryScore(BaseModel):
    faithfulness: float | None = None
    answer_relevancy: float | None = None
    context_precision: float | None = None
    context_recall: float | None = None


class QueryResult(BaseModel):
    query: str
    answer: str
    contexts: list[str]
    scores: QueryScore


class EvaluationSummary(BaseModel):
    id: str
    dataset_id: str
    status: str
    collection: str | None
    aggregate_scores: QueryScore | None
    created_at: str
    completed_at: str | None


class EvaluationDetail(BaseModel):
    id: str
    dataset_id: str
    status: str
    collection: str | None
    aggregate_scores: QueryScore | None
    results: list[QueryResult] | None
    error: str | None
    created_at: str
    completed_at: str | None
```

- [ ] **Step 2: Commit**

```bash
git add services/eval/app/models.py
git commit -m "feat(eval): add Pydantic request/response models"
```

---

### Task 5: RAGAS evaluator module

Create the module that wraps RAGAS `evaluate()` with the project's LLM configuration.

**Files:**
- Create: `services/eval/app/evaluator.py`
- Create: `services/eval/tests/test_evaluator.py`

- [ ] **Step 1: Write failing tests for the evaluator**

Create `services/eval/tests/test_evaluator.py`:

```python
from unittest.mock import AsyncMock, MagicMock, patch

import pytest

from app.evaluator import build_ragas_dataset, run_evaluation
from app.rag_client import RAGClient


@pytest.fixture
def golden_items():
    return [
        {
            "query": "What is chunking?",
            "expected_answer": "Splitting text into smaller pieces for embedding.",
            "expected_sources": ["ingestion.pdf"],
        },
        {
            "query": "What model is used for embeddings?",
            "expected_answer": "nomic-embed-text produces 768-dimensional vectors.",
            "expected_sources": ["chat.pdf"],
        },
    ]


@pytest.fixture
def mock_search_results():
    return [
        {"text": "Text chunking splits documents into smaller pieces.", "filename": "ingestion.pdf", "page_number": 1, "score": 0.92},
        {"text": "Chunk sizes of 1000 with 200 overlap are used.", "filename": "ingestion.pdf", "page_number": 2, "score": 0.85},
    ]


@pytest.fixture
def mock_chat_answer():
    return {
        "answer": "Chunking splits text into smaller pieces for embedding and retrieval.",
        "sources": [{"file": "ingestion.pdf", "page": 1}],
    }


@pytest.mark.asyncio
async def test_build_ragas_dataset(golden_items, mock_search_results, mock_chat_answer):
    rag_client = MagicMock(spec=RAGClient)
    rag_client.search = AsyncMock(return_value=mock_search_results)
    rag_client.ask = AsyncMock(return_value=mock_chat_answer)

    dataset = await build_ragas_dataset(
        items=golden_items,
        rag_client=rag_client,
        collection=None,
    )

    assert len(dataset) == 2
    assert dataset[0]["user_input"] == "What is chunking?"
    assert dataset[0]["response"] == "Chunking splits text into smaller pieces for embedding and retrieval."
    assert len(dataset[0]["retrieved_contexts"]) == 2
    assert dataset[0]["reference"] == "Splitting text into smaller pieces for embedding."

    assert rag_client.search.call_count == 2
    assert rag_client.ask.call_count == 2


@pytest.mark.asyncio
async def test_build_ragas_dataset_with_collection(golden_items, mock_search_results, mock_chat_answer):
    rag_client = MagicMock(spec=RAGClient)
    rag_client.search = AsyncMock(return_value=mock_search_results)
    rag_client.ask = AsyncMock(return_value=mock_chat_answer)

    await build_ragas_dataset(
        items=golden_items,
        rag_client=rag_client,
        collection="my-docs",
    )

    call_args = rag_client.search.call_args_list[0]
    assert call_args.kwargs.get("collection") == "my-docs" or call_args[0][1] == "my-docs"


@pytest.mark.asyncio
@patch("app.evaluator.ragas_evaluate")
async def test_run_evaluation(mock_ragas_evaluate, golden_items, mock_search_results, mock_chat_answer):
    rag_client = MagicMock(spec=RAGClient)
    rag_client.search = AsyncMock(return_value=mock_search_results)
    rag_client.ask = AsyncMock(return_value=mock_chat_answer)

    # Mock RAGAS evaluate to return fake scores
    mock_result = MagicMock()
    mock_result.scores = [
        {"faithfulness": 0.9, "answer_relevancy": 0.85, "context_precision": 0.8, "context_recall": 0.88},
        {"faithfulness": 0.82, "answer_relevancy": 0.9, "context_precision": 0.75, "context_recall": 0.8},
    ]
    mock_ragas_evaluate.return_value = mock_result

    aggregate, results = await run_evaluation(
        items=golden_items,
        rag_client=rag_client,
        collection=None,
        llm_provider="ollama",
        llm_base_url="http://localhost:11434",
        llm_model="qwen2.5:14b",
        llm_api_key="",
    )

    assert "faithfulness" in aggregate
    assert "answer_relevancy" in aggregate
    assert len(results) == 2
    assert results[0]["query"] == "What is chunking?"
    assert results[0]["scores"]["faithfulness"] == 0.9

    mock_ragas_evaluate.assert_called_once()
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd services/eval && python -m pytest tests/test_evaluator.py -v`

Expected: FAIL — `app.evaluator` module not found

- [ ] **Step 3: Implement the evaluator module**

Create `services/eval/app/evaluator.py`:

```python
import logging

from openai import AsyncOpenAI
from ragas import evaluate as ragas_evaluate
from ragas import EvaluationDataset
from ragas.dataset_schema import SingleTurnSample
from ragas.llms import llm_factory
from ragas.metrics import AnswerRelevancy, ContextPrecision, ContextRecall, Faithfulness

from app.rag_client import RAGClient

logger = logging.getLogger(__name__)


async def build_ragas_dataset(
    items: list[dict],
    rag_client: RAGClient,
    collection: str | None,
) -> list[dict]:
    """Run each golden item through the RAG pipeline and build RAGAS evaluation rows."""
    dataset = []
    for item in items:
        query = item["query"]
        search_results = await rag_client.search(query, collection=collection, limit=5)
        chat_response = await rag_client.ask(query, collection=collection)

        dataset.append(
            {
                "user_input": query,
                "retrieved_contexts": [r["text"] for r in search_results],
                "response": chat_response["answer"],
                "reference": item["expected_answer"],
            }
        )
    return dataset


def _create_llm(provider: str, base_url: str, model: str, api_key: str):
    """Create a RAGAS-compatible LLM from the service config."""
    if provider == "ollama":
        client = AsyncOpenAI(api_key="ollama", base_url=f"{base_url}/v1")
        return llm_factory(model, provider="openai", client=client)
    else:
        client = AsyncOpenAI(api_key=api_key, base_url=base_url)
        return llm_factory(model, provider="openai", client=client)


async def run_evaluation(
    items: list[dict],
    rag_client: RAGClient,
    collection: str | None,
    llm_provider: str,
    llm_base_url: str,
    llm_model: str,
    llm_api_key: str,
) -> tuple[dict, list[dict]]:
    """Run a full RAGAS evaluation and return (aggregate_scores, per_query_results)."""
    # Step 1: Build dataset by running queries through RAG pipeline
    raw_dataset = await build_ragas_dataset(items, rag_client, collection)

    # Step 2: Convert to RAGAS EvaluationDataset
    samples = [
        SingleTurnSample(
            user_input=row["user_input"],
            retrieved_contexts=row["retrieved_contexts"],
            response=row["response"],
            reference=row["reference"],
        )
        for row in raw_dataset
    ]
    eval_dataset = EvaluationDataset(samples=samples)

    # Step 3: Create LLM for judge calls
    judge_llm = _create_llm(llm_provider, llm_base_url, llm_model, llm_api_key)

    # Step 4: Run RAGAS evaluate
    metrics = [
        Faithfulness(llm=judge_llm),
        AnswerRelevancy(llm=judge_llm),
        ContextPrecision(llm=judge_llm),
        ContextRecall(llm=judge_llm),
    ]

    result = ragas_evaluate(dataset=eval_dataset, metrics=metrics)

    # Step 5: Extract scores
    scores = result.scores
    metric_names = ["faithfulness", "answer_relevancy", "context_precision", "context_recall"]

    # Compute aggregates
    aggregate = {}
    for name in metric_names:
        values = [s.get(name) for s in scores if s.get(name) is not None]
        aggregate[name] = round(sum(values) / len(values), 4) if values else None

    # Build per-query results
    per_query = []
    for i, row in enumerate(raw_dataset):
        per_query.append(
            {
                "query": row["user_input"],
                "answer": row["response"],
                "contexts": row["retrieved_contexts"],
                "scores": scores[i] if i < len(scores) else {},
            }
        )

    return aggregate, per_query
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd services/eval && python -m pytest tests/test_evaluator.py -v`

Expected: All 3 tests pass

- [ ] **Step 5: Commit**

```bash
git add services/eval/app/evaluator.py services/eval/tests/test_evaluator.py
git commit -m "feat(eval): add RAGAS evaluator module with dataset builder and score extraction"
```

---

### Task 6: Prometheus metrics

Create custom metrics for the eval service.

**Files:**
- Create: `services/eval/app/metrics.py`

- [ ] **Step 1: Create the metrics module**

Create `services/eval/app/metrics.py`:

```python
from prometheus_client import Counter, Gauge, Histogram

eval_run_duration_seconds = Histogram(
    "eval_run_duration_seconds",
    "Duration of a full evaluation run",
    buckets=[10, 30, 60, 120, 300, 600, 1200],
)

eval_ragas_score = Gauge(
    "eval_ragas_score",
    "Latest RAGAS metric score",
    ["metric"],
)

eval_queries_total = Counter(
    "eval_queries_total",
    "Total number of queries evaluated",
)
```

- [ ] **Step 2: Commit**

```bash
git add services/eval/app/metrics.py
git commit -m "feat(eval): add Prometheus metrics for evaluation runs"
```

---

### Task 7: API endpoints

Wire everything together in `main.py` with the full set of endpoints.

**Files:**
- Modify: `services/eval/app/main.py`
- Create: `services/eval/tests/test_main.py`

- [ ] **Step 1: Write failing tests for all endpoints**

Create `services/eval/tests/test_main.py`:

```python
from unittest.mock import AsyncMock, patch

import pytest
from fastapi.testclient import TestClient

from app.main import app

client = TestClient(app)


# --- Dataset endpoints ---


@patch("app.main.get_db")
def test_create_dataset(mock_get_db):
    mock_db = AsyncMock()
    mock_db.create_dataset.return_value = "ds-123"
    mock_get_db.return_value = mock_db

    response = client.post(
        "/datasets",
        json={
            "name": "test-dataset",
            "items": [
                {
                    "query": "What is chunking?",
                    "expected_answer": "Splitting text into smaller pieces",
                    "expected_sources": ["ingestion.pdf"],
                }
            ],
        },
    )
    assert response.status_code == 201
    assert response.json()["id"] == "ds-123"


def test_create_dataset_invalid_name():
    response = client.post(
        "/datasets",
        json={
            "name": "invalid name with spaces!",
            "items": [{"query": "q", "expected_answer": "a", "expected_sources": []}],
        },
    )
    assert response.status_code == 422


def test_create_dataset_empty_items():
    response = client.post(
        "/datasets",
        json={"name": "valid-name", "items": []},
    )
    assert response.status_code == 422


@patch("app.main.get_db")
def test_create_dataset_duplicate_name(mock_get_db):
    mock_db = AsyncMock()
    mock_db.create_dataset.side_effect = ValueError("Dataset 'dup' already exists")
    mock_get_db.return_value = mock_db

    response = client.post(
        "/datasets",
        json={
            "name": "dup",
            "items": [{"query": "q", "expected_answer": "a", "expected_sources": []}],
        },
    )
    assert response.status_code == 409


@patch("app.main.get_db")
def test_list_datasets(mock_get_db):
    mock_db = AsyncMock()
    mock_db.list_datasets.return_value = [
        {"id": "ds-1", "name": "ds1", "created_at": "2026-04-16T00:00:00Z"},
        {"id": "ds-2", "name": "ds2", "created_at": "2026-04-16T01:00:00Z"},
    ]
    mock_get_db.return_value = mock_db

    response = client.get("/datasets")
    assert response.status_code == 200
    assert len(response.json()["datasets"]) == 2


# --- Evaluation endpoints ---


@patch("app.main.get_db")
def test_start_evaluation(mock_get_db):
    mock_db = AsyncMock()
    mock_db.get_dataset.return_value = {
        "id": "ds-123",
        "name": "test",
        "items": [{"query": "q", "expected_answer": "a", "expected_sources": []}],
        "created_at": "2026-04-16T00:00:00Z",
    }
    mock_db.create_evaluation.return_value = "eval-456"
    mock_get_db.return_value = mock_db

    response = client.post(
        "/evaluations",
        json={"dataset_id": "ds-123"},
    )
    assert response.status_code == 202
    assert response.json()["id"] == "eval-456"


@patch("app.main.get_db")
def test_start_evaluation_dataset_not_found(mock_get_db):
    mock_db = AsyncMock()
    mock_db.get_dataset.return_value = None
    mock_get_db.return_value = mock_db

    response = client.post(
        "/evaluations",
        json={"dataset_id": "nonexistent"},
    )
    assert response.status_code == 404


@patch("app.main.get_db")
def test_get_evaluation(mock_get_db):
    mock_db = AsyncMock()
    mock_db.get_evaluation.return_value = {
        "id": "eval-456",
        "dataset_id": "ds-123",
        "status": "completed",
        "collection": "documents",
        "aggregate_scores": {"faithfulness": 0.87, "answer_relevancy": 0.92},
        "results": [{"query": "q", "answer": "a", "contexts": [], "scores": {"faithfulness": 0.87}}],
        "error": None,
        "created_at": "2026-04-16T00:00:00Z",
        "completed_at": "2026-04-16T00:05:00Z",
    }
    mock_get_db.return_value = mock_db

    response = client.get("/evaluations/eval-456")
    assert response.status_code == 200
    assert response.json()["status"] == "completed"
    assert response.json()["aggregate_scores"]["faithfulness"] == 0.87


@patch("app.main.get_db")
def test_get_evaluation_not_found(mock_get_db):
    mock_db = AsyncMock()
    mock_db.get_evaluation.return_value = None
    mock_get_db.return_value = mock_db

    response = client.get("/evaluations/nonexistent")
    assert response.status_code == 404


@patch("app.main.get_db")
def test_list_evaluations(mock_get_db):
    mock_db = AsyncMock()
    mock_db.list_evaluations.return_value = [
        {
            "id": "eval-1",
            "dataset_id": "ds-1",
            "status": "completed",
            "collection": None,
            "aggregate_scores": {"faithfulness": 0.87},
            "created_at": "2026-04-16T00:00:00Z",
            "completed_at": "2026-04-16T00:05:00Z",
        }
    ]
    mock_get_db.return_value = mock_db

    response = client.get("/evaluations")
    assert response.status_code == 200
    assert len(response.json()["evaluations"]) == 1


# --- Health check ---


@patch("app.main.httpx.AsyncClient")
def test_health_degraded_when_chat_unreachable(mock_client_cls):
    mock_client = AsyncMock()
    mock_client.__aenter__ = AsyncMock(return_value=mock_client)
    mock_client.__aexit__ = AsyncMock(return_value=False)
    mock_client.get.side_effect = Exception("connection refused")
    mock_client_cls.return_value = mock_client

    response = client.get("/health")
    assert response.status_code == 503
    assert response.json()["status"] == "degraded"
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd services/eval && python -m pytest tests/test_main.py -v`

Expected: FAIL — endpoints not implemented yet

- [ ] **Step 3: Replace main.py with all endpoints**

Replace the entire contents of `services/eval/app/main.py`:

```python
import logging
import os
import time

import httpx
from fastapi import BackgroundTasks, Depends, FastAPI, HTTPException
from fastapi.middleware.cors import CORSMiddleware
from prometheus_fastapi_instrumentator import Instrumentator
from slowapi import Limiter, _rate_limit_exceeded_handler
from slowapi.errors import RateLimitExceeded
from slowapi.util import get_remote_address
from starlette.requests import Request
from starlette.responses import JSONResponse

from app.config import settings
from app.db import EvalDB
from app.evaluator import run_evaluation
from app.metrics import eval_queries_total, eval_ragas_score, eval_run_duration_seconds
from app.models import CreateDatasetRequest, StartEvaluationRequest
from app.rag_client import RAGClient
from shared.auth import create_auth_dependency

logger = logging.getLogger(__name__)

app = FastAPI(title="Eval API")

app.add_middleware(
    CORSMiddleware,
    allow_origins=settings.allowed_origins.split(","),
    allow_methods=["GET", "POST"],
    allow_headers=["Authorization", "Content-Type"],
)

instrumentator = Instrumentator()
instrumentator.instrument(app).expose(app, include_in_schema=False)

limiter = Limiter(key_func=get_remote_address)
app.state.limiter = limiter
app.add_exception_handler(RateLimitExceeded, _rate_limit_exceeded_handler)

require_auth = create_auth_dependency(settings.jwt_secret)

_db: EvalDB | None = None


async def get_db() -> EvalDB:
    global _db
    if _db is None:
        os.makedirs(os.path.dirname(settings.db_path) or ".", exist_ok=True)
        _db = EvalDB(settings.db_path)
        await _db.init()
    return _db


@app.on_event("shutdown")
async def shutdown():
    if _db:
        await _db.close()


# --- Health ---


@app.get("/health")
async def health():
    """Health check — verifies chat service is reachable."""
    chat_ok = True
    try:
        async with httpx.AsyncClient(timeout=5.0) as client:
            resp = await client.get(f"{settings.chat_service_url}/health")
            if resp.status_code != 200:
                chat_ok = False
    except Exception:
        chat_ok = False

    status = "healthy" if chat_ok else "degraded"
    code = 200 if chat_ok else 503
    return JSONResponse(
        status_code=code,
        content={"status": status, "chat_service": "ok" if chat_ok else "unreachable"},
    )


# --- Datasets ---


@app.post("/datasets", status_code=201)
@limiter.limit("10/minute")
async def create_dataset(
    request: Request, body: CreateDatasetRequest, user_id: str = Depends(require_auth)
):
    db = await get_db()
    try:
        ds_id = await db.create_dataset(
            name=body.name,
            items=[item.model_dump() for item in body.items],
        )
    except ValueError as e:
        raise HTTPException(status_code=409, detail=str(e))
    return {"id": ds_id}


@app.get("/datasets")
@limiter.limit("30/minute")
async def list_datasets(request: Request, user_id: str = Depends(require_auth)):
    db = await get_db()
    datasets = await db.list_datasets()
    return {"datasets": datasets}


# --- Evaluations ---


async def _run_evaluation_task(
    eval_id: str, items: list[dict], collection: str | None
):
    """Background task that runs the RAGAS evaluation."""
    db = await get_db()
    rag_client = RAGClient(base_url=settings.chat_service_url)
    start = time.perf_counter()

    try:
        aggregate, results = await run_evaluation(
            items=items,
            rag_client=rag_client,
            collection=collection,
            llm_provider=settings.llm_provider,
            llm_base_url=settings.llm_base_url,
            llm_model=settings.llm_model,
            llm_api_key=settings.llm_api_key,
        )
        await db.complete_evaluation(eval_id, aggregate_scores=aggregate, results=results)

        # Update metrics
        eval_run_duration_seconds.observe(time.perf_counter() - start)
        eval_queries_total.inc(len(items))
        for metric_name, score in aggregate.items():
            if score is not None:
                eval_ragas_score.labels(metric=metric_name).set(score)

        logger.info("Evaluation %s completed: %s", eval_id, aggregate)
    except Exception as e:
        logger.error("Evaluation %s failed: %s", eval_id, e, exc_info=True)
        await db.fail_evaluation(eval_id, error=str(e))
    finally:
        await rag_client.close()


@app.post("/evaluations", status_code=202)
@limiter.limit("5/minute")
async def start_evaluation(
    request: Request,
    body: StartEvaluationRequest,
    background_tasks: BackgroundTasks,
    user_id: str = Depends(require_auth),
):
    db = await get_db()
    dataset = await db.get_dataset(body.dataset_id)
    if not dataset:
        raise HTTPException(status_code=404, detail="Dataset not found")

    eval_id = await db.create_evaluation(
        dataset_id=body.dataset_id, collection=body.collection or "documents"
    )

    background_tasks.add_task(
        _run_evaluation_task, eval_id, dataset["items"], body.collection
    )

    return {"id": eval_id, "status": "running"}


@app.get("/evaluations/{eval_id}")
@limiter.limit("30/minute")
async def get_evaluation(request: Request, eval_id: str, user_id: str = Depends(require_auth)):
    db = await get_db()
    evaluation = await db.get_evaluation(eval_id)
    if not evaluation:
        raise HTTPException(status_code=404, detail="Evaluation not found")
    return evaluation


@app.get("/evaluations")
@limiter.limit("30/minute")
async def list_evaluations(
    request: Request,
    limit: int = 20,
    offset: int = 0,
    user_id: str = Depends(require_auth),
):
    db = await get_db()
    evaluations = await db.list_evaluations(limit=limit, offset=offset)
    return {"evaluations": evaluations}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd services/eval && python -m pytest tests/test_main.py -v`

Expected: All 10 tests pass

- [ ] **Step 5: Run all eval service tests**

Run: `cd services/eval && python -m pytest tests/ -v`

Expected: All tests pass (db + rag_client + evaluator + main)

- [ ] **Step 6: Run preflight**

Run: `make preflight-python`

Expected: ruff lint + format + pytest all pass

- [ ] **Step 7: Commit**

```bash
git add services/eval/app/main.py services/eval/tests/test_main.py
git commit -m "feat(eval): add dataset and evaluation API endpoints with background task runner"
```

---

### Task 8: Docker Compose integration

Add the eval service to docker-compose.yml and nginx config.

**Files:**
- Modify: `docker-compose.yml`
- Modify: `nginx/nginx.conf`

- [ ] **Step 1: Add eval service to docker-compose.yml**

Add after the `debug` service block in `docker-compose.yml`:

```yaml
  eval:
    image: ghcr.io/kabradshaw1/portfolio/eval:latest
    build:
      context: ./services
      dockerfile: eval/Dockerfile
    env_file: .env
    volumes:
      - eval_data:/app/data
    depends_on:
      chat:
        condition: service_started
    extra_hosts:
      - "host.docker.internal:host-gateway"
```

Add `eval_data:` to the `volumes:` section at the bottom of the file (alongside `qdrant_data`):

```yaml
  eval_data:
```

- [ ] **Step 2: Add eval upstream and location to nginx.conf**

Add an upstream block in `nginx/nginx.conf` (after the existing upstream blocks around line 39):

```nginx
    upstream eval {
        server eval:8000;
    }
```

Add a rate limit zone (after the existing zones around line 27):

```nginx
    limit_req_zone $binary_remote_addr zone=eval:10m rate=5r/m;
```

Add a location block (after the existing `/debug/` block):

```nginx
        location /eval/ {
            limit_req zone=eval burst=2 nodelay;
            proxy_pass http://eval/;
            proxy_set_header Host $host;
            proxy_set_header X-Real-IP $remote_addr;
            proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
            proxy_set_header X-Forwarded-Proto $scheme;
            proxy_set_header Connection '';
            proxy_http_version 1.1;
            chunked_transfer_encoding off;
        }
```

- [ ] **Step 3: Add eval to gateway depends_on**

In `docker-compose.yml`, add `eval` to the gateway service's `depends_on` list (alongside ingestion, chat, debug):

```yaml
    depends_on:
      - ingestion
      - chat
      - debug
      - eval
```

- [ ] **Step 4: Commit**

```bash
git add docker-compose.yml nginx/nginx.conf
git commit -m "chore: add eval service to Docker Compose and nginx config"
```

---

### Task 9: Kubernetes manifests

Create the K8s deployment, service, configmap, and update kustomization/ingress.

**Files:**
- Create: `k8s/ai-services/deployments/eval.yml`
- Create: `k8s/ai-services/services/eval.yml`
- Create: `k8s/ai-services/configmaps/eval-config.yml`
- Modify: `k8s/ai-services/kustomization.yaml`
- Modify: `k8s/ai-services/ingress.yml`

- [ ] **Step 1: Create the eval deployment**

Create `k8s/ai-services/deployments/eval.yml`:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: eval
  namespace: ai-services
spec:
  replicas: 1
  selector:
    matchLabels:
      app: eval
  template:
    metadata:
      labels:
        app: eval
      annotations:
        prometheus.io/scrape: "true"
        prometheus.io/port: "8000"
        prometheus.io/path: "/metrics"
    spec:
      imagePullSecrets:
        - name: ghcr-secret
      containers:
        - name: eval
          image: ghcr.io/kabradshaw1/portfolio/eval:latest
          ports:
            - containerPort: 8000
          envFrom:
            - configMapRef:
                name: eval-config
          volumeMounts:
            - name: eval-data
              mountPath: /app/data
          resources:
            requests:
              memory: "128Mi"
              cpu: "100m"
            limits:
              memory: "512Mi"
              cpu: "500m"
          readinessProbe:
            httpGet:
              path: /health
              port: 8000
            initialDelaySeconds: 10
            periodSeconds: 10
            timeoutSeconds: 5
      volumes:
        - name: eval-data
          emptyDir: {}
```

- [ ] **Step 2: Create the eval service**

Create `k8s/ai-services/services/eval.yml`:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: eval
  namespace: ai-services
spec:
  selector:
    app: eval
  ports:
    - port: 8000
      targetPort: 8000
```

- [ ] **Step 3: Create the eval configmap**

Create `k8s/ai-services/configmaps/eval-config.yml`:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: eval-config
  namespace: ai-services
data:
  CHAT_SERVICE_URL: http://chat:8000
  LLM_PROVIDER: ollama
  LLM_BASE_URL: http://ollama:11434
  LLM_MODEL: qwen2.5:14b
  DB_PATH: /app/data/eval.db
  ALLOWED_ORIGINS: http://localhost:3000,https://kylebradshaw.dev
```

- [ ] **Step 4: Update kustomization.yaml**

Add the new resources to `k8s/ai-services/kustomization.yaml`:

Add these three lines to the `resources` list:
```yaml
  - configmaps/eval-config.yml
  - deployments/eval.yml
  - services/eval.yml
```

- [ ] **Step 5: Update ingress.yml**

Add the eval path to `k8s/ai-services/ingress.yml`, after the `/debug` path block:

```yaml
          - path: /eval(/|$)(.*)
            pathType: ImplementationSpecific
            backend:
              service:
                name: eval
                port:
                  number: 8000
```

- [ ] **Step 6: Commit**

```bash
git add k8s/ai-services/
git commit -m "chore(k8s): add eval service deployment, service, configmap, and ingress route"
```

---

### Task 10: CI pipeline updates

Add the eval service to all CI matrices.

**Files:**
- Modify: `.github/workflows/ci.yml`

- [ ] **Step 1: Add eval to python-tests matrix**

In `.github/workflows/ci.yml`, find the `python-tests` job matrix (around line 53):

```yaml
      service: [ingestion, chat, debug]
```

Change to:

```yaml
      service: [ingestion, chat, debug, eval]
```

- [ ] **Step 2: Add eval to pip-audit matrix**

Find the `security-pip-audit` job matrix (around line 470):

```yaml
      service: [ingestion, chat, debug]
```

Change to:

```yaml
      service: [ingestion, chat, debug, eval]
```

- [ ] **Step 3: Add eval Dockerfile to hadolint matrix**

Find the `security-hadolint` job matrix (around line 539-549) and add after the debug Dockerfile line:

```yaml
        - services/eval/Dockerfile
```

- [ ] **Step 4: Add eval to build-images matrix**

Find the `build-images` job include list (around line 593-604) and add after the debug entry:

```yaml
          - service: eval
            context: services
            file: services/eval/Dockerfile
            image: eval
```

- [ ] **Step 5: Commit**

```bash
git add .github/workflows/ci.yml
git commit -m "ci: add eval service to test, security, and build matrices"
```

---

### Task 11: End-to-end verification

Run all preflight checks and verify the service works.

**Files:** None (verification only)

- [ ] **Step 1: Run full preflight**

Run: `make preflight-python`

Expected: ruff lint + format + pytest all pass for all services including eval

- [ ] **Step 2: Run security checks**

Run: `make preflight-security`

Expected: bandit + pip-audit pass

- [ ] **Step 3: Verify Docker Compose builds**

Run: `docker compose build eval`

Expected: Image builds successfully

- [ ] **Step 4: Push and watch CI**

```bash
git push origin HEAD
```

Monitor GitHub Actions. Fix any CI failures before creating PR.

- [ ] **Step 5: Create PR to qa**

```bash
gh pr create --base qa --title "feat: RAG evaluation service with RAGAS metrics" --body "$(cat <<'EOF'
## Summary
- New `services/eval` Python microservice for evaluating RAG pipeline quality
- Uses RAGAS framework (faithfulness, answer relevancy, context precision, context recall)
- Golden dataset management (upload, list, reuse)
- Async evaluation runs with SQLite-stored scorecards
- Full K8s deployment, Docker Compose, CI integration

## Test plan
- [ ] Unit tests for DB layer, RAG client, evaluator, and API endpoints
- [ ] ruff lint/format pass
- [ ] pip-audit + bandit pass
- [ ] Docker image builds
- [ ] Health check returns 200 (degraded if chat service not running)
- [ ] Manual: upload golden dataset, trigger eval, verify scorecard

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```
