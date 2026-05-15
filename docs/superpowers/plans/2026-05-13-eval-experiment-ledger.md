# Eval Experiment Ledger Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add first-class RAG experiment records to the Python eval service so experiments can track hypotheses, baselines, candidate runs, status, and decisions.

**Architecture:** Extend the existing `services/eval` FastAPI app and SQLite-backed `EvalDB` with additive experiment tables and endpoints. Keep evaluations as the source of run data, attach runs through a join table with labels, and return compact evaluation summaries in experiment responses.

**Tech Stack:** Python, FastAPI, Pydantic, aiosqlite, pytest, pytest-asyncio.

---

## File Structure

- Modify `services/eval/app/models.py`
  - Add request and response models for experiments.
  - Extend `StartEvaluationRequest` with optional experiment attachment fields.

- Modify `services/eval/app/db.py`
  - Add `experiments` and `experiment_runs` schema creation.
  - Add experiment CRUD methods.
  - Add attach/list run methods.
  - Keep existing evaluation row conversion behavior unchanged.

- Modify `services/eval/app/main.py`
  - Add experiment endpoints.
  - Add validation helpers for experiment/run compatibility.
  - Extend `POST /evaluations` to attach newly created runs to an experiment when requested.

- Modify `services/eval/tests/test_models.py`
  - Add Pydantic validation coverage for the new models.

- Modify `services/eval/tests/test_db.py`
  - Add database-level tests for experiments and run attachments.

- Modify `services/eval/tests/test_main.py`
  - Add API endpoint tests and evaluation-start integration tests.

No frontend, Go MCP, Kubernetes, or Docker changes are part of this implementation.

## Task 1: Add Experiment Models

**Files:**
- Modify: `services/eval/app/models.py`
- Modify: `services/eval/tests/test_models.py`

- [ ] **Step 1: Write failing model tests**

First update the import block in `services/eval/tests/test_models.py` to include
the new model names:

```python
from app.models import (
    AttachExperimentRunRequest,
    CreateExperimentRequest,
    ExperimentDetail,
    ExperimentRun,
    ExperimentRunEvaluation,
    QueryScore,
    StartEvaluationRequest,
    UpdateExperimentRequest,
)
from pydantic import ValidationError
```

Then append these tests to `services/eval/tests/test_models.py`:

```python

def test_create_experiment_request_defaults_to_planned():
    req = CreateExperimentRequest(
        name="precision tuning",
        hypothesis="Reranking improves context precision",
        dataset_id="ds-1",
        collection="documents",
    )

    assert req.status == "planned"
    assert req.baseline_eval_id is None
    assert req.notes is None


def test_create_experiment_request_accepts_running_status():
    req = CreateExperimentRequest(
        name="precision tuning",
        hypothesis="Reranking improves context precision",
        dataset_id="ds-1",
        collection="documents",
        status="running",
    )

    assert req.status == "running"


def test_create_experiment_request_rejects_final_initial_status():
    with pytest.raises(ValidationError):
        CreateExperimentRequest(
            name="precision tuning",
            hypothesis="Reranking improves context precision",
            dataset_id="ds-1",
            collection="documents",
            status="completed",
        )


def test_update_experiment_request_accepts_decision_values():
    req = UpdateExperimentRequest(status="completed", decision="keep")

    assert req.status == "completed"
    assert req.decision == "keep"


def test_update_experiment_request_rejects_unknown_decision():
    with pytest.raises(ValidationError):
        UpdateExperimentRequest(decision="ship_it")


def test_attach_experiment_run_request_requires_label():
    with pytest.raises(ValidationError):
        AttachExperimentRunRequest(evaluation_id="eval-1", label="")


def test_start_evaluation_request_accepts_experiment_attachment():
    req = StartEvaluationRequest(
        dataset_id="ds-1",
        experiment_id="exp-1",
        experiment_label="rerank_on",
    )

    assert req.experiment_id == "exp-1"
    assert req.experiment_label == "rerank_on"


def test_experiment_detail_includes_attached_runs():
    detail = ExperimentDetail(
        id="exp-1",
        name="precision tuning",
        hypothesis="Reranking improves context precision",
        dataset_id="ds-1",
        collection="documents",
        baseline_eval_id="eval-base",
        status="running",
        decision=None,
        notes="first pass",
        created_at="2026-05-13T10:00:00+00:00",
        updated_at="2026-05-13T10:00:00+00:00",
        runs=[
            ExperimentRun(
                evaluation_id="eval-base",
                label="baseline",
                notes="rerank off",
                attached_at="2026-05-13T10:01:00+00:00",
                evaluation=ExperimentRunEvaluation(
                    id="eval-base",
                    dataset_id="ds-1",
                    status="completed",
                    collection="documents",
                    aggregate_scores=QueryScore(context_precision=0.31),
                    created_at="2026-05-13T09:50:00+00:00",
                    completed_at="2026-05-13T09:55:00+00:00",
                    notes="baseline",
                    config=None,
                    baseline_eval_id=None,
                ),
            )
        ],
    )

    assert detail.runs[0].label == "baseline"
    assert detail.runs[0].evaluation.aggregate_scores.context_precision == 0.31
```

- [ ] **Step 2: Run model tests and verify they fail**

Run:

```bash
PYTHONPATH=services/eval pytest services/eval/tests/test_models.py -v
```

Expected: FAIL because the experiment models and `StartEvaluationRequest.experiment_id` fields do not exist.

- [ ] **Step 3: Implement Pydantic models**

In `services/eval/app/models.py`, add imports and models exactly around the existing model definitions:

```python
from typing import Any, Literal
```

