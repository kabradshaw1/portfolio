# RAG Eval RabbitMQ Item Worker Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace in-process eval execution with durable RabbitMQ item jobs, idempotent eval item state, bounded retries, DLQ handling, and richer run evidence.

**Architecture:** `services/eval` remains the source of truth in SQLite. `POST /evaluations` creates an eval run plus one durable item row per dataset item, publishes persistent RabbitMQ item messages, and returns `202`; a separate eval worker process claims item leases, runs search/chat/judge/scoring for one item, stores item results, and idempotently aggregates the run when all items are terminal.

**Tech Stack:** Python 3.11, FastAPI, aiosqlite, aio-pika/RabbitMQ, Prometheus client, pytest/pytest-asyncio, Docker Compose, Kubernetes manifests.

---

## File Map

- Modify `services/eval/requirements.txt`: add `aio-pika`.
- Modify `services/eval/app/config.py`: add RabbitMQ, worker, retry, lease, and recovery settings.
- Modify `services/eval/app/db.py`: add item table, WAL pragmas, item CRUD/claim/aggregation helpers, and queued/run status support.
- Modify `services/eval/app/evaluator.py`: extract single-item evaluation logic from the current sequential run path.
- Create `services/eval/app/eval_items.py`: item result data models and conversion helpers.
- Create `services/eval/app/broker.py`: RabbitMQ publisher/consumer protocol, aio-pika implementation, and message schema helpers.
- Create `services/eval/app/worker.py`: item worker loop, retry classification, item processing, ack/nack behavior, recovery hooks.
- Modify `services/eval/app/main.py`: create queued runs/items, publish item messages, expose item progress in run responses, remove new-run dependency on `BackgroundTasks`.
- Modify `services/eval/app/metrics.py`: add bounded queue/item metrics.
- Inspect `services/eval/Dockerfile`: keep one image and use command overrides for the worker.
- Modify `docker-compose.yml`: add RabbitMQ and `eval-worker`.
- Modify `k8s/ai-services/configmaps/eval-config.yml`: add non-secret RabbitMQ/worker settings.
- Modify `k8s/ai-services/deployments/eval.yml`: add API RabbitMQ env refs where safe.
- Create `k8s/ai-services/deployments/eval-worker.yml`: worker deployment.
- Modify `k8s/ai-services/kustomization.yaml`: include worker deployment.
- Modify `monitoring/prometheus.yml` only if local compose needs to scrape the worker separately.
- Modify `services/eval/tests/test_db.py`: DB item state, claim, lease, aggregation tests.
- Create `services/eval/tests/test_eval_items.py`: single-item evaluation result shape tests.
- Create `services/eval/tests/test_broker.py`: message schema and publish tests with fake channel.
- Create `services/eval/tests/test_worker.py`: worker item processing, retry, duplicate, DLQ behavior with fakes.
- Modify `services/eval/tests/test_main.py`: API queued-run and progress response tests.

## Task 1: Eval Item Persistence

**Files:**
- Modify: `services/eval/app/db.py`
- Test: `services/eval/tests/test_db.py`

- [ ] **Step 1: Write failing DB tests for queued runs and item creation**

Add these tests near the existing evaluation DB tests in `services/eval/tests/test_db.py`:

```python
@pytest.mark.asyncio
async def test_create_evaluation_can_start_queued(db):
    ds_id = await db.create_dataset(name="ds-queued", items=SIMPLE_ITEM)

    eval_id = await db.create_evaluation(
        dataset_id=ds_id,
        collection="documents",
        status="queued",
    )

    evaluation = await db.get_evaluation(eval_id)
    assert evaluation["status"] == "queued"


@pytest.mark.asyncio
async def test_create_items_for_evaluation_persists_dataset_order(db):
    items = [
        {"query": "q1", "expected_answer": "a1", "expected_sources": ["s1"]},
        {"query": "q2", "expected_answer": "a2", "expected_sources": []},
    ]
    ds_id = await db.create_dataset(name="ds-items", items=items)
    eval_id = await db.create_evaluation(
        dataset_id=ds_id,
        collection="documents",
        status="queued",
    )

    created = await db.create_evaluation_items(eval_id, items, max_attempts=3)
    stored = await db.list_evaluation_items(eval_id)

    assert [item["item_index"] for item in created] == [0, 1]
    assert [item["query"] for item in stored] == ["q1", "q2"]
    assert stored[0]["expected_sources"] == ["s1"]
    assert stored[0]["status"] == "queued"
    assert stored[0]["attempt_count"] == 0
    assert stored[0]["max_attempts"] == 3
```

- [ ] **Step 2: Run the new tests and verify they fail**

Run:

```bash
PYTHONPATH=services pytest services/eval/tests/test_db.py::test_create_evaluation_can_start_queued services/eval/tests/test_db.py::test_create_items_for_evaluation_persists_dataset_order -v
```

Expected: fail because `create_evaluation` has no `status` parameter and item persistence methods do not exist.

- [ ] **Step 3: Add WAL/busy-timeout and eval item table**

In `services/eval/app/db.py`, update `init()` immediately after opening the connection:

```python
self._db = await aiosqlite.connect(self.db_path)
self._db.row_factory = aiosqlite.Row
await self._db.execute("PRAGMA journal_mode=WAL")
await self._db.execute("PRAGMA busy_timeout=5000")
await self._db.execute("PRAGMA foreign_keys=ON")
```

Add this table to the existing `executescript` block:

```sql
CREATE TABLE IF NOT EXISTS evaluation_items (
    id TEXT PRIMARY KEY,
    evaluation_id TEXT NOT NULL REFERENCES evaluations(id),
    item_index INTEGER NOT NULL,
    query TEXT NOT NULL,
    expected_answer TEXT NOT NULL,
    expected_sources TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'queued',
    attempt_count INTEGER NOT NULL DEFAULT 0,
    max_attempts INTEGER NOT NULL DEFAULT 3,
    lease_owner TEXT,
    lease_expires_at TEXT,
    last_error TEXT,
    result TEXT,
    scores TEXT,
    score_reasons TEXT,
    started_at TEXT,
    completed_at TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE (evaluation_id, item_index)
);
CREATE INDEX IF NOT EXISTS idx_evaluation_items_eval_status
    ON evaluation_items(evaluation_id, status);
CREATE INDEX IF NOT EXISTS idx_evaluation_items_status_lease
    ON evaluation_items(status, lease_expires_at);
```

- [ ] **Step 4: Add status-aware evaluation creation and item methods**

In `services/eval/app/db.py`, change `create_evaluation` signature and insert:

```python
async def create_evaluation(
    self,
    dataset_id: str,
    collection: str,
    notes: str | None = None,
    baseline_eval_id: str | None = None,
    status: str = "running",
) -> str:
    eval_id = str(uuid.uuid4())
    now = datetime.now(_UTC).isoformat()
    await self._db.execute(
        "INSERT INTO evaluations "
        "(id, dataset_id, status, collection, created_at, notes, baseline_eval_id) "
        "VALUES (?, ?, ?, ?, ?, ?, ?)",
        (eval_id, dataset_id, status, collection, now, notes, baseline_eval_id),
    )
    await self._db.commit()
    return eval_id
```

Add helpers:

```python
def _item_row_to_dict(self, row) -> dict:
    return {
        "id": row["id"],
        "evaluation_id": row["evaluation_id"],
        "item_index": row["item_index"],
        "query": row["query"],
        "expected_answer": row["expected_answer"],
        "expected_sources": json.loads(row["expected_sources"]),
        "status": row["status"],
        "attempt_count": row["attempt_count"],
        "max_attempts": row["max_attempts"],
        "lease_owner": row["lease_owner"],
        "lease_expires_at": row["lease_expires_at"],
        "last_error": json.loads(row["last_error"]) if row["last_error"] else None,
        "result": json.loads(row["result"]) if row["result"] else None,
        "scores": json.loads(row["scores"]) if row["scores"] else None,
        "score_reasons": (
            json.loads(row["score_reasons"]) if row["score_reasons"] else None
        ),
        "started_at": row["started_at"],
        "completed_at": row["completed_at"],
        "created_at": row["created_at"],
        "updated_at": row["updated_at"],
    }


async def create_evaluation_items(
    self, eval_id: str, items: list[dict], max_attempts: int
) -> list[dict]:
    now = datetime.now(_UTC).isoformat()
    created = []
    for index, item in enumerate(items):
        item_id = str(uuid.uuid4())
        await self._db.execute(
            "INSERT INTO evaluation_items "
            "(id, evaluation_id, item_index, query, expected_answer, "
            "expected_sources, status, attempt_count, max_attempts, "
            "created_at, updated_at) "
            "VALUES (?, ?, ?, ?, ?, ?, 'queued', 0, ?, ?, ?)",
            (
                item_id,
                eval_id,
                index,
                item["query"],
                item["expected_answer"],
                json.dumps(item.get("expected_sources", [])),
                max_attempts,
                now,
                now,
            ),
        )
        created.append(
            {
                "id": item_id,
                "evaluation_id": eval_id,
                "item_index": index,
                "query": item["query"],
                "expected_answer": item["expected_answer"],
                "expected_sources": item.get("expected_sources", []),
                "status": "queued",
                "attempt_count": 0,
                "max_attempts": max_attempts,
            }
        )
    await self._db.commit()
    return created


async def list_evaluation_items(self, eval_id: str) -> list[dict]:
    cursor = await self._db.execute(
        "SELECT * FROM evaluation_items WHERE evaluation_id = ? ORDER BY item_index",
        (eval_id,),
    )
    rows = await cursor.fetchall()
    return [self._item_row_to_dict(row) for row in rows]
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
git commit -m "feat(eval): persist evaluation item state"
```

## Task 2: Item Claiming, Completion, Failure, And Aggregation

**Files:**
- Modify: `services/eval/app/db.py`
- Test: `services/eval/tests/test_db.py`

- [ ] **Step 1: Write failing tests for idempotent claim and duplicate completion**

Add:

```python
@pytest.mark.asyncio
async def test_claim_evaluation_item_sets_running_lease(db):
    ds_id = await db.create_dataset(name="ds-claim", items=SIMPLE_ITEM)
    eval_id = await db.create_evaluation(ds_id, "documents", status="queued")
    [item] = await db.create_evaluation_items(eval_id, SIMPLE_ITEM, max_attempts=3)

    claimed = await db.claim_evaluation_item(
        item["id"], worker_id="worker-1", lease_seconds=60
    )

    assert claimed is not None
    assert claimed["status"] == "running"
    assert claimed["attempt_count"] == 1
    assert claimed["lease_owner"] == "worker-1"


@pytest.mark.asyncio
async def test_claim_evaluation_item_returns_none_for_completed_item(db):
    ds_id = await db.create_dataset(name="ds-claim-completed", items=SIMPLE_ITEM)
    eval_id = await db.create_evaluation(ds_id, "documents", status="queued")
    [item] = await db.create_evaluation_items(eval_id, SIMPLE_ITEM, max_attempts=3)
    await db.mark_evaluation_item_completed(
        item["id"],
        result={"query": "q", "answer": "a", "contexts": []},
        scores={"faithfulness": 1.0},
        score_reasons={"faithfulness": "supported"},
    )

    claimed = await db.claim_evaluation_item(
        item["id"], worker_id="worker-1", lease_seconds=60
    )

    assert claimed is None
```

- [ ] **Step 2: Write failing aggregation tests**

Add:

```python
@pytest.mark.asyncio
async def test_finalize_evaluation_completes_all_successful_items(db):
    items = [
        {"query": "q1", "expected_answer": "a1", "expected_sources": []},
        {"query": "q2", "expected_answer": "a2", "expected_sources": []},
    ]
    ds_id = await db.create_dataset(name="ds-finalize", items=items)
    eval_id = await db.create_evaluation(ds_id, "documents", status="running")
    created = await db.create_evaluation_items(eval_id, items, max_attempts=3)
    for index, item in enumerate(created):
        await db.mark_evaluation_item_completed(
            item["id"],
            result={"query": f"q{index + 1}", "answer": "a", "contexts": []},
            scores={
                "faithfulness": 1.0,
                "answer_relevancy": 0.5,
                "context_precision": 0.25,
                "context_recall": 0.75,
            },
            score_reasons={"faithfulness": "ok"},
        )

    finalized = await db.finalize_evaluation_if_terminal(eval_id)

    assert finalized is True
    evaluation = await db.get_evaluation(eval_id)
    assert evaluation["status"] == "completed"
    assert evaluation["aggregate_scores"]["faithfulness"] == 1.0
    assert len(evaluation["results"]) == 2


@pytest.mark.asyncio
async def test_finalize_evaluation_marks_completed_with_failures(db):
    items = [
        {"query": "q1", "expected_answer": "a1", "expected_sources": []},
        {"query": "q2", "expected_answer": "a2", "expected_sources": []},
    ]
    ds_id = await db.create_dataset(name="ds-partial", items=items)
    eval_id = await db.create_evaluation(ds_id, "documents", status="running")
    completed, failed = await db.create_evaluation_items(eval_id, items, max_attempts=1)
    await db.mark_evaluation_item_completed(
        completed["id"],
        result={"query": "q1", "answer": "a1", "contexts": []},
        scores={"faithfulness": 0.8},
        score_reasons={"faithfulness": "ok"},
    )
    await db.mark_evaluation_item_failed(
        failed["id"],
        error={"error_type": "timeout", "retryable": False},
    )

    finalized = await db.finalize_evaluation_if_terminal(eval_id)

    assert finalized is True
    evaluation = await db.get_evaluation(eval_id)
    assert evaluation["status"] == "completed_with_failures"
    assert evaluation["aggregate_scores"]["faithfulness"] == 0.8
    assert "failed_items=1" in evaluation["error"]
```

- [ ] **Step 3: Run tests and verify they fail**

Run:

```bash
PYTHONPATH=services pytest services/eval/tests/test_db.py::test_claim_evaluation_item_sets_running_lease services/eval/tests/test_db.py::test_finalize_evaluation_completes_all_successful_items -v
```

Expected: fail because item claim/finalization methods are missing.

- [ ] **Step 4: Implement item claim and terminal updates**

Add these methods to `services/eval/app/db.py`:

```python
async def get_evaluation_item(self, item_id: str) -> dict | None:
    cursor = await self._db.execute(
        "SELECT * FROM evaluation_items WHERE id = ?", (item_id,)
    )
    row = await cursor.fetchone()
    return self._item_row_to_dict(row) if row else None


async def claim_evaluation_item(
    self, item_id: str, worker_id: str, lease_seconds: float
) -> dict | None:
    now = datetime.now(_UTC)
    now_text = now.isoformat()
    lease_until = (now + timedelta(seconds=lease_seconds)).isoformat()
    await self._db.execute(
        "UPDATE evaluation_items "
        "SET status = 'running', attempt_count = attempt_count + 1, "
        "lease_owner = ?, lease_expires_at = ?, started_at = COALESCE(started_at, ?), "
        "updated_at = ? "
        "WHERE id = ? AND status = 'queued'",
        (worker_id, lease_until, now_text, now_text, item_id),
    )
    await self._db.commit()
    return await self.get_evaluation_item(item_id)


async def mark_evaluation_running(self, eval_id: str) -> None:
    now = datetime.now(_UTC).isoformat()
    await self._db.execute(
        "UPDATE evaluations SET status = 'running' WHERE id = ? AND status = 'queued'",
        (eval_id,),
    )
    await self._db.execute(
        "UPDATE evaluations SET completed_at = NULL WHERE id = ? AND status = 'running'",
        (eval_id,),
    )
    await self._db.commit()


async def mark_evaluation_item_completed(
    self,
    item_id: str,
    result: dict,
    scores: dict,
    score_reasons: dict,
) -> None:
    now = datetime.now(_UTC).isoformat()
    await self._db.execute(
        "UPDATE evaluation_items "
        "SET status = 'completed', result = ?, scores = ?, score_reasons = ?, "
        "lease_owner = NULL, lease_expires_at = NULL, completed_at = ?, updated_at = ? "
        "WHERE id = ? AND status != 'completed'",
        (json.dumps(result), json.dumps(scores), json.dumps(score_reasons), now, now, item_id),
    )
    await self._db.commit()


async def mark_evaluation_item_failed(self, item_id: str, error: dict) -> None:
    now = datetime.now(_UTC).isoformat()
    await self._db.execute(
        "UPDATE evaluation_items "
        "SET status = 'failed', last_error = ?, lease_owner = NULL, "
        "lease_expires_at = NULL, completed_at = ?, updated_at = ? "
        "WHERE id = ? AND status != 'completed'",
        (json.dumps(error), now, now, item_id),
    )
    await self._db.commit()


async def release_evaluation_item_for_retry(self, item_id: str, error: dict) -> None:
    now = datetime.now(_UTC).isoformat()
    await self._db.execute(
        "UPDATE evaluation_items "
        "SET status = 'queued', last_error = ?, lease_owner = NULL, "
        "lease_expires_at = NULL, updated_at = ? "
        "WHERE id = ? AND status = 'running'",
        (json.dumps(error), now, item_id),
    )
    await self._db.commit()
```

