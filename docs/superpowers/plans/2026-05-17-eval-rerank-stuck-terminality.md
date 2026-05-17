# Eval Rerank Stuck Terminality Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ensure rerank-on RAG eval runs become terminal failed runs when they time out, are cancelled, or are recovered as stale, and make MCP waits report long-running runs clearly.

**Architecture:** Keep the eval API's existing in-process background task model, but add explicit terminality boundaries: overall run timeout, cancellation persistence, and stale-running recovery on startup. Keep `go/eval-mcp-service` read-only for waits, improving the timeout result with latest run metadata rather than mutating eval rows.

**Tech Stack:** FastAPI background tasks, asyncio, aiosqlite, pytest/pytest-asyncio, Go eval MCP service, Go unit tests.

---

### Task 1: Server-Side Timeout And Cancellation

**Files:**
- Modify: `services/eval/app/config.py`
- Modify: `services/eval/app/main.py`
- Test: `services/eval/tests/test_main.py`

- [ ] **Step 1: Write failing tests**

Add imports in `services/eval/tests/test_main.py`:

```python
import asyncio
from app.main import _run_evaluation_task, recover_stale_evaluations
```

Add tests near the existing rerank metadata tests:

```python
@pytest.mark.asyncio
@patch("app.main.RAGClient")
@patch("app.main.run_evaluation", new_callable=AsyncMock)
@patch("app.main.capture_run_config", new_callable=AsyncMock)
@patch("app.main.get_db")
async def test_run_evaluation_task_marks_http_timeout_failed(
    mock_get_db, mock_capture, mock_run_evaluation, mock_rag_client
):
    mock_db = AsyncMock()
    mock_get_db.return_value = mock_db
    mock_capture.return_value = {"captured_at": "x"}
    mock_run_evaluation.side_effect = httpx.ReadTimeout("rerank request timed out")
    mock_rag_client.return_value.close = AsyncMock()

    await _run_evaluation_task(
        "eval-timeout",
        [{"query": "q", "expected_answer": "a"}],
        "documents",
        rerank=True,
    )

    mock_db.fail_evaluation.assert_awaited_once()
    eval_id, error = mock_db.fail_evaluation.await_args.args
    assert eval_id == "eval-timeout"
    assert "eval-timeout" in error
    assert "documents" in error
    assert "rerank=true" in error
    assert "rerank request timed out" in error
```

```python
@pytest.mark.asyncio
@patch("app.main.RAGClient")
@patch("app.main.asyncio.wait_for", new_callable=AsyncMock)
@patch("app.main.capture_run_config", new_callable=AsyncMock)
@patch("app.main.get_db")
async def test_run_evaluation_task_marks_overall_timeout_failed(
    mock_get_db, mock_capture, mock_wait_for, mock_rag_client
):
    mock_db = AsyncMock()
    mock_get_db.return_value = mock_db
    mock_capture.return_value = {"captured_at": "x"}
    mock_wait_for.side_effect = asyncio.TimeoutError
    mock_rag_client.return_value.close = AsyncMock()

    await _run_evaluation_task(
        "eval-max-runtime",
        [{"query": "q", "expected_answer": "a"}],
        "documents",
        rerank=True,
    )

    mock_db.fail_evaluation.assert_awaited_once()
    eval_id, error = mock_db.fail_evaluation.await_args.args
    assert eval_id == "eval-max-runtime"
    assert "timed out" in error
    assert "eval-max-runtime" in error
    assert "documents" in error
    assert "rerank=true" in error
```

```python
@pytest.mark.asyncio
@patch("app.main.RAGClient")
@patch("app.main.run_evaluation", new_callable=AsyncMock)
@patch("app.main.capture_run_config", new_callable=AsyncMock)
@patch("app.main.get_db")
async def test_run_evaluation_task_marks_cancellation_failed(
    mock_get_db, mock_capture, mock_run_evaluation, mock_rag_client
):
    mock_db = AsyncMock()
    mock_get_db.return_value = mock_db
    mock_capture.return_value = {"captured_at": "x"}
    mock_run_evaluation.side_effect = asyncio.CancelledError
    mock_rag_client.return_value.close = AsyncMock()

    with pytest.raises(asyncio.CancelledError):
        await _run_evaluation_task(
            "eval-cancelled",
            [{"query": "q", "expected_answer": "a"}],
            "documents",
            rerank=True,
        )

    mock_db.fail_evaluation.assert_awaited_once()
    eval_id, error = mock_db.fail_evaluation.await_args.args
    assert eval_id == "eval-cancelled"
    assert "cancelled" in error
    assert "eval-cancelled" in error
    assert "documents" in error
    assert "rerank=true" in error
```

- [ ] **Step 2: Run tests to verify red**

Run:

```bash
PYTHONPATH=services/eval:services pytest \
  services/eval/tests/test_main.py::test_run_evaluation_task_marks_http_timeout_failed \
  services/eval/tests/test_main.py::test_run_evaluation_task_marks_overall_timeout_failed \
  services/eval/tests/test_main.py::test_run_evaluation_task_marks_cancellation_failed -q
```

