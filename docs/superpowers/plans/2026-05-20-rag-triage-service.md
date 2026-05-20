# RAG Triage Service Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a new Python `services/rag-triage` service that diagnoses RAG eval regressions and expose it through one additive eval MCP tool.

**Architecture:** The triage service calls the existing eval API over HTTP and returns deterministic, structured diagnoses. The eval MCP stays thin: it validates tool inputs, calls the triage HTTP API, and returns JSON. LangGraph and LLM summaries are deferred until triage becomes a stateful investigation workflow.

**Tech Stack:** Python, FastAPI, Pydantic, httpx, pytest, respx, Prometheus FastAPI instrumentator, Docker Compose, Go MCP SDK, Go `net/http`.

---

## Execution Notes

- Do implementation from a feature worktree because this changes application code and Docker wiring.
- Suggested branch/worktree: `.codex/worktrees/rag-triage-service` on branch `feature/rag-triage-service`.
- Before editing files, confirm `pwd`, `git branch --show-current`, and `git rev-parse --show-toplevel` point at the worktree.
- Keep the eval MCP wrapper until the Python service tests pass. This reduces conflicts with concurrent eval MCP work.
- Commit after each task that leaves tests passing.

## File Map

Create Python service files:

- `services/rag-triage/requirements.txt`: service dependencies.
- `services/rag-triage/Dockerfile`: production image.
- `services/rag-triage/app/__init__.py`: package marker.
- `services/rag-triage/app/config.py`: env-based settings.
- `services/rag-triage/app/models.py`: request, response, eval DTO, diagnosis models.
- `services/rag-triage/app/eval_client.py`: async HTTP client for eval API.
- `services/rag-triage/app/rules.py`: deterministic failure classification and recommendations.
- `services/rag-triage/app/service.py`: orchestration for run and comparison triage.
- `services/rag-triage/app/metrics.py`: FastAPI Prometheus instrumentation and counters.
- `services/rag-triage/app/main.py`: FastAPI app and endpoints.

Create Python tests:

- `services/rag-triage/tests/__init__.py`
- `services/rag-triage/tests/conftest.py`
- `services/rag-triage/tests/test_config.py`
- `services/rag-triage/tests/test_eval_client.py`
- `services/rag-triage/tests/test_rules.py`
- `services/rag-triage/tests/test_service.py`
- `services/rag-triage/tests/test_main.py`

Modify repo wiring:

- `docker-compose.yml`: add `rag-triage` service and gateway dependency.
- `nginx/nginx.conf`: route `/rag-triage/` to the new service if existing gateway patterns require explicit upstreams.
- `.github/workflows/ci.yml`: add `rag-triage` to Python test, Docker build, pip-audit, and Hadolint matrices.

Create or modify eval MCP files:

- `go/eval-mcp-service/internal/triageapi/client.go`: triage HTTP client.
- `go/eval-mcp-service/internal/triageapi/client_test.go`: client tests.
- `go/eval-mcp-service/internal/config/config.go`: add triage API URL setting.
- `go/eval-mcp-service/internal/evalworkflow/service.go`: add `TriageRAGRegression`.
- `go/eval-mcp-service/internal/evalworkflow/service_test.go`: workflow tests.
- `go/eval-mcp-service/internal/mcpserver/server.go`: add interface method, schema, handler, and tool registration.
- `go/eval-mcp-service/internal/mcpserver/server_test.go`: registration and handler tests.
- `go/eval-mcp-service/cmd/eval-mcp/main.go`: construct triage client and pass it to workflow service.
- `go/eval-mcp-service/README.md`: document tool and config.

---

### Task 1: Create Feature Worktree

**Files:**
- No tracked file edits.

- [ ] **Step 1: Confirm current branch and status**

Run:

```bash
git branch --show-current
git status --short
```

Expected: current branch is `main` or another non-feature branch; existing unrelated changes, such as `.gitignore`, are noted and left untouched.

- [ ] **Step 2: Create the worktree**

Run:

```bash
git worktree add .codex/worktrees/rag-triage-service -b feature/rag-triage-service
```

Expected: worktree is created successfully.

- [ ] **Step 3: Switch all work into the worktree**

Run:

```bash
cd .codex/worktrees/rag-triage-service
pwd
git branch --show-current
git rev-parse --show-toplevel
```

Expected: `pwd` and top-level path are inside `.codex/worktrees/rag-triage-service`; branch is `feature/rag-triage-service`.

---

### Task 2: Scaffold Python Service Config, Models, Health, And Eval Client

**Files:**
- Create: `services/rag-triage/requirements.txt`
- Create: `services/rag-triage/app/__init__.py`
- Create: `services/rag-triage/app/config.py`
- Create: `services/rag-triage/app/models.py`
- Create: `services/rag-triage/app/eval_client.py`
- Create: `services/rag-triage/app/metrics.py`
- Create: `services/rag-triage/app/main.py`
- Create: `services/rag-triage/tests/__init__.py`
- Create: `services/rag-triage/tests/conftest.py`
- Create: `services/rag-triage/tests/test_config.py`
- Create: `services/rag-triage/tests/test_eval_client.py`
- Create: `services/rag-triage/tests/test_main.py`

- [ ] **Step 1: Write config tests**

Create `services/rag-triage/tests/test_config.py`:

```python
from app.config import Settings


def test_default_settings():
    settings = Settings()

    assert settings.eval_api_url == "http://eval:8000"
    assert settings.request_timeout_seconds == 30.0
    assert settings.default_metric == "context_precision"
    assert settings.default_limit == 5
    assert settings.max_limit == 20


def test_metric_validation_rejects_unknown_default():
    try:
        Settings(default_metric="latency")
    except ValueError as exc:
        assert "default_metric must be one of" in str(exc)
    else:
        raise AssertionError("expected invalid metric to raise")
```

- [ ] **Step 2: Write eval client tests**

Create `services/rag-triage/tests/test_eval_client.py`:

```python
import pytest
import respx
from httpx import Response

from app.eval_client import EvalAPIError, EvalClient


@pytest.mark.asyncio
@respx.mock
async def test_get_evaluation_sends_bearer_token():
    route = respx.get("http://eval:8000/evaluations/eval-1").mock(
        return_value=Response(
            200,
            json={
                "id": "eval-1",
                "dataset_id": "dataset-1",
                "status": "completed",
                "collection": "documents",
                "aggregate_scores": {"context_precision": 0.4},
                "results": [],
                "config": {"effective_retrieval_config": {"top_k": 5}},
            },
        )
    )

    client = EvalClient(base_url="http://eval:8000", token="token-1")
    try:
        result = await client.get_evaluation("eval-1")
    finally:
        await client.close()

    assert result.id == "eval-1"
    assert route.calls[0].request.headers["authorization"] == "Bearer token-1"


@pytest.mark.asyncio
@respx.mock
async def test_get_evaluation_raises_for_non_200():
    respx.get("http://eval:8000/evaluations/missing").mock(
        return_value=Response(404, json={"detail": "Evaluation not found"})
    )

    client = EvalClient(base_url="http://eval:8000", token="")
    try:
        with pytest.raises(EvalAPIError) as exc:
            await client.get_evaluation("missing")
    finally:
        await client.close()

    assert exc.value.status_code == 404
    assert "Evaluation not found" in str(exc.value)
```