- [ ] **Step 5: Implement idempotent aggregation**

Add:

```python
def _aggregate_item_scores(self, completed: list[dict]) -> dict:
    metric_names = (
        "faithfulness",
        "answer_relevancy",
        "context_precision",
        "context_recall",
    )
    aggregate = {}
    for name in metric_names:
        values = [
            item["scores"].get(name)
            for item in completed
            if item["scores"] and item["scores"].get(name) is not None
        ]
        aggregate[name] = round(sum(values) / len(values), 4) if values else None
    return aggregate


async def finalize_evaluation_if_terminal(self, eval_id: str) -> bool:
    items = await self.list_evaluation_items(eval_id)
    if not items:
        return False
    if any(item["status"] in {"queued", "running"} for item in items):
        return False

    completed = [item for item in items if item["status"] == "completed"]
    failed = [item for item in items if item["status"] == "failed"]
    now = datetime.now(_UTC).isoformat()
    if completed:
        status = "completed" if not failed else "completed_with_failures"
        aggregate = self._aggregate_item_scores(completed)
        results = [item["result"] | {"scores": item["scores"]} for item in completed]
        error = None if not failed else f"failed_items={len(failed)}"
        await self._db.execute(
            "UPDATE evaluations "
            "SET status = ?, aggregate_scores = ?, results = ?, error = ?, "
            "completed_at = ? WHERE id = ? AND status NOT IN ('completed', 'failed')",
            (status, json.dumps(aggregate), json.dumps(results), error, now, eval_id),
        )
    else:
        await self._db.execute(
            "UPDATE evaluations "
            "SET status = 'failed', error = ?, completed_at = ? "
            "WHERE id = ? AND status != 'failed'",
            ("all evaluation items failed", now, eval_id),
        )
    await self._db.commit()
    return True
```

- [ ] **Step 6: Run DB tests**

Run:

```bash
PYTHONPATH=services pytest services/eval/tests/test_db.py -v
```

Expected: all DB tests pass.

- [ ] **Step 7: Commit**

```bash
git add services/eval/app/db.py services/eval/tests/test_db.py
git commit -m "feat(eval): add item claiming and aggregation"
```

## Task 3: Single-Item Evaluation Extraction

**Files:**
- Create: `services/eval/app/eval_items.py`
- Modify: `services/eval/app/evaluator.py`
- Test: `services/eval/tests/test_eval_items.py`

- [ ] **Step 1: Write failing single-item evaluation test**

Create `services/eval/tests/test_eval_items.py`:

```python
import pytest
from app.evaluator import EvalRunContext, JudgeScores, evaluate_item


class FakeRAGClient:
    async def search(self, query, collection, limit, rerank=False):
        assert query == "What is RAG?"
        assert collection == "documents"
        assert limit == 3
        assert rerank is True
        return [{"text": "RAG combines retrieval with generation."}]

    async def ask(
        self, question, collection, rerank=False, retrieval_config=None, answer_model=None
    ):
        assert retrieval_config == {"top_k": 3}
        return {
            "answer": "RAG combines retrieval with generation.",
            "retrieval": {"retrieval_mode": "hybrid"},
            "usage": {"answer_model": "qwen"},
        }


@pytest.mark.asyncio
async def test_evaluate_item_returns_result_scores_and_reasons():
    async def judge(row):
        assert row["user_input"] == "What is RAG?"
        return JudgeScores(
            faithfulness=1.0,
            answer_relevancy=0.9,
            reasons={"faithfulness": "supported", "answer_relevancy": "direct"},
        )

    result = await evaluate_item(
        item={
            "query": "What is RAG?",
            "expected_answer": "RAG combines retrieval with generation.",
            "expected_sources": [],
        },
        rag_client=FakeRAGClient(),
        collection="documents",
        rerank=True,
        top_k=3,
        judge=judge,
        run_context=EvalRunContext(
            eval_id="eval-1", collection="documents", requested_rerank=True
        ),
        answer_model=None,
        item_index=0,
    )

    assert result["result"]["query"] == "What is RAG?"
    assert result["result"]["answer"] == "RAG combines retrieval with generation."
    assert result["scores"]["faithfulness"] == 1.0
    assert result["scores"]["answer_relevancy"] == 0.9
    assert result["score_reasons"]["faithfulness"] == "supported"
```

- [ ] **Step 2: Run the test and verify it fails**

Run:

```bash
PYTHONPATH=services pytest services/eval/tests/test_eval_items.py -v
```

Expected: fail because `evaluate_item` does not exist.

- [ ] **Step 3: Extract `evaluate_item` in `services/eval/app/evaluator.py`**

Add this function above `build_evaluation_dataset`:

```python
async def evaluate_item(
    item: dict,
    rag_client: RAGClient,
    collection: str | None,
    rerank: bool,
    top_k: int,
    judge: JudgeFn,
    run_context: EvalRunContext | None,
    answer_model: dict | None,
    item_index: int,
) -> dict:
    started_at = time.perf_counter()
    query = item["query"]
    requested_rerank = str(rerank).lower()
    if run_context:
        logger.info(
            "eval_item_start eval_id=%s item_index=%s collection=%s rerank=%s",
            run_context.eval_id,
            item_index,
            run_context.collection,
            requested_rerank,
        )
    try:
        search_results = await rag_client.search(
            query, collection=collection, limit=top_k, rerank=rerank
        )
        chat_response = await rag_client.ask(
            query,
            collection=collection,
            rerank=rerank,
            retrieval_config={"top_k": top_k},
            answer_model=answer_model,
        )
        row = {
            "user_input": query,
            "retrieved_contexts": [r["text"] for r in search_results],
            "response": chat_response["answer"],
            "reference": item["expected_answer"],
            "expected_sources": item.get("expected_sources", []),
        }
        if "retrieval" in chat_response:
            row["retrieval"] = chat_response["retrieval"]
        if "usage" in chat_response:
            row["usage"] = _safe_usage(chat_response["usage"])
        judge_scores = await judge(row)
    except Exception:
        eval_items_total.labels(
            status="failed", requested_rerank=requested_rerank
        ).inc()
        eval_item_duration_seconds.labels(
            stage="total", requested_rerank=requested_rerank
        ).observe(time.perf_counter() - started_at)
        raise

    scores = {
        "faithfulness": judge_scores.faithfulness,
        "answer_relevancy": judge_scores.answer_relevancy,
        "context_precision": score_context_precision(
            query=row["user_input"],
            reference=row["reference"],
            contexts=row["retrieved_contexts"],
        ),
        "context_recall": score_context_recall(
            reference=row["reference"],
            contexts=row["retrieved_contexts"],
        ),
    }
    result = {
        "query": row["user_input"],
        "answer": row["response"],
        "contexts": row["retrieved_contexts"],
    }
    if "retrieval" in row:
        result["retrieval"] = row["retrieval"]
    if "usage" in row:
        result["usage"] = row["usage"]
    eval_items_total.labels(status="completed", requested_rerank=requested_rerank).inc()
    eval_item_duration_seconds.labels(
        stage="total", requested_rerank=requested_rerank
    ).observe(time.perf_counter() - started_at)
    return {
        "result": result,
        "scores": scores,
        "score_reasons": judge_scores.reasons,
    }
```

