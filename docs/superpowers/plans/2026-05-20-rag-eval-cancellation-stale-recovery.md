# RAG Eval Cancellation And Stale Recovery Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add explicit cancellation and repair-oriented stale recovery for RabbitMQ-backed RAG eval item work.

**Architecture:** `services/eval` remains the source of truth. The API records cancellation and exposes richer evidence, the worker treats cancellation as a normal terminal item outcome, recovery republish/reset logic preserves retryable item work, and `go/eval-mcp-service` converts the richer API response into operator guidance.

**Tech Stack:** Python 3.11, FastAPI, aiosqlite, aio-pika/RabbitMQ, pytest/pytest-asyncio, Go eval MCP service tests.

---

## File Map

- Modify `services/eval/app/db.py`: add item cancellation helpers, run cancellation helper, richer item evidence counts, recovery query helpers, and terminal repair safeguards.
- Modify `services/eval/app/main.py`: add cancel endpoint, richer `item_counts`, and recovery republish/finalization flow.
- Modify `services/eval/app/evaluator.py`: add optional cancellation check hook before search, chat, and judge stages.
- Modify `services/eval/app/worker.py`: check parent run cancellation before claim, after claim, and handle cancellation from `evaluate_item`.
- Modify `services/eval/tests/test_db.py`: cover cancellation state transitions, expired lease recovery, queued republish candidates, evidence counts, and cancelled finalization guard.
- Modify `services/eval/tests/test_main.py`: cover cancel API, richer evidence response, and startup recovery publisher calls.
- Modify `services/eval/tests/test_evaluator.py`: cover cancellation hook before expensive stages.
- Modify `services/eval/tests/test_worker.py`: cover cancelled run ack behavior and no retry/DLQ path.
- Modify `go/eval-mcp-service/internal/evalworkflow/service.go`: update evidence next steps for cancelled, stale retryable, and stale exhausted cases.
- Modify `go/eval-mcp-service/internal/evalworkflow/service_test.go`: cover the new evidence guidance.

## Task 1: Eval DB Cancellation State

**Files:**
- Modify: `services/eval/app/db.py`
- Test: `services/eval/tests/test_db.py`

- [ ] **Step 1: Write failing DB tests for cancellation helpers**

Add these tests after `test_claim_evaluation_item_returns_none_for_completed_item` in `services/eval/tests/test_db.py`:

```python
@pytest.mark.asyncio
async def test_cancel_evaluation_marks_run_and_non_terminal_items_cancelled(db):
    items = [
        {"query": "q1", "expected_answer": "a1", "expected_sources": []},
        {"query": "q2", "expected_answer": "a2", "expected_sources": []},
    ]
    ds_id = await db.create_dataset(name="ds-cancel", items=items)
    eval_id = await db.create_evaluation(ds_id, "documents", status="running")
    first, second = await db.create_evaluation_items(eval_id, items, max_attempts=3)
    await db.mark_evaluation_item_completed(
        first["id"],
        result={"query": "q1", "answer": "a1", "contexts": []},
        scores={"faithfulness": 1.0},
        score_reasons={"faithfulness": "ok"},
    )
    await db.claim_evaluation_item(second["id"], worker_id="worker-1", lease_seconds=60)

    cancelled = await db.cancel_evaluation(eval_id)

    assert cancelled is not None
    assert cancelled["status"] == "cancelled"
    assert cancelled["error"] == "evaluation cancelled by operator"
    stored = await db.list_evaluation_items(eval_id)
    assert stored[0]["status"] == "completed"
    assert stored[1]["status"] == "cancelled"
    assert stored[1]["lease_owner"] is None
    assert stored[1]["lease_expires_at"] is None


@pytest.mark.asyncio
async def test_cancel_evaluation_returns_none_for_terminal_run(db):
    ds_id = await db.create_dataset(name="ds-cancel-terminal", items=SIMPLE_ITEM)
    eval_id = await db.create_evaluation(ds_id, "documents", status="running")
    await db.complete_evaluation(eval_id, aggregate_scores={}, results=[])

    cancelled = await db.cancel_evaluation(eval_id)

    assert cancelled is None
    evaluation = await db.get_evaluation(eval_id)
    assert evaluation["status"] == "completed"


@pytest.mark.asyncio
async def test_mark_evaluation_item_cancelled_clears_lease(db):
    ds_id = await db.create_dataset(name="ds-cancel-item", items=SIMPLE_ITEM)
    eval_id = await db.create_evaluation(ds_id, "documents", status="running")
    [item] = await db.create_evaluation_items(eval_id, SIMPLE_ITEM, max_attempts=3)
    await db.claim_evaluation_item(item["id"], worker_id="worker-1", lease_seconds=60)

    await db.mark_evaluation_item_cancelled(item["id"])

    stored = await db.get_evaluation_item(item["id"])
    assert stored["status"] == "cancelled"
    assert stored["lease_owner"] is None
    assert stored["lease_expires_at"] is None
    assert stored["completed_at"] is not None
```

- [ ] **Step 2: Run the new DB tests and verify they fail**

Run:

```bash
PYTHONPATH=services pytest \
  services/eval/tests/test_db.py::test_cancel_evaluation_marks_run_and_non_terminal_items_cancelled \
  services/eval/tests/test_db.py::test_cancel_evaluation_returns_none_for_terminal_run \
  services/eval/tests/test_db.py::test_mark_evaluation_item_cancelled_clears_lease \
  -v
```