- [ ] **Step 3: Write health endpoint tests**

Create `services/rag-triage/tests/conftest.py`:

```python
import pytest

from app.main import app


@pytest.fixture
def anyio_backend():
    return "asyncio"


@pytest.fixture
def test_app():
    return app
```

Create `services/rag-triage/tests/test_main.py`:

```python
from fastapi.testclient import TestClient

from app.main import app


def test_health_returns_healthy():
    client = TestClient(app)

    response = client.get("/health")

    assert response.status_code == 200
    assert response.json()["status"] == "healthy"
```

- [ ] **Step 4: Run tests to verify they fail**

Run:

```bash
cd services/rag-triage
python -m pytest tests/test_config.py tests/test_eval_client.py tests/test_main.py -v
```

Expected: FAIL because `app.config`, `app.eval_client`, and `app.main` do not exist yet.

- [ ] **Step 5: Add requirements**

Create `services/rag-triage/requirements.txt`:

```text
fastapi==0.135.3
uvicorn[standard]==0.44.0
httpx==0.27.0
pydantic-settings==2.3.0
pytest==9.0.3
pytest-asyncio==1.3.0
pytest-cov==7.1.0
respx==0.21.1
prometheus-fastapi-instrumentator==7.0.2
```

- [ ] **Step 6: Add package marker**

Create `services/rag-triage/app/__init__.py`:

```python
"""RAG regression triage service."""
```

Create `services/rag-triage/tests/__init__.py`:

```python
"""Tests for the RAG triage service."""
```

- [ ] **Step 7: Add config implementation**

Create `services/rag-triage/app/config.py`:

```python
from pydantic import field_validator
from pydantic_settings import BaseSettings


METRIC_NAMES = {
    "faithfulness",
    "answer_relevancy",
    "context_precision",
    "context_recall",
}


class Settings(BaseSettings):
    eval_api_url: str = "http://eval:8000"
    eval_api_token: str = ""
    request_timeout_seconds: float = 30.0
    default_metric: str = "context_precision"
    default_limit: int = 5
    max_limit: int = 20
    allowed_origins: str = "https://kylebradshaw.dev"

    @field_validator("default_metric")
    @classmethod
    def validate_default_metric(cls, value: str) -> str:
        if value not in METRIC_NAMES:
            allowed = ", ".join(sorted(METRIC_NAMES))
            raise ValueError(f"default_metric must be one of: {allowed}")
        return value

    @field_validator("request_timeout_seconds")
    @classmethod
    def validate_timeout(cls, value: float) -> float:
        if value <= 0:
            raise ValueError("request_timeout_seconds must be positive")
        return value

    @field_validator("default_limit", "max_limit")
    @classmethod
    def validate_limit(cls, value: int) -> int:
        if value <= 0:
            raise ValueError("limits must be positive")
        return value


settings = Settings()
```

- [ ] **Step 8: Add DTO models**

Create `services/rag-triage/app/models.py`:

```python
from __future__ import annotations

from typing import Any, Literal

from pydantic import BaseModel, Field


MetricName = Literal[
    "faithfulness",
    "answer_relevancy",
    "context_precision",
    "context_recall",
]

FailureMode = Literal[
    "retrieval_recall",
    "retrieval_precision",
    "generation_faithfulness",
    "answer_relevance",
    "runtime_or_config",
    "insufficient_evidence",
]

Confidence = Literal["low", "medium", "high"]


class Scores(BaseModel):
    faithfulness: float | None = None
    answer_relevancy: float | None = None
    context_precision: float | None = None
    context_recall: float | None = None


class QueryResult(BaseModel):
    query: str
    answer: str
    contexts: list[str] = Field(default_factory=list)
    scores: Scores
    score_reasons: dict[str, str] = Field(default_factory=dict)
    retrieval: dict[str, Any] = Field(default_factory=dict)


class EvaluationDetail(BaseModel):
    id: str
    dataset_id: str
    status: str
    collection: str | None = None
    aggregate_scores: Scores | None = None
    results: list[QueryResult] = Field(default_factory=list)
    error: str | None = None
    created_at: str | None = None
    completed_at: str | None = None
    notes: str | None = None
    config: dict[str, Any] = Field(default_factory=dict)
    baseline_eval_id: str | None = None


class TriageEvalRunRequest(BaseModel):
    eval_id: str
    metric: MetricName | None = None
    limit: int | None = None
    include_observability: bool = False


class TriageComparisonRequest(BaseModel):
    baseline_eval_id: str
    candidate_eval_id: str
    metric: MetricName | None = None
    limit: int | None = None
    include_observability: bool = False


class Diagnosis(BaseModel):
    primary_failure_mode: FailureMode
    confidence: Confidence
    summary: str


class CaseDiagnosis(BaseModel):
    query: str
    answer: str
    scores: Scores
    score_reasons: dict[str, str] = Field(default_factory=dict)
    failure_mode: FailureMode
    confidence: Confidence
    rationale: str
    evidence: dict[str, Any] = Field(default_factory=dict)


class Cluster(BaseModel):
    failure_mode: FailureMode
    count: int
    confidence: Confidence
    summary: str
    queries: list[str]


class Recommendation(BaseModel):
    action: str
    reason: str
    expected_impact: str
    evidence: dict[str, Any] = Field(default_factory=dict)


class TriageSubject(BaseModel):
    type: Literal["eval_run", "comparison"]
    eval_id: str | None = None
    baseline_eval_id: str | None = None
    candidate_eval_id: str | None = None


class TriageResponse(BaseModel):
    subject: TriageSubject
    status: str
    aggregate_scores: Scores | None = None
    config: dict[str, Any] = Field(default_factory=dict)
    diagnosis: Diagnosis
    clusters: list[Cluster]
    cases: list[CaseDiagnosis]
    recommendations: list[Recommendation]
    metric: MetricName
```

- [ ] **Step 9: Add eval client**

Create `services/rag-triage/app/eval_client.py`:

```python
from __future__ import annotations

import httpx

from app.models import EvaluationDetail


class EvalAPIError(Exception):
    def __init__(self, status_code: int, message: str):
        super().__init__(message)
        self.status_code = status_code


class EvalClient:
    def __init__(
        self,
        base_url: str,
        token: str,
        timeout_seconds: float = 30.0,
        transport: httpx.AsyncBaseTransport | None = None,
    ):
        kwargs: dict = {"base_url": base_url.rstrip("/"), "timeout": timeout_seconds}
        if transport is not None:
            kwargs["transport"] = transport
        self._client = httpx.AsyncClient(**kwargs)
        self._token = token

    def _headers(self) -> dict[str, str]:
        if not self._token:
            return {}
        return {"Authorization": f"Bearer {self._token}"}

    async def get_evaluation(self, eval_id: str) -> EvaluationDetail:
        response = await self._client.get(
            f"/evaluations/{eval_id}",
            headers=self._headers(),
        )
        if response.status_code >= 400:
            raise EvalAPIError(response.status_code, self._error_message(response))
        return EvaluationDetail.model_validate(response.json())

    @staticmethod
    def _error_message(response: httpx.Response) -> str:
        try:
            payload = response.json()
        except ValueError:
            return response.text[:256]
        detail = payload.get("detail")
        if isinstance(detail, str):
            return detail
        return str(payload)[:256]

    async def close(self) -> None:
        await self._client.aclose()
```