- [ ] **Step 4: Refactor `run_evaluation` to use `evaluate_item`**

Replace the full body of `run_evaluation` after the docstring with this code so
the legacy sequential eval path does not call chat/search twice:

```python
if not items:
    return {name: None for name in METRIC_NAMES}, []

if judge is None:

    async def judge(row: dict) -> JudgeScores:
        return await judge_generation_scores(
            row=row,
            provider=llm_provider,
            base_url=llm_base_url,
            model=llm_model,
            api_key=llm_api_key,
        )
```

Then continue with:

```python
per_query = []
all_scores = []
for index, item in enumerate(items):
    evaluated = await evaluate_item(
        item=item,
        rag_client=rag_client,
        collection=collection,
        rerank=rerank,
        top_k=top_k,
        judge=judge,
        run_context=run_context,
        answer_model=answer_model,
        item_index=index,
    )
    result = evaluated["result"] | {
        "scores": evaluated["scores"],
        "score_reasons": evaluated["score_reasons"],
    }
    all_scores.append(evaluated["scores"])
    per_query.append(result)

return _aggregate(all_scores), per_query
```

Leave `build_evaluation_dataset` in place if existing tests still cover it; remove it only if all references are gone and tests remain readable.

- [ ] **Step 5: Run evaluator tests**

Run:

```bash
PYTHONPATH=services pytest services/eval/tests/test_eval_items.py services/eval/tests/test_evaluator.py -v
```

Expected: all tests pass.

- [ ] **Step 6: Commit**

```bash
git add services/eval/app/evaluator.py services/eval/tests/test_eval_items.py
git commit -m "refactor(eval): extract single item evaluation"
```

## Task 4: RabbitMQ Broker Boundary

**Files:**
- Modify: `services/eval/requirements.txt`
- Modify: `services/eval/app/config.py`
- Create: `services/eval/app/broker.py`
- Test: `services/eval/tests/test_broker.py`

- [ ] **Step 1: Write failing broker message tests**

Create `services/eval/tests/test_broker.py`:

```python
import json

from app.broker import EvalItemMessage, encode_eval_item_message


def test_encode_eval_item_message_contains_only_identifiers():
    payload = encode_eval_item_message(
        EvalItemMessage(
            evaluation_id="eval-1",
            item_id="item-1",
            item_index=3,
            attempt=1,
        )
    )

    decoded = json.loads(payload)
    assert decoded == {
        "message_version": 1,
        "evaluation_id": "eval-1",
        "item_id": "item-1",
        "item_index": 3,
        "attempt": 1,
    }
    assert "query" not in decoded
    assert "expected_answer" not in decoded
    assert "api_key" not in decoded
```

- [ ] **Step 2: Run the test and verify it fails**

Run:

```bash
PYTHONPATH=services pytest services/eval/tests/test_broker.py -v
```

Expected: fail because `app.broker` does not exist.

- [ ] **Step 3: Add dependency and config**

Append to `services/eval/requirements.txt`. Planning verified PyPI lists
`aio-pika` `9.6.2` as the latest release as of May 20, 2026:

```text
aio-pika==9.6.2
```

Add to `services/eval/app/config.py`:

```python
    # RabbitMQ eval item worker
    rabbitmq_url: str = ""
    eval_item_queue: str = "eval.item.requested"
    eval_item_dlq: str = "eval.item.requested.dlq"
    eval_worker_prefetch: int = 2
    eval_worker_concurrency: int = 2
    eval_item_max_attempts: int = 3
    eval_item_lease_seconds: float = 300.0
    eval_stale_item_seconds: float = 900.0
```

- [ ] **Step 4: Implement broker message helpers and publisher**

Create `services/eval/app/broker.py`:

```python
from __future__ import annotations

import json
from dataclasses import dataclass

import aio_pika
from aio_pika.abc import AbstractChannel, AbstractRobustConnection


@dataclass(frozen=True)
class EvalItemMessage:
    evaluation_id: str
    item_id: str
    item_index: int
    attempt: int
    message_version: int = 1


def encode_eval_item_message(message: EvalItemMessage) -> bytes:
    return json.dumps(
        {
            "message_version": message.message_version,
            "evaluation_id": message.evaluation_id,
            "item_id": message.item_id,
            "item_index": message.item_index,
            "attempt": message.attempt,
        }
    ).encode("utf-8")


def decode_eval_item_message(body: bytes) -> EvalItemMessage:
    payload = json.loads(body.decode("utf-8"))
    return EvalItemMessage(
        message_version=int(payload["message_version"]),
        evaluation_id=str(payload["evaluation_id"]),
        item_id=str(payload["item_id"]),
        item_index=int(payload["item_index"]),
        attempt=int(payload["attempt"]),
    )


class EvalItemPublisher:
    def __init__(self, rabbitmq_url: str, queue_name: str, dlq_name: str):
        self.rabbitmq_url = rabbitmq_url
        self.queue_name = queue_name
        self.dlq_name = dlq_name
        self._conn: AbstractRobustConnection | None = None
        self._channel: AbstractChannel | None = None

    async def connect(self) -> None:
        self._conn = await aio_pika.connect_robust(self.rabbitmq_url)
        self._channel = await self._conn.channel(publisher_confirms=True)
        await self._channel.declare_queue(self.dlq_name, durable=True)
        await self._channel.declare_queue(
            self.queue_name,
            durable=True,
            arguments={
                "x-dead-letter-exchange": "",
                "x-dead-letter-routing-key": self.dlq_name,
            },
        )

    async def publish(self, message: EvalItemMessage) -> None:
        if self._channel is None:
            await self.connect()
        assert self._channel is not None
        await self._channel.default_exchange.publish(
            aio_pika.Message(
                body=encode_eval_item_message(message),
                delivery_mode=aio_pika.DeliveryMode.PERSISTENT,
                content_type="application/json",
            ),
            routing_key=self.queue_name,
        )

    async def close(self) -> None:
        if self._conn is not None:
            await self._conn.close()
```

- [ ] **Step 5: Run broker tests**

Run:

```bash
PYTHONPATH=services pytest services/eval/tests/test_broker.py -v
```

Expected: pass.

- [ ] **Step 6: Commit**

```bash
git add services/eval/requirements.txt services/eval/app/config.py services/eval/app/broker.py services/eval/tests/test_broker.py
git commit -m "feat(eval): add rabbitmq item broker boundary"
```

## Task 5: API Queues Eval Items

**Files:**
- Modify: `services/eval/app/main.py`
- Modify: `services/eval/app/metrics.py`
- Modify: `services/eval/tests/test_main.py`

- [ ] **Step 1: Write failing API test for queued item publish**

Update `services/eval/tests/test_main.py` `test_start_evaluation` or add a new test:

```python
@patch("app.main.get_item_publisher")
@patch("app.main.validate_collection_exists", new_callable=AsyncMock)
@patch("app.main.get_db")
def test_start_evaluation_creates_items_and_publishes_messages(
    mock_get_db, mock_validate_collection, mock_get_item_publisher
):
    mock_db = AsyncMock()
    dataset_items = [
        {"query": "q1", "expected_answer": "a1", "expected_sources": []},
        {"query": "q2", "expected_answer": "a2", "expected_sources": []},
    ]
    mock_db.get_dataset.return_value = {
        "id": "ds-123",
        "name": "test",
        "items": dataset_items,
        "created_at": "2026-04-16T00:00:00Z",
    }
    mock_db.create_evaluation.return_value = "eval-456"
    mock_db.create_evaluation_items.return_value = [
        {"id": "item-1", "item_index": 0},
        {"id": "item-2", "item_index": 1},
    ]
    mock_get_db.return_value = mock_db
    publisher = AsyncMock()
    mock_get_item_publisher.return_value = publisher

    response = client.post("/evaluations", json={"dataset_id": "ds-123"})

    assert response.status_code == 202
    assert response.json() == {"id": "eval-456", "status": "queued"}
    mock_db.create_evaluation.assert_awaited_once()
    assert mock_db.create_evaluation.await_args.kwargs["status"] == "queued"
    mock_db.create_evaluation_items.assert_awaited_once_with(
        "eval-456", dataset_items, max_attempts=settings.eval_item_max_attempts
    )
    assert publisher.publish.await_count == 2
```

