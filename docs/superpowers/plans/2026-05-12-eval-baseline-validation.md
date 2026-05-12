# Eval Baseline Validation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Validate `baseline_eval_id` before starting eval runs so only completed runs from the same dataset and effective collection can be used as baselines.

**Architecture:** Keep validation in the eval API layer with a private helper in `services/eval/app/main.py`. The endpoint will reuse `EvalDB.get_evaluation()` for baseline lookup, map invalid relationships to HTTP errors, and only create/schedule a new run after validation passes.

**Tech Stack:** Python 3, FastAPI, Pydantic, pytest, `unittest.mock.AsyncMock`, existing eval service SQLite access through `EvalDB`.

---

## File Structure

- Modify `services/eval/app/main.py`
  - Add `_validate_baseline(...)`.
  - Compute the effective collection once in `start_evaluation`.
  - Call baseline validation before `create_evaluation`.
- Modify `services/eval/tests/test_main.py`
  - Add API tests for valid and invalid baseline links.
  - Update the existing baseline persistence test so the mocked baseline lookup is explicit.
- No DB schema, model, or migration changes.

## Execution Prerequisite

This plan changes application behavior. If execution starts from `qa`, create an
isolated feature worktree first with the `superpowers:using-git-worktrees` skill.
Use a branch name such as `feature/eval-baseline-validation` and target the final
PR to `qa`.

## Task 1: Add Failing API Tests For Baseline Validation

**Files:**
- Modify: `services/eval/tests/test_main.py`

- [ ] **Step 1: Add a reusable completed baseline fixture helper near the evaluation endpoint tests**

Add this helper after `test_start_evaluation_dataset_not_found` and before `test_get_evaluation`:

```python
def _baseline_run(
    *,
    status="completed",
    dataset_id="ds-123",
    collection="documents",
):
    return {
        "id": "eval-prev",
        "dataset_id": dataset_id,
        "status": status,
        "collection": collection,
        "aggregate_scores": {"faithfulness": 0.87},
        "results": [],
        "error": None,
        "created_at": "2026-04-16T00:00:00Z",
        "completed_at": "2026-04-16T00:05:00Z",
        "notes": None,
        "config": None,
        "baseline_eval_id": None,
    }
```

- [ ] **Step 2: Update the existing baseline persistence test to mock baseline lookup**

In `test_start_evaluation_persists_notes_and_baseline`, add this line before `mock_db.create_evaluation.return_value = "eval-789"`:

```python
    mock_db.get_evaluation.return_value = _baseline_run()
```

Then add this assertion after the response status assertion:

```python
    mock_db.get_evaluation.assert_awaited_once_with("eval-prev")
```

- [ ] **Step 3: Add a test for unknown baseline id**

Add this test after `test_start_evaluation_persists_notes_and_baseline`:

```python
@patch("app.main.get_db")
def test_start_evaluation_rejects_unknown_baseline(mock_get_db):
    mock_db = AsyncMock()
    mock_db.get_dataset.return_value = {
        "id": "ds-123",
        "name": "test",
        "items": [{"query": "q", "expected_answer": "a", "expected_sources": []}],
        "created_at": "2026-04-16T00:00:00Z",
    }
    mock_db.get_evaluation.return_value = None
    mock_get_db.return_value = mock_db

    response = client.post(
        "/evaluations",
        json={"dataset_id": "ds-123", "baseline_eval_id": "missing-eval"},
    )

    assert response.status_code == 404
    assert response.json()["detail"] == "Baseline evaluation not found"
    mock_db.get_evaluation.assert_awaited_once_with("missing-eval")
    mock_db.create_evaluation.assert_not_awaited()
```

- [ ] **Step 4: Add a test for non-completed baseline status**

Add this test after the unknown baseline test:

```python
@patch("app.main.get_db")
def test_start_evaluation_rejects_incomplete_baseline(mock_get_db):
    mock_db = AsyncMock()
    mock_db.get_dataset.return_value = {
        "id": "ds-123",
        "name": "test",
        "items": [{"query": "q", "expected_answer": "a", "expected_sources": []}],
        "created_at": "2026-04-16T00:00:00Z",
    }
    mock_db.get_evaluation.return_value = _baseline_run(status="running")
    mock_get_db.return_value = mock_db

    response = client.post(
        "/evaluations",
        json={"dataset_id": "ds-123", "baseline_eval_id": "eval-prev"},
    )

    assert response.status_code == 400
    assert response.json()["detail"] == "Baseline evaluation must be completed"
    mock_db.create_evaluation.assert_not_awaited()
```

- [ ] **Step 5: Add a test for failed baseline status**

Add this test after the incomplete baseline test:

```python
@patch("app.main.get_db")
def test_start_evaluation_rejects_failed_baseline(mock_get_db):
    mock_db = AsyncMock()
    mock_db.get_dataset.return_value = {
        "id": "ds-123",
        "name": "test",
        "items": [{"query": "q", "expected_answer": "a", "expected_sources": []}],
        "created_at": "2026-04-16T00:00:00Z",
    }
    mock_db.get_evaluation.return_value = _baseline_run(status="failed")
    mock_get_db.return_value = mock_db

    response = client.post(
        "/evaluations",
        json={"dataset_id": "ds-123", "baseline_eval_id": "eval-prev"},
    )

    assert response.status_code == 400
    assert response.json()["detail"] == "Baseline evaluation must be completed"
    mock_db.create_evaluation.assert_not_awaited()
```

- [ ] **Step 6: Add a test for dataset mismatch**

Add this test after the failed baseline test:

```python
@patch("app.main.get_db")
def test_start_evaluation_rejects_baseline_for_different_dataset(mock_get_db):
    mock_db = AsyncMock()
    mock_db.get_dataset.return_value = {
        "id": "ds-123",
        "name": "test",
        "items": [{"query": "q", "expected_answer": "a", "expected_sources": []}],
        "created_at": "2026-04-16T00:00:00Z",
    }
    mock_db.get_evaluation.return_value = _baseline_run(dataset_id="other-ds")
    mock_get_db.return_value = mock_db

    response = client.post(
        "/evaluations",
        json={"dataset_id": "ds-123", "baseline_eval_id": "eval-prev"},
    )

    assert response.status_code == 400
    assert response.json()["detail"] == "Baseline evaluation must use the same dataset"
    mock_db.create_evaluation.assert_not_awaited()
```

- [ ] **Step 7: Add a test for collection mismatch**

Add this test after the dataset mismatch test:

```python
@patch("app.main.get_db")
def test_start_evaluation_rejects_baseline_for_different_collection(mock_get_db):
    mock_db = AsyncMock()
    mock_db.get_dataset.return_value = {
        "id": "ds-123",
        "name": "test",
        "items": [{"query": "q", "expected_answer": "a", "expected_sources": []}],
        "created_at": "2026-04-16T00:00:00Z",
    }
    mock_db.get_evaluation.return_value = _baseline_run(collection="other-docs")
    mock_get_db.return_value = mock_db

    response = client.post(
        "/evaluations",
        json={"dataset_id": "ds-123", "baseline_eval_id": "eval-prev"},
    )

    assert response.status_code == 400
    assert response.json()["detail"] == "Baseline evaluation must use the same collection"
    mock_db.create_evaluation.assert_not_awaited()
```

- [ ] **Step 8: Add a test for valid custom collection baseline**

Add this test after the collection mismatch test:

```python
@patch("app.main.get_db")
def test_start_evaluation_accepts_valid_baseline_for_custom_collection(mock_get_db):
    mock_db = AsyncMock()
    mock_db.get_dataset.return_value = {
        "id": "ds-123",
        "name": "test",
        "items": [{"query": "q", "expected_answer": "a", "expected_sources": []}],
        "created_at": "2026-04-16T00:00:00Z",
    }
    mock_db.get_evaluation.return_value = _baseline_run(collection="release-notes")
    mock_db.create_evaluation.return_value = "eval-new"
    mock_get_db.return_value = mock_db

    response = client.post(
        "/evaluations",
        json={
            "dataset_id": "ds-123",
            "collection": "release-notes",
            "baseline_eval_id": "eval-prev",
        },
    )

    assert response.status_code == 202
    mock_db.create_evaluation.assert_awaited_once_with(
        dataset_id="ds-123",
        collection="release-notes",
        notes=None,
        baseline_eval_id="eval-prev",
    )
```

- [ ] **Step 9: Run the new/changed tests and verify they fail**

Run:

```bash
PYTHONPATH=services/eval pytest services/eval/tests/test_main.py \
  -k "baseline or omits_optional_fields or dataset_not_found" -q
```

Expected: at least the new invalid-baseline tests fail because the endpoint does not validate `baseline_eval_id` yet.

- [ ] **Step 10: Commit the failing tests**

```bash
git add services/eval/tests/test_main.py
git commit -m "test: cover eval baseline validation"
```

## Task 2: Implement API-Layer Baseline Validation

**Files:**
- Modify: `services/eval/app/main.py`

- [ ] **Step 1: Add the private validation helper above `start_evaluation`**

Add this function after `_run_evaluation_task`:

```python
async def _validate_baseline(
    db: EvalDB,
    baseline_eval_id: str,
    dataset_id: str,
    collection: str,
) -> None:
    baseline = await db.get_evaluation(baseline_eval_id)
    if not baseline:
        raise HTTPException(status_code=404, detail="Baseline evaluation not found")

    if baseline["status"] != "completed":
        raise HTTPException(
            status_code=400,
            detail="Baseline evaluation must be completed",
        )

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

- [ ] **Step 2: Update `start_evaluation` to compute collection once and validate before creating**

Replace the body section after dataset lookup with:

```python
    collection = body.collection or "documents"

    if body.baseline_eval_id is not None:
        await _validate_baseline(
            db=db,
            baseline_eval_id=body.baseline_eval_id,
            dataset_id=body.dataset_id,
            collection=collection,
        )

    eval_id = await db.create_evaluation(
        dataset_id=body.dataset_id,
        collection=collection,
        notes=body.notes,
        baseline_eval_id=body.baseline_eval_id,
    )
```

Leave the existing `background_tasks.add_task(...)` call in place:

```python
    background_tasks.add_task(
        _run_evaluation_task, eval_id, dataset["items"], body.collection, body.rerank
    )
```

- [ ] **Step 3: Run the targeted API tests**

Run:

```bash
PYTHONPATH=services/eval pytest services/eval/tests/test_main.py \
  -k "baseline or omits_optional_fields or dataset_not_found" -q
```

Expected: all selected tests pass.

- [ ] **Step 4: Run all eval service tests**

Run:

```bash
PYTHONPATH=services/eval pytest services/eval/tests -q
```

Expected: all eval service tests pass.

- [ ] **Step 5: Commit the implementation**

```bash
git add services/eval/app/main.py
git commit -m "feat: validate eval baseline links"
```

## Task 3: Run Required Preflights

**Files:**
- No file changes expected unless tools auto-format.

- [ ] **Step 1: Run Python preflight**

Run:

```bash
make preflight-python
```

Expected: command exits 0. If ruff or formatting changes files, inspect the diff and commit those changes with:

```bash
git add services/eval/app/main.py services/eval/tests/test_main.py
git commit -m "style: format eval baseline validation"
```

- [ ] **Step 2: Run security preflight**

Run:

```bash
make preflight-security
```

Expected: command exits 0.

- [ ] **Step 3: Verify final git state**

Run:

```bash
git status --short
git log --oneline -3
```

Expected: working tree is clean except for unrelated user changes that predated execution, and the recent commits include the test and implementation commits.