- [ ] **Step 10: Add metrics and app shell**

Create `services/rag-triage/app/metrics.py`:

```python
from prometheus_client import Counter
from prometheus_fastapi_instrumentator import Instrumentator


instrumentator = Instrumentator(excluded_handlers=["/health", "/metrics"])

triage_requests_total = Counter(
    "rag_triage_requests_total",
    "RAG triage requests by endpoint and outcome",
    ["endpoint", "outcome"],
)
```

Create `services/rag-triage/app/main.py`:

```python
from fastapi import FastAPI
from fastapi.middleware.cors import CORSMiddleware

from app.config import settings
from app.metrics import instrumentator


app = FastAPI(title="RAG Triage API")

app.add_middleware(
    CORSMiddleware,
    allow_origins=settings.allowed_origins.split(","),
    allow_credentials=True,
    allow_methods=["GET", "POST"],
    allow_headers=["Authorization", "Content-Type"],
)

instrumentator.instrument(app).expose(app, include_in_schema=False)


@app.get("/health")
async def health():
    return {"status": "healthy"}
```

- [ ] **Step 11: Run scaffold tests**

Run:

```bash
cd services/rag-triage
python -m pytest tests/test_config.py tests/test_eval_client.py tests/test_main.py -v
```

Expected: PASS.

- [ ] **Step 12: Commit scaffold**

Run:

```bash
git add services/rag-triage
git commit -m "feat: scaffold rag triage service"
```

Expected: commit succeeds.

---

### Task 3: Implement Deterministic Single-Run Triage

**Files:**
- Create: `services/rag-triage/app/rules.py`
- Create: `services/rag-triage/app/service.py`
- Create: `services/rag-triage/tests/test_rules.py`
- Create: `services/rag-triage/tests/test_service.py`
- Modify: `services/rag-triage/app/main.py`
- Modify: `services/rag-triage/tests/test_main.py`

- [ ] **Step 1: Write rule tests**

Create `services/rag-triage/tests/test_rules.py`:

```python
from app.models import QueryResult, Scores
from app.rules import classify_case, cluster_cases, recommendations_for_clusters


def result(scores: Scores) -> QueryResult:
    return QueryResult(query="What is supported?", answer="Answer", scores=scores)


def test_low_context_recall_classifies_retrieval_recall():
    diagnosis = classify_case(
        result(Scores(context_recall=0.2, context_precision=0.8, faithfulness=0.4))
    )

    assert diagnosis.failure_mode == "retrieval_recall"
    assert diagnosis.confidence == "high"
    assert "recall" in diagnosis.rationale


def test_low_precision_with_recall_classifies_retrieval_precision():
    diagnosis = classify_case(
        result(Scores(context_recall=0.8, context_precision=0.25, faithfulness=0.7))
    )

    assert diagnosis.failure_mode == "retrieval_precision"
    assert diagnosis.confidence == "high"


def test_low_faithfulness_with_context_classifies_generation():
    diagnosis = classify_case(
        result(Scores(context_recall=0.8, context_precision=0.8, faithfulness=0.2))
    )

    assert diagnosis.failure_mode == "generation_faithfulness"


def test_low_relevancy_classifies_answer_relevance():
    diagnosis = classify_case(
        result(Scores(answer_relevancy=0.2, context_recall=0.9, context_precision=0.9))
    )

    assert diagnosis.failure_mode == "answer_relevance"


def test_missing_scores_classifies_insufficient_evidence():
    diagnosis = classify_case(result(Scores()))

    assert diagnosis.failure_mode == "insufficient_evidence"
    assert diagnosis.confidence == "low"


def test_clusters_and_recommendations_are_structured():
    cases = [
        classify_case(result(Scores(context_recall=0.1))),
        classify_case(result(Scores(context_recall=0.2))),
    ]

    clusters = cluster_cases(cases)
    recommendations = recommendations_for_clusters(clusters)

    assert clusters[0].failure_mode == "retrieval_recall"
    assert clusters[0].count == 2
    assert recommendations[0].action == "increase_top_k"
```

- [ ] **Step 2: Write service tests**

Create `services/rag-triage/tests/test_service.py`:

```python
import pytest

from app.models import EvaluationDetail, QueryResult, Scores
from app.service import RAGTriageService


class FakeEvalClient:
    def __init__(self, evaluation: EvaluationDetail):
        self.evaluation = evaluation

    async def get_evaluation(self, eval_id: str) -> EvaluationDetail:
        assert eval_id == self.evaluation.id
        return self.evaluation


@pytest.mark.asyncio
async def test_triage_eval_run_returns_worst_cases_first():
    evaluation = EvaluationDetail(
        id="eval-1",
        dataset_id="dataset-1",
        status="completed",
        aggregate_scores=Scores(context_precision=0.4),
        results=[
            QueryResult(query="good", answer="a", scores=Scores(context_precision=0.9)),
            QueryResult(query="bad", answer="a", scores=Scores(context_precision=0.1, context_recall=0.9)),
        ],
        config={"effective_retrieval_config": {"top_k": 5}},
    )
    service = RAGTriageService(eval_client=FakeEvalClient(evaluation), default_metric="context_precision", default_limit=5, max_limit=20)

    response = await service.triage_eval_run("eval-1", metric=None, limit=None)

    assert response.subject.eval_id == "eval-1"
    assert response.metric == "context_precision"
    assert response.cases[0].query == "bad"
    assert response.diagnosis.primary_failure_mode == "retrieval_precision"


@pytest.mark.asyncio
async def test_failed_eval_run_is_runtime_or_config():
    evaluation = EvaluationDetail(
        id="eval-1",
        dataset_id="dataset-1",
        status="failed",
        error="evaluation timed out",
        results=[],
    )
    service = RAGTriageService(eval_client=FakeEvalClient(evaluation), default_metric="context_precision", default_limit=5, max_limit=20)

    response = await service.triage_eval_run("eval-1", metric="context_precision", limit=5)

    assert response.diagnosis.primary_failure_mode == "runtime_or_config"
    assert response.recommendations[0].action == "inspect_runtime_evidence"
```

- [ ] **Step 3: Run tests to verify they fail**

Run:

```bash
cd services/rag-triage
python -m pytest tests/test_rules.py tests/test_service.py -v
```

Expected: FAIL because `app.rules` and `app.service` do not exist yet.

- [ ] **Step 4: Add rules implementation**

Create `services/rag-triage/app/rules.py`:

```python
from __future__ import annotations

from collections import Counter, defaultdict

from app.models import CaseDiagnosis, Cluster, QueryResult, Recommendation


LOW = 0.5
ACCEPTABLE = 0.7


def classify_case(result: QueryResult) -> CaseDiagnosis:
    scores = result.scores
    evidence = {
        "context_recall": scores.context_recall,
        "context_precision": scores.context_precision,
        "faithfulness": scores.faithfulness,
        "answer_relevancy": scores.answer_relevancy,
    }

    if scores.context_recall is not None and scores.context_recall < LOW:
        return _case(result, "retrieval_recall", "high", "Low context recall indicates the expected answer is not covered by retrieved context.", evidence)

    if (
        scores.context_precision is not None
        and scores.context_precision < LOW
        and scores.context_recall is not None
        and scores.context_recall >= ACCEPTABLE
    ):
        return _case(result, "retrieval_precision", "high", "Low context precision with acceptable recall indicates noisy retrieval or reranking.", evidence)

    if (
        scores.faithfulness is not None
        and scores.faithfulness < LOW
        and _has_usable_context(scores.context_recall, scores.context_precision)
    ):
        return _case(result, "generation_faithfulness", "medium", "Retrieved context is usable, but the generated answer is weakly supported.", evidence)

    if scores.answer_relevancy is not None and scores.answer_relevancy < LOW:
        return _case(result, "answer_relevance", "medium", "Answer relevancy is low, suggesting the response does not directly target the question.", evidence)

    return _case(result, "insufficient_evidence", "low", "Scores do not point to one clear failure mode.", evidence)


def cluster_cases(cases: list[CaseDiagnosis]) -> list[Cluster]:
    counts = Counter(case.failure_mode for case in cases)
    queries_by_mode: dict[str, list[str]] = defaultdict(list)
    confidence_by_mode: dict[str, str] = {}
    for case in cases:
        queries_by_mode[case.failure_mode].append(case.query)
        confidence_by_mode.setdefault(case.failure_mode, case.confidence)

    clusters = [
        Cluster(
            failure_mode=mode,
            count=count,
            confidence=confidence_by_mode.get(mode, "low"),
            summary=_cluster_summary(mode, count),
            queries=queries_by_mode[mode],
        )
        for mode, count in counts.most_common()
    ]
    return clusters


def recommendations_for_clusters(clusters: list[Cluster]) -> list[Recommendation]:
    recommendations: list[Recommendation] = []
    for cluster in clusters:
        if cluster.failure_mode == "retrieval_recall":
            recommendations.append(Recommendation(action="increase_top_k", reason="Worst cases lack expected answer coverage in retrieved contexts.", expected_impact="Improves context recall for answerable questions.", evidence={"failure_mode": cluster.failure_mode, "case_count": cluster.count}))
        elif cluster.failure_mode == "retrieval_precision":
            recommendations.append(Recommendation(action="enable_or_tune_rerank", reason="Worst cases retrieve relevant material plus too much noise.", expected_impact="Improves context precision without requiring corpus changes.", evidence={"failure_mode": cluster.failure_mode, "case_count": cluster.count}))
        elif cluster.failure_mode == "generation_faithfulness":
            recommendations.append(Recommendation(action="prompt_grounding_change", reason="Retrieved contexts look usable but answers are not sufficiently supported.", expected_impact="Reduces unsupported claims in generated answers.", evidence={"failure_mode": cluster.failure_mode, "case_count": cluster.count}))
        elif cluster.failure_mode == "answer_relevance":
            recommendations.append(Recommendation(action="review_expected_answer", reason="Answer relevancy is weak; expected answer or prompt targeting may be misaligned.", expected_impact="Clarifies whether the failure is model behavior or dataset expectation.", evidence={"failure_mode": cluster.failure_mode, "case_count": cluster.count}))
        elif cluster.failure_mode == "runtime_or_config":
            recommendations.append(Recommendation(action="inspect_runtime_evidence", reason="Run status or configuration prevents quality-only diagnosis.", expected_impact="Separates infrastructure failures from RAG quality regressions.", evidence={"failure_mode": cluster.failure_mode, "case_count": cluster.count}))
    return recommendations


def _has_usable_context(recall: float | None, precision: float | None) -> bool:
    return (recall is not None and recall >= ACCEPTABLE) or (precision is not None and precision >= ACCEPTABLE)


def _case(result: QueryResult, mode, confidence, rationale: str, evidence: dict) -> CaseDiagnosis:
    return CaseDiagnosis(query=result.query, answer=result.answer, scores=result.scores, score_reasons=result.score_reasons, failure_mode=mode, confidence=confidence, rationale=rationale, evidence=evidence)


def _cluster_summary(mode: str, count: int) -> str:
    labels = {
        "retrieval_recall": "Retrieved contexts miss expected answer coverage.",
        "retrieval_precision": "Retrieved contexts are noisy for the question.",
        "generation_faithfulness": "Generated answers are not sufficiently grounded.",
        "answer_relevance": "Generated answers do not directly address the expected answer.",
        "runtime_or_config": "Runtime or configuration evidence blocks quality-only diagnosis.",
        "insufficient_evidence": "Scores do not identify a clear failure mode.",
    }
    return f"{labels[mode]} Cases: {count}."
```

- [ ] **Step 5: Add service implementation**

Create `services/rag-triage/app/service.py`:

```python
from __future__ import annotations

from app.models import Cluster, Diagnosis, EvaluationDetail, MetricName, TriageResponse, TriageSubject
from app.rules import classify_case, cluster_cases, recommendations_for_clusters


class RAGTriageService:
    def __init__(self, eval_client, default_metric: str, default_limit: int, max_limit: int):
        self._eval_client = eval_client
        self._default_metric = default_metric
        self._default_limit = default_limit
        self._max_limit = max_limit

    async def triage_eval_run(self, eval_id: str, metric: MetricName | None, limit: int | None) -> TriageResponse:
        selected_metric = metric or self._default_metric
        selected_limit = self._bounded_limit(limit)
        evaluation = await self._eval_client.get_evaluation(eval_id)

        if evaluation.status != "completed" or not evaluation.results:
            return self._runtime_response(evaluation, selected_metric)

        worst = sorted(
            evaluation.results,
            key=lambda result: _score_for_metric(result.scores, selected_metric),
        )[:selected_limit]
        cases = [classify_case(result) for result in worst]
        clusters = cluster_cases(cases)
        recommendations = recommendations_for_clusters(clusters)
        primary = clusters[0].failure_mode if clusters else "insufficient_evidence"
        confidence = clusters[0].confidence if clusters else "low"

        return TriageResponse(
            subject=TriageSubject(type="eval_run", eval_id=evaluation.id),
            status=evaluation.status,
            aggregate_scores=evaluation.aggregate_scores,
            config=evaluation.config,
            diagnosis=Diagnosis(
                primary_failure_mode=primary,
                confidence=confidence,
                summary=_summary(primary),
            ),
            clusters=clusters,
            cases=cases,
            recommendations=recommendations,
            metric=selected_metric,
        )

    def _bounded_limit(self, limit: int | None) -> int:
        if limit is None:
            return self._default_limit
        return min(max(limit, 1), self._max_limit)

    def _runtime_response(self, evaluation: EvaluationDetail, metric: str) -> TriageResponse:
        runtime_cluster = Cluster(
            failure_mode="runtime_or_config",
            count=1,
            confidence="high",
            summary="Runtime or configuration evidence blocks quality-only diagnosis.",
            queries=[],
        )
        diagnosis = Diagnosis(
            primary_failure_mode="runtime_or_config",
            confidence="high",
            summary="The evaluation did not produce completed results, so triage should inspect runtime or configuration evidence.",
        )
        return TriageResponse(
            subject=TriageSubject(type="eval_run", eval_id=evaluation.id),
            status=evaluation.status,
            aggregate_scores=evaluation.aggregate_scores,
            config=evaluation.config,
            diagnosis=diagnosis,
            clusters=[runtime_cluster],
            cases=[],
            recommendations=recommendations_for_clusters([runtime_cluster]),
            metric=metric,
        )


def _score_for_metric(scores, metric: str) -> float:
    value = getattr(scores, metric)
    return 1.0 if value is None else value


def _summary(mode: str) -> str:
    summaries = {
        "retrieval_recall": "Worst cases suggest retrieved contexts miss required answer coverage.",
        "retrieval_precision": "Worst cases suggest retrieved contexts are noisy relative to the questions.",
        "generation_faithfulness": "Worst cases suggest generation is not sufficiently grounded in retrieved contexts.",
        "answer_relevance": "Worst cases suggest answers are not targeting the expected response.",
        "runtime_or_config": "Runtime or configuration evidence blocks quality-only diagnosis.",
        "insufficient_evidence": "Available scores do not identify one dominant regression cause.",
    }
    return summaries[mode]
```