- [ ] **Step 2: Run the API test and verify it fails**

Run:

```bash
PYTHONPATH=services pytest services/eval/tests/test_main.py::test_start_evaluation_creates_items_and_publishes_messages -v
```

Expected: fail because `get_item_publisher` does not exist and `start_evaluation` still uses `BackgroundTasks`.

- [ ] **Step 3: Add queue metrics**

In `services/eval/app/metrics.py`, add:

```python
eval_queue_publish_total = Counter(
    "eval_queue_publish_total",
    "Evaluation item queue publish attempts",
    ["status"],
)
```

- [ ] **Step 4: Add publisher factory and publish helper**

In `services/eval/app/main.py`, import:

```python
from app.broker import EvalItemMessage, EvalItemPublisher
from app.metrics import eval_queue_publish_total
```

Add globals/helpers near `_db`:

```python
_item_publisher: EvalItemPublisher | None = None


async def get_item_publisher() -> EvalItemPublisher:
    global _item_publisher
    if _item_publisher is None:
        if not settings.rabbitmq_url:
            raise HTTPException(status_code=503, detail="eval queue is not configured")
        _item_publisher = EvalItemPublisher(
            rabbitmq_url=settings.rabbitmq_url,
            queue_name=settings.eval_item_queue,
            dlq_name=settings.eval_item_dlq,
        )
    return _item_publisher


async def publish_evaluation_items(eval_id: str, items: list[dict]) -> None:
    publisher = await get_item_publisher()
    for item in items:
        await publisher.publish(
            EvalItemMessage(
                evaluation_id=eval_id,
                item_id=item["id"],
                item_index=item["item_index"],
                attempt=1,
            )
        )
        eval_queue_publish_total.labels(status="success").inc()
```

Update `shutdown()`:

```python
    if _item_publisher:
        await _item_publisher.close()
```

- [ ] **Step 5: Replace background task path in `start_evaluation`**

Keep the `background_tasks` parameter temporarily for route compatibility, but stop using it. Change evaluation creation and return:

```python
    eval_id = await db.create_evaluation(
        dataset_id=body.dataset_id,
        collection=collection,
        notes=body.notes,
        baseline_eval_id=body.baseline_eval_id,
        status="queued",
    )
```

After experiment attachment:

```python
    created_items = await db.create_evaluation_items(
        eval_id,
        dataset["items"],
        max_attempts=settings.eval_item_max_attempts,
    )
    try:
        await publish_evaluation_items(eval_id, created_items)
    except Exception:
        eval_queue_publish_total.labels(status="failed").inc()
        logger.exception("evaluation_item_publish_failed eval_id=%s", eval_id)
        raise HTTPException(status_code=503, detail="unable to queue evaluation") from None
```

Remove the `background_tasks.add_task(...)` call and return:

```python
    return {"id": eval_id, "status": "queued"}
```

- [ ] **Step 6: Run API tests**

Run:

```bash
PYTHONPATH=services pytest services/eval/tests/test_main.py -v
```

Expected: update older assertions expecting `"running"` to expect `"queued"` only for new start responses; all tests pass.

- [ ] **Step 7: Commit**

```bash
git add services/eval/app/main.py services/eval/app/metrics.py services/eval/tests/test_main.py
git commit -m "feat(eval): queue evaluation item jobs from api"
```

## Task 6: Worker Processes Items With Retry And DLQ

**Files:**
- Create: `services/eval/app/worker.py`
- Modify: `services/eval/app/metrics.py`
- Test: `services/eval/tests/test_worker.py`

- [ ] **Step 1: Write failing worker tests with fakes**

Create `services/eval/tests/test_worker.py`:

```python
import pytest
from app.broker import EvalItemMessage
from app.worker import ItemProcessor, RetryableEvalItemError


class FakeDB:
    def __init__(self):
        self.item = {
            "id": "item-1",
            "evaluation_id": "eval-1",
            "item_index": 0,
            "query": "q",
            "expected_answer": "a",
            "expected_sources": [],
            "attempt_count": 0,
            "max_attempts": 3,
        }
        self.completed = None
        self.failed = None
        self.finalized = False

    async def claim_evaluation_item(self, item_id, worker_id, lease_seconds):
        assert item_id == "item-1"
        return self.item

    async def get_evaluation(self, eval_id):
        return {
            "id": eval_id,
            "collection": "documents",
            "config": {"effective_retrieval_config": {"top_k": 5}},
        }

    async def mark_evaluation_running(self, eval_id):
        self.running = eval_id

    async def mark_evaluation_item_completed(self, item_id, result, scores, score_reasons):
        self.completed = (item_id, result, scores, score_reasons)

    async def mark_evaluation_item_failed(self, item_id, error):
        self.failed = (item_id, error)

    async def release_evaluation_item_for_retry(self, item_id, error):
        self.released = (item_id, error)

    async def finalize_evaluation_if_terminal(self, eval_id):
        self.finalized = True
        return True


@pytest.mark.asyncio
async def test_item_processor_completes_claimed_item():
    db = FakeDB()

    async def evaluate_item(**kwargs):
        return {
            "result": {"query": "q", "answer": "a", "contexts": []},
            "scores": {"faithfulness": 1.0},
            "score_reasons": {"faithfulness": "ok"},
        }

    processor = ItemProcessor(db=db, evaluate_item_fn=evaluate_item, worker_id="w1")

    await processor.process(
        EvalItemMessage(evaluation_id="eval-1", item_id="item-1", item_index=0, attempt=1)
    )

    assert db.completed[0] == "item-1"
    assert db.finalized is True
    assert db.failed is None


@pytest.mark.asyncio
async def test_item_processor_marks_failed_after_retry_exhaustion():
    db = FakeDB()
    db.item["attempt_count"] = 3
    db.item["max_attempts"] = 3

    async def evaluate_item(**kwargs):
        raise RetryableEvalItemError("chat timeout")

    processor = ItemProcessor(db=db, evaluate_item_fn=evaluate_item, worker_id="w1")

    await processor.process(
        EvalItemMessage(evaluation_id="eval-1", item_id="item-1", item_index=0, attempt=3)
    )

    assert db.failed[0] == "item-1"
    assert db.failed[1]["retryable"] is False


@pytest.mark.asyncio
async def test_item_processor_releases_retryable_item_before_requeue():
    db = FakeDB()
    db.item["attempt_count"] = 1
    db.item["max_attempts"] = 3

    async def evaluate_item(**kwargs):
        raise TimeoutError("chat timeout")

    processor = ItemProcessor(db=db, evaluate_item_fn=evaluate_item, worker_id="w1")

    with pytest.raises(RetryableEvalItemError):
        await processor.process(
            EvalItemMessage(
                evaluation_id="eval-1", item_id="item-1", item_index=0, attempt=1
            )
        )

    assert db.released[0] == "item-1"
    assert db.released[1]["retryable"] is True
```

- [ ] **Step 2: Run tests and verify they fail**

Run:

```bash
PYTHONPATH=services pytest services/eval/tests/test_worker.py -v
```

Expected: fail because `app.worker` does not exist.

- [ ] **Step 3: Add worker metrics**

In `services/eval/app/metrics.py`, add:

```python
eval_item_messages_total = Counter(
    "eval_item_messages_total",
    "Evaluation item messages by outcome",
    ["outcome"],
)

eval_item_retries_total = Counter(
    "eval_item_retries_total",
    "Evaluation item retries by reason",
    ["reason"],
)

eval_item_dlq_total = Counter(
    "eval_item_dlq_total",
    "Evaluation item messages sent to DLQ by reason",
    ["reason"],
)
```

