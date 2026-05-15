# Eval Dashboard Summary API Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `GET /evaluations/dashboard?dataset_id=...&collection=documents&recent_limit=10` to return compact RAG performance dashboard metadata from `services/eval`.

**Architecture:** Keep the feature inside the Python eval service. Add typed Pydantic response models, a dashboard-specific DB helper that returns completed runs without per-query results, and a FastAPI route that validates inputs, checks dataset existence, and computes trends, recent run summaries, and first-to-latest deltas server-side.

**Tech Stack:** Python, FastAPI, Pydantic, aiosqlite, pytest, pytest-asyncio, FastAPI TestClient.

---

## Worktree And Branch Setup

This is runtime API work. Before implementation, create/use a feature worktree from the repo root:

```bash
git branch --show-current
git worktree add .codex/worktrees/eval-dashboard-summary-api -b feat/eval-dashboard-summary-api
cd .codex/worktrees/eval-dashboard-summary-api
pwd
git branch --show-current
git rev-parse --show-toplevel
```

Expected:

- Branch is `feat/eval-dashboard-summary-api`.
- `pwd` and `git rev-parse --show-toplevel` point inside `.codex/worktrees/eval-dashboard-summary-api`.

All implementation commands below run from `.codex/worktrees/eval-dashboard-summary-api`.

## File Structure

- Modify `services/eval/app/models.py`: add dashboard response models beside existing eval response models.
- Modify `services/eval/app/db.py`: add a compact dashboard run helper that excludes `results` and `error`.
- Modify `services/eval/app/main.py`: import dashboard models, add helper functions for summary construction, and add `GET /evaluations/dashboard` before `/evaluations/{eval_id}`.
- Modify `services/eval/tests/test_models.py`: add model serialization coverage.
- Modify `services/eval/tests/test_db.py`: add DB helper coverage for ordering, completed-only filtering, and omitted per-query results.
- Modify `services/eval/tests/test_main.py`: add route coverage for validation, 404 behavior, empty summaries, capped recent runs, trend points, and deltas.

## Task 1: Dashboard Response Models

**Files:**
- Modify: `services/eval/app/models.py`
- Test: `services/eval/tests/test_models.py`

- [ ] **Step 1: Write the failing model test**

Add imports in `services/eval/tests/test_models.py`:

```python
from app.models import (
    AttachExperimentRunRequest,
    CreateExperimentRequest,
    DashboardBaselineDeltas,
    DashboardDatasetSummary,
    DashboardRunSummary,
    EvaluationDashboard,
    EvaluationDetail,
    ExperimentDetail,
    ExperimentRun,
    ExperimentRunEvaluation,
    MetricTrendPoint,
    QueryScore,
    RunComparison,
    RunHistory,
    StartEvaluationRequest,
    UpdateExperimentRequest,
)
```

Add this test after `test_run_history_shape`:

```python
def test_evaluation_dashboard_shape_excludes_detail_payloads():
    dashboard = EvaluationDashboard(
        dataset=DashboardDatasetSummary(
            id="ds-1",
            name="rag-golden",
            item_count=2,
        ),
        collection="documents",
        completed_run_count=2,
        first_completed_run=DashboardRunSummary(
            id="eval-1",
            created_at="2026-05-01T00:00:00+00:00",
            completed_at="2026-05-01T00:01:00+00:00",
            notes="baseline",
            config_captured=True,
            aggregate_scores=QueryScore(
                faithfulness=0.8,
                answer_relevancy=0.7,
                context_precision=0.6,
                context_recall=0.5,
            ),
            baseline_eval_id=None,
        ),
        latest_completed_run=DashboardRunSummary(
            id="eval-2",
            created_at="2026-05-02T00:00:00+00:00",
            completed_at="2026-05-02T00:01:00+00:00",
            notes="rerank on",
            config_captured=False,
            aggregate_scores=QueryScore(
                faithfulness=0.9,
                answer_relevancy=0.75,
                context_precision=0.65,
                context_recall=0.55,
            ),
            baseline_eval_id="eval-1",
        ),
        metric_trends={
            "faithfulness": [
                MetricTrendPoint(
                    evaluation_id="eval-1",
                    completed_at="2026-05-01T00:01:00+00:00",
                    score=0.8,
                )
            ],
            "answer_relevancy": [],
            "context_precision": [],
            "context_recall": [],
        },
        recent_runs=[],
        baseline_to_latest_deltas=DashboardBaselineDeltas(
            baseline_eval_id="eval-1",
            latest_eval_id="eval-2",
            deltas=QueryScore(
                faithfulness=0.1,
                answer_relevancy=0.05,
                context_precision=0.05,
                context_recall=0.05,
            ),
        ),
    )

    payload = dashboard.model_dump()

    assert payload["dataset"]["item_count"] == 2
    assert payload["first_completed_run"]["config_captured"] is True
    assert payload["baseline_to_latest_deltas"]["deltas"]["faithfulness"] == 0.1
    assert "results" not in payload["first_completed_run"]
    assert "error" not in payload["first_completed_run"]
    assert "config" not in payload["first_completed_run"]
```