Replace the current `StartEvaluationRequest` with:

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
```

Add these models after `EvaluationSummary`:

```python
ExperimentStatus = Literal["planned", "running", "completed", "abandoned"]
InitialExperimentStatus = Literal["planned", "running"]
ExperimentDecision = Literal["keep", "revert", "needs_more_data"]


class CreateExperimentRequest(BaseModel):
    name: str = Field(min_length=1, max_length=100)
    hypothesis: str = Field(min_length=1, max_length=1000)
    dataset_id: str
    collection: str = Field(pattern=r"^[a-zA-Z0-9_-]{1,100}$")
    baseline_eval_id: str | None = None
    status: InitialExperimentStatus = "planned"
    notes: str | None = Field(default=None, max_length=2000)


class UpdateExperimentRequest(BaseModel):
    hypothesis: str | None = Field(default=None, min_length=1, max_length=1000)
    baseline_eval_id: str | None = None
    status: ExperimentStatus | None = None
    decision: ExperimentDecision | None = None
    notes: str | None = Field(default=None, max_length=2000)


class AttachExperimentRunRequest(BaseModel):
    evaluation_id: str
    label: str = Field(min_length=1, max_length=100, pattern=r"^[a-zA-Z0-9_-]+$")
    notes: str | None = Field(default=None, max_length=1000)


class ExperimentRunEvaluation(BaseModel):
    id: str
    dataset_id: str
    status: str
    collection: str | None
    aggregate_scores: QueryScore | None
    created_at: str
    completed_at: str | None
    notes: str | None = None
    config: dict[str, Any] | None = None
    baseline_eval_id: str | None = None


class ExperimentRun(BaseModel):
    evaluation_id: str
    label: str
    notes: str | None = None
    attached_at: str
    evaluation: ExperimentRunEvaluation


class ExperimentSummary(BaseModel):
    id: str
    name: str
    hypothesis: str
    dataset_id: str
    collection: str
    baseline_eval_id: str | None = None
    status: ExperimentStatus
    decision: ExperimentDecision | None = None
    notes: str | None = None
    created_at: str
    updated_at: str


class ExperimentDetail(ExperimentSummary):
    runs: list[ExperimentRun] = Field(default_factory=list)
```

- [ ] **Step 4: Run model tests and verify they pass**

Run:

```bash
PYTHONPATH=services/eval pytest services/eval/tests/test_models.py -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

Run:

```bash
git add services/eval/app/models.py services/eval/tests/test_models.py
git commit -m "feat: add eval experiment models"
```

## Task 2: Add Experiment Records To EvalDB

**Files:**
- Modify: `services/eval/app/db.py`
- Modify: `services/eval/tests/test_db.py`

- [ ] **Step 1: Write failing DB tests for experiment CRUD**

Append these tests to `services/eval/tests/test_db.py`:

```python
@pytest.mark.asyncio
async def test_create_get_and_list_experiment(db):
    ds_id = await db.create_dataset(name="ds-exp", items=SIMPLE_ITEM)

    exp_id = await db.create_experiment(
        name="precision tuning",
        hypothesis="Reranking improves context precision",
        dataset_id=ds_id,
        collection="documents",
        baseline_eval_id=None,
        status="running",
        notes="first pass",
    )

    detail = await db.get_experiment(exp_id)
    assert detail["id"] == exp_id
    assert detail["name"] == "precision tuning"
    assert detail["hypothesis"] == "Reranking improves context precision"
    assert detail["dataset_id"] == ds_id
    assert detail["collection"] == "documents"
    assert detail["status"] == "running"
    assert detail["decision"] is None
    assert detail["notes"] == "first pass"
    assert detail["runs"] == []

    experiments = await db.list_experiments(dataset_id=ds_id, collection="documents")
    assert [exp["id"] for exp in experiments] == [exp_id]


@pytest.mark.asyncio
async def test_update_experiment_changes_mutable_fields(db):
    ds_id = await db.create_dataset(name="ds-update", items=SIMPLE_ITEM)
    exp_id = await db.create_experiment(
        name="precision tuning",
        hypothesis="initial hypothesis",
        dataset_id=ds_id,
        collection="documents",
    )

    await db.update_experiment(
        exp_id,
        hypothesis="revised hypothesis",
        baseline_eval_id=None,
        status="completed",
        decision="keep",
        notes="rerank won",
    )

    detail = await db.get_experiment(exp_id)
    assert detail["hypothesis"] == "revised hypothesis"
    assert detail["status"] == "completed"
    assert detail["decision"] == "keep"
    assert detail["notes"] == "rerank won"
    assert detail["updated_at"] >= detail["created_at"]


@pytest.mark.asyncio
async def test_list_experiments_filters_by_status(db):
    ds_id = await db.create_dataset(name="ds-filter", items=SIMPLE_ITEM)
    running_id = await db.create_experiment(
        name="running exp",
        hypothesis="running hypothesis",
        dataset_id=ds_id,
        collection="documents",
        status="running",
    )
    await db.create_experiment(
        name="planned exp",
        hypothesis="planned hypothesis",
        dataset_id=ds_id,
        collection="documents",
        status="planned",
    )

    experiments = await db.list_experiments(status="running")

    assert [exp["id"] for exp in experiments] == [running_id]
```

- [ ] **Step 2: Run DB tests and verify they fail**

Run:

```bash
PYTHONPATH=services/eval pytest services/eval/tests/test_db.py -v
```

Expected: FAIL because `EvalDB.create_experiment`, `get_experiment`, `list_experiments`, and `update_experiment` do not exist.

- [ ] **Step 3: Add experiment schema**