- [ ] **Step 4: Implement `ItemProcessor`**

Create `services/eval/app/worker.py`:

```python
from __future__ import annotations

import asyncio
import logging
import socket

from llm.factory import get_llm_provider

from app.broker import EvalItemMessage
from app.config import settings
from app.db import EvalDB
from app.evaluator import EvalRunContext, EvaluationError, evaluate_item, judge_generation_scores
from app.metrics import eval_item_dlq_total, eval_item_messages_total, eval_item_retries_total
from app.rag_client import RAGClient

logger = logging.getLogger(__name__)


class RetryableEvalItemError(RuntimeError):
    pass


class PermanentEvalItemError(RuntimeError):
    pass


def classify_item_error(exc: Exception) -> tuple[str, bool]:
    if isinstance(exc, PermanentEvalItemError):
        return (type(exc).__name__, False)
    if isinstance(exc, EvaluationError):
        return (type(exc).__name__, False)
    return (type(exc).__name__, True)


class ItemProcessor:
    def __init__(
        self,
        db: EvalDB,
        evaluate_item_fn=evaluate_item,
        worker_id: str | None = None,
    ):
        self.db = db
        self.evaluate_item_fn = evaluate_item_fn
        self.worker_id = worker_id or socket.gethostname()

    async def process(self, message: EvalItemMessage) -> None:
        item = await self.db.claim_evaluation_item(
            message.item_id,
            worker_id=self.worker_id,
            lease_seconds=settings.eval_item_lease_seconds,
        )
        if item is None:
            eval_item_messages_total.labels(outcome="duplicate_or_terminal").inc()
            return
        evaluation = await self.db.get_evaluation(message.evaluation_id)
        if evaluation is None:
            await self.db.mark_evaluation_item_failed(
                message.item_id,
                {"error_type": "missing_evaluation", "retryable": False},
            )
            eval_item_dlq_total.labels(reason="missing_evaluation").inc()
            return

        await self.db.mark_evaluation_running(message.evaluation_id)
        rag_client = RAGClient(
            base_url=settings.chat_service_url,
            internal_token=settings.rag_internal_eval_token,
        )
        try:
            top_k = (
                (evaluation.get("config") or {})
                .get("effective_retrieval_config", {})
                .get("top_k", 5)
            )

            async def judge(row: dict):
                return await judge_generation_scores(
                    row=row,
                    provider=settings.llm_provider,
                    base_url=settings.llm_base_url,
                    model=settings.llm_model,
                    api_key=settings.llm_api_key,
                )

            evaluated = await self.evaluate_item_fn(
                item=item,
                rag_client=rag_client,
                collection=evaluation["collection"],
                rerank=False,
                top_k=top_k,
                judge=judge,
                run_context=EvalRunContext(
                    eval_id=message.evaluation_id,
                    collection=evaluation["collection"],
                    requested_rerank=False,
                ),
                answer_model=None,
                item_index=message.item_index,
            )
            await self.db.mark_evaluation_item_completed(
                message.item_id,
                result=evaluated["result"],
                scores=evaluated["scores"],
                score_reasons=evaluated["score_reasons"],
            )
            eval_item_messages_total.labels(outcome="completed").inc()
        except Exception as exc:
            error_type, retryable = classify_item_error(exc)
            attempts = item["attempt_count"]
            max_attempts = item["max_attempts"]
            if retryable and attempts < max_attempts:
                await self.db.release_evaluation_item_for_retry(
                    message.item_id,
                    {"error_type": error_type, "retryable": True},
                )
                eval_item_retries_total.labels(reason=error_type).inc()
                raise RetryableEvalItemError(str(exc)) from exc
            await self.db.mark_evaluation_item_failed(
                message.item_id,
                {"error_type": error_type, "retryable": False},
            )
            eval_item_dlq_total.labels(reason=error_type).inc()
        finally:
            await rag_client.close()

        await self.db.finalize_evaluation_if_terminal(message.evaluation_id)
```

Remove the unused `get_llm_provider` import if ruff reports it.

- [ ] **Step 5: Run worker tests**

Run:

```bash
PYTHONPATH=services pytest services/eval/tests/test_worker.py -v
```

Expected: pass.

- [ ] **Step 6: Commit**

```bash
git add services/eval/app/worker.py services/eval/app/metrics.py services/eval/tests/test_worker.py
git commit -m "feat(eval): process eval item jobs in worker"
```

## Task 7: Worker Entrypoint And RabbitMQ Consumer

**Files:**
- Modify: `services/eval/app/broker.py`
- Modify: `services/eval/app/worker.py`
- Test: `services/eval/tests/test_broker.py`

- [ ] **Step 1: Add failing decode validation test**

Add to `services/eval/tests/test_broker.py`:

```python
from app.broker import decode_eval_item_message


def test_decode_eval_item_message_round_trips():
    encoded = encode_eval_item_message(
        EvalItemMessage(
            evaluation_id="eval-1",
            item_id="item-1",
            item_index=2,
            attempt=4,
        )
    )

    decoded = decode_eval_item_message(encoded)

    assert decoded.evaluation_id == "eval-1"
    assert decoded.item_id == "item-1"
    assert decoded.item_index == 2
    assert decoded.attempt == 4
```

- [ ] **Step 2: Run broker tests**

Run:

```bash
PYTHONPATH=services pytest services/eval/tests/test_broker.py -v
```

Expected: pass if Task 4 already added decode; otherwise implement it now.

- [ ] **Step 3: Add broker consumer**

In `services/eval/app/broker.py`, add:

```python
class EvalItemConsumer:
    def __init__(self, rabbitmq_url: str, queue_name: str, dlq_name: str, prefetch: int):
        self.rabbitmq_url = rabbitmq_url
        self.queue_name = queue_name
        self.dlq_name = dlq_name
        self.prefetch = prefetch
        self._conn: AbstractRobustConnection | None = None
        self._channel: AbstractChannel | None = None

    async def connect(self) -> aio_pika.abc.AbstractQueue:
        self._conn = await aio_pika.connect_robust(self.rabbitmq_url)
        self._channel = await self._conn.channel()
        await self._channel.set_qos(prefetch_count=self.prefetch)
        await self._channel.declare_queue(self.dlq_name, durable=True)
        return await self._channel.declare_queue(
            self.queue_name,
            durable=True,
            arguments={
                "x-dead-letter-exchange": "",
                "x-dead-letter-routing-key": self.dlq_name,
            },
        )

    async def close(self) -> None:
        if self._conn is not None:
            await self._conn.close()
```

- [ ] **Step 4: Add `main()` worker loop**

Append to `services/eval/app/worker.py`:

```python
from app.broker import EvalItemConsumer, decode_eval_item_message


async def run_worker() -> None:
    db = EvalDB(settings.db_path)
    await db.init()
    consumer = EvalItemConsumer(
        rabbitmq_url=settings.rabbitmq_url,
        queue_name=settings.eval_item_queue,
        dlq_name=settings.eval_item_dlq,
        prefetch=settings.eval_worker_prefetch,
    )
    queue = await consumer.connect()
    processor = ItemProcessor(db=db)
    semaphore = asyncio.Semaphore(settings.eval_worker_concurrency)

    async def handle(message):
        async with semaphore:
            try:
                decoded = decode_eval_item_message(message.body)
                await processor.process(decoded)
            except RetryableEvalItemError:
                await message.nack(requeue=True)
                return
            except Exception:
                logger.exception("eval_item_message_failed")
                await message.reject(requeue=False)
                return
            await message.ack()

    try:
        await queue.consume(handle)
        await asyncio.Future()
    finally:
        await consumer.close()
        await db.close()


def main() -> None:
    if not settings.rabbitmq_url:
        raise RuntimeError("RABBITMQ_URL is required for eval worker")
    asyncio.run(run_worker())


if __name__ == "__main__":
    main()
```