Expected: fail with missing `cancel_evaluation` and `mark_evaluation_item_cancelled`.

- [ ] **Step 3: Add cancellation helpers**

In `services/eval/app/db.py`, add these methods after `mark_evaluation_item_failed`:

```python
    async def mark_evaluation_item_cancelled(self, item_id: str) -> None:
        now = datetime.now(_UTC).isoformat()
        await self._db.execute(
            "UPDATE evaluation_items "
            "SET status = 'cancelled', lease_owner = NULL, lease_expires_at = NULL, "
            "completed_at = ?, updated_at = ? "
            "WHERE id = ? AND status NOT IN ('completed', 'failed', 'cancelled')",
            (now, now, item_id),
        )
        await self._db.commit()

    async def cancel_evaluation(self, eval_id: str) -> dict | None:
        now = datetime.now(_UTC).isoformat()
        cursor = await self._db.execute(
            "UPDATE evaluations "
            "SET status = 'cancelled', error = ?, completed_at = ? "
            "WHERE id = ? AND status IN ('queued', 'running')",
            ("evaluation cancelled by operator", now, eval_id),
        )
        if cursor.rowcount == 0:
            await self._db.commit()
            return None
        await self._db.execute(
            "UPDATE evaluation_items "
            "SET status = 'cancelled', lease_owner = NULL, lease_expires_at = NULL, "
            "completed_at = COALESCE(completed_at, ?), updated_at = ? "
            "WHERE evaluation_id = ? "
            "AND status NOT IN ('completed', 'failed', 'cancelled')",
            (now, now, eval_id),
        )
        await self._db.commit()
        return await self.get_evaluation(eval_id)
```

- [ ] **Step 4: Protect aggregation from overwriting cancelled runs**

In `finalize_evaluation_if_terminal`, change both update guards so `cancelled` is terminal:

```python
                "completed_at = ? WHERE id = ? AND status NOT IN "
                "('completed', 'completed_with_failures', 'failed', 'cancelled')",
```

and:

```python
                "WHERE id = ? AND status NOT IN ('failed', 'cancelled')",
```

- [ ] **Step 5: Run DB tests**

Run:

```bash
PYTHONPATH=services pytest services/eval/tests/test_db.py -v
```

Expected: all DB tests pass.

- [ ] **Step 6: Commit**

```bash
git add services/eval/app/db.py services/eval/tests/test_db.py
git commit -m "feat(eval): persist cancellation state"
```

## Task 2: Cancellation API And Item Counts

**Files:**
- Modify: `services/eval/app/main.py`
- Test: `services/eval/tests/test_main.py`

- [ ] **Step 1: Write failing API tests**

Add these tests after `test_get_evaluation_includes_item_summary` in `services/eval/tests/test_main.py`:

```python
@patch("app.main.get_db")
def test_get_evaluation_includes_cancelled_retryable_and_stale_counts(mock_get_db):
    mock_db = AsyncMock()
    mock_db.get_evaluation.return_value = {
        "id": "eval-456",
        "dataset_id": "ds-123",
        "status": "running",
        "collection": "documents",
        "aggregate_scores": None,
        "results": None,
        "error": None,
        "created_at": "2026-04-16T00:00:00Z",
        "completed_at": None,
        "notes": None,
        "config": None,
        "baseline_eval_id": None,
    }
    mock_db.count_evaluation_items_by_status.return_value = {
        "queued": 1,
        "running": 1,
        "completed": 2,
        "failed": 1,
        "cancelled": 1,
        "retryable": 2,
        "stale": 1,
    }
    mock_get_db.return_value = mock_db

    response = client.get("/evaluations/eval-456")

    assert response.status_code == 200
    assert response.json()["item_counts"] == {
        "queued": 1,
        "running": 1,
        "completed": 2,
        "failed": 1,
        "cancelled": 1,
        "retryable": 2,
        "stale": 1,
        "total": 6,
    }


@patch("app.main.get_db")
def test_cancel_evaluation_marks_run_cancelled(mock_get_db):
    mock_db = AsyncMock()
    mock_db.get_evaluation.return_value = _baseline_run(status="running")
    mock_db.cancel_evaluation.return_value = _baseline_run(status="cancelled") | {
        "error": "evaluation cancelled by operator",
        "completed_at": "2026-04-16T00:05:00Z",
    }
    mock_db.count_evaluation_items_by_status.return_value = {
        "queued": 0,
        "running": 0,
        "completed": 1,
        "failed": 0,
        "cancelled": 1,
        "retryable": 0,
        "stale": 0,
    }
    mock_get_db.return_value = mock_db

    response = client.post("/evaluations/eval-prev/cancel")

    assert response.status_code == 200
    assert response.json()["status"] == "cancelled"
    assert response.json()["item_counts"]["cancelled"] == 1
    mock_db.cancel_evaluation.assert_awaited_once_with("eval-prev")


@patch("app.main.get_db")
def test_cancel_evaluation_rejects_terminal_run(mock_get_db):
    mock_db = AsyncMock()
    mock_db.get_evaluation.return_value = _baseline_run(status="completed")
    mock_get_db.return_value = mock_db

    response = client.post("/evaluations/eval-prev/cancel")

    assert response.status_code == 409
    assert response.json()["detail"] == "evaluation is already terminal"
    mock_db.cancel_evaluation.assert_not_awaited()
```