- [ ] **Step 2: Run the model test to verify it fails**

Run:

```bash
PYTHONPATH=services/eval pytest services/eval/tests/test_models.py::test_evaluation_dashboard_shape_excludes_detail_payloads -v
```

Expected: FAIL with an import error for `DashboardDatasetSummary`, `DashboardRunSummary`, `MetricTrendPoint`, `DashboardBaselineDeltas`, or `EvaluationDashboard`.

- [ ] **Step 3: Add dashboard models**

In `services/eval/app/models.py`, add these models after `RunHistory`:

```python
class DashboardDatasetSummary(BaseModel):
    id: str
    name: str
    item_count: int


class DashboardRunSummary(BaseModel):
    id: str
    created_at: str
    completed_at: str | None
    notes: str | None = None
    config_captured: bool
    aggregate_scores: QueryScore | None
    baseline_eval_id: str | None = None


class MetricTrendPoint(BaseModel):
    evaluation_id: str
    completed_at: str | None
    score: float | None


class DashboardBaselineDeltas(BaseModel):
    baseline_eval_id: str
    latest_eval_id: str
    deltas: QueryScore


class EvaluationDashboard(BaseModel):
    dataset: DashboardDatasetSummary
    collection: str
    completed_run_count: int
    first_completed_run: DashboardRunSummary | None = None
    latest_completed_run: DashboardRunSummary | None = None
    metric_trends: dict[str, list[MetricTrendPoint]]
    recent_runs: list[DashboardRunSummary]
    baseline_to_latest_deltas: DashboardBaselineDeltas | None = None
```

- [ ] **Step 4: Run the model test to verify it passes**

Run:

```bash
PYTHONPATH=services/eval pytest services/eval/tests/test_models.py::test_evaluation_dashboard_shape_excludes_detail_payloads -v
```

Expected: PASS.

- [ ] **Step 5: Commit Task 1**

Run:

```bash
git add services/eval/app/models.py services/eval/tests/test_models.py
git commit -m "feat: add eval dashboard response models"
```

## Task 2: Dashboard DB Helper

**Files:**
- Modify: `services/eval/app/db.py`
- Test: `services/eval/tests/test_db.py`

- [ ] **Step 1: Write the failing DB test**

Add this test after `test_set_config_persists_json` in `services/eval/tests/test_db.py`:

```python
@pytest.mark.asyncio
async def test_get_dashboard_completed_runs_filters_orders_and_omits_results(db):
    ds_id = await db.create_dataset(name="ds-dashboard", items=SIMPLE_ITEM)
    other_ds_id = await db.create_dataset(name="ds-other-dashboard", items=SIMPLE_ITEM)

    running_id = await db.create_evaluation(dataset_id=ds_id, collection="documents")
    baseline_id = await db.create_evaluation(
        dataset_id=ds_id,
        collection="documents",
        notes="baseline",
    )
    await db.set_evaluation_config(baseline_id, {"chat": {"llm_model": "qwen"}})
    latest_id = await db.create_evaluation(
        dataset_id=ds_id,
        collection="documents",
        notes="rerank on",
        baseline_eval_id=baseline_id,
    )
    other_collection_id = await db.create_evaluation(
        dataset_id=ds_id,
        collection="archive",
    )
    other_dataset_id = await db.create_evaluation(
        dataset_id=other_ds_id,
        collection="documents",
    )
    failed_id = await db.create_evaluation(dataset_id=ds_id, collection="documents")

    await db.complete_evaluation(
        baseline_id,
        aggregate_scores={"faithfulness": 0.8},
        results=[{"query": "q1", "answer": "a1", "contexts": [], "scores": {}}],
    )
    await db.complete_evaluation(
        latest_id,
        aggregate_scores={"faithfulness": 0.9},
        results=[{"query": "q2", "answer": "a2", "contexts": [], "scores": {}}],
    )
    await db.complete_evaluation(
        other_collection_id,
        aggregate_scores={"faithfulness": 0.1},
        results=[],
    )
    await db.complete_evaluation(
        other_dataset_id,
        aggregate_scores={"faithfulness": 0.2},
        results=[],
    )
    await db.fail_evaluation(failed_id, error="judge failed")

    runs = await db.get_completed_evaluations_for_dashboard(
        dataset_id=ds_id,
        collection="documents",
    )

    assert [run["id"] for run in runs] == [baseline_id, latest_id]
    assert running_id not in [run["id"] for run in runs]
    assert runs[0]["notes"] == "baseline"
    assert runs[0]["config"] == {"chat": {"llm_model": "qwen"}}
    assert runs[1]["baseline_eval_id"] == baseline_id
    assert "results" not in runs[0]
    assert "error" not in runs[0]
```

- [ ] **Step 2: Run the DB test to verify it fails**

Run:

```bash
PYTHONPATH=services/eval pytest services/eval/tests/test_db.py::test_get_dashboard_completed_runs_filters_orders_and_omits_results -v
```

Expected: FAIL with `AttributeError: 'EvalDB' object has no attribute 'get_completed_evaluations_for_dashboard'`.

- [ ] **Step 3: Add the DB helper**

In `services/eval/app/db.py`, add this method directly after `get_history`:

```python
    async def get_completed_evaluations_for_dashboard(
        self, dataset_id: str, collection: str
    ) -> list[dict]:
        """Completed compact runs for dashboard summaries, ordered ASC."""
        cursor = await self._db.execute(
            "SELECT * FROM evaluations "
            "WHERE dataset_id = ? AND collection = ? AND status = 'completed' "
            "ORDER BY created_at ASC",
            (dataset_id, collection),
        )
        rows = await cursor.fetchall()
        return [self._row_to_dict(r, include_results=False) for r in rows]
```

- [ ] **Step 4: Run the DB test to verify it passes**

Run:

```bash
PYTHONPATH=services/eval pytest services/eval/tests/test_db.py::test_get_dashboard_completed_runs_filters_orders_and_omits_results -v
```

Expected: PASS.

- [ ] **Step 5: Commit Task 2**

Run:

```bash
git add services/eval/app/db.py services/eval/tests/test_db.py
git commit -m "feat: add eval dashboard db helper"
```

## Task 3: Dashboard Route And Summary Builder

**Files:**
- Modify: `services/eval/app/main.py`
- Test: `services/eval/tests/test_main.py`

- [ ] **Step 1: Add route tests**

In `services/eval/tests/test_main.py`, add these tests after `test_history_empty_returns_200` and before the experiment endpoint tests:

```python
# --- Dashboard endpoint ---


def _dashboard_dataset():
    return {
        "id": "ds-1",
        "name": "rag-golden",
        "items": [
            {"query": "q1", "expected_answer": "a1", "expected_sources": []},
            {"query": "q2", "expected_answer": "a2", "expected_sources": []},
        ],
        "created_at": "2026-05-01T00:00:00+00:00",
    }


def _dashboard_run(run_id, scores, *, notes=None, config=None, baseline_eval_id=None):
    return {
        "id": run_id,
        "dataset_id": "ds-1",
        "status": "completed",
        "collection": "documents",
        "aggregate_scores": scores,
        "created_at": f"2026-05-0{run_id[-1]}T00:00:00+00:00",
        "completed_at": f"2026-05-0{run_id[-1]}T00:01:00+00:00",
        "notes": notes,
        "config": config,
        "baseline_eval_id": baseline_eval_id,
    }


@patch("app.main.get_db")
def test_dashboard_happy_path_uses_all_trends_and_capped_recent_runs(mock_get_db):
    runs = [
        _dashboard_run(
            "eval-1",
            {
                "faithfulness": 0.8,
                "answer_relevancy": 0.7,
                "context_precision": 0.6,
                "context_recall": 0.5,
            },
            notes="baseline",
            config={"chat": {"llm_model": "qwen"}},
        ),
        _dashboard_run(
            "eval-2",
            {
                "faithfulness": 0.85,
                "answer_relevancy": 0.72,
                "context_precision": 0.66,
                "context_recall": 0.51,
            },
            notes="middle",
            baseline_eval_id="eval-1",
        ),
        _dashboard_run(
            "eval-3",
            {
                "faithfulness": 0.9,
                "answer_relevancy": 0.75,
                "context_precision": 0.7,
                "context_recall": 0.55,
            },
            notes="latest",
            baseline_eval_id="eval-1",
        ),
    ]
    mock_db = AsyncMock()
    mock_db.get_dataset.return_value = _dashboard_dataset()
    mock_db.get_completed_evaluations_for_dashboard.return_value = runs
    mock_get_db.return_value = mock_db

    response = client.get(
        "/evaluations/dashboard?dataset_id=ds-1&collection=documents&recent_limit=2"
    )

    assert response.status_code == 200
    body = response.json()
    assert body["dataset"] == {
        "id": "ds-1",
        "name": "rag-golden",
        "item_count": 2,
    }
    assert body["collection"] == "documents"
    assert body["completed_run_count"] == 3
    assert body["first_completed_run"]["id"] == "eval-1"
    assert body["first_completed_run"]["config_captured"] is True
    assert body["latest_completed_run"]["id"] == "eval-3"
    assert [run["id"] for run in body["recent_runs"]] == ["eval-3", "eval-2"]
    assert len(body["metric_trends"]["faithfulness"]) == 3
    assert body["metric_trends"]["faithfulness"][0] == {
        "evaluation_id": "eval-1",
        "completed_at": "2026-05-01T00:01:00+00:00",
        "score": 0.8,
    }
    assert body["baseline_to_latest_deltas"]["baseline_eval_id"] == "eval-1"
    assert body["baseline_to_latest_deltas"]["latest_eval_id"] == "eval-3"
    assert body["baseline_to_latest_deltas"]["deltas"]["faithfulness"] == pytest.approx(
        0.1,
        abs=1e-6,
    )
    assert "results" not in body["recent_runs"][0]
    assert "error" not in body["recent_runs"][0]
    mock_db.get_completed_evaluations_for_dashboard.assert_awaited_once_with(
        dataset_id="ds-1",
        collection="documents",
    )


def test_dashboard_400_when_dataset_id_missing():
    response = client.get("/evaluations/dashboard?collection=documents")

    assert response.status_code == 400
    assert "dataset_id and collection" in response.json()["detail"]


def test_dashboard_400_when_collection_missing():
    response = client.get("/evaluations/dashboard?dataset_id=ds-1")

    assert response.status_code == 400
    assert "dataset_id and collection" in response.json()["detail"]


def test_dashboard_400_when_recent_limit_too_low():
    response = client.get(
        "/evaluations/dashboard?dataset_id=ds-1&collection=documents&recent_limit=0"
    )

    assert response.status_code == 400
    assert "recent_limit must be between 1 and 100" in response.json()["detail"]


def test_dashboard_400_when_recent_limit_too_high():
    response = client.get(
        "/evaluations/dashboard?dataset_id=ds-1&collection=documents&recent_limit=101"
    )

    assert response.status_code == 400
    assert "recent_limit must be between 1 and 100" in response.json()["detail"]


@patch("app.main.get_db")
def test_dashboard_404_when_dataset_missing(mock_get_db):
    mock_db = AsyncMock()
    mock_db.get_dataset.return_value = None
    mock_get_db.return_value = mock_db

    response = client.get(
        "/evaluations/dashboard?dataset_id=missing&collection=documents"
    )

    assert response.status_code == 404
    assert response.json()["detail"] == "Dataset not found"


@patch("app.main.get_db")
def test_dashboard_empty_existing_dataset_returns_200(mock_get_db):
    mock_db = AsyncMock()
    mock_db.get_dataset.return_value = _dashboard_dataset()
    mock_db.get_completed_evaluations_for_dashboard.return_value = []
    mock_get_db.return_value = mock_db

    response = client.get("/evaluations/dashboard?dataset_id=ds-1&collection=documents")

    assert response.status_code == 200
    body = response.json()
    assert body["completed_run_count"] == 0
    assert body["first_completed_run"] is None
    assert body["latest_completed_run"] is None
    assert body["baseline_to_latest_deltas"] is None
    assert body["recent_runs"] == []
    assert body["metric_trends"] == {
        "faithfulness": [],
        "answer_relevancy": [],
        "context_precision": [],
        "context_recall": [],
    }


@patch("app.main.get_db")
def test_dashboard_missing_metric_scores_return_null_values(mock_get_db):
    runs = [
        _dashboard_run("eval-1", {"faithfulness": 0.8}),
        _dashboard_run("eval-2", {"faithfulness": 0.9}, baseline_eval_id="eval-1"),
    ]
    mock_db = AsyncMock()
    mock_db.get_dataset.return_value = _dashboard_dataset()
    mock_db.get_completed_evaluations_for_dashboard.return_value = runs
    mock_get_db.return_value = mock_db

    response = client.get("/evaluations/dashboard?dataset_id=ds-1&collection=documents")

    assert response.status_code == 200
    body = response.json()
    assert body["metric_trends"]["answer_relevancy"] == [
        {
            "evaluation_id": "eval-1",
            "completed_at": "2026-05-01T00:01:00+00:00",
            "score": None,
        },
        {
            "evaluation_id": "eval-2",
            "completed_at": "2026-05-02T00:01:00+00:00",
            "score": None,
        },
    ]
    assert body["baseline_to_latest_deltas"]["deltas"]["faithfulness"] == pytest.approx(
        0.1,
        abs=1e-6,
    )
    assert body["baseline_to_latest_deltas"]["deltas"]["answer_relevancy"] is None
```