- [ ] **Step 6: Wire endpoint**

Modify `services/rag-triage/app/main.py` to include:

```python
from fastapi import FastAPI, HTTPException
from fastapi.middleware.cors import CORSMiddleware

from app.config import settings
from app.eval_client import EvalAPIError, EvalClient
from app.metrics import instrumentator, triage_requests_total
from app.models import TriageEvalRunRequest
from app.service import RAGTriageService
```

Add below `/health`:

```python
def build_service() -> RAGTriageService:
    client = EvalClient(
        base_url=settings.eval_api_url,
        token=settings.eval_api_token,
        timeout_seconds=settings.request_timeout_seconds,
    )
    return RAGTriageService(
        eval_client=client,
        default_metric=settings.default_metric,
        default_limit=settings.default_limit,
        max_limit=settings.max_limit,
    )


@app.post("/triage/eval-run")
async def triage_eval_run(body: TriageEvalRunRequest):
    service = build_service()
    try:
        result = await service.triage_eval_run(
            eval_id=body.eval_id,
            metric=body.metric,
            limit=body.limit,
        )
    except EvalAPIError as exc:
        triage_requests_total.labels(endpoint="eval-run", outcome="eval_api_error").inc()
        raise HTTPException(status_code=exc.status_code, detail=str(exc)) from exc
    finally:
        await service._eval_client.close()
    triage_requests_total.labels(endpoint="eval-run", outcome="success").inc()
    return result
```

- [ ] **Step 7: Extend main endpoint test**

Append to `services/rag-triage/tests/test_main.py`:

```python
from app.models import Diagnosis, Scores, TriageResponse, TriageSubject


def test_triage_eval_run_endpoint(monkeypatch):
    class FakeService:
        async def triage_eval_run(self, eval_id, metric, limit):
            assert eval_id == "eval-1"
            assert metric is None
            assert limit is None
            return TriageResponse(
                subject=TriageSubject(type="eval_run", eval_id="eval-1"),
                status="completed",
                aggregate_scores=Scores(context_precision=0.4),
                config={},
                diagnosis=Diagnosis(
                    primary_failure_mode="retrieval_precision",
                    confidence="medium",
                    summary="summary",
                ),
                clusters=[],
                cases=[],
                recommendations=[],
                metric="context_precision",
            )

    class FakeClient:
        async def close(self):
            return None

    service = FakeService()
    service._eval_client = FakeClient()
    monkeypatch.setattr("app.main.build_service", lambda: service)
    client = TestClient(app)

    response = client.post("/triage/eval-run", json={"eval_id": "eval-1"})

    assert response.status_code == 200
    assert response.json()["diagnosis"]["primary_failure_mode"] == "retrieval_precision"
```

- [ ] **Step 8: Run single-run tests**

Run:

```bash
cd services/rag-triage
python -m pytest tests/test_rules.py tests/test_service.py tests/test_main.py -v
```

Expected: PASS.

- [ ] **Step 9: Commit single-run triage**

Run:

```bash
git add services/rag-triage
git commit -m "feat: add deterministic rag run triage"
```

Expected: commit succeeds.

---

### Task 4: Add Comparison Triage

**Files:**
- Modify: `services/rag-triage/app/models.py`
- Modify: `services/rag-triage/app/service.py`
- Modify: `services/rag-triage/app/main.py`
- Modify: `services/rag-triage/tests/test_service.py`
- Modify: `services/rag-triage/tests/test_main.py`

- [ ] **Step 1: Add comparison service test**

Append to `services/rag-triage/tests/test_service.py`:

```python
class FakeComparisonEvalClient:
    def __init__(self, evaluations):
        self.evaluations = evaluations

    async def get_evaluation(self, eval_id: str) -> EvaluationDetail:
        return self.evaluations[eval_id]


@pytest.mark.asyncio
async def test_triage_comparison_uses_candidate_worst_cases_and_delta():
    baseline = EvaluationDetail(
        id="base",
        dataset_id="dataset-1",
        status="completed",
        aggregate_scores=Scores(context_precision=0.8),
        results=[QueryResult(query="q1", answer="a", scores=Scores(context_precision=0.8, context_recall=0.8))],
    )
    candidate = EvaluationDetail(
        id="cand",
        dataset_id="dataset-1",
        status="completed",
        aggregate_scores=Scores(context_precision=0.3),
        results=[QueryResult(query="q1", answer="a", scores=Scores(context_precision=0.2, context_recall=0.8))],
    )
    service = RAGTriageService(
        eval_client=FakeComparisonEvalClient({"base": baseline, "cand": candidate}),
        default_metric="context_precision",
        default_limit=5,
        max_limit=20,
    )

    response = await service.triage_comparison("base", "cand", metric=None, limit=None)

    assert response.subject.type == "comparison"
    assert response.subject.baseline_eval_id == "base"
    assert response.subject.candidate_eval_id == "cand"
    assert response.diagnosis.primary_failure_mode == "retrieval_precision"
    assert response.config["metric_delta"] == -0.5
```

- [ ] **Step 2: Add comparison request model if missing**

Confirm `TriageComparisonRequest` already exists in `services/rag-triage/app/models.py` from Task 2. If it does not, add:

```python
class TriageComparisonRequest(BaseModel):
    baseline_eval_id: str
    candidate_eval_id: str
    metric: MetricName | None = None
    limit: int | None = None
    include_observability: bool = False
```

- [ ] **Step 3: Implement comparison method**

Add to `RAGTriageService` in `services/rag-triage/app/service.py`:

```python
    async def triage_comparison(
        self,
        baseline_eval_id: str,
        candidate_eval_id: str,
        metric: MetricName | None,
        limit: int | None,
    ) -> TriageResponse:
        selected_metric = metric or self._default_metric
        selected_limit = self._bounded_limit(limit)
        baseline = await self._eval_client.get_evaluation(baseline_eval_id)
        candidate = await self._eval_client.get_evaluation(candidate_eval_id)

        if candidate.status != "completed" or not candidate.results:
            response = self._runtime_response(candidate, selected_metric)
            response.subject = TriageSubject(
                type="comparison",
                baseline_eval_id=baseline.id,
                candidate_eval_id=candidate.id,
            )
            response.config = {
                **candidate.config,
                "baseline_status": baseline.status,
                "candidate_status": candidate.status,
                "metric_delta": _metric_delta(baseline, candidate, selected_metric),
            }
            return response

        worst = sorted(
            candidate.results,
            key=lambda result: _score_for_metric(result.scores, selected_metric),
        )[:selected_limit]
        cases = [classify_case(result) for result in worst]
        clusters = cluster_cases(cases)
        recommendations = recommendations_for_clusters(clusters)
        primary = clusters[0].failure_mode if clusters else "insufficient_evidence"
        confidence = clusters[0].confidence if clusters else "low"

        return TriageResponse(
            subject=TriageSubject(
                type="comparison",
                baseline_eval_id=baseline.id,
                candidate_eval_id=candidate.id,
            ),
            status=candidate.status,
            aggregate_scores=candidate.aggregate_scores,
            config={
                **candidate.config,
                "baseline_status": baseline.status,
                "candidate_status": candidate.status,
                "metric_delta": _metric_delta(baseline, candidate, selected_metric),
            },
            diagnosis=Diagnosis(
                primary_failure_mode=primary,
                confidence=confidence,
                summary=_summary(primary),
            ),
            clusters=clusters,
            cases=cases,
            recommendations=recommendations,
            metric=selected_metric,
        )
```

Add helper:

```python
def _metric_delta(baseline: EvaluationDetail, candidate: EvaluationDetail, metric: str) -> float | None:
    if baseline.aggregate_scores is None or candidate.aggregate_scores is None:
        return None
    baseline_value = getattr(baseline.aggregate_scores, metric)
    candidate_value = getattr(candidate.aggregate_scores, metric)
    if baseline_value is None or candidate_value is None:
        return None
    return round(candidate_value - baseline_value, 4)
```

- [ ] **Step 4: Wire comparison endpoint**

Modify imports in `services/rag-triage/app/main.py`:

```python
from app.models import TriageComparisonRequest, TriageEvalRunRequest
```

Add endpoint:

```python
@app.post("/triage/comparison")
async def triage_comparison(body: TriageComparisonRequest):
    service = build_service()
    try:
        result = await service.triage_comparison(
            baseline_eval_id=body.baseline_eval_id,
            candidate_eval_id=body.candidate_eval_id,
            metric=body.metric,
            limit=body.limit,
        )
    except EvalAPIError as exc:
        triage_requests_total.labels(endpoint="comparison", outcome="eval_api_error").inc()
        raise HTTPException(status_code=exc.status_code, detail=str(exc)) from exc
    finally:
        await service._eval_client.close()
    triage_requests_total.labels(endpoint="comparison", outcome="success").inc()
    return result
```

- [ ] **Step 5: Add comparison endpoint test**

Append to `services/rag-triage/tests/test_main.py`:

```python
def test_triage_comparison_endpoint(monkeypatch):
    class FakeService:
        async def triage_comparison(self, baseline_eval_id, candidate_eval_id, metric, limit):
            assert baseline_eval_id == "base"
            assert candidate_eval_id == "cand"
            return TriageResponse(
                subject=TriageSubject(type="comparison", baseline_eval_id="base", candidate_eval_id="cand"),
                status="completed",
                aggregate_scores=Scores(context_precision=0.3),
                config={"metric_delta": -0.5},
                diagnosis=Diagnosis(
                    primary_failure_mode="retrieval_precision",
                    confidence="high",
                    summary="summary",
                ),
                clusters=[],
                cases=[],
                recommendations=[],
                metric="context_precision",
            )

    class FakeClient:
        async def close(self):
            return None

    service = FakeService()
    service._eval_client = FakeClient()
    monkeypatch.setattr("app.main.build_service", lambda: service)
    client = TestClient(app)

    response = client.post(
        "/triage/comparison",
        json={"baseline_eval_id": "base", "candidate_eval_id": "cand"},
    )

    assert response.status_code == 200
    assert response.json()["config"]["metric_delta"] == -0.5
```

- [ ] **Step 6: Run comparison tests**

Run:

```bash
cd services/rag-triage
python -m pytest tests/test_service.py tests/test_main.py -v
```

Expected: PASS.

- [ ] **Step 7: Commit comparison triage**

Run:

```bash
git add services/rag-triage
git commit -m "feat: add rag comparison triage"
```

Expected: commit succeeds.

---

### Task 5: Add Docker, Compose, Gateway, And CI Wiring

**Files:**
- Create: `services/rag-triage/Dockerfile`
- Modify: `docker-compose.yml`
- Modify: `nginx/nginx.conf`
- Modify: `.github/workflows/ci.yml`

- [ ] **Step 1: Add Dockerfile**

Create `services/rag-triage/Dockerfile`:

```dockerfile
FROM python:3.12-slim

WORKDIR /app

COPY rag-triage/requirements.txt /app/requirements.txt
RUN pip install --no-cache-dir -r /app/requirements.txt

COPY shared /app/shared
COPY rag-triage/app /app/app

ENV PYTHONPATH=/app:/app/shared

EXPOSE 8000

CMD ["uvicorn", "app.main:app", "--host", "0.0.0.0", "--port", "8000"]
```

- [ ] **Step 2: Add compose service**

In `docker-compose.yml`, add `rag-triage` after `eval`:

```yaml
  rag-triage:
    image: ghcr.io/kabradshaw1/portfolio/rag-triage:latest
    build:
      context: ./services
      dockerfile: rag-triage/Dockerfile
    env_file: .env
    environment:
      - EVAL_API_URL=${RAG_TRIAGE_EVAL_API_URL:-http://eval:8000}
      - EVAL_API_TOKEN=${RAG_TRIAGE_EVAL_API_TOKEN:-}
    depends_on:
      eval:
        condition: service_started
```

In the `gateway` service `depends_on`, add:

```yaml
      - rag-triage
```

- [ ] **Step 3: Add gateway route if nginx uses explicit locations**

Inspect `nginx/nginx.conf`. If it has explicit service locations, add:

```nginx
        location /rag-triage/ {
            proxy_pass http://rag-triage:8000/;
            proxy_set_header Host $host;
            proxy_set_header X-Real-IP $remote_addr;
            proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
            proxy_set_header X-Forwarded-Proto $scheme;
        }
```

If the file uses a generated or wildcard route scheme, add the equivalent route following the existing local pattern.

- [ ] **Step 4: Add CI matrix entries**

Open `.github/workflows/ci.yml` and add `rag-triage` anywhere existing Python service matrices list `ingestion`, `chat`, `debug`, or `eval`.

For Python test matrix entries, use:

```yaml
rag-triage
```

For Dockerfile matrix entries, use:

```yaml
services/rag-triage/Dockerfile
```

For image build matrix entries, use:

```yaml
service: rag-triage
dockerfile: services/rag-triage/Dockerfile
context: services
```

Match the exact key names used by the existing workflow.

- [ ] **Step 5: Build and run local service tests**

Run:

```bash
docker compose build rag-triage
docker compose run --rm rag-triage python -m pytest tests -v
```

Expected: image builds; tests pass. If tests are not copied into the runtime image, run local tests instead with `PYTHONPATH=services/rag-triage:services/shared python -m pytest services/rag-triage/tests -v` and keep the Docker build as the image verification.

- [ ] **Step 6: Commit service wiring**

Run:

```bash
git add services/rag-triage/Dockerfile docker-compose.yml nginx/nginx.conf .github/workflows/ci.yml
git commit -m "build: wire rag triage service"
```

Expected: commit succeeds.

---

### Task 6: Add Eval MCP Triage API Client