In `EvalDB.init()` in `services/eval/app/db.py`, extend the `executescript` block after the `evaluations` table:

```python
            CREATE TABLE IF NOT EXISTS experiments (
                id TEXT PRIMARY KEY,
                name TEXT NOT NULL,
                hypothesis TEXT NOT NULL,
                dataset_id TEXT NOT NULL REFERENCES datasets(id),
                collection TEXT NOT NULL,
                baseline_eval_id TEXT REFERENCES evaluations(id),
                status TEXT NOT NULL,
                decision TEXT,
                notes TEXT,
                created_at TEXT NOT NULL,
                updated_at TEXT NOT NULL
            );
            CREATE TABLE IF NOT EXISTS experiment_runs (
                experiment_id TEXT NOT NULL REFERENCES experiments(id),
                evaluation_id TEXT NOT NULL REFERENCES evaluations(id),
                label TEXT NOT NULL,
                notes TEXT,
                created_at TEXT NOT NULL,
                PRIMARY KEY (experiment_id, evaluation_id),
                UNIQUE (experiment_id, label)
            );
```

- [ ] **Step 4: Add row conversion helpers and CRUD methods**

Add these methods inside `EvalDB` in `services/eval/app/db.py`:

```python
    def _experiment_row_to_dict(self, row, *, runs: list[dict] | None = None) -> dict:
        out = {
            "id": row["id"],
            "name": row["name"],
            "hypothesis": row["hypothesis"],
            "dataset_id": row["dataset_id"],
            "collection": row["collection"],
            "baseline_eval_id": row["baseline_eval_id"],
            "status": row["status"],
            "decision": row["decision"],
            "notes": row["notes"],
            "created_at": row["created_at"],
            "updated_at": row["updated_at"],
        }
        if runs is not None:
            out["runs"] = runs
        return out

    async def create_experiment(
        self,
        name: str,
        hypothesis: str,
        dataset_id: str,
        collection: str,
        baseline_eval_id: str | None = None,
        status: str = "planned",
        notes: str | None = None,
    ) -> str:
        exp_id = str(uuid.uuid4())
        now = datetime.now(_UTC).isoformat()
        await self._db.execute(
            "INSERT INTO experiments "
            "(id, name, hypothesis, dataset_id, collection, baseline_eval_id, "
            "status, decision, notes, created_at, updated_at) "
            "VALUES (?, ?, ?, ?, ?, ?, ?, NULL, ?, ?, ?)",
            (
                exp_id,
                name,
                hypothesis,
                dataset_id,
                collection,
                baseline_eval_id,
                status,
                notes,
                now,
                now,
            ),
        )
        await self._db.commit()
        return exp_id

    async def get_experiment(self, experiment_id: str) -> dict | None:
        cursor = await self._db.execute(
            "SELECT * FROM experiments WHERE id = ?", (experiment_id,)
        )
        row = await cursor.fetchone()
        if not row:
            return None
        runs = await self.list_experiment_runs(experiment_id)
        return self._experiment_row_to_dict(row, runs=runs)

    async def list_experiments(
        self,
        dataset_id: str | None = None,
        collection: str | None = None,
        status: str | None = None,
    ) -> list[dict]:
        clauses = []
        params: list[str] = []
        if dataset_id is not None:
            clauses.append("dataset_id = ?")
            params.append(dataset_id)
        if collection is not None:
            clauses.append("collection = ?")
            params.append(collection)
        if status is not None:
            clauses.append("status = ?")
            params.append(status)

        where = f"WHERE {' AND '.join(clauses)}" if clauses else ""
        cursor = await self._db.execute(
            f"SELECT * FROM experiments {where} ORDER BY created_at DESC",  # nosec B608
            tuple(params),
        )
        rows = await cursor.fetchall()
        return [self._experiment_row_to_dict(r) for r in rows]

    async def update_experiment(
        self,
        experiment_id: str,
        *,
        hypothesis: str | None = None,
        baseline_eval_id: str | None = None,
        status: str | None = None,
        decision: str | None = None,
        notes: str | None = None,
    ) -> None:
        now = datetime.now(_UTC).isoformat()
        await self._db.execute(
            "UPDATE experiments "
            "SET hypothesis = COALESCE(?, hypothesis), "
            "baseline_eval_id = ?, "
            "status = COALESCE(?, status), "
            "decision = ?, "
            "notes = COALESCE(?, notes), "
            "updated_at = ? "
            "WHERE id = ?",
            (
                hypothesis,
                baseline_eval_id,
                status,
                decision,
                notes,
                now,
                experiment_id,
            ),
        )
        await self._db.commit()
```

- [ ] **Step 5: Add temporary `list_experiment_runs` stub**

Add this stub below `update_experiment` so Task 2 can pass before run attachment is implemented:

```python
    async def list_experiment_runs(self, experiment_id: str) -> list[dict]:
        return []
```

- [ ] **Step 6: Run DB tests and verify Task 2 passes**

Run:

```bash
PYTHONPATH=services/eval pytest services/eval/tests/test_db.py -v
```

Expected: PASS.

- [ ] **Step 7: Commit**

Run:

```bash
git add services/eval/app/db.py services/eval/tests/test_db.py
git commit -m "feat: persist eval experiments"
```

## Task 3: Add Experiment Run Attachment Rules

**Files:**
- Modify: `services/eval/app/db.py`
- Modify: `services/eval/tests/test_db.py`

- [ ] **Step 1: Write failing DB tests for attached runs**

Append these tests to `services/eval/tests/test_db.py`:

```python
@pytest.mark.asyncio
async def test_attach_running_completed_and_failed_runs_to_experiment(db):
    ds_id = await db.create_dataset(name="ds-runs", items=SIMPLE_ITEM)
    exp_id = await db.create_experiment(
        name="precision tuning",
        hypothesis="Reranking improves context precision",
        dataset_id=ds_id,
        collection="documents",
        status="running",
    )
    running_id = await db.create_evaluation(dataset_id=ds_id, collection="documents")
    completed_id = await db.create_evaluation(dataset_id=ds_id, collection="documents")
    failed_id = await db.create_evaluation(dataset_id=ds_id, collection="documents")
    await db.complete_evaluation(
        completed_id,
        aggregate_scores={"context_precision": 0.4},
        results=[],
    )
    await db.fail_evaluation(failed_id, error="judge failed")

    await db.attach_experiment_run(
        exp_id, running_id, label="candidate_running", notes="still running"
    )
    await db.attach_experiment_run(
        exp_id, completed_id, label="candidate_completed", notes="finished"
    )
    await db.attach_experiment_run(
        exp_id, failed_id, label="candidate_failed", notes="failed"
    )

    detail = await db.get_experiment(exp_id)
    labels = [run["label"] for run in detail["runs"]]
    statuses = [run["evaluation"]["status"] for run in detail["runs"]]
    assert labels == ["candidate_running", "candidate_completed", "candidate_failed"]
    assert statuses == ["running", "completed", "failed"]


@pytest.mark.asyncio
async def test_attach_experiment_run_rejects_duplicate_label(db):
    ds_id = await db.create_dataset(name="ds-dupe-label", items=SIMPLE_ITEM)
    exp_id = await db.create_experiment(
        name="precision tuning",
        hypothesis="Reranking improves context precision",
        dataset_id=ds_id,
        collection="documents",
    )
    run_1 = await db.create_evaluation(dataset_id=ds_id, collection="documents")
    run_2 = await db.create_evaluation(dataset_id=ds_id, collection="documents")

    await db.attach_experiment_run(exp_id, run_1, label="candidate")

    with pytest.raises(ValueError, match="duplicate experiment run label"):
        await db.attach_experiment_run(exp_id, run_2, label="candidate")


@pytest.mark.asyncio
async def test_attach_experiment_run_rejects_completed_experiment(db):
    ds_id = await db.create_dataset(name="ds-completed-exp", items=SIMPLE_ITEM)
    exp_id = await db.create_experiment(
        name="precision tuning",
        hypothesis="Reranking improves context precision",
        dataset_id=ds_id,
        collection="documents",
        status="completed",
    )
    run_id = await db.create_evaluation(dataset_id=ds_id, collection="documents")

    with pytest.raises(ValueError, match="completed experiments cannot accept runs"):
        await db.attach_experiment_run(exp_id, run_id, label="candidate")


@pytest.mark.asyncio
async def test_attach_experiment_run_returns_none_for_missing_experiment_or_run(db):
    ds_id = await db.create_dataset(name="ds-missing-run", items=SIMPLE_ITEM)
    exp_id = await db.create_experiment(
        name="precision tuning",
        hypothesis="Reranking improves context precision",
        dataset_id=ds_id,
        collection="documents",
    )

    assert await db.attach_experiment_run("missing-exp", "missing-run", "candidate") is None
    assert await db.attach_experiment_run(exp_id, "missing-run", "candidate") is None


@pytest.mark.asyncio
async def test_attach_experiment_run_rejects_dataset_or_collection_mismatch(db):
    ds_id = await db.create_dataset(name="ds-match", items=SIMPLE_ITEM)
    other_ds_id = await db.create_dataset(name="ds-other", items=SIMPLE_ITEM)
    exp_id = await db.create_experiment(
        name="precision tuning",
        hypothesis="Reranking improves context precision",
        dataset_id=ds_id,
        collection="documents",
    )
    other_dataset_run = await db.create_evaluation(
        dataset_id=other_ds_id, collection="documents"
    )
    other_collection_run = await db.create_evaluation(
        dataset_id=ds_id, collection="release-notes"
    )

    with pytest.raises(ValueError, match="same dataset"):
        await db.attach_experiment_run(exp_id, other_dataset_run, label="other_ds")

    with pytest.raises(ValueError, match="same collection"):
        await db.attach_experiment_run(
            exp_id, other_collection_run, label="other_collection"
        )
```

- [ ] **Step 2: Run DB tests and verify they fail**

Run:

```bash
PYTHONPATH=services/eval pytest services/eval/tests/test_db.py -v
```

Expected: FAIL because the `attach_experiment_run` method does not exist and `list_experiment_runs` is still a stub.

- [ ] **Step 3: Implement run row conversion and attachment**

In `services/eval/app/db.py`, replace the `list_experiment_runs` stub with:

```python
    def _experiment_run_row_to_dict(self, row) -> dict:
        evaluation = self._row_to_dict(row, include_results=False)
        return {
            "evaluation_id": row["evaluation_id"],
            "label": row["label"],
            "notes": row["run_notes"],
            "attached_at": row["attached_at"],
            "evaluation": evaluation,
        }

    async def attach_experiment_run(
        self,
        experiment_id: str,
        evaluation_id: str,
        label: str,
        notes: str | None = None,
    ) -> dict | None:
        experiment_cursor = await self._db.execute(
            "SELECT * FROM experiments WHERE id = ?", (experiment_id,)
        )
        experiment = await experiment_cursor.fetchone()
        if not experiment:
            return None

        evaluation = await self.get_evaluation(evaluation_id)
        if not evaluation:
            return None

        if experiment["status"] == "completed":
            raise ValueError("completed experiments cannot accept runs")
        if experiment["dataset_id"] != evaluation["dataset_id"]:
            raise ValueError("experiment run must use the same dataset")
        if experiment["collection"] != evaluation["collection"]:
            raise ValueError("experiment run must use the same collection")

        now = datetime.now(_UTC).isoformat()
        try:
            await self._db.execute(
                "INSERT INTO experiment_runs "
                "(experiment_id, evaluation_id, label, notes, created_at) "
                "VALUES (?, ?, ?, ?, ?)",
                (experiment_id, evaluation_id, label, notes, now),
            )
        except aiosqlite.IntegrityError as exc:
            if "experiment_runs.experiment_id, experiment_runs.label" in str(exc):
                raise ValueError("duplicate experiment run label") from exc
            raise
        await self._db.commit()
        return await self.get_experiment(experiment_id)

    async def list_experiment_runs(self, experiment_id: str) -> list[dict]:
        cursor = await self._db.execute(
            "SELECT "
            "er.evaluation_id, er.label, er.notes AS run_notes, "
            "er.created_at AS attached_at, "
            "e.* "
            "FROM experiment_runs er "
            "JOIN evaluations e ON e.id = er.evaluation_id "
            "WHERE er.experiment_id = ? "
            "ORDER BY er.created_at ASC",
            (experiment_id,),
        )
        rows = await cursor.fetchall()
        return [self._experiment_run_row_to_dict(r) for r in rows]
```

- [ ] **Step 4: Run DB tests and verify they pass**

Run:

```bash
PYTHONPATH=services/eval pytest services/eval/tests/test_db.py -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

Run:

```bash
git add services/eval/app/db.py services/eval/tests/test_db.py
git commit -m "feat: attach eval runs to experiments"
```

## Task 4: Add Experiment API Endpoints

**Files:**
- Modify: `services/eval/app/main.py`
- Modify: `services/eval/tests/test_main.py`

- [ ] **Step 1: Write failing API tests**

Append these tests to `services/eval/tests/test_main.py` before the health-check tests:

```python
def _stub_experiment(exp_id="exp-1", status="running", runs=None):
    return {
        "id": exp_id,
        "name": "precision tuning",
        "hypothesis": "Reranking improves context precision",
        "dataset_id": "ds-1",
        "collection": "documents",
        "baseline_eval_id": None,
        "status": status,
        "decision": None,
        "notes": "first pass",
        "created_at": "2026-05-13T10:00:00+00:00",
        "updated_at": "2026-05-13T10:00:00+00:00",
        "runs": runs or [],
    }


@patch("app.main.get_db")
def test_create_experiment(mock_get_db):
    mock_db = AsyncMock()
    mock_db.get_dataset.return_value = {"id": "ds-1", "name": "ds", "items": []}
    mock_db.create_experiment.return_value = "exp-1"
    mock_db.get_experiment.return_value = _stub_experiment()
    mock_get_db.return_value = mock_db

    response = client.post(
        "/experiments",
        json={
            "name": "precision tuning",
            "hypothesis": "Reranking improves context precision",
            "dataset_id": "ds-1",
            "collection": "documents",
            "status": "running",
            "notes": "first pass",
        },
    )

    assert response.status_code == 201
    assert response.json()["id"] == "exp-1"
    mock_db.create_experiment.assert_awaited_once_with(
        name="precision tuning",
        hypothesis="Reranking improves context precision",
        dataset_id="ds-1",
        collection="documents",
        baseline_eval_id=None,
        status="running",
        notes="first pass",
    )


@patch("app.main.get_db")
def test_create_experiment_rejects_unknown_dataset(mock_get_db):
    mock_db = AsyncMock()
    mock_db.get_dataset.return_value = None
    mock_get_db.return_value = mock_db

    response = client.post(
        "/experiments",
        json={
            "name": "precision tuning",
            "hypothesis": "Reranking improves context precision",
            "dataset_id": "missing",
            "collection": "documents",
        },
    )

    assert response.status_code == 404
    assert response.json()["detail"] == "Dataset not found"


@patch("app.main.get_db")
def test_list_experiments(mock_get_db):
    mock_db = AsyncMock()
    mock_db.list_experiments.return_value = [_stub_experiment(runs=None)]
    mock_get_db.return_value = mock_db

    response = client.get(
        "/experiments?dataset_id=ds-1&collection=documents&status=running"
    )

    assert response.status_code == 200
    assert len(response.json()["experiments"]) == 1
    mock_db.list_experiments.assert_awaited_once_with(
        dataset_id="ds-1", collection="documents", status="running"
    )


@patch("app.main.get_db")
def test_get_experiment(mock_get_db):
    mock_db = AsyncMock()
    mock_db.get_experiment.return_value = _stub_experiment()
    mock_get_db.return_value = mock_db

    response = client.get("/experiments/exp-1")

    assert response.status_code == 200
    assert response.json()["id"] == "exp-1"


@patch("app.main.get_db")
def test_get_experiment_not_found(mock_get_db):
    mock_db = AsyncMock()
    mock_db.get_experiment.return_value = None
    mock_get_db.return_value = mock_db

    response = client.get("/experiments/missing")

    assert response.status_code == 404
    assert response.json()["detail"] == "Experiment not found"


@patch("app.main.get_db")
def test_patch_experiment_records_decision_when_completed(mock_get_db):
    mock_db = AsyncMock()
    mock_db.get_experiment.side_effect = [
        _stub_experiment(status="running"),
        _stub_experiment(status="completed"),
    ]
    mock_get_db.return_value = mock_db

    response = client.patch(
        "/experiments/exp-1",
        json={"status": "completed", "decision": "keep", "notes": "rerank won"},
    )

    assert response.status_code == 200
    mock_db.update_experiment.assert_awaited_once_with(
        "exp-1",
        hypothesis=None,
        baseline_eval_id=None,
        status="completed",
        decision="keep",
        notes="rerank won",
    )