- [ ] **Step 2: Run the new route tests to verify they fail**

Run:

```bash
PYTHONPATH=services/eval pytest \
  services/eval/tests/test_main.py::test_dashboard_happy_path_uses_all_trends_and_capped_recent_runs \
  services/eval/tests/test_main.py::test_dashboard_400_when_dataset_id_missing \
  services/eval/tests/test_main.py::test_dashboard_400_when_collection_missing \
  services/eval/tests/test_main.py::test_dashboard_400_when_recent_limit_too_low \
  services/eval/tests/test_main.py::test_dashboard_400_when_recent_limit_too_high \
  services/eval/tests/test_main.py::test_dashboard_404_when_dataset_missing \
  services/eval/tests/test_main.py::test_dashboard_empty_existing_dataset_returns_200 \
  services/eval/tests/test_main.py::test_dashboard_missing_metric_scores_return_null_values \
  -v
```

Expected: route tests fail with `404 Not Found` for `/evaluations/dashboard` before implementation.

- [ ] **Step 3: Import dashboard models**

Change the models import block to include the dashboard models:

```python
from app.models import (
    AttachExperimentRunRequest,
    CreateDatasetRequest,
    CreateExperimentRequest,
    DashboardBaselineDeltas,
    DashboardDatasetSummary,
    DashboardRunSummary,
    EvaluationDashboard,
    MetricTrendPoint,
    QueryScore,
    StartEvaluationRequest,
    UpdateExperimentRequest,
)
```