- [ ] **Step 2: Run the new API tests and verify they fail**

Run:

```bash
PYTHONPATH=services pytest \
  services/eval/tests/test_main.py::test_get_evaluation_includes_cancelled_retryable_and_stale_counts \
  services/eval/tests/test_main.py::test_cancel_evaluation_marks_run_cancelled \
  services/eval/tests/test_main.py::test_cancel_evaluation_rejects_terminal_run \
  -v
```

Expected: fail because the route and richer counts do not exist.

- [ ] **Step 3: Add shared item count builder**

In `services/eval/app/main.py`, add this helper above `get_evaluation`:

```python
TERMINAL_EVALUATION_STATUSES = {
    "completed",
    "completed_with_failures",
    "failed",
    "cancelled",
}


def _item_counts_response(counts: dict) -> dict:
    queued = counts.get("queued", 0)
    running = counts.get("running", 0)
    completed = counts.get("completed", 0)
    failed = counts.get("failed", 0)
    cancelled = counts.get("cancelled", 0)
    return {
        "queued": queued,
        "running": running,
        "completed": completed,
        "failed": failed,
        "cancelled": cancelled,
        "retryable": counts.get("retryable", 0),
        "stale": counts.get("stale", 0),
        "total": queued + running + completed + failed + cancelled,
    }


async def _attach_item_counts(db: EvalDB, evaluation: dict) -> dict:
    counts = await db.count_evaluation_items_by_status(
        evaluation["id"], stale_seconds=settings.eval_stale_item_seconds
    )
    if isinstance(counts, dict) and counts:
        item_counts = _item_counts_response(counts)
        evaluation["item_counts"] = item_counts
        evaluation["item_summary"] = item_counts
    return evaluation
```

- [ ] **Step 4: Update `get_evaluation` to use the helper**

Replace the count-building body in `get_evaluation` with:

```python
    evaluation = await _attach_item_counts(db, evaluation)
    return evaluation
```

- [ ] **Step 5: Add cancellation endpoint**

Add this route immediately before `get_evaluation`:

```python
@app.post("/evaluations/{eval_id}/cancel", dependencies=[Depends(enforce_eval_write)])
async def cancel_evaluation(request: Request, eval_id: str):
    db = await get_db()
    evaluation = await db.get_evaluation(eval_id)
    if not evaluation:
        raise HTTPException(status_code=404, detail="Evaluation not found")
    if evaluation["status"] in TERMINAL_EVALUATION_STATUSES:
        raise HTTPException(status_code=409, detail="evaluation is already terminal")
    cancelled = await db.cancel_evaluation(eval_id)
    if cancelled is None:
        raise HTTPException(status_code=409, detail="evaluation is already terminal")
    return await _attach_item_counts(db, cancelled)
```

- [ ] **Step 6: Run API tests**

Run:

```bash
PYTHONPATH=services pytest services/eval/tests/test_main.py -v
```

Expected: all eval API tests pass.

- [ ] **Step 7: Commit**

```bash
git add services/eval/app/main.py services/eval/tests/test_main.py
git commit -m "feat(eval): add cancellation endpoint"
```

## Task 3: DB Recovery Evidence And Republish Queries

**Files:**
- Modify: `services/eval/app/db.py`
- Test: `services/eval/tests/test_db.py`

- [ ] **Step 1: Write failing DB recovery tests**

Add these tests near the existing expired item test in `services/eval/tests/test_db.py`:

```python
@pytest.mark.asyncio
async def test_reset_expired_running_items_returns_republishable_items(db):
    ds_id = await db.create_dataset(name="ds-expired-return", items=SIMPLE_ITEM)
    eval_id = await db.create_evaluation(ds_id, "documents", status="running")
    [item] = await db.create_evaluation_items(eval_id, SIMPLE_ITEM, max_attempts=3)
    await db.claim_evaluation_item(item["id"], worker_id="worker-1", lease_seconds=1)
    expired = (datetime.now(UTC) - timedelta(minutes=5)).isoformat()
    await db._db.execute(  # noqa: SLF001
        "UPDATE evaluation_items SET lease_expires_at = ? WHERE id = ?",
        (expired, item["id"]),
    )
    await db._db.commit()  # noqa: SLF001

    reset_items = await db.reset_expired_running_items(max_age_seconds=60)

    assert [item["id"] for item in reset_items] == [item["id"]]
    assert reset_items[0]["item_index"] == 0


@pytest.mark.asyncio
async def test_fail_expired_running_items_without_attempts_remaining(db):
    ds_id = await db.create_dataset(name="ds-expired-fail", items=SIMPLE_ITEM)
    eval_id = await db.create_evaluation(ds_id, "documents", status="running")
    [item] = await db.create_evaluation_items(eval_id, SIMPLE_ITEM, max_attempts=1)
    await db.claim_evaluation_item(item["id"], worker_id="worker-1", lease_seconds=1)
    expired = (datetime.now(UTC) - timedelta(minutes=5)).isoformat()
    await db._db.execute(  # noqa: SLF001
        "UPDATE evaluation_items SET lease_expires_at = ? WHERE id = ?",
        (expired, item["id"]),
    )
    await db._db.commit()  # noqa: SLF001

    failed_items = await db.fail_expired_running_items(max_age_seconds=60)

    assert [failed["id"] for failed in failed_items] == [item["id"]]
    stored = await db.get_evaluation_item(item["id"])
    assert stored["status"] == "failed"
    assert stored["last_error"]["error_type"] == "stale_item_lease"
    assert stored["last_error"]["retryable"] is False


@pytest.mark.asyncio
async def test_list_queued_items_for_republish_excludes_cancelled_runs(db):
    ds_id = await db.create_dataset(name="ds-republish", items=SIMPLE_ITEM)
    running_eval = await db.create_evaluation(ds_id, "documents", status="running")
    cancelled_eval = await db.create_evaluation(ds_id, "documents", status="cancelled")
    [running_item] = await db.create_evaluation_items(
        running_eval, SIMPLE_ITEM, max_attempts=3
    )
    await db.create_evaluation_items(cancelled_eval, SIMPLE_ITEM, max_attempts=3)

    queued = await db.list_queued_items_for_republish(max_age_seconds=0)

    assert [item["id"] for item in queued] == [running_item["id"]]


@pytest.mark.asyncio
async def test_count_evaluation_items_by_status_includes_retryable_and_stale(db):
    ds_id = await db.create_dataset(name="ds-count-rich", items=SIMPLE_ITEM)
    eval_id = await db.create_evaluation(ds_id, "documents", status="running")
    [item] = await db.create_evaluation_items(eval_id, SIMPLE_ITEM, max_attempts=3)
    old = (datetime.now(UTC) - timedelta(minutes=30)).isoformat()
    await db._db.execute(  # noqa: SLF001
        "UPDATE evaluation_items SET updated_at = ? WHERE id = ?",
        (old, item["id"]),
    )
    await db._db.commit()  # noqa: SLF001

    counts = await db.count_evaluation_items_by_status(eval_id)

    assert counts["queued"] == 1
    assert counts["retryable"] == 1
    assert counts["stale"] == 1
```

- [ ] **Step 2: Run the new recovery tests and verify they fail**

Run:

```bash
PYTHONPATH=services pytest \
  services/eval/tests/test_db.py::test_reset_expired_running_items_returns_republishable_items \
  services/eval/tests/test_db.py::test_fail_expired_running_items_without_attempts_remaining \
  services/eval/tests/test_db.py::test_list_queued_items_for_republish_excludes_cancelled_runs \
  services/eval/tests/test_db.py::test_count_evaluation_items_by_status_includes_retryable_and_stale \
  -v
```

Expected: fail because recovery helpers and richer counts are not implemented.

- [ ] **Step 3: Change `reset_expired_running_items` to return reset rows**

Replace `reset_expired_running_items` in `services/eval/app/db.py` with:

```python
    async def reset_expired_running_items(self, max_age_seconds: float) -> list[dict]:
        now = datetime.now(_UTC)
        cutoff = (now - timedelta(seconds=max_age_seconds)).isoformat()
        cursor = await self._db.execute(
            "SELECT * FROM evaluation_items "
            "WHERE status = 'running' AND lease_expires_at < ? "
            "AND attempt_count < max_attempts "
            "ORDER BY updated_at ASC",
            (cutoff,),
        )
        rows = await cursor.fetchall()
        items = [self._item_row_to_dict(row) for row in rows]
        await self._db.execute(
            "UPDATE evaluation_items "
            "SET status = 'queued', lease_owner = NULL, lease_expires_at = NULL, "
            "updated_at = ? "
            "WHERE status = 'running' AND lease_expires_at < ? "
            "AND attempt_count < max_attempts",
            (now.isoformat(), cutoff),
        )
        await self._db.commit()
        return items
```

- [ ] **Step 4: Add exhausted lease and queued republish helpers**

Add these methods after `reset_expired_running_items`:

```python
    async def fail_expired_running_items(self, max_age_seconds: float) -> list[dict]:
        now = datetime.now(_UTC)
        cutoff = (now - timedelta(seconds=max_age_seconds)).isoformat()
        cursor = await self._db.execute(
            "SELECT * FROM evaluation_items "
            "WHERE status = 'running' AND lease_expires_at < ? "
            "AND attempt_count >= max_attempts "
            "ORDER BY updated_at ASC",
            (cutoff,),
        )
        rows = await cursor.fetchall()
        items = [self._item_row_to_dict(row) for row in rows]
        error = json.dumps({"error_type": "stale_item_lease", "retryable": False})
        await self._db.execute(
            "UPDATE evaluation_items "
            "SET status = 'failed', last_error = ?, lease_owner = NULL, "
            "lease_expires_at = NULL, completed_at = ?, updated_at = ? "
            "WHERE status = 'running' AND lease_expires_at < ? "
            "AND attempt_count >= max_attempts",
            (error, now.isoformat(), now.isoformat(), cutoff),
        )
        await self._db.commit()
        return items

    async def list_queued_items_for_republish(
        self, max_age_seconds: float
    ) -> list[dict]:
        cutoff = (datetime.now(_UTC) - timedelta(seconds=max_age_seconds)).isoformat()
        cursor = await self._db.execute(
            "SELECT i.* FROM evaluation_items i "
            "JOIN evaluations e ON e.id = i.evaluation_id "
            "WHERE i.status = 'queued' AND i.updated_at < ? "
            "AND e.status NOT IN "
            "('completed', 'completed_with_failures', 'failed', 'cancelled') "
            "ORDER BY i.updated_at ASC",
            (cutoff,),
        )
        rows = await cursor.fetchall()
        return [self._item_row_to_dict(row) for row in rows]
```