@patch("app.main.get_db")
def test_patch_experiment_rejects_decision_without_completed_status(mock_get_db):
    mock_db = AsyncMock()
    mock_db.get_experiment.return_value = _stub_experiment(status="running")
    mock_get_db.return_value = mock_db

    response = client.patch("/experiments/exp-1", json={"decision": "keep"})

    assert response.status_code == 400
    assert "decision requires completed status" in response.json()["detail"]


@patch("app.main.get_db")
def test_attach_experiment_run_endpoint(mock_get_db):
    mock_db = AsyncMock()
    mock_db.attach_experiment_run.return_value = _stub_experiment(
        runs=[
            {
                "evaluation_id": "eval-1",
                "label": "candidate",
                "notes": None,
                "attached_at": "2026-05-13T10:01:00+00:00",
                "evaluation": _stub_run(
                    "eval-1", "ds-1", {"context_precision": 0.42}
                ),
            }
        ]
    )
    mock_get_db.return_value = mock_db

    response = client.post(
        "/experiments/exp-1/runs",
        json={"evaluation_id": "eval-1", "label": "candidate"},
    )

    assert response.status_code == 200
    assert response.json()["runs"][0]["label"] == "candidate"
    mock_db.attach_experiment_run.assert_awaited_once_with(
        "exp-1", "eval-1", label="candidate", notes=None
    )


@patch("app.main.get_db")
def test_attach_experiment_run_duplicate_label_returns_409(mock_get_db):
    mock_db = AsyncMock()
    mock_db.attach_experiment_run.side_effect = ValueError(
        "duplicate experiment run label"
    )
    mock_get_db.return_value = mock_db

    response = client.post(
        "/experiments/exp-1/runs",
        json={"evaluation_id": "eval-1", "label": "candidate"},
    )

    assert response.status_code == 409
    assert response.json()["detail"] == "duplicate experiment run label"
```

- [ ] **Step 2: Run API tests and verify they fail**

Run:

```bash
PYTHONPATH=services/eval pytest services/eval/tests/test_main.py -v
```

Expected: FAIL because `/experiments` endpoints do not exist.

- [ ] **Step 3: Import experiment models in main**

In `services/eval/app/main.py`, replace the current models import with:

```python
from app.models import (
    AttachExperimentRunRequest,
    CreateDatasetRequest,
    CreateExperimentRequest,
    StartEvaluationRequest,
    UpdateExperimentRequest,
)
```

- [ ] **Step 4: Add experiment validation helper**

Add this helper near `_validate_baseline`:

```python
async def _validate_experiment_baseline(
    db: EvalDB, baseline_eval_id: str, dataset_id: str, collection: str
) -> None:
    baseline = await db.get_evaluation(baseline_eval_id)
    if not baseline:
        raise HTTPException(status_code=404, detail="Baseline evaluation not found")
    if baseline["dataset_id"] != dataset_id:
        raise HTTPException(
            status_code=400,
            detail="Baseline evaluation must use the same dataset",
        )
    if baseline["collection"] != collection:
        raise HTTPException(
            status_code=400,
            detail="Baseline evaluation must use the same collection",
        )
```

- [ ] **Step 5: Add experiment endpoints before evaluation literal routes**

In `services/eval/app/main.py`, add these endpoints after `start_evaluation` and before `@app.get("/evaluations")`:

```python
@app.post("/experiments", status_code=201)
@limiter.limit("10/minute")
async def create_experiment(
    request: Request,
    body: CreateExperimentRequest,
    user_id: str = Depends(require_auth),
):
    db = await get_db()
    dataset = await db.get_dataset(body.dataset_id)
    if not dataset:
        raise HTTPException(status_code=404, detail="Dataset not found")
    if body.baseline_eval_id is not None:
        await _validate_experiment_baseline(
            db, body.baseline_eval_id, body.dataset_id, body.collection
        )

    exp_id = await db.create_experiment(
        name=body.name,
        hypothesis=body.hypothesis,
        dataset_id=body.dataset_id,
        collection=body.collection,
        baseline_eval_id=body.baseline_eval_id,
        status=body.status,
        notes=body.notes,
    )
    if body.baseline_eval_id is not None:
        await db.attach_experiment_run(
            exp_id, body.baseline_eval_id, label="baseline", notes="baseline"
        )
    experiment = await db.get_experiment(exp_id)
    return experiment


@app.get("/experiments")
@limiter.limit("30/minute")
async def list_experiments(
    request: Request,
    dataset_id: str | None = None,
    collection: str | None = None,
    status: str | None = None,
    user_id: str = Depends(require_auth),
):
    db = await get_db()
    experiments = await db.list_experiments(
        dataset_id=dataset_id, collection=collection, status=status
    )
    return {"experiments": experiments}


@app.get("/experiments/{experiment_id}")
@limiter.limit("30/minute")
async def get_experiment(
    request: Request, experiment_id: str, user_id: str = Depends(require_auth)
):
    db = await get_db()
    experiment = await db.get_experiment(experiment_id)
    if not experiment:
        raise HTTPException(status_code=404, detail="Experiment not found")
    return experiment