- [ ] **Step 4: Add dashboard summary helper functions**

In `services/eval/app/main.py`, add these helper functions after `_EVAL_METRICS`:

```python
def _dashboard_run_summary(run: dict) -> DashboardRunSummary:
    return DashboardRunSummary(
        id=run["id"],
        created_at=run["created_at"],
        completed_at=run["completed_at"],
        notes=run.get("notes"),
        config_captured=run.get("config") is not None,
        aggregate_scores=run.get("aggregate_scores"),
        baseline_eval_id=run.get("baseline_eval_id"),
    )


def _metric_trends(runs: list[dict]) -> dict[str, list[MetricTrendPoint]]:
    trends: dict[str, list[MetricTrendPoint]] = {}
    for metric in _EVAL_METRICS:
        trends[metric] = [
            MetricTrendPoint(
                evaluation_id=run["id"],
                completed_at=run.get("completed_at"),
                score=(run.get("aggregate_scores") or {}).get(metric),
            )
            for run in runs
        ]
    return trends


def _baseline_to_latest_deltas(runs: list[dict]) -> DashboardBaselineDeltas | None:
    if len(runs) < 2:
        return None

    baseline = runs[0]
    latest = runs[-1]
    baseline_scores = baseline.get("aggregate_scores") or {}
    latest_scores = latest.get("aggregate_scores") or {}
    deltas: dict[str, float | None] = {}
    for metric in _EVAL_METRICS:
        baseline_score = baseline_scores.get(metric)
        latest_score = latest_scores.get(metric)
        if baseline_score is None or latest_score is None:
            deltas[metric] = None
        else:
            deltas[metric] = round(latest_score - baseline_score, 6)

    return DashboardBaselineDeltas(
        baseline_eval_id=baseline["id"],
        latest_eval_id=latest["id"],
        deltas=QueryScore(**deltas),
    )


def _empty_metric_trends() -> dict[str, list[MetricTrendPoint]]:
    return {metric: [] for metric in _EVAL_METRICS}
```

- [ ] **Step 5: Add the dashboard route before `/evaluations/{eval_id}`**

In `services/eval/app/main.py`, add this route after `get_history` and before `get_evaluation`:

```python
@app.get("/evaluations/dashboard", response_model=EvaluationDashboard)
@limiter.limit("30/minute")
async def get_dashboard(
    request: Request,
    dataset_id: str | None = None,
    collection: str | None = None,
    recent_limit: int = 10,
    user_id: str = Depends(require_auth),
):
    """Compact dashboard summary for completed runs on one dataset+collection."""
    if not dataset_id or not collection:
        raise HTTPException(
            status_code=400,
            detail="dataset_id and collection are both required",
        )
    if not (1 <= recent_limit <= 100):
        raise HTTPException(
            status_code=400,
            detail="recent_limit must be between 1 and 100",
        )

    db = await get_db()
    dataset = await db.get_dataset(dataset_id)
    if not dataset:
        raise HTTPException(status_code=404, detail="Dataset not found")

    runs = await db.get_completed_evaluations_for_dashboard(
        dataset_id=dataset_id,
        collection=collection,
    )
    run_summaries = [_dashboard_run_summary(run) for run in runs]
    recent_runs = list(reversed(run_summaries))[:recent_limit]

    return EvaluationDashboard(
        dataset=DashboardDatasetSummary(
            id=dataset["id"],
            name=dataset["name"],
            item_count=len(dataset["items"]),
        ),
        collection=collection,
        completed_run_count=len(runs),
        first_completed_run=run_summaries[0] if run_summaries else None,
        latest_completed_run=run_summaries[-1] if run_summaries else None,
        metric_trends=_metric_trends(runs) if runs else _empty_metric_trends(),
        recent_runs=recent_runs,
        baseline_to_latest_deltas=_baseline_to_latest_deltas(runs),
    )
```

- [ ] **Step 6: Run the route tests to verify they pass**

Run:

```bash
PYTHONPATH=services/eval pytest \
  services/eval/tests/test_main.py::test_dashboard_happy_path_uses_all_trends_and_capped_recent_runs \
  services/eval/tests/test_main.py::test_dashboard_400_when_dataset_id_missing \
  services/eval/tests/test_main.py::test_dashboard_400_when_collection_missing \
  services/eval/tests/test_main.py::test_dashboard_400_when_recent_limit_too_low \
  services/eval/tests/test_main.py::test_dashboard_400_when_recent_limit_too_high \
  services/eval/tests/test_main.py::test_dashboard_404_when_dataset_missing \
  services/eval/tests/test_main.py::test_dashboard_empty_existing_dataset_returns_200 \
  services/eval/tests/test_main.py::test_dashboard_missing_metric_scores_return_null_values \
  -v
```

Expected: PASS.

- [ ] **Step 7: Run existing history and compare tests to guard compatibility**

Run:

```bash
PYTHONPATH=services/eval pytest \
  services/eval/tests/test_main.py::test_compare_happy_path \
  services/eval/tests/test_main.py::test_compare_handles_missing_metric_scores \
  services/eval/tests/test_main.py::test_history_returns_completed_runs \
  services/eval/tests/test_main.py::test_history_empty_returns_200 \
  -v
```

Expected: PASS.

- [ ] **Step 8: Commit Task 3**

Run:

```bash
git add services/eval/app/main.py services/eval/tests/test_main.py
git commit -m "feat: add eval dashboard summary endpoint"
```

## Task 4: Focused And Required Verification

**Files:**
- Verify: `services/eval/app/models.py`
- Verify: `services/eval/app/db.py`
- Verify: `services/eval/app/main.py`
- Verify: `services/eval/tests/test_models.py`
- Verify: `services/eval/tests/test_db.py`
- Verify: `services/eval/tests/test_main.py`

- [ ] **Step 1: Run focused eval tests**

Run:

```bash
PYTHONPATH=services/eval pytest services/eval/tests/test_models.py services/eval/tests/test_db.py services/eval/tests/test_main.py -v
```

Expected: PASS.

- [ ] **Step 2: Run Python preflight**

Run:

```bash
make preflight-python
```

Expected: PASS. If formatting tools change files, inspect the diff, stage the formatter changes, and rerun this command.

- [ ] **Step 3: Run security preflight**

Run:

```bash
make preflight-security
```

Expected: PASS.

- [ ] **Step 4: Confirm the final diff**

Run:

```bash
git status --short
git diff --stat HEAD
```

Expected: only the intended eval service files are modified since the last task commit.

- [ ] **Step 5: Commit verification fixes if needed**

Only if Step 2 or Step 3 required formatter or lint fixes, run:

```bash
git add services/eval/app/models.py services/eval/app/db.py services/eval/app/main.py services/eval/tests/test_models.py services/eval/tests/test_db.py services/eval/tests/test_main.py
git commit -m "chore: polish eval dashboard summary api"
```

Expected: either a small polish commit is created, or no commit is needed because the branch is already clean.

## Task 5: Push And PR

**Files:**
- No file edits.

- [ ] **Step 1: Confirm branch and status**

Run:

```bash
pwd
git branch --show-current
git status --short
```

Expected: inside `.codex/worktrees/eval-dashboard-summary-api`, branch `feat/eval-dashboard-summary-api`, clean working tree.

- [ ] **Step 2: Push the feature branch**

Run:

```bash
git push -u origin feat/eval-dashboard-summary-api
```

Expected: push succeeds.

- [ ] **Step 3: Create PR to `qa`**

Run:

```bash
gh pr create \
  --base qa \
  --head feat/eval-dashboard-summary-api \
  --title "Add eval dashboard summary API" \
  --body "## Summary
- add compact GET /evaluations/dashboard for dataset+collection RAG performance summaries
- add dashboard response models and compact DB helper
- cover missing dataset, empty summary, recent_limit, trend, and delta behavior

## Verification
- make preflight-python
- make preflight-security"
```

Expected: PR URL is printed. Per repo rules, do not watch CI unless explicitly asked.