- [ ] **Step 5: Run focused tests and import check**

Run:

```bash
PYTHONPATH=services pytest services/eval/tests/test_broker.py services/eval/tests/test_worker.py -v
PYTHONPATH=services python -m app.worker --help
```

Expected: tests pass. The import check may exit with `RuntimeError: RABBITMQ_URL is required for eval worker`; that is acceptable because the module imported and reached runtime config validation.

- [ ] **Step 6: Commit**

```bash
git add services/eval/app/broker.py services/eval/app/worker.py services/eval/tests/test_broker.py
git commit -m "feat(eval): add rabbitmq eval worker entrypoint"
```

## Task 8: Run Evidence And Progress Counts

**Files:**
- Modify: `services/eval/app/db.py`
- Modify: `services/eval/app/main.py`
- Modify: `services/eval/tests/test_main.py`

- [ ] **Step 1: Write failing API evidence test**

Add to `services/eval/tests/test_main.py`:

```python
@patch("app.main.get_db")
def test_get_evaluation_includes_item_summary(mock_get_db):
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
        "failed": 0,
    }
    mock_get_db.return_value = mock_db

    response = client.get("/evaluations/eval-456")

    assert response.status_code == 200
    assert response.json()["item_summary"] == {
        "queued": 1,
        "running": 1,
        "completed": 2,
        "failed": 0,
        "total": 4,
    }
```

- [ ] **Step 2: Run test and verify it fails**

Run:

```bash
PYTHONPATH=services pytest services/eval/tests/test_main.py::test_get_evaluation_includes_item_summary -v
```

Expected: fail because `item_summary` is not included.

- [ ] **Step 3: Add DB count helper**

In `services/eval/app/db.py`:

```python
async def count_evaluation_items_by_status(self, eval_id: str) -> dict[str, int]:
    cursor = await self._db.execute(
        "SELECT status, COUNT(*) AS count FROM evaluation_items "
        "WHERE evaluation_id = ? GROUP BY status",
        (eval_id,),
    )
    rows = await cursor.fetchall()
    return {row["status"]: row["count"] for row in rows}
```

- [ ] **Step 4: Add item summary to get-evaluation endpoint**

In `services/eval/app/main.py`, update `get_evaluation`:

```python
@app.get("/evaluations/{eval_id}", dependencies=[Depends(enforce_eval_read)])
async def get_evaluation(request: Request, eval_id: str):
    db = await get_db()
    evaluation = await db.get_evaluation(eval_id)
    if not evaluation:
        raise HTTPException(status_code=404, detail="Evaluation not found")
    counts = await db.count_evaluation_items_by_status(eval_id)
    if counts:
        evaluation["item_summary"] = {
            "queued": counts.get("queued", 0),
            "running": counts.get("running", 0),
            "completed": counts.get("completed", 0),
            "failed": counts.get("failed", 0),
            "total": sum(counts.values()),
        }
    return evaluation
```

- [ ] **Step 5: Run API tests**

Run:

```bash
PYTHONPATH=services pytest services/eval/tests/test_main.py -v
```

Expected: all tests pass.

- [ ] **Step 6: Commit**

```bash
git add services/eval/app/db.py services/eval/app/main.py services/eval/tests/test_main.py
git commit -m "feat(eval): expose evaluation item progress"
```

## Task 9: Local Compose And Container Wiring

**Files:**
- Inspect: `services/eval/Dockerfile`
- Modify: `docker-compose.yml`

- [ ] **Step 1: Verify Dockerfile supports worker command override**

No Dockerfile code change is required if the worker is invoked by overriding `command` in Compose/Kubernetes. Verify the existing file copies `eval/app` into `/app/app`; it does.

- [ ] **Step 2: Add RabbitMQ and eval worker to compose**

Modify `docker-compose.yml`:

```yaml
  rabbitmq:
    image: rabbitmq:3-management-alpine
    ports:
      - "127.0.0.1:5672:5672"
      - "127.0.0.1:15672:15672"
    healthcheck:
      test: ["CMD", "rabbitmq-diagnostics", "-q", "ping"]
      interval: 5s
      timeout: 3s
      retries: 10
    volumes:
      - rabbitmq_data:/var/lib/rabbitmq

  eval:
    environment:
      - RABBITMQ_URL=${EVAL_RABBITMQ_URL:-amqp://guest:guest@rabbitmq:5672/}
      - EVAL_ITEM_QUEUE=${EVAL_ITEM_QUEUE:-eval.item.requested}
      - EVAL_ITEM_DLQ=${EVAL_ITEM_DLQ:-eval.item.requested.dlq}
    depends_on:
      chat:
        condition: service_started
      rabbitmq:
        condition: service_healthy

  eval-worker:
    image: ghcr.io/kabradshaw1/portfolio/eval:latest
    build:
      context: ./services
      dockerfile: eval/Dockerfile
    command: ["python", "-m", "app.worker"]
    env_file: .env
    environment:
      - JWT_SECRET=${JWT_SECRET}
      - RABBITMQ_URL=${EVAL_RABBITMQ_URL:-amqp://guest:guest@rabbitmq:5672/}
      - EVAL_ITEM_QUEUE=${EVAL_ITEM_QUEUE:-eval.item.requested}
      - EVAL_ITEM_DLQ=${EVAL_ITEM_DLQ:-eval.item.requested.dlq}
      - RAG_INTERNAL_EVAL_TOKEN=${RAG_INTERNAL_EVAL_TOKEN:-}
      - OLLAMA_GENERATE_MAX_IN_FLIGHT=${OLLAMA_GENERATE_MAX_IN_FLIGHT:-2}
      - OLLAMA_ADMISSION_QUEUE_TIMEOUT=${OLLAMA_ADMISSION_QUEUE_TIMEOUT:-5.0}
    volumes:
      - eval_data:/app/data
    depends_on:
      rabbitmq:
        condition: service_healthy
      chat:
        condition: service_started
    extra_hosts:
      - "host.docker.internal:host-gateway"
```

Add volume:

```yaml
  rabbitmq_data:
```

- [ ] **Step 3: Validate compose config**

Run:

```bash
docker compose config >/tmp/eval-compose.yml
```

Expected: command exits 0.

- [ ] **Step 4: Commit**

```bash
git add docker-compose.yml
git commit -m "chore(eval): wire local rabbitmq eval worker"
```

## Task 10: Kubernetes Worker Manifests

**Files:**
- Modify: `k8s/ai-services/configmaps/eval-config.yml`
- Modify: `k8s/ai-services/deployments/eval.yml`
- Create: `k8s/ai-services/deployments/eval-worker.yml`
- Modify: `k8s/ai-services/kustomization.yaml`

- [ ] **Step 1: Add non-secret config**

Add to `k8s/ai-services/configmaps/eval-config.yml`:

```yaml
  EVAL_ITEM_QUEUE: eval.item.requested
  EVAL_ITEM_DLQ: eval.item.requested.dlq
  EVAL_WORKER_PREFETCH: "2"
  EVAL_WORKER_CONCURRENCY: "2"
  EVAL_ITEM_MAX_ATTEMPTS: "3"
  EVAL_ITEM_LEASE_SECONDS: "300.0"
  EVAL_STALE_ITEM_SECONDS: "900.0"
```

Do not add RabbitMQ credentials to this ConfigMap.

- [ ] **Step 2: Add secret env refs to eval API deployment**

In `k8s/ai-services/deployments/eval.yml`, add under `envFrom` or `env`:

```yaml
          env:
            - name: RABBITMQ_URL
              valueFrom:
                secretKeyRef:
                  name: eval-rabbitmq
                  key: url
                  optional: true
```

The `optional: true` keeps existing clusters deployable until the sealed secret is added through the repo's secret workflow.

- [ ] **Step 3: Add worker deployment**

Create `k8s/ai-services/deployments/eval-worker.yml`:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: eval-worker
  namespace: ai-services