@app.patch("/experiments/{experiment_id}")
@limiter.limit("10/minute")
async def update_experiment(
    request: Request,
    experiment_id: str,
    body: UpdateExperimentRequest,
    user_id: str = Depends(require_auth),
):
    db = await get_db()
    experiment = await db.get_experiment(experiment_id)
    if not experiment:
        raise HTTPException(status_code=404, detail="Experiment not found")

    final_status = body.status or experiment["status"]
    if body.decision is not None and final_status != "completed":
        raise HTTPException(
            status_code=400, detail="decision requires completed status"
        )
    baseline_eval_id = body.baseline_eval_id
    if baseline_eval_id is not None:
        await _validate_experiment_baseline(
            db,
            baseline_eval_id,
            experiment["dataset_id"],
            experiment["collection"],
        )

    await db.update_experiment(
        experiment_id,
        hypothesis=body.hypothesis,
        baseline_eval_id=baseline_eval_id,
        status=body.status,
        decision=body.decision,
        notes=body.notes,
    )
    updated = await db.get_experiment(experiment_id)
    return updated


@app.get("/experiments/{experiment_id}/runs")
@limiter.limit("30/minute")
async def list_experiment_runs(
    request: Request, experiment_id: str, user_id: str = Depends(require_auth)
):
    db = await get_db()
    experiment = await db.get_experiment(experiment_id)
    if not experiment:
        raise HTTPException(status_code=404, detail="Experiment not found")
    return {"runs": experiment["runs"]}


@app.post("/experiments/{experiment_id}/runs")
@limiter.limit("10/minute")
async def attach_experiment_run(
    request: Request,
    experiment_id: str,
    body: AttachExperimentRunRequest,
    user_id: str = Depends(require_auth),
):
    db = await get_db()
    try:
        experiment = await db.attach_experiment_run(
            experiment_id, body.evaluation_id, label=body.label, notes=body.notes
        )
    except ValueError as exc:
        detail = str(exc)
        status_code = 409 if "duplicate" in detail else 400
        raise HTTPException(status_code=status_code, detail=detail) from exc
    if not experiment:
        raise HTTPException(status_code=404, detail="Experiment or evaluation not found")
    return experiment
```

- [ ] **Step 6: Run API tests and fix mock expectations**

Run:

```bash
PYTHONPATH=services/eval pytest services/eval/tests/test_main.py -v
```

Expected: PASS. If any mocked experiment response fails Pydantic serialization, add the missing field to `_stub_experiment` or `_stub_run` so the mocked response matches the API response shape.

- [ ] **Step 7: Commit**

Run:

```bash
git add services/eval/app/main.py services/eval/tests/test_main.py
git commit -m "feat: add eval experiment endpoints"
```

## Task 5: Attach New Evaluation Runs To Experiments

**Files:**
- Modify: `services/eval/app/main.py`
- Modify: `services/eval/tests/test_main.py`

- [ ] **Step 1: Write failing API tests for start-evaluation integration**

Append these tests near the existing `test_start_evaluation_*` tests in `services/eval/tests/test_main.py`:

```python
@patch("app.main.get_db")
def test_start_evaluation_attaches_run_to_experiment(mock_get_db):
    mock_db = AsyncMock()
    mock_db.get_dataset.return_value = {
        "id": "ds-123",
        "name": "test",
        "items": [{"query": "q", "expected_answer": "a", "expected_sources": []}],
        "created_at": "2026-04-16T00:00:00Z",
    }
    mock_db.get_experiment.return_value = _stub_experiment(status="running")
    mock_db.create_evaluation.return_value = "eval-candidate"
    mock_db.attach_experiment_run.return_value = _stub_experiment()
    mock_get_db.return_value = mock_db

    response = client.post(
        "/evaluations",
        json={
            "dataset_id": "ds-123",
            "collection": "documents",
            "experiment_id": "exp-1",
            "experiment_label": "rerank_on",
        },
    )

    assert response.status_code == 202
    mock_db.attach_experiment_run.assert_awaited_once_with(
        "exp-1", "eval-candidate", label="rerank_on", notes=None
    )


@patch("app.main.get_db")
def test_start_evaluation_rejects_experiment_attachment_without_label(mock_get_db):
    mock_db = AsyncMock()
    mock_db.get_dataset.return_value = {
        "id": "ds-123",
        "name": "test",
        "items": [{"query": "q", "expected_answer": "a", "expected_sources": []}],
        "created_at": "2026-04-16T00:00:00Z",
    }
    mock_get_db.return_value = mock_db

    response = client.post(
        "/evaluations",
        json={"dataset_id": "ds-123", "experiment_id": "exp-1"},
    )

    assert response.status_code == 400
    assert response.json()["detail"] == "experiment_label is required with experiment_id"
    mock_db.create_evaluation.assert_not_awaited()


@patch("app.main.get_db")
def test_start_evaluation_rejects_missing_experiment(mock_get_db):
    mock_db = AsyncMock()
    mock_db.get_dataset.return_value = {
        "id": "ds-123",
        "name": "test",
        "items": [{"query": "q", "expected_answer": "a", "expected_sources": []}],
        "created_at": "2026-04-16T00:00:00Z",
    }
    mock_db.get_experiment.return_value = None
    mock_get_db.return_value = mock_db

    response = client.post(
        "/evaluations",
        json={
            "dataset_id": "ds-123",
            "experiment_id": "missing-exp",
            "experiment_label": "candidate",
        },
    )

    assert response.status_code == 404
    assert response.json()["detail"] == "Experiment not found"
    mock_db.create_evaluation.assert_not_awaited()