Expected: fail because `_run_evaluation_task` does not use an overall timeout
and does not catch `asyncio.CancelledError`.

- [ ] **Step 3: Implement minimal code**

In `services/eval/app/config.py`, add:

```python
    # Evaluation runtime guardrails
    eval_run_max_seconds: float = 900.0
    eval_stale_grace_seconds: float = 300.0
```

In `services/eval/app/main.py`, import `asyncio` and add a helper:

```python
def _failure_message(
    eval_id: str,
    collection: str,
    rerank: bool,
    elapsed_seconds: float,
    reason: str,
) -> str:
    return (
        f"evaluation {eval_id} failed for collection={collection} "
        f"rerank={str(rerank).lower()} after {elapsed_seconds:.2f}s: {reason}"
    )
```

Wrap `run_evaluation(...)`:

```python
        aggregate, results = await asyncio.wait_for(
            run_evaluation(
                items=items,
                rag_client=rag_client,
                collection=collection,
                llm_provider=settings.llm_provider,
                llm_base_url=settings.llm_base_url,
                llm_model=settings.llm_model,
                llm_api_key=settings.llm_api_key,
                rerank=rerank,
            ),
            timeout=settings.eval_run_max_seconds,
        )
```

Add explicit branches before the existing `except Exception`:

```python
    except asyncio.TimeoutError:
        elapsed = time.perf_counter() - start
        error = _failure_message(
            eval_id,
            coll_name,
            rerank,
            elapsed,
            f"timed out after {settings.eval_run_max_seconds:.2f}s max runtime",
        )
        logger.error("%s", error)
        await db.fail_evaluation(eval_id, error=error)
    except asyncio.CancelledError:
        elapsed = time.perf_counter() - start
        error = _failure_message(eval_id, coll_name, rerank, elapsed, "cancelled")
        logger.error("%s", error)
        await db.fail_evaluation(eval_id, error=error)
        raise
    except Exception as e:
        elapsed = time.perf_counter() - start
        error = _failure_message(eval_id, coll_name, rerank, elapsed, str(e))
        logger.error("Evaluation %s failed: %s", eval_id, e, exc_info=True)
        await db.fail_evaluation(eval_id, error=error)
```

- [ ] **Step 4: Run tests to verify green**

Run the same three pytest tests. Expected: pass.

### Task 2: Stale Running Recovery

**Files:**
- Modify: `services/eval/app/db.py`
- Modify: `services/eval/app/main.py`
- Test: `services/eval/tests/test_db.py`
- Test: `services/eval/tests/test_main.py`

- [ ] **Step 1: Write failing DB test**

Add imports:

```python
from datetime import datetime, timedelta, timezone
```

Add the test:

```python
@pytest.mark.asyncio
async def test_fail_stale_running_evaluations_only_marks_old_running_rows(db):
    ds_id = await db.create_dataset(name="ds-stale", items=SIMPLE_ITEM)
    old_running_id = await db.create_evaluation(
        dataset_id=ds_id, collection="documents"
    )
    fresh_running_id = await db.create_evaluation(
        dataset_id=ds_id, collection="documents"
    )
    completed_id = await db.create_evaluation(dataset_id=ds_id, collection="documents")
    failed_id = await db.create_evaluation(dataset_id=ds_id, collection="documents")

    old_time = (datetime.now(timezone.utc) - timedelta(minutes=30)).isoformat()
    await db._db.execute(  # noqa: SLF001 - test sets up precise stale timestamps
        "UPDATE evaluations SET created_at = ? WHERE id = ?",
        (old_time, old_running_id),
    )
    await db._db.commit()  # noqa: SLF001
    await db.complete_evaluation(completed_id, aggregate_scores={}, results=[])
    await db.fail_evaluation(failed_id, error="already failed")

    recovered = await db.fail_stale_running_evaluations(max_age_seconds=600)

    assert recovered == 1
    old_running = await db.get_evaluation(old_running_id)
    fresh_running = await db.get_evaluation(fresh_running_id)
    completed = await db.get_evaluation(completed_id)
    failed = await db.get_evaluation(failed_id)

    assert old_running["status"] == "failed"
    assert "exceeded max runtime" in old_running["error"]
    assert fresh_running["status"] == "running"
    assert completed["status"] == "completed"
    assert failed["status"] == "failed"
    assert failed["error"] == "already failed"
```

- [ ] **Step 2: Verify DB red**

Run:

```bash
PYTHONPATH=services/eval:services pytest services/eval/tests/test_db.py::test_fail_stale_running_evaluations_only_marks_old_running_rows -q
```

Expected: fail because the DB method does not exist.

- [ ] **Step 3: Implement DB method**

In `services/eval/app/db.py`, add:

```python
    async def fail_stale_running_evaluations(self, max_age_seconds: float) -> int:
        now = datetime.now(_UTC)
        cutoff = (now - timedelta(seconds=max_age_seconds)).isoformat()
        error = "evaluation exceeded max runtime and was recovered as stale"
        cursor = await self._db.execute(
            "UPDATE evaluations "
            "SET status = 'failed', error = ?, completed_at = ? "
            "WHERE status = 'running' AND created_at < ?",
            (error, now.isoformat(), cutoff),
        )
        await self._db.commit()
        return cursor.rowcount
```