- [ ] **Step 5: Enrich `count_evaluation_items_by_status`**

Replace `count_evaluation_items_by_status` with:

```python
    async def count_evaluation_items_by_status(
        self, eval_id: str, stale_seconds: float = 900.0
    ) -> dict[str, int]:
        cursor = await self._db.execute(
            "SELECT status, COUNT(*) AS count FROM evaluation_items "
            "WHERE evaluation_id = ? GROUP BY status",
            (eval_id,),
        )
        rows = await cursor.fetchall()
        counts = {row["status"]: row["count"] for row in rows}
        stale_cutoff = (
            datetime.now(_UTC) - timedelta(seconds=stale_seconds)
        ).isoformat()
        retryable_cursor = await self._db.execute(
            "SELECT COUNT(*) AS count FROM evaluation_items "
            "WHERE evaluation_id = ? "
            "AND status IN ('queued', 'running', 'failed') "
            "AND attempt_count < max_attempts",
            (eval_id,),
        )
        stale_cursor = await self._db.execute(
            "SELECT COUNT(*) AS count FROM evaluation_items "
            "WHERE evaluation_id = ? "
            "AND status IN ('queued', 'running') "
            "AND (updated_at < ? OR lease_expires_at < ?)",
            (eval_id, stale_cutoff, datetime.now(_UTC).isoformat()),
        )
        retryable_row = await retryable_cursor.fetchone()
        stale_row = await stale_cursor.fetchone()
        counts["retryable"] = int(retryable_row["count"])
        counts["stale"] = int(stale_row["count"])
        return counts
```

- [ ] **Step 6: Run DB tests**

Run:

```bash
PYTHONPATH=services pytest services/eval/tests/test_db.py -v
```

Expected: all DB tests pass. If an older test expects `reset_expired_running_items` to equal `1`, update it to assert `len(reset) == 1` and the stored item state.

- [ ] **Step 7: Commit**

```bash
git add services/eval/app/db.py services/eval/tests/test_db.py
git commit -m "feat(eval): repair stale item state"
```

## Task 4: Startup Recovery Republish Flow

**Files:**
- Modify: `services/eval/app/main.py`
- Test: `services/eval/tests/test_main.py`

- [ ] **Step 1: Write failing recovery tests**

Replace `test_recover_stale_evaluations_uses_max_runtime_plus_grace` in `services/eval/tests/test_main.py` with:

```python
@pytest.mark.asyncio
@patch("app.main.get_item_publisher")
@patch("app.main.get_db")
async def test_recover_stale_evaluations_republishes_retryable_items(
    mock_get_db, mock_get_item_publisher
):
    mock_db = AsyncMock()
    mock_db.reset_expired_running_items.return_value = [
        {"evaluation_id": "eval-1", "id": "item-1", "item_index": 0}
    ]
    mock_db.fail_expired_running_items.return_value = []
    mock_db.list_queued_items_for_republish.return_value = [
        {"evaluation_id": "eval-2", "id": "item-2", "item_index": 1}
    ]
    mock_db.count_stale_running_evaluations.return_value = 0
    mock_get_db.return_value = mock_db
    publisher = AsyncMock()
    mock_get_item_publisher.return_value = publisher

    await recover_stale_evaluations()

    assert publisher.publish.await_count == 2
    published = [call.args[0] for call in publisher.publish.await_args_list]
    assert published[0].evaluation_id == "eval-1"
    assert published[0].item_id == "item-1"
    assert published[1].evaluation_id == "eval-2"
    assert published[1].item_id == "item-2"
    mock_db.fail_stale_running_evaluations.assert_not_awaited()


@pytest.mark.asyncio
@patch("app.main.get_item_publisher")
@patch("app.main.get_db")
async def test_recover_stale_evaluations_finalizes_failed_expired_items(
    mock_get_db, mock_get_item_publisher
):
    mock_db = AsyncMock()
    mock_db.reset_expired_running_items.return_value = []
    mock_db.fail_expired_running_items.return_value = [
        {"evaluation_id": "eval-1", "id": "item-1", "item_index": 0}
    ]
    mock_db.list_queued_items_for_republish.return_value = []
    mock_db.count_stale_running_evaluations.return_value = 1
    mock_get_db.return_value = mock_db

    await recover_stale_evaluations()

    mock_db.finalize_evaluation_if_terminal.assert_awaited_once_with("eval-1")
    mock_get_item_publisher.assert_not_awaited()
```

- [ ] **Step 2: Run the recovery tests and verify they fail**

Run:

```bash
PYTHONPATH=services pytest \
  services/eval/tests/test_main.py::test_recover_stale_evaluations_republishes_retryable_items \
  services/eval/tests/test_main.py::test_recover_stale_evaluations_finalizes_failed_expired_items \
  -v
```

Expected: fail because `recover_stale_evaluations` still expects counts and fails stale runs.