**Files:**
- Create: `go/eval-mcp-service/internal/triageapi/client.go`
- Create: `go/eval-mcp-service/internal/triageapi/client_test.go`
- Modify: `go/eval-mcp-service/internal/config/config.go`

- [ ] **Step 1: Write triage client tests**

Create `go/eval-mcp-service/internal/triageapi/client_test.go`:

```go
package triageapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientTriageRAGRegressionSingleRun(t *testing.T) {
	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/triage/eval-run" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		gotAuth = r.Header.Get("Authorization")
		var req TriageRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		if req.EvalID != "eval-1" {
			t.Fatalf("eval_id = %q", req.EvalID)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "completed"})
	}))
	defer server.Close()

	client := New(server.URL, staticToken("token-1"), server.Client())
	result, err := client.TriageRAGRegression(context.Background(), TriageRequest{EvalID: "eval-1"})
	if err != nil {
		t.Fatal(err)
	}
	if gotAuth != "Bearer token-1" {
		t.Fatalf("Authorization = %q", gotAuth)
	}
	if string(result["status"].(string)) != "completed" {
		t.Fatalf("result = %#v", result)
	}
}

func TestClientTriageRAGRegressionComparison(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/triage/comparison" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "completed"})
	}))
	defer server.Close()

	client := New(server.URL, nil, server.Client())
	_, err := client.TriageRAGRegression(context.Background(), TriageRequest{
		EvalID:         "candidate",
		BaselineEvalID: "baseline",
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestClientTriageRAGRegressionReturnsHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"detail":"missing"}`, http.StatusNotFound)
	}))
	defer server.Close()

	client := New(server.URL, nil, server.Client())
	_, err := client.TriageRAGRegression(context.Background(), TriageRequest{EvalID: "missing"})
	if err == nil {
		t.Fatal("expected error")
	}
}

type staticToken string

func (s staticToken) Token(context.Context) (string, error) { return string(s), nil }
func (s staticToken) Invalidate()                          {}
```

- [ ] **Step 2: Run client tests to verify they fail**

Run:

```bash
cd go/eval-mcp-service
go test ./internal/triageapi -run TestClient -v
```

Expected: FAIL because `internal/triageapi` client code does not exist yet.

- [ ] **Step 3: Add triage client implementation**

Create `go/eval-mcp-service/internal/triageapi/client.go`:

```go
package triageapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const defaultHTTPTimeout = 30 * time.Second

type TokenProvider interface {
	Token(context.Context) (string, error)
	Invalidate()
}

type Client struct {
	baseURL       string
	tokenProvider TokenProvider
	httpClient    *http.Client
}

type TriageRequest struct {
	EvalID               string `json:"eval_id"`
	BaselineEvalID       string `json:"baseline_eval_id,omitempty"`
	Metric               string `json:"metric,omitempty"`
	Limit                int    `json:"limit,omitempty"`
	IncludeObservability bool   `json:"include_observability,omitempty"`
}

func New(baseURL string, tokenProvider TokenProvider, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: defaultHTTPTimeout}
	}
	return &Client{
		baseURL:       strings.TrimRight(baseURL, "/"),
		tokenProvider: tokenProvider,
		httpClient:    httpClient,
	}
}