@patch("app.main.get_db")
def test_start_evaluation_rejects_experiment_dataset_mismatch(mock_get_db):
    mock_db = AsyncMock()
    mock_db.get_dataset.return_value = {
        "id": "ds-123",
        "name": "test",
        "items": [{"query": "q", "expected_answer": "a", "expected_sources": []}],
        "created_at": "2026-04-16T00:00:00Z",
    }
    mock_db.get_experiment.return_value = {
        **_stub_experiment(status="running"),
        "dataset_id": "other-ds",
    }
    mock_get_db.return_value = mock_db

    response = client.post(
        "/evaluations",
        json={
            "dataset_id": "ds-123",
            "experiment_id": "exp-1",
            "experiment_label": "candidate",
        },
    )

    assert response.status_code == 400
    assert response.json()["detail"] == "Experiment must use the same dataset"
    mock_db.create_evaluation.assert_not_awaited()


@patch("app.main.get_db")
def test_start_evaluation_rejects_experiment_collection_mismatch(mock_get_db):
    mock_db = AsyncMock()
    mock_db.get_dataset.return_value = {
        "id": "ds-123",
        "name": "test",
        "items": [{"query": "q", "expected_answer": "a", "expected_sources": []}],
        "created_at": "2026-04-16T00:00:00Z",
    }
    mock_db.get_experiment.return_value = {
        **_stub_experiment(status="running"),
        "collection": "release-notes",
    }
    mock_get_db.return_value = mock_db

    response = client.post(
        "/evaluations",
        json={
            "dataset_id": "ds-123",
            "collection": "documents",
            "experiment_id": "exp-1",
            "experiment_label": "candidate",
        },
    )

    assert response.status_code == 400
    assert response.json()["detail"] == "Experiment must use the same collection"
    mock_db.create_evaluation.assert_not_awaited()
```

- [ ] **Step 2: Run targeted tests and verify they fail**

Run:

```bash
PYTHONPATH=services/eval pytest services/eval/tests/test_main.py -v
```

Expected: FAIL because `start_evaluation` ignores experiment attachment fields.

- [ ] **Step 3: Add experiment attachment validation helper**

In `services/eval/app/main.py`, add this helper near `_validate_experiment_baseline`:

```python
async def _validate_experiment_for_run(
    db: EvalDB, experiment_id: str, dataset_id: str, collection: str
) -> None:
    experiment = await db.get_experiment(experiment_id)
    if not experiment:
        raise HTTPException(status_code=404, detail="Experiment not found")
    if experiment["dataset_id"] != dataset_id:
        raise HTTPException(
            status_code=400, detail="Experiment must use the same dataset"
        )
    if experiment["collection"] != collection:
        raise HTTPException(
            status_code=400, detail="Experiment must use the same collection"
        )
    if experiment["status"] == "completed":
        raise HTTPException(
            status_code=400, detail="completed experiments cannot accept runs"
        )
```

- [ ] **Step 4: Extend `start_evaluation`**

In `start_evaluation`, after baseline validation and before `db.create_evaluation`, add:

```python
    if body.experiment_id is not None:
        if body.experiment_label is None:
            raise HTTPException(
                status_code=400,
                detail="experiment_label is required with experiment_id",
            )
        await _validate_experiment_for_run(
            db, body.experiment_id, body.dataset_id, collection
        )
```

After `eval_id = await db.create_evaluation(...)` and before `background_tasks.add_task(...)`, add:

```python
    if body.experiment_id is not None and body.experiment_label is not None:
        try:
            await db.attach_experiment_run(
                body.experiment_id,
                eval_id,
                label=body.experiment_label,
                notes=body.notes,
            )
        except ValueError as exc:
            detail = str(exc)
            status_code = 409 if "duplicate" in detail else 400
            raise HTTPException(status_code=status_code, detail=detail) from exc
```

- [ ] **Step 5: Run API tests and verify they pass**

Run:

```bash
PYTHONPATH=services/eval pytest services/eval/tests/test_main.py -v
```

Expected: PASS.

- [ ] **Step 6: Commit**

Run:

```bash
git add services/eval/app/main.py services/eval/tests/test_main.py
git commit -m "feat: attach eval runs when starting experiments"
```

## Task 6: Full Verification

**Files:**
- No source edits expected.

- [ ] **Step 1: Run focused eval service tests**

Run:

```bash
PYTHONPATH=services/eval pytest services/eval/tests/ -v
```

Expected: PASS.

- [ ] **Step 2: Run Python preflight**

Run:

```bash
make preflight-python
```

Expected: PASS.

- [ ] **Step 3: Run security preflight**

Run:

```bash
make preflight-security
```

Expected: PASS.

- [ ] **Step 4: Check worktree status**

Run:

```bash
git status --short
```

Expected: only intentional source changes are present before the final commit, or a clean tree if every task has already been committed.

## Self-Review

- Spec coverage:
  - First-class experiment records: Tasks 1, 2, and 4.
  - In-progress experiment support: Tasks 1, 2, 3, 4, and 5.
  - Attach running/completed/failed evaluations: Task 3.
  - Dataset and collection boundaries: Tasks 3, 4, and 5.
  - Stable run labels: Tasks 1, 3, 4, and 5.
  - Compact responses without per-query payloads: Tasks 1, 3, and 4.
  - Existing eval behavior unchanged: Task 6 plus regression coverage in Tasks 4 and 5.

- Completion-marker scan:
  - The plan contains no unresolved marker text.
  - Every task names concrete files, commands, expected outcomes, and commit messages.

- Type consistency:
  - Request fields use `experiment_id` and `experiment_label` consistently.
  - Run attachment labels use the same `label` field in models, DB, and API.
  - Experiment status values are `planned`, `running`, `completed`, and `abandoned`.
  - Decision values are `keep`, `revert`, and `needs_more_data`.