- [ ] **Step 3: Add recovery publish helper**

In `services/eval/app/main.py`, add this helper after `publish_evaluation_items`:

```python
async def republish_evaluation_items(items: list[dict]) -> None:
    if not items:
        return
    publisher = await get_item_publisher()
    for item in items:
        await publisher.publish(
            EvalItemMessage(
                evaluation_id=item["evaluation_id"],
                item_id=item["id"],
                item_index=item["item_index"],
                attempt=item.get("attempt_count", 0) + 1,
            )
        )
        eval_queue_publish_total.labels(status="success").inc()
```

- [ ] **Step 4: Replace startup recovery logic**

Replace `recover_stale_evaluations` with:

```python
@app.on_event("startup")
async def recover_stale_evaluations():
    db = await get_db()
    reset_items = await db.reset_expired_running_items(settings.eval_stale_item_seconds)
    if reset_items:
        await republish_evaluation_items(reset_items)
        logger.warning(
            "Recovered %s expired running evaluation item(s)", len(reset_items)
        )
    failed_items = await db.fail_expired_running_items(settings.eval_stale_item_seconds)
    finalized_eval_ids = {item["evaluation_id"] for item in failed_items}
    for eval_id in finalized_eval_ids:
        await db.finalize_evaluation_if_terminal(eval_id)
    queued_items = await db.list_queued_items_for_republish(
        settings.eval_stale_item_seconds
    )
    if queued_items:
        await republish_evaluation_items(queued_items)
        logger.warning("Republished %s stale queued evaluation item(s)", len(queued_items))
    max_age = settings.eval_run_max_seconds + settings.eval_stale_grace_seconds
    stale_count = await db.count_stale_running_evaluations(max_age)
    eval_stale_running_runs.set(stale_count)
```

- [ ] **Step 5: Run API tests**

Run:

```bash
PYTHONPATH=services pytest services/eval/tests/test_main.py -v
```

Expected: all eval API tests pass.

- [ ] **Step 6: Commit**

```bash
git add services/eval/app/main.py services/eval/tests/test_main.py
git commit -m "feat(eval): republish stale item work"
```

## Task 5: Evaluator Cancellation Boundaries

**Files:**
- Modify: `services/eval/app/evaluator.py`
- Test: `services/eval/tests/test_evaluator.py`

- [ ] **Step 1: Write failing evaluator tests**

Add this test near the existing `evaluate_item` tests in `services/eval/tests/test_evaluator.py`:

```python
@pytest.mark.asyncio
async def test_evaluate_item_checks_cancellation_before_each_expensive_stage():
    events = []

    class FakeRAGClient:
        async def search(self, *args, **kwargs):
            events.append("search")
            return [{"text": "ctx"}]

        async def ask(self, *args, **kwargs):
            events.append("chat")
            return {"answer": "answer"}

    async def judge(row):
        events.append("judge")
        return JudgeScores(
            faithfulness=1.0,
            answer_relevancy=1.0,
            reasons={"faithfulness": "ok", "answer_relevancy": "ok"},
        )

    async def check_cancelled():
        events.append("check")

    await evaluate_item(
        item={"query": "q", "expected_answer": "a", "expected_sources": []},
        rag_client=FakeRAGClient(),
        collection="documents",
        rerank=False,
        top_k=5,
        judge=judge,
        run_context=None,
        answer_model=None,
        item_index=0,
        check_cancelled=check_cancelled,
    )

    assert events == ["check", "search", "check", "chat", "check", "judge"]
```

- [ ] **Step 2: Run the evaluator test and verify it fails**

Run:

```bash
PYTHONPATH=services pytest services/eval/tests/test_evaluator.py::test_evaluate_item_checks_cancellation_before_each_expensive_stage -v
```

Expected: fail because `evaluate_item` has no `check_cancelled` parameter.

- [ ] **Step 3: Add optional cancellation hook to `evaluate_item`**

In `services/eval/app/evaluator.py`, update the signature:

```python
async def evaluate_item(
    *,
    item: dict,
    rag_client: RAGClient,
    collection: str | None,
    rerank: bool,
    top_k: int,
    judge,
    run_context: EvalRunContext | None,
    answer_model: dict | None,
    item_index: int,
    check_cancelled=None,
) -> dict:
```

Then call the hook before each expensive stage:

```python
        if check_cancelled is not None:
            await check_cancelled()
        search_results = await rag_client.search(
            query, collection=collection, limit=top_k, rerank=rerank
        )
        if check_cancelled is not None:
            await check_cancelled()
        chat_response = await rag_client.ask(
            query,
            collection=collection,
            rerank=rerank,
            retrieval_config={"top_k": top_k},
            answer_model=answer_model,
        )
```

and immediately before judging:

```python
        if check_cancelled is not None:
            await check_cancelled()
        judge_scores = await judge(row)
```

- [ ] **Step 4: Run evaluator tests**

Run:

```bash
PYTHONPATH=services pytest services/eval/tests/test_evaluator.py -v
```

Expected: all evaluator tests pass.

- [ ] **Step 5: Commit**

```bash
git add services/eval/app/evaluator.py services/eval/tests/test_evaluator.py
git commit -m "feat(eval): check cancellation before item stages"
```

## Task 6: Worker Cancellation Handling

