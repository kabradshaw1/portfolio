# Eval Chat Runtime Metadata Hardening Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make Python eval runs fail fast for missing retrieval collections, record per-run rerank and collection metadata, keep readiness stable under downstream chat slowness, and reject comparisons of non-completed runs.

**Architecture:** Add eval-side helpers for collection validation and local health readiness, extend config capture with request metadata, and tighten compare validation in `services/eval/app/main.py`. Keep changes inside `services/eval` unless tests reveal a required compatibility adjustment.

**Tech Stack:** Python, FastAPI, httpx, pytest, respx, existing eval SQLite abstraction.

---

## File Structure

- Modify `services/eval/app/config_capture.py`: include `requested_rerank` and `effective_collection` in returned config.
- Modify `services/eval/tests/test_config_capture.py`: test new metadata and existing warning behavior.
- Create `services/eval/app/collection_validation.py`: validate collection existence through ingestion `/collections`.
- Create `services/eval/tests/test_collection_validation.py`: success, missing collection, and dependency failure tests.
- Modify `services/eval/app/main.py`: call collection validation before run creation, pass effective collection to background task, adjust health, reject incomplete comparisons.
- Modify `services/eval/tests/test_main.py`: route-level tests for startup validation, config metadata, health, and compare behavior.

### Task 1: Config Capture Metadata

**Files:**
- Modify: `services/eval/app/config_capture.py`
- Modify: `services/eval/tests/test_config_capture.py`

- [ ] **Step 1: Write failing config capture metadata test**

In `services/eval/tests/test_config_capture.py`, update `test_capture_merges_chat_and_collection` call:

```python
cfg = await capture_run_config(
    chat_url="http://chat",
    ingestion_url="http://ingestion",
    collection="documents",
    requested_rerank=True,
)
```

Add assertions:

```python
assert cfg["requested_rerank"] is True
assert cfg["effective_collection"] == "documents"
```

Add a second test:

```python
@pytest.mark.asyncio
@respx.mock
async def test_capture_records_baseline_rerank_intent():
    respx.get("http://chat/config").mock(
        return_value=httpx.Response(200, json={"rerank_enabled": True})
    )
    respx.get("http://ingestion/collections/documents/config").mock(
        return_value=httpx.Response(404, json={"detail": "not found"})
    )

    cfg = await capture_run_config(
        chat_url="http://chat",
        ingestion_url="http://ingestion",
        collection="documents",
        requested_rerank=False,
    )

    assert cfg["requested_rerank"] is False
    assert cfg["effective_collection"] == "documents"
    assert cfg["chat"]["rerank_enabled"] is True
    assert "_capture_error" in cfg
```

- [ ] **Step 2: Run config capture tests and verify they fail**

Run:

```bash
pytest services/eval/tests/test_config_capture.py -q
```

Expected: FAIL because `capture_run_config` has no `requested_rerank` parameter.

- [ ] **Step 3: Update config capture signature**

In `services/eval/app/config_capture.py`, change:

```python
async def capture_run_config(
    chat_url: str,
    ingestion_url: str,
    collection: str,
    requested_rerank: bool,
) -> dict:
```

Initialize output as:

```python
out: dict = {
    "captured_at": captured_at,
    "effective_collection": collection,
    "requested_rerank": requested_rerank,
}
```

- [ ] **Step 4: Update existing tests to pass requested_rerank**

Every existing `capture_run_config(...)` call in `services/eval/tests/test_config_capture.py` should pass `requested_rerank=False` unless the test is specifically about rerank.

- [ ] **Step 5: Run config capture tests**

Run:

```bash
pytest services/eval/tests/test_config_capture.py -q
```

Expected: PASS.

- [ ] **Step 6: Commit config capture metadata**

```bash
git add services/eval/app/config_capture.py services/eval/tests/test_config_capture.py
git commit -m "feat: capture eval run rerank metadata"
```

### Task 2: Collection Validation Helper

**Files:**
- Create: `services/eval/app/collection_validation.py`
- Create: `services/eval/tests/test_collection_validation.py`

- [ ] **Step 1: Write failing helper tests**

Create `services/eval/tests/test_collection_validation.py`:

```python
import httpx
import pytest
import respx
from fastapi import HTTPException

from app.collection_validation import validate_collection_exists


@pytest.mark.asyncio
@respx.mock
async def test_validate_collection_exists_accepts_existing_collection():
    respx.get("http://ingestion/collections").mock(
        return_value=httpx.Response(
            200,
            json={"collections": [{"name": "documents", "points_count": 15}]},
        )
    )

    await validate_collection_exists("http://ingestion", "documents")


@pytest.mark.asyncio
@respx.mock
async def test_validate_collection_exists_rejects_missing_collection():
    respx.get("http://ingestion/collections").mock(
        return_value=httpx.Response(
            200,
            json={"collections": [{"name": "documents", "points_count": 15}]},
        )
    )

    with pytest.raises(HTTPException) as exc:
        await validate_collection_exists("http://ingestion", "missing")

    assert exc.value.status_code == 422
    assert 'retrieval collection "missing" does not exist' in exc.value.detail


@pytest.mark.asyncio
@respx.mock
async def test_validate_collection_exists_reports_dependency_failure():
    respx.get("http://ingestion/collections").mock(
        side_effect=httpx.ConnectError("boom")
    )

    with pytest.raises(HTTPException) as exc:
        await validate_collection_exists("http://ingestion", "documents")

    assert exc.value.status_code == 503
    assert "unable to validate retrieval collection" in exc.value.detail
```

- [ ] **Step 2: Run helper tests and verify they fail**

Run:

```bash
pytest services/eval/tests/test_collection_validation.py -q
```

Expected: FAIL because `app.collection_validation` does not exist.

- [ ] **Step 3: Implement helper**

Create `services/eval/app/collection_validation.py`:

```python
from __future__ import annotations

import httpx
from fastapi import HTTPException


async def validate_collection_exists(ingestion_url: str, collection: str) -> None:
    """Raise HTTPException if the requested Qdrant collection is unavailable."""
    try:
        async with httpx.AsyncClient() as client:
            resp = await client.get(f"{ingestion_url}/collections", timeout=5.0)
            resp.raise_for_status()
    except httpx.HTTPError as exc:
        raise HTTPException(
            status_code=503,
            detail=f"unable to validate retrieval collection {collection!r}: {exc}",
        ) from exc

    collections = resp.json().get("collections", [])
    names = {item.get("name") for item in collections if isinstance(item, dict)}
    if collection not in names:
        raise HTTPException(
            status_code=422,
            detail=f'retrieval collection "{collection}" does not exist',
        )
```

- [ ] **Step 4: Run helper tests**

Run:

```bash
pytest services/eval/tests/test_collection_validation.py -q
```

Expected: PASS.

- [ ] **Step 5: Commit helper**

```bash
git add services/eval/app/collection_validation.py services/eval/tests/test_collection_validation.py
git commit -m "feat: validate eval retrieval collections"
```

### Task 3: Evaluation Startup Validation

**Files:**
- Modify: `services/eval/app/main.py`
- Modify: `services/eval/tests/test_main.py`

- [ ] **Step 1: Add failing route tests for missing collection**

In `services/eval/tests/test_main.py`, add:

```python
@patch("app.main.validate_collection_exists", new_callable=AsyncMock)
@patch("app.main.get_db")
def test_start_evaluation_rejects_missing_collection_before_create(
    mock_get_db, mock_validate_collection
):
    mock_db = AsyncMock()
    mock_db.get_dataset.return_value = {
        "id": "ds-123",
        "name": "test",
        "items": [{"query": "q", "expected_answer": "a", "expected_sources": []}],
        "created_at": "2026-04-16T00:00:00Z",
    }
    mock_get_db.return_value = mock_db
    mock_validate_collection.side_effect = HTTPException(
        status_code=422,
        detail='retrieval collection "missing" does not exist',
    )

    response = client.post(
        "/evaluations",
        json={"dataset_id": "ds-123", "collection": "missing"},
    )

    assert response.status_code == 422
    assert response.json()["detail"] == 'retrieval collection "missing" does not exist'
    mock_db.create_evaluation.assert_not_awaited()
```

Ensure `HTTPException` is imported from `fastapi`.

- [ ] **Step 2: Add failing test for default effective collection**

Update `test_start_evaluation_omits_optional_fields` with `@patch("app.main.validate_collection_exists", new_callable=AsyncMock)` and assert:

```python
mock_validate_collection.assert_awaited_once_with(
    settings.ingestion_service_url, "documents"
)
```

Import `settings` from `app.config` if not already imported.

- [ ] **Step 3: Run startup tests and verify they fail**

Run:

```bash
pytest services/eval/tests/test_main.py -q -k "collection_before_create or omits_optional_fields"
```

Expected: FAIL because `validate_collection_exists` is not imported or called.