func (c *Client) TriageRAGRegression(ctx context.Context, in TriageRequest) (map[string]any, error) {
	path := "/triage/eval-run"
	body := map[string]any{
		"eval_id": in.EvalID,
	}
	if in.BaselineEvalID != "" {
		path = "/triage/comparison"
		body = map[string]any{
			"baseline_eval_id": in.BaselineEvalID,
			"candidate_eval_id": in.EvalID,
		}
	}
	if in.Metric != "" {
		body["metric"] = in.Metric
	}
	if in.Limit > 0 {
		body["limit"] = in.Limit
	}
	if in.IncludeObservability {
		body["include_observability"] = true
	}

	var out map[string]any
	if err := c.do(ctx, http.MethodPost, path, body, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) do(ctx context.Context, method, path string, body any, out any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.tokenProvider != nil {
		token, err := c.tokenProvider.Token(ctx)
		if err != nil {
			return err
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("%s %s: status %d: %s", method, path, resp.StatusCode, string(data))
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(data, out)
}
```

- [ ] **Step 4: Add config setting**

Modify `go/eval-mcp-service/internal/config/config.go` to add a field:

```go
TriageAPIURL string
```

In the returned `Config`, add:

```go
TriageAPIURL: getenv("RAG_TRIAGE_API_URL", "http://localhost:8000/rag-triage"),
```

Use the config file's existing `getenv` helper and formatting.

- [ ] **Step 5: Run triage client tests**

Run:

```bash
cd go/eval-mcp-service
go test ./internal/triageapi -v
go test ./internal/config -v
```

Expected: PASS.

- [ ] **Step 6: Commit triage API client**

Run:

```bash
git add go/eval-mcp-service/internal/triageapi go/eval-mcp-service/internal/config/config.go
git commit -m "feat: add eval mcp triage client"
```

Expected: commit succeeds.

---

### Task 7: Add Eval MCP Tool Wrapper

**Files:**
- Modify: `go/eval-mcp-service/internal/evalworkflow/service.go`
- Modify: `go/eval-mcp-service/internal/evalworkflow/service_test.go`
- Modify: `go/eval-mcp-service/internal/mcpserver/server.go`
- Modify: `go/eval-mcp-service/internal/mcpserver/server_test.go`
- Modify: `go/eval-mcp-service/cmd/eval-mcp/main.go`
- Modify: `go/eval-mcp-service/README.md`

- [ ] **Step 1: Add workflow interface and method tests**

Append to `go/eval-mcp-service/internal/evalworkflow/service_test.go`:

```go
func TestTriageRAGRegressionCallsTriageAPI(t *testing.T) {
	triage := &fakeTriageAPI{result: map[string]any{"status": "completed"}}
	svc := New(nil, nil, nil, time.Second, time.Minute)
	svc.triage = triage

	got, err := svc.TriageRAGRegression(context.Background(), TriageInput{EvalID: "eval-1", Metric: "context_precision", Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	if triage.input.EvalID != "eval-1" {
		t.Fatalf("eval id = %q", triage.input.EvalID)
	}
	if got["status"] != "completed" {
		t.Fatalf("result = %#v", got)
	}
}

type fakeTriageAPI struct {
	input  TriageInput
	result map[string]any
}

func (f *fakeTriageAPI) TriageRAGRegression(ctx context.Context, in TriageInput) (map[string]any, error) {
	f.input = in
	return f.result, nil
}
```

- [ ] **Step 2: Update workflow service**

In `go/eval-mcp-service/internal/evalworkflow/service.go`, add:

```go
type TriageAPI interface {
	TriageRAGRegression(context.Context, TriageInput) (map[string]any, error)
}

type TriageInput struct {
	EvalID               string
	BaselineEvalID       string
	Metric               string
	Limit                int
	IncludeObservability bool
}
```

Add field to `Service`:

```go
triage TriageAPI
```

Add constructor variant:

```go
func (s *Service) WithTriageAPI(triage TriageAPI) *Service {
	s.triage = triage
	return s
}
```

Add method:

```go
func (s *Service) TriageRAGRegression(ctx context.Context, in TriageInput) (map[string]any, error) {
	if strings.TrimSpace(in.EvalID) == "" {
		return nil, errors.New("eval_id is required")
	}
	if s.triage == nil {
		return nil, errors.New("triage API client is not configured")
	}
	return s.triage.TriageRAGRegression(ctx, in)
}
```

- [ ] **Step 3: Resolve type mismatch between workflow and triageapi**

If `triageapi.TriageRequest` and `evalworkflow.TriageInput` are separate types, implement an adapter in `cmd/eval-mcp/main.go` or change `evalworkflow.TriageAPI` to accept the workflow input and wrap the triage client. Preferred adapter:

```go
type triageAdapter struct {
	client *triageapi.Client
}

func (a triageAdapter) TriageRAGRegression(ctx context.Context, in evalworkflow.TriageInput) (map[string]any, error) {
	return a.client.TriageRAGRegression(ctx, triageapi.TriageRequest{
		EvalID:               in.EvalID,
		BaselineEvalID:       in.BaselineEvalID,
		Metric:               in.Metric,
		Limit:                in.Limit,
		IncludeObservability: in.IncludeObservability,
	})
}
```

- [ ] **Step 4: Add MCP server tool tests**

In `go/eval-mcp-service/internal/mcpserver/server_test.go`, add assertions following existing tool registration tests:

```go
func TestNewRegistersTriageTool(t *testing.T) {
	srv := New(&fakeEvalService{})
	tools, err := srv.ListTools(context.Background(), &sdkmcp.ListToolsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range tools.Tools {
		if tool.Name == "triage_rag_regression" {
			return
		}
	}
	t.Fatal("triage_rag_regression tool not registered")
}
```

Add method to fake service used by server tests:

```go
func (f *fakeEvalService) TriageRAGRegression(context.Context, evalworkflow.TriageInput) (map[string]any, error) {
	return map[string]any{"status": "completed"}, nil
}
```

- [ ] **Step 5: Add MCP schema, interface method, and handler**

In `go/eval-mcp-service/internal/mcpserver/server.go`, add to `EvalService`:

```go
TriageRAGRegression(context.Context, evalworkflow.TriageInput) (map[string]any, error)
```

Register tool after `get_worst_eval_cases`:

```go
addTool(srv, "triage_rag_regression", "Diagnose likely RAG regression causes for one eval run or a baseline/candidate comparison.", triageRAGRegressionSchema(), triageRAGRegressionHandler(service))
```

Add schema:

```go
func triageRAGRegressionSchema() json.RawMessage {
	return json.RawMessage(`{
		"type":"object",
		"properties":{
			"eval_id":{"type":"string","description":"Candidate or single eval run ID to triage."},
			"baseline_eval_id":{"type":"string","description":"Optional baseline eval run ID for comparison triage."},
			"metric":{"type":"string","enum":["faithfulness","answer_relevancy","context_precision","context_recall"]},
			"limit":{"type":"integer","minimum":1,"maximum":20},
			"include_observability":{"type":"boolean"}
		},
		"required":["eval_id"]
	}`)
}
```

Add handler:

```go
func triageRAGRegressionHandler(service EvalService) sdkmcp.ToolHandler {
	return func(ctx context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		var args struct {
			EvalID               string `json:"eval_id"`
			BaselineEvalID       string `json:"baseline_eval_id,omitempty"`
			Metric               string `json:"metric,omitempty"`
			Limit                int    `json:"limit,omitempty"`
			IncludeObservability bool   `json:"include_observability,omitempty"`
		}
		if err := decodeArgs(req, &args); err != nil {
			return toolError(err.Error()), nil
		}
		if strings.TrimSpace(args.EvalID) == "" {
			return toolError("eval_id is required"), nil
		}
		if args.Limit < 0 || args.Limit > maxWorstLimit {
			return toolError("limit must be between 1 and 20 when provided"), nil
		}
		result, err := service.TriageRAGRegression(ctx, evalworkflow.TriageInput{
			EvalID:               args.EvalID,
			BaselineEvalID:       args.BaselineEvalID,
			Metric:               args.Metric,
			Limit:                args.Limit,
			IncludeObservability: args.IncludeObservability,
		})
		return resultOrError(result, err), nil
	}
}
```

- [ ] **Step 6: Wire main**

In `go/eval-mcp-service/cmd/eval-mcp/main.go`, import:

```go
"github.com/kabradshaw1/portfolio/go/eval-mcp-service/internal/triageapi"
```

Construct the client near the eval API client:

```go
triageClient := triageapi.New(cfg.TriageAPIURL, tokenProvider, nil)
```

When constructing workflow service, chain:

```go
service := evalworkflow.New(apiClient, ingestionClient, fixtures, cfg.PollInterval, cfg.WaitTimeout, cfg.MaxBackoff).WithTriageAPI(triageAdapter{client: triageClient})
```

Add the `triageAdapter` type from Step 3 in the same file.

- [ ] **Step 7: Update README**

In `go/eval-mcp-service/README.md`, add config:

```markdown
- `RAG_TRIAGE_API_URL`: RAG triage API base URL, defaults to
  `http://localhost:8000/rag-triage`.
```

Add tool to the tools list:

```markdown
- `triage_rag_regression`
```

Add usage note:

```markdown
Use `triage_rag_regression` after an eval run or comparison completes to
classify likely regression causes and get structured follow-up experiment
recommendations.
```

- [ ] **Step 8: Run Go tests**

Run:

```bash
cd go/eval-mcp-service
go test ./...
```

Expected: PASS.

- [ ] **Step 9: Commit MCP wrapper**

Run:

```bash
git add go/eval-mcp-service
git commit -m "feat: expose rag triage through eval mcp"
```

Expected: commit succeeds.

---

### Task 8: Final Verification And PR Prep

**Files:**
- No required edits unless verification exposes defects.

- [ ] **Step 1: Run Python preflight**

Run:

```bash
make preflight-python
```

Expected: PASS.

- [ ] **Step 2: Run Go preflight**

Run:

```bash
make preflight-go
```

Expected: PASS.

- [ ] **Step 3: Run security preflight**

Run:

```bash
make preflight-security
```

Expected: PASS.

- [ ] **Step 4: Check final diff**

Run:

```bash
git status --short
git diff --stat main...HEAD
```

Expected: only RAG triage service, compose/gateway/CI wiring, and eval MCP wrapper changes are present.

- [ ] **Step 5: Push branch and create PR to qa**

Run:

```bash
git push -u origin feature/rag-triage-service
gh pr create --base qa --head feature/rag-triage-service --title "Add RAG triage service" --body "Adds a Python RAG triage service and an eval MCP wrapper tool for deterministic eval regression diagnosis."
```

Expected: PR is created targeting `qa`. Do not watch CI unless explicitly requested.

---

## Self-Review

- Spec coverage: covered new Python service, HTTP eval API integration, deterministic rules, no direct DB, no gRPC, no v1 LangGraph, and additive eval MCP wrapper.
- Scope: one implementation plan is acceptable because the MCP wrapper is a thin adapter over the new service and is necessary for v1 usability.
- Red-flag scan: the plan contains concrete file paths, commands, and code blocks for each implementation step.
- Type consistency: Python request/response names match endpoint tests; Go `TriageInput` maps to `triageapi.TriageRequest` through an adapter.