**Files:**
- Modify: `services/eval/app/worker.py`
- Test: `services/eval/tests/test_worker.py`

- [ ] **Step 1: Expand fake DB for cancellation tests**

Update `FakeDB.__init__` in `services/eval/tests/test_worker.py`:

```python
        self.evaluation = {
            "id": "eval-1",
            "status": "running",
            "collection": "documents",
            "config": {"effective_retrieval_config": {"top_k": 5}},
        }
        self.cancelled = None
        self.running = None
```

Replace `get_evaluation` with:

```python
    async def get_evaluation(self, eval_id):
        return self.evaluation | {"id": eval_id}
```

Add this method:

```python
    async def mark_evaluation_item_cancelled(self, item_id):
        self.cancelled = item_id
```

- [ ] **Step 2: Write failing worker cancellation tests**

Add these tests after `test_item_processor_completes_claimed_item`:

```python
@pytest.mark.asyncio
async def test_item_processor_acks_cancelled_run_without_claiming_item():
    db = FakeDB()
    db.evaluation["status"] = "cancelled"
    db.claimed = False

    async def claim_evaluation_item(item_id, worker_id, lease_seconds):
        db.claimed = True
        return db.item

    db.claim_evaluation_item = claim_evaluation_item

    async def evaluate_item(**kwargs):
        raise AssertionError("cancelled work must not be evaluated")

    processor = ItemProcessor(db=db, evaluate_item_fn=evaluate_item, worker_id="w1")

    await processor.process(
        EvalItemMessage(
            evaluation_id="eval-1",
            item_id="item-1",
            item_index=0,
            attempt=1,
        )
    )

    assert db.claimed is False
    assert db.cancelled == "item-1"
    assert db.failed is None


@pytest.mark.asyncio
async def test_item_processor_marks_item_cancelled_when_stage_check_observes_cancel():
    db = FakeDB()
    calls = 0

    async def get_evaluation(eval_id):
        nonlocal calls
        calls += 1
        status = "running" if calls <= 2 else "cancelled"
        return db.evaluation | {"id": eval_id, "status": status}

    db.get_evaluation = get_evaluation

    async def evaluate_item(**kwargs):
        await kwargs["check_cancelled"]()
        raise AssertionError("check_cancelled should raise first")

    processor = ItemProcessor(db=db, evaluate_item_fn=evaluate_item, worker_id="w1")

    await processor.process(
        EvalItemMessage(
            evaluation_id="eval-1",
            item_id="item-1",
            item_index=0,
            attempt=1,
        )
    )

    assert db.cancelled == "item-1"
    assert db.failed is None
    assert not hasattr(db, "released")
```

- [ ] **Step 3: Run worker tests and verify they fail**

Run:

```bash
PYTHONPATH=services pytest \
  services/eval/tests/test_worker.py::test_item_processor_acks_cancelled_run_without_claiming_item \
  services/eval/tests/test_worker.py::test_item_processor_marks_item_cancelled_when_stage_check_observes_cancel \
  -v
```

Expected: fail because worker cancellation behavior is not implemented.

- [ ] **Step 4: Add cancellation error and helper**

In `services/eval/app/worker.py`, add:

```python
class CancelledEvaluationError(RuntimeError):
    pass
```

Inside `ItemProcessor`, add:

```python
    async def _raise_if_cancelled(self, eval_id: str) -> None:
        evaluation = await self.db.get_evaluation(eval_id)
        if evaluation and evaluation.get("status") == "cancelled":
            raise CancelledEvaluationError(f"evaluation {eval_id} is cancelled")
```

- [ ] **Step 5: Check parent status before claim**

At the start of `ItemProcessor.process`, before `claim_evaluation_item`, add:

```python
        evaluation = await self.db.get_evaluation(message.evaluation_id)
        if evaluation is None:
            await self.db.mark_evaluation_item_failed(
                message.item_id,
                {"error_type": "missing_evaluation", "retryable": False},
            )
            eval_item_dlq_total.labels(reason="missing_evaluation").inc()
            return
        if evaluation.get("status") == "cancelled":
            await self.db.mark_evaluation_item_cancelled(message.item_id)
            eval_item_messages_total.labels(outcome="cancelled").inc()
            return
```

Remove the later missing-evaluation block after claim so missing evaluation is handled once.

- [ ] **Step 6: Re-check after claim and pass stage hook**

After a successful claim, re-read the parent run and add:

```python
        evaluation = await self.db.get_evaluation(message.evaluation_id)
        if evaluation.get("status") == "cancelled":
            await self.db.mark_evaluation_item_cancelled(message.item_id)
            eval_item_messages_total.labels(outcome="cancelled").inc()
            return
```

In the `evaluate_item_fn` call, add:

```python
                check_cancelled=lambda: self._raise_if_cancelled(
                    message.evaluation_id
                ),
```

- [ ] **Step 7: Handle cancellation as terminal, not retryable**

Add this `except` block before the generic `except Exception as exc`:

```python
        except CancelledEvaluationError:
            await self.db.mark_evaluation_item_cancelled(message.item_id)
            eval_item_messages_total.labels(outcome="cancelled").inc()
```

- [ ] **Step 8: Run worker tests**

Run:

```bash
PYTHONPATH=services pytest services/eval/tests/test_worker.py -v
```

Expected: all worker tests pass.