- [ ] **Step 4: Wire validation into start_evaluation**

In `services/eval/app/main.py`, add import:

```python
from app.collection_validation import validate_collection_exists
```

In `start_evaluation`, after baseline and experiment consistency checks:

```python
    collection = body.collection or "documents"
```

is already present. Add before `db.create_evaluation`:

```python
    await validate_collection_exists(settings.ingestion_service_url, collection)
```

Update background task call:

```python
    background_tasks.add_task(
        _run_evaluation_task, eval_id, dataset["items"], collection, body.rerank
    )
```

This replaces `body.collection` so defaulting is consistent.

- [ ] **Step 5: Update tests that start evaluations**

Any `services/eval/tests/test_main.py` route test that posts to `/evaluations` and is not testing validation should patch:

```python
@patch("app.main.validate_collection_exists", new_callable=AsyncMock)
```

or configure it to return successfully. This prevents tests from making real HTTP calls.

- [ ] **Step 6: Run focused startup tests**

Run:

```bash
pytest services/eval/tests/test_main.py -q -k "start_evaluation"
```

Expected: PASS.

- [ ] **Step 7: Commit startup validation**

```bash
git add services/eval/app/main.py services/eval/tests/test_main.py
git commit -m "feat: fail fast for missing eval collections"
```

### Task 4: Pass Metadata Through Evaluation Task

**Files:**
- Modify: `services/eval/app/main.py`
- Modify: `services/eval/tests/test_main.py`

- [ ] **Step 1: Add failing test for rerank metadata passed to capture**

In `test_start_evaluation_passes_rerank_to_background_run`, assert:

```python
assert mock_capture.await_args.kwargs["requested_rerank"] is True
assert mock_capture.await_args.kwargs["collection"] == "documents"
```

Ensure the test patches `validate_collection_exists`.

- [ ] **Step 2: Add failing test for baseline metadata**

Add:

```python
@patch("app.main.validate_collection_exists", new_callable=AsyncMock)
@patch("app.main.run_evaluation", new_callable=AsyncMock)
@patch("app.main.capture_run_config", new_callable=AsyncMock)
@patch("app.main.get_db")
def test_start_evaluation_records_baseline_rerank_metadata(
    mock_get_db, mock_capture, mock_run_evaluation, mock_validate_collection
):
    mock_db = AsyncMock()
    mock_db.get_dataset.return_value = {
        "id": "ds-base",
        "name": "test",
        "items": [{"query": "q", "expected_answer": "a", "expected_sources": []}],
        "created_at": "2026-04-16T00:00:00Z",
    }
    mock_db.create_evaluation.return_value = "eval-base"
    mock_get_db.return_value = mock_db
    mock_capture.return_value = {"captured_at": "x", "requested_rerank": False}
    mock_run_evaluation.return_value = ({"faithfulness": 0.7}, [])

    response = client.post("/evaluations", json={"dataset_id": "ds-base"})

    assert response.status_code == 202
    assert mock_capture.await_args.kwargs["requested_rerank"] is False
    mock_db.set_evaluation_config.assert_awaited_once_with(
        "eval-base", mock_capture.return_value
    )
```

- [ ] **Step 3: Run metadata tests and verify they fail**

Run:

```bash
pytest services/eval/tests/test_main.py -q -k "rerank_metadata or passes_rerank"
```

Expected: FAIL because `_run_evaluation_task` does not pass `requested_rerank`.

- [ ] **Step 4: Update `_run_evaluation_task` capture call**

In `services/eval/app/main.py`, inside `_run_evaluation_task`, update:

```python
        cfg = await capture_run_config(
            chat_url=settings.chat_service_url,
            ingestion_url=settings.ingestion_service_url,
            collection=coll_name,
            requested_rerank=rerank,
        )
```

- [ ] **Step 5: Run metadata tests**

Run:

```bash
pytest services/eval/tests/test_main.py services/eval/tests/test_config_capture.py -q -k "rerank or config"
```

Expected: PASS.

- [ ] **Step 6: Commit metadata wiring**

```bash
git add services/eval/app/main.py services/eval/tests/test_main.py
git commit -m "feat: persist eval rerank request metadata"
```

### Task 5: Health Readiness Behavior

**Files:**
- Modify: `services/eval/app/main.py`
- Modify: `services/eval/tests/test_main.py`

- [ ] **Step 1: Add failing health test**

In `services/eval/tests/test_main.py`, add:

```python
@patch("app.main.httpx.AsyncClient")
def test_health_returns_200_when_chat_degraded(mock_client_cls):
    mock_client = AsyncMock()
    mock_client.__aenter__.return_value = mock_client
    mock_client.get.side_effect = httpx.ConnectError("boom")
    mock_client_cls.return_value = mock_client

    response = client.get("/health")

    assert response.status_code == 200
    body = response.json()
    assert body["status"] == "healthy"
    assert body["chat_service"] == "unreachable"
```

Ensure `httpx` is imported in the test file.

- [ ] **Step 2: Run health test and verify it fails**

Run:

```bash
pytest services/eval/tests/test_main.py -q -k "health_returns_200"
```

Expected: FAIL because `/health` currently returns `503` when chat is unreachable.

- [ ] **Step 3: Update health route**

In `services/eval/app/main.py`, change the response status logic to:

```python
    return JSONResponse(
        status_code=200,
        content={"status": "healthy", "chat_service": "ok" if chat_ok else "unreachable"},
    )
```

Keep the short chat health timeout.

- [ ] **Step 4: Run health tests**

Run:

```bash
pytest services/eval/tests/test_main.py -q -k "health"
```

Expected: PASS.

- [ ] **Step 5: Commit health readiness change**

```bash
git add services/eval/app/main.py services/eval/tests/test_main.py
git commit -m "fix: decouple eval readiness from chat health"
```

### Task 6: Completed-Only Compare

**Files:**
- Modify: `services/eval/app/main.py`
- Modify: `services/eval/tests/test_main.py`

- [ ] **Step 1: Add failing compare tests**

In `services/eval/tests/test_main.py`, add:

```python
@patch("app.main.get_db")
def test_compare_rejects_running_runs(mock_get_db):
    mock_db = AsyncMock()
    mock_db.get_evaluations_by_ids.return_value = [
        _stub_run("base", "ds-1", {"faithfulness": 0.8}) | {"status": "completed"},
        _stub_run("candidate", "ds-1", None) | {"status": "running"},
    ]
    mock_get_db.return_value = mock_db

    response = client.get("/evaluations/compare?ids=base,candidate")

    assert response.status_code == 400
    assert "candidate=running" in response.json()["detail"]


@patch("app.main.get_db")
def test_compare_rejects_failed_runs(mock_get_db):
    mock_db = AsyncMock()
    mock_db.get_evaluations_by_ids.return_value = [
        _stub_run("base", "ds-1", {"faithfulness": 0.8}) | {"status": "completed"},
        _stub_run("candidate", "ds-1", None) | {"status": "failed"},
    ]
    mock_get_db.return_value = mock_db

    response = client.get("/evaluations/compare?ids=base,candidate")

    assert response.status_code == 400
    assert "candidate=failed" in response.json()["detail"]
```

If `_stub_run` already includes `status`, create explicit dicts instead of using `|`.

- [ ] **Step 2: Run compare tests and verify they fail**

Run:

```bash
pytest services/eval/tests/test_main.py -q -k "compare_rejects"
```

Expected: FAIL because compare currently accepts non-completed runs.

- [ ] **Step 3: Update compare route**

In `services/eval/app/main.py`, after same-dataset validation:

```python
    invalid_statuses = [
        f"{r['id']}={r.get('status')}" for r in runs if r.get("status") != "completed"
    ]
    if invalid_statuses:
        raise HTTPException(
            status_code=400,
            detail=(
                "compare requires completed runs; invalid statuses: "
                + ", ".join(invalid_statuses)
            ),
        )
```

Keep existing delta calculation unchanged after this guard.

- [ ] **Step 4: Run compare tests**

Run:

```bash
pytest services/eval/tests/test_main.py -q -k "compare"
```

Expected: PASS.

- [ ] **Step 5: Commit compare guard**

```bash
git add services/eval/app/main.py services/eval/tests/test_main.py
git commit -m "fix: reject incomplete eval comparisons"
```

### Task 7: Full Python Verification

**Files:**
- No planned source edits unless verification fails.

- [ ] **Step 1: Run eval test suite**

Run:

```bash
pytest services/eval/tests -q
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

- [ ] **Step 4: Fix any failures with focused commits**

If tests fail, make the smallest source/test correction and commit:

```bash
git add services/eval/app services/eval/tests
git commit -m "fix: stabilize eval runtime hardening"
```

- [ ] **Step 5: Push and open PR**

Use a feature branch/worktree for implementation. Push the branch and create a PR to `qa`.