Also import `timedelta`.

- [ ] **Step 4: Write startup recovery test**

In `services/eval/tests/test_main.py`, add:

```python
@pytest.mark.asyncio
@patch("app.main.get_db")
async def test_recover_stale_evaluations_uses_max_runtime_plus_grace(mock_get_db):
    mock_db = AsyncMock()
    mock_db.fail_stale_running_evaluations.return_value = 2
    mock_get_db.return_value = mock_db

    await recover_stale_evaluations()

    expected_age = settings.eval_run_max_seconds + settings.eval_stale_grace_seconds
    mock_db.fail_stale_running_evaluations.assert_awaited_once_with(expected_age)
```

- [ ] **Step 5: Implement startup hook**

In `services/eval/app/main.py`, add:

```python
@app.on_event("startup")
async def recover_stale_evaluations():
    db = await get_db()
    max_age = settings.eval_run_max_seconds + settings.eval_stale_grace_seconds
    recovered = await db.fail_stale_running_evaluations(max_age)
    if recovered:
        logger.warning("Recovered %s stale running evaluation(s)", recovered)
```

### Task 3: MCP Wait Timeout Metadata

**Files:**
- Modify: `go/eval-mcp-service/internal/evalworkflow/service.go`
- Test: `go/eval-mcp-service/internal/evalworkflow/service_test.go`

- [ ] **Step 1: Write failing Go test**

Add:

```go
func TestWaitForRunTimeoutIncludesLatestRunMetadata(t *testing.T) {
	ctx := context.Background()
	collection := "documents"
	api := &fakeAPI{detailsByID: map[string][]evalapi.EvaluationDetail{
		"eval-1": {{
			ID:         "eval-1",
			Status:     "running",
			Collection: &collection,
			CreatedAt:  "2026-05-17T01:10:15Z",
		}},
	}}
	svc := newTestServiceWithTiming(api, time.Millisecond, 3*time.Millisecond)

	got, err := svc.WaitForRun(ctx, "eval-1")
	if err == nil {
		t.Fatal("WaitForRun error = nil, want timeout")
	}
	if !got.TimedOut || got.Run.Status != "running" {
		t.Fatalf("WaitResult = %#v", got)
	}
	for _, want := range []string{
		`evaluation "eval-1"`,
		`latest status "running"`,
		`created_at "2026-05-17T01:10:15Z"`,
		`collection "documents"`,
		"eval API run may still finish after the MCP wait timeout",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("WaitForRun error = %q, want substring %q", err.Error(), want)
		}
	}
}
```

- [ ] **Step 2: Verify Go red**

Run:

```bash
cd go/eval-mcp-service && go test ./internal/evalworkflow -run TestWaitForRunTimeoutIncludesLatestRunMetadata -count=1
```

Expected: fail because `waitTimeoutError` includes only status.

- [ ] **Step 3: Implement minimal Go behavior**

Change `waitTimeoutError` to accept `latest evalapi.EvaluationDetail` and build
a compact metadata message. Update both call sites in `WaitForRun`.

```go
func waitTimeoutError(evalID string, timeout time.Duration, latest evalapi.EvaluationDetail) error {
	parts := []string{fmt.Sprintf("latest status %q", latest.Status)}
	if latest.CreatedAt != "" {
		parts = append(parts, fmt.Sprintf("created_at %q", latest.CreatedAt))
	}
	if latest.CompletedAt != nil {
		parts = append(parts, fmt.Sprintf("completed_at %q", *latest.CompletedAt))
	}
	if latest.Collection != nil {
		parts = append(parts, fmt.Sprintf("collection %q", *latest.Collection))
	}
	if latest.Error != nil {
		parts = append(parts, fmt.Sprintf("error %q", *latest.Error))
	}
	return fmt.Errorf(
		"wait for evaluation %q timed out after %s with %s; eval API run may still finish after the MCP wait timeout",
		evalID,
		timeout,
		strings.Join(parts, ", "),
	)
}
```

- [ ] **Step 4: Verify Go green**

Run:

```bash
cd go/eval-mcp-service && go test ./internal/evalworkflow -run TestWaitForRunTimeoutIncludesLatestRunMetadata -count=1
cd go/eval-mcp-service && go test ./internal/evalworkflow ./internal/evalapi
```

### Task 4: Focused Verification

**Files:**
- No production changes.

- [ ] **Step 1: Run eval tests**

```bash
PYTHONPATH=services/eval:services pytest services/eval/tests/test_main.py services/eval/tests/test_db.py services/eval/tests/test_rag_client.py -q
```

- [ ] **Step 2: Run Go MCP tests**

```bash
cd go/eval-mcp-service && go test ./internal/evalworkflow ./internal/evalapi
```

- [ ] **Step 3: Run required preflights before commit**

```bash
make preflight-python
make preflight-security
make preflight-go
```

If a preflight is blocked by missing tooling or unrelated failing suites,
record the exact blocker and keep the focused tests above as local evidence.