- [ ] **Step 9: Commit**

```bash
git add services/eval/app/worker.py services/eval/tests/test_worker.py
git commit -m "feat(eval): stop cancelled item work"
```

## Task 7: MCP Evidence Guidance

**Files:**
- Modify: `go/eval-mcp-service/internal/evalworkflow/service.go`
- Test: `go/eval-mcp-service/internal/evalworkflow/service_test.go`

- [ ] **Step 1: Write failing MCP evidence tests**

Add these tests near existing `RunEvidence` tests in `go/eval-mcp-service/internal/evalworkflow/service_test.go`:

```go
func TestRunEvidenceNextStepsCancelled(t *testing.T) {
	evidence := RunEvidence{Status: "cancelled"}

	got := runEvidenceNextSteps(evidence)

	if len(got) != 1 || !strings.Contains(got[0], "cancelled") {
		t.Fatalf("cancelled next steps = %#v", got)
	}
}

func TestRunEvidenceNextStepsStaleRetryable(t *testing.T) {
	evidence := RunEvidence{
		Status:       "running",
		StaleRunning: true,
		ItemCounts: map[string]int{
			"stale":     2,
			"retryable": 2,
		},
	}

	got := runEvidenceNextSteps(evidence)

	joined := strings.Join(got, " ")
	if !strings.Contains(joined, "republish") {
		t.Fatalf("retryable stale next steps should mention republish: %#v", got)
	}
}

func TestRunEvidenceNextStepsStaleExhausted(t *testing.T) {
	evidence := RunEvidence{
		Status:       "running",
		StaleRunning: true,
		ItemCounts: map[string]int{
			"stale":     2,
			"retryable": 0,
		},
	}

	got := runEvidenceNextSteps(evidence)

	joined := strings.Join(got, " ")
	if !strings.Contains(joined, "exhausted") {
		t.Fatalf("exhausted stale next steps should mention exhausted work: %#v", got)
	}
}
```

- [ ] **Step 2: Run MCP tests and verify they fail**

Run:

```bash
go test ./go/eval-mcp-service/internal/evalworkflow -run 'TestRunEvidenceNextSteps(Cancelled|StaleRetryable|StaleExhausted)' -v
```

Expected: fail because the evidence guidance does not handle these cases yet.

- [ ] **Step 3: Update terminal status helper**

In `go/eval-mcp-service/internal/evalworkflow/service.go`, update `isTerminalRunStatus`:

```go
func isTerminalRunStatus(status string) bool {
	return status == "completed" ||
		status == "completed_with_failures" ||
		status == "failed" ||
		status == "cancelled"
}
```

- [ ] **Step 4: Update `runEvidenceNextSteps`**

In `runEvidenceNextSteps`, add this case before `case "failed":`:

```go
	case "cancelled":
		return []string{
			"Run was cancelled; start a new eval run if evaluation is still needed.",
		}
```

Then replace the `running` stale block with:

```go
		if evidence.StaleRunning {
			staleItems := evidence.ItemCounts["stale"]
			retryableItems := evidence.ItemCounts["retryable"]
			if staleItems > 0 && retryableItems > 0 {
				return []string{
					"Recovery should reset or republish retryable eval item work.",
					"Inspect eval worker logs if stale or retryable item counts do not change.",
				}
			}
			if staleItems > 0 {
				return []string{
					"Stale eval item work appears exhausted or terminal repair is needed.",
					"Use investigate_eval_run in observability-mcp-service with this eval_id.",
				}
			}
			return []string{
				"Use investigate_eval_run in observability-mcp-service with this eval_id.",
				"Check eval service logs for eval_item_start without eval_item_completed.",
			}
		}
```

- [ ] **Step 5: Run MCP tests**

Run:

```bash
go test ./go/eval-mcp-service/internal/evalworkflow -v
```

Expected: all eval workflow tests pass.

- [ ] **Step 6: Commit**

```bash
git add go/eval-mcp-service/internal/evalworkflow/service.go go/eval-mcp-service/internal/evalworkflow/service_test.go
git commit -m "feat(eval): clarify stale run evidence"
```

## Task 8: Final Verification

**Files:**
- Verify Python and Go changes from prior tasks.

- [ ] **Step 1: Run targeted eval Python tests**

Run:

```bash
PYTHONPATH=services pytest \
  services/eval/tests/test_db.py \
  services/eval/tests/test_main.py \
  services/eval/tests/test_evaluator.py \
  services/eval/tests/test_worker.py \
  -v
```

Expected: all selected Python tests pass.

- [ ] **Step 2: Run targeted Go eval MCP tests**

Run:

```bash
go test ./go/eval-mcp-service/internal/evalworkflow -v
```

Expected: all selected Go tests pass.

- [ ] **Step 3: Run required preflights before final commit or PR**

Run:

```bash
make preflight-python
make preflight-go
make preflight-security
```

Expected: all preflights pass. If a preflight is blocked by a missing local tool or platform constraint, capture the exact blocker in the final handoff.

- [ ] **Step 4: Review final diff**

Run:

```bash
git diff --stat qa...HEAD
git diff qa...HEAD -- services/eval/app services/eval/tests go/eval-mcp-service/internal/evalworkflow
```

Expected: diff is limited to issue 313 cancellation, recovery, evidence, and tests.