spec:
  replicas: 1
  selector:
    matchLabels:
      app: eval-worker
  template:
    metadata:
      labels:
        app: eval-worker
    spec:
      imagePullSecrets:
        - name: ghcr-secret
      containers:
        - name: eval-worker
          image: ghcr.io/kabradshaw1/portfolio/eval:latest
          command: ["python", "-m", "app.worker"]
          envFrom:
            - configMapRef:
                name: eval-config
          env:
            - name: RABBITMQ_URL
              valueFrom:
                secretKeyRef:
                  name: eval-rabbitmq
                  key: url
                  optional: true
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
      volumes:
        - name: eval-data
          persistentVolumeClaim:
            claimName: eval-data
```

- [ ] **Step 4: Add to kustomization**

Add to `k8s/ai-services/kustomization.yaml` resources:

```yaml
  - deployments/eval-worker.yml
```

- [ ] **Step 5: Validate manifests locally**

Run:

```bash
kubectl kustomize k8s/ai-services >/tmp/ai-services-rendered.yml
```

Expected: command exits 0. This is a local render only, not a cluster mutation.

- [ ] **Step 6: Commit**

```bash
git add k8s/ai-services/configmaps/eval-config.yml k8s/ai-services/deployments/eval.yml k8s/ai-services/deployments/eval-worker.yml k8s/ai-services/kustomization.yaml
git commit -m "chore(eval): add kubernetes eval worker manifests"
```

## Task 11: Recovery And Republish Helpers

**Files:**
- Modify: `services/eval/app/db.py`
- Modify: `services/eval/app/main.py`
- Test: `services/eval/tests/test_db.py`

- [ ] **Step 1: Write failing recovery test**

Add to `services/eval/tests/test_db.py`:

```python
@pytest.mark.asyncio
async def test_reset_expired_running_items_to_queued(db):
    ds_id = await db.create_dataset(name="ds-expired", items=SIMPLE_ITEM)
    eval_id = await db.create_evaluation(ds_id, "documents", status="running")
    [item] = await db.create_evaluation_items(eval_id, SIMPLE_ITEM, max_attempts=3)
    await db.claim_evaluation_item(item["id"], worker_id="worker-1", lease_seconds=1)
    expired = (datetime.now(UTC) - timedelta(minutes=5)).isoformat()
    await db._db.execute(  # noqa: SLF001
        "UPDATE evaluation_items SET lease_expires_at = ? WHERE id = ?",
        (expired, item["id"]),
    )
    await db._db.commit()  # noqa: SLF001

    reset = await db.reset_expired_running_items(max_age_seconds=60)

    assert reset == 1
    stored = await db.get_evaluation_item(item["id"])
    assert stored["status"] == "queued"
    assert stored["lease_owner"] is None
```

- [ ] **Step 2: Run and verify failure**

Run:

```bash
PYTHONPATH=services pytest services/eval/tests/test_db.py::test_reset_expired_running_items_to_queued -v
```

Expected: fail because method is missing.

- [ ] **Step 3: Implement recovery DB method**

Add to `services/eval/app/db.py`:

```python
async def reset_expired_running_items(self, max_age_seconds: float) -> int:
    cutoff = (datetime.now(_UTC) - timedelta(seconds=max_age_seconds)).isoformat()
    cursor = await self._db.execute(
        "UPDATE evaluation_items "
        "SET status = 'queued', lease_owner = NULL, lease_expires_at = NULL, "
        "updated_at = ? "
        "WHERE status = 'running' AND lease_expires_at < ? "
        "AND attempt_count < max_attempts",
        (datetime.now(_UTC).isoformat(), cutoff),
    )
    await self._db.commit()
    return cursor.rowcount
```

- [ ] **Step 4: Add startup recovery call**

In `services/eval/app/main.py`, update `recover_stale_evaluations()` to reset expired items before failing whole runs:

```python
    reset_items = await db.reset_expired_running_items(settings.eval_stale_item_seconds)
    if reset_items:
        logger.warning("Recovered %s expired running evaluation item(s)", reset_items)
```

Republishing reset items can be a separate route or follow-up if direct publisher access is unavailable during startup. The first recovery slice must at least make stale items visible as queued again.

- [ ] **Step 5: Run tests**

Run:

```bash
PYTHONPATH=services pytest services/eval/tests/test_db.py services/eval/tests/test_main.py -v
```

Expected: all tests pass.

- [ ] **Step 6: Commit**

```bash
git add services/eval/app/db.py services/eval/app/main.py services/eval/tests/test_db.py
git commit -m "feat(eval): recover expired evaluation item leases"
```

## Task 12: Verification And Roadmap Issue Drafts

**Files:**
- Create: `docs/superpowers/plans/2026-05-20-rag-eval-roadmap-issues.md`

- [ ] **Step 1: Create roadmap issue draft doc**

Create `docs/superpowers/plans/2026-05-20-rag-eval-roadmap-issues.md` with:

```markdown
# RAG Eval Platform Roadmap Issue Drafts

## Issue 1: DLQ Triage And Replay Tooling

Build read-only DLQ inspection and explicit replay support for eval item jobs.
The tool must show message identifiers only, load detailed evidence from the
eval API, and require an explicit replay command.

Acceptance criteria:
- List eval item DLQ messages without removing them.
- Show evaluation id, item id, attempt count, and original routing metadata.
- Replay one selected DLQ item by id or index.
- Record replay attempts in metrics and logs.

## Issue 2: Cancellation And Stale Run Recovery

Add cancellation support and stronger stale run recovery for queued/running evals.

Acceptance criteria:
- API can mark a queued or running run as cancelled.
- Worker stops processing cancelled items before upstream calls.
- Expired item leases are reset and republished.
- Stale terminal aggregation is repaired automatically.

## Issue 3: Eval Dashboard Item Progress

Expose eval run item progress and failure causes in the frontend dashboard.

Acceptance criteria:
- Dashboard shows item counts by status.
- Dashboard shows failed item reason counts.
- Dashboard distinguishes completed from completed_with_failures.
- Dashboard links worst cases only for comparable completed runs.

## Issue 4: Kafka Eval Lifecycle Events

Publish eval lifecycle events for analytics and replayable experiment history.

Acceptance criteria:
- Publish bounded events for run queued/running/completed/failed.
- Publish bounded events for item completed/failed.
- Do not include raw query text or model secrets in event payloads.
- Add one consumer that produces useful analytics from the stream.

## Issue 5: LangGraph Eval Orchestrator Spike

Explore LangGraph inside the eval worker for multi-step judge, critique, and
failure diagnosis workflows.

Acceptance criteria:
- Keep RabbitMQ as the job substrate.
- Run graph orchestration inside one item worker claim.
- Compare direct judge vs graph judge on the same dataset.
- Record latency, cost, score stability, and failure-mode differences.
```

- [ ] **Step 2: Run focused eval tests**

Run:

```bash
PYTHONPATH=services pytest services/eval/tests -v
```

Expected: all eval tests pass.

- [ ] **Step 3: Run Python preflight**

Run:

```bash
make preflight-python
```

Expected: ruff, format checks, and Python tests pass.

- [ ] **Step 4: Run security preflight**

Run:

```bash
make preflight-security
```

Expected: security checks pass.

- [ ] **Step 5: Run compose and manifest validation**

Run:

```bash
docker compose config >/tmp/eval-compose.yml
kubectl kustomize k8s/ai-services >/tmp/ai-services-rendered.yml
```

Expected: both commands exit 0.

- [ ] **Step 6: Commit final docs**

```bash
git add docs/superpowers/plans/2026-05-20-rag-eval-roadmap-issues.md
git commit -m "docs: draft rag eval platform roadmap issues"
```

## Final Pre-PR Checklist

- [ ] `git status --short` shows only intentional files.
- [ ] `make preflight-python` passed.
- [ ] `make preflight-security` passed.
- [ ] `docker compose config >/tmp/eval-compose.yml` passed.
- [ ] `kubectl kustomize k8s/ai-services >/tmp/ai-services-rendered.yml` passed.
- [ ] The branch contains frequent commits matching the tasks above.
- [ ] The PR description calls out the Kubernetes `eval-rabbitmq` sealed secret requirement before enabling the worker in shared environments.
