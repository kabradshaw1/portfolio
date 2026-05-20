# RAG Eval DLQ Triage Replay Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add operator-only RAG eval item DLQ inspection and explicit replay tooling through the eval API and eval MCP service.

**Architecture:** `services/eval` owns RabbitMQ DLQ access, replay state transitions, redaction, metrics, and logs. The Go eval MCP service remains a thin workflow adapter over eval API endpoints and never connects directly to RabbitMQ.

**Tech Stack:** Python 3.11, FastAPI, aiosqlite, aio-pika, Prometheus client, pytest/pytest-asyncio, Go 1.x, MCP Go SDK, `net/http` tests.

---

## File Map

- Modify `services/eval/app/db.py`: add replay metadata columns and item reset helper.
- Modify `services/eval/app/broker.py`: add DLQ listing/replay primitives and safe DLQ entry models.
- Modify `services/eval/app/metrics.py`: add DLQ inspection and replay counters.
- Modify `services/eval/app/models.py`: add API response/request models for DLQ list and replay.
- Modify `services/eval/app/main.py`: add operator-only dependency plus DLQ list/replay endpoints.
- Modify `services/eval/tests/test_db.py`: add replay metadata tests.
- Modify `services/eval/tests/test_broker.py`: add DLQ peek/replay behavior tests with fakes.
- Modify `services/eval/tests/test_main.py`: add auth, redaction, replay endpoint tests.
- Modify `go/eval-mcp-service/internal/evalapi/client.go`: add DLQ DTOs and client methods.
- Modify `go/eval-mcp-service/internal/evalapi/client_test.go`: add API path/body tests.
- Modify `go/eval-mcp-service/internal/evalworkflow/service.go`: expose DLQ list/replay methods.
- Modify `go/eval-mcp-service/internal/evalworkflow/service_test.go`: add workflow delegation tests.
- Modify `go/eval-mcp-service/internal/mcpserver/server.go`: register MCP tools and schemas.
- Modify `go/eval-mcp-service/internal/mcpserver/server_test.go`: add schema and validation tests.

## Task 1: Eval DB Replay Metadata

**Files:**
- Modify: `services/eval/app/db.py`
- Test: `services/eval/tests/test_db.py`

- [ ] **Step 1: Add failing DB tests for replay metadata**

Append these tests to `services/eval/tests/test_db.py` near the existing evaluation item tests:

```python
@pytest.mark.asyncio
async def test_requeue_failed_item_for_replay_records_audit_state(db):
    ds_id = await db.create_dataset(
        name="ds-replay",
        items=[{"query": "q", "expected_answer": "a", "expected_sources": []}],
    )
    eval_id = await db.create_evaluation(
        dataset_id=ds_id,
        collection="documents",
        status="queued",
    )
    [item] = await db.create_evaluation_items(eval_id, (await db.get_dataset(ds_id))["items"], max_attempts=3)
    claimed = await db.claim_evaluation_item(item["id"], worker_id="worker-1", lease_seconds=30)
    assert claimed is not None
    await db.mark_evaluation_item_failed(
        item["id"], {"error_type": "TimeoutError", "retryable": False}
    )

    replayed = await db.requeue_failed_item_for_replay(item["id"])

    assert replayed is not None
    assert replayed["status"] == "queued"
    assert replayed["attempt_count"] == 1
    assert replayed["last_error"] == {"error_type": "TimeoutError", "retryable": False}
    assert replayed["replay_count"] == 1
    assert replayed["last_replayed_at"] is not None
    assert replayed["lease_owner"] is None
    assert replayed["lease_expires_at"] is None


@pytest.mark.asyncio
async def test_requeue_failed_item_for_replay_rejects_completed_item(db):
    ds_id = await db.create_dataset(
        name="ds-no-replay-completed",
        items=[{"query": "q", "expected_answer": "a", "expected_sources": []}],
    )
    eval_id = await db.create_evaluation(
        dataset_id=ds_id,
        collection="documents",
        status="queued",
    )
    [item] = await db.create_evaluation_items(eval_id, (await db.get_dataset(ds_id))["items"], max_attempts=3)
    await db.mark_evaluation_item_completed(
        item["id"],
        result={"query": "q", "answer": "a", "contexts": []},
        scores={"faithfulness": 1.0},
        score_reasons={},
    )

    replayed = await db.requeue_failed_item_for_replay(item["id"])

    assert replayed is None
```

- [ ] **Step 2: Run DB replay tests and verify they fail**

Run:

```bash
PYTHONPATH=services pytest services/eval/tests/test_db.py::test_requeue_failed_item_for_replay_records_audit_state services/eval/tests/test_db.py::test_requeue_failed_item_for_replay_rejects_completed_item -v
```

Expected: fail because `requeue_failed_item_for_replay` and replay columns do not exist.

- [ ] **Step 3: Add replay columns and row mapping**

In `services/eval/app/db.py`, add columns to the `evaluation_items` table definition:

```sql
replay_count INTEGER NOT NULL DEFAULT 0,
last_replayed_at TEXT,
```

Add idempotent migrations in `init()` after the existing `ALTER TABLE` statements:

```python
for column_ddl in (
    "ALTER TABLE evaluation_items ADD COLUMN replay_count INTEGER NOT NULL DEFAULT 0",
    "ALTER TABLE evaluation_items ADD COLUMN last_replayed_at TEXT",
):
    try:
        await self._db.execute(column_ddl)
    except aiosqlite.OperationalError as exc:
        if "duplicate column name" not in str(exc).lower():
            raise
```

Add these fields to `_item_row_to_dict()`:

```python
"replay_count": row["replay_count"],
"last_replayed_at": row["last_replayed_at"],
```

- [ ] **Step 4: Add failed-item replay reset helper**

Add this method to `EvalDB` after `release_evaluation_item_for_retry`:

```python
async def requeue_failed_item_for_replay(self, item_id: str) -> dict | None:
    now = datetime.now(_UTC).isoformat()
    cursor = await self._db.execute(
        "UPDATE evaluation_items "
        "SET status = 'queued', lease_owner = NULL, lease_expires_at = NULL, "
        "replay_count = replay_count + 1, last_replayed_at = ?, updated_at = ? "
        "WHERE id = ? AND status = 'failed'",
        (now, now, item_id),
    )
    await self._db.commit()
    if cursor.rowcount == 0:
        return None
    return await self.get_evaluation_item(item_id)
```

- [ ] **Step 5: Run DB tests**

Run:

```bash
PYTHONPATH=services pytest services/eval/tests/test_db.py -v
```

Expected: all DB tests pass.

- [ ] **Step 6: Commit DB replay metadata**

```bash
git add services/eval/app/db.py services/eval/tests/test_db.py
git commit -m "feat(eval): record item replay metadata"
```

## Task 2: RabbitMQ DLQ Broker Primitives

**Files:**
- Modify: `services/eval/app/broker.py`
- Test: `services/eval/tests/test_broker.py`

- [ ] **Step 1: Add failing broker tests for safe DLQ parsing**

Append to `services/eval/tests/test_broker.py`:

```python
from app.broker import (
    DLQRoutingMetadata,
    build_dlq_entry,
    safe_x_death_metadata,
)


class FakeIncomingMessage:
    def __init__(self, body, headers=None, redelivered=False, delivery_tag=7):
        self.body = body
        self.headers = headers or {}
        self.redelivered = redelivered
        self.delivery_tag = delivery_tag


def test_build_dlq_entry_redacts_payload_to_identifiers():
    message = FakeIncomingMessage(
        body=json.dumps(
            {
                "message_version": 1,
                "evaluation_id": "eval-1",
                "item_id": "item-1",
                "item_index": 2,
                "attempt": 3,
                "query": "secret query",
                "expected_answer": "secret answer",
            }
        ).encode("utf-8"),
        headers={
            "x-death": [
                {
                    "count": 2,
                    "reason": "rejected",
                    "exchange": "",
                    "routing-keys": ["eval.item.requested"],
                }
            ]
        },
    )

    entry = build_dlq_entry(index=0, message=message, dlq_name="eval.item.requested.dlq")

    assert entry.payload == {
        "message_version": 1,
        "evaluation_id": "eval-1",
        "item_id": "item-1",
        "item_index": 2,
        "attempt": 3,
    }
    assert entry.invalid_payload is None
    assert entry.routing == DLQRoutingMetadata(
        exchange="",
        routing_key="eval.item.requested",
        queue="eval.item.requested.dlq",
        death_count=2,
        death_reason="rejected",
    )
    assert "secret query" not in json.dumps(entry.__dict__)
    assert "secret answer" not in json.dumps(entry.__dict__)


def test_build_dlq_entry_marks_invalid_payload_without_body_text():
    entry = build_dlq_entry(
        index=0,
        message=FakeIncomingMessage(body=b"{not-json"),
        dlq_name="eval.item.requested.dlq",
    )

    assert entry.payload is None
    assert entry.invalid_payload == "invalid_json"
    assert "{not-json" not in json.dumps(entry.__dict__)
```

- [ ] **Step 2: Run broker parsing tests and verify they fail**

Run:

```bash
PYTHONPATH=services pytest services/eval/tests/test_broker.py::test_build_dlq_entry_redacts_payload_to_identifiers services/eval/tests/test_broker.py::test_build_dlq_entry_marks_invalid_payload_without_body_text -v
```

Expected: fail because DLQ entry helpers do not exist.

- [ ] **Step 3: Add safe DLQ dataclasses and parsing helpers**

In `services/eval/app/broker.py`, add:

```python
@dataclass(frozen=True)
class DLQRoutingMetadata:
    exchange: str
    routing_key: str
    queue: str
    death_count: int
    death_reason: str


@dataclass(frozen=True)
class DLQEntry:
    index: int
    delivery_tag: str
    redelivered: bool
    payload: dict[str, Any] | None
    routing: DLQRoutingMetadata
    invalid_payload: str | None = None


def safe_x_death_metadata(headers: dict[str, Any], dlq_name: str) -> DLQRoutingMetadata:
    deaths = headers.get("x-death") or []
    first = deaths[0] if deaths else {}
    routing_keys = first.get("routing-keys") or []
    routing_key = str(routing_keys[0]) if routing_keys else ""
    return DLQRoutingMetadata(
        exchange=str(first.get("exchange") or ""),
        routing_key=routing_key,
        queue=dlq_name,
        death_count=int(first.get("count") or 0),
        death_reason=str(first.get("reason") or ""),
    )


def _safe_payload_dict(decoded: EvalItemMessage) -> dict[str, Any]:
    return {
        "message_version": decoded.message_version,
        "evaluation_id": decoded.evaluation_id,
        "item_id": decoded.item_id,
        "item_index": decoded.item_index,
        "attempt": decoded.attempt,
    }


def build_dlq_entry(index: int, message: Any, dlq_name: str) -> DLQEntry:
    routing = safe_x_death_metadata(getattr(message, "headers", {}) or {}, dlq_name)
    try:
        decoded = decode_eval_item_message(message.body)
    except json.JSONDecodeError:
        return DLQEntry(
            index=index,
            delivery_tag=str(getattr(message, "delivery_tag", "")),
            redelivered=bool(getattr(message, "redelivered", False)),
            payload=None,
            routing=routing,
            invalid_payload="invalid_json",
        )
    except (KeyError, TypeError, ValueError):
        return DLQEntry(
            index=index,
            delivery_tag=str(getattr(message, "delivery_tag", "")),
            redelivered=bool(getattr(message, "redelivered", False)),
            payload=None,
            routing=routing,
            invalid_payload="invalid_schema",
        )
    return DLQEntry(
        index=index,
        delivery_tag=str(getattr(message, "delivery_tag", "")),
        redelivered=bool(getattr(message, "redelivered", False)),
        payload=_safe_payload_dict(decoded),
        routing=routing,
    )
```

- [ ] **Step 4: Add failing tests for peek and selected replay mechanics**

Append to `services/eval/tests/test_broker.py`:

```python
import pytest

from app.broker import EvalItemDLQClient


class FakeQueue:
    def __init__(self, messages):
        self.messages = list(messages)
        self.calls = 0

    async def get(self, fail=True, no_ack=False):
        assert no_ack is False
        if self.calls >= len(self.messages):
            return None
        message = self.messages[self.calls]
        self.calls += 1
        return message


class AckableMessage(FakeIncomingMessage):
    def __init__(self, body, headers=None):
        super().__init__(body=body, headers=headers)
        self.acked = False
        self.nacked = False

    async def ack(self):
        self.acked = True

    async def nack(self, requeue=True):
        assert requeue is True
        self.nacked = True


class FakeDLQClient(EvalItemDLQClient):
    def __init__(self, queue):
        self.dlq_name = "eval.item.requested.dlq"
        self._queue = queue

    async def _dlq_queue(self):
        return self._queue


@pytest.mark.asyncio
async def test_list_peeks_and_requeues_messages():
    body = encode_eval_item_message(
        EvalItemMessage("eval-1", "item-1", 0, 3)
    )
    msg = AckableMessage(body=body)
    client = FakeDLQClient(FakeQueue([msg]))

    entries = await client.list(limit=10)

    assert len(entries) == 1
    assert entries[0].payload["item_id"] == "item-1"
    assert msg.nacked is True
    assert msg.acked is False


@pytest.mark.asyncio
async def test_take_by_item_id_acks_only_selected_message_and_requeues_others():
    first = AckableMessage(
        body=encode_eval_item_message(EvalItemMessage("eval-1", "item-1", 0, 3))
    )
    second = AckableMessage(
        body=encode_eval_item_message(EvalItemMessage("eval-1", "item-2", 1, 3))
    )
    client = FakeDLQClient(FakeQueue([first, second]))

    taken = await client.take(item_id="item-2", index=None, scan_limit=10)

    assert taken is not None
    assert taken.entry.payload["item_id"] == "item-2"
    assert first.nacked is True
    assert second.acked is True
```

- [ ] **Step 5: Implement DLQ client list and take**

Add this class to `services/eval/app/broker.py`:

```python
@dataclass(frozen=True)
class TakenDLQMessage:
    entry: DLQEntry
    message: Any


class EvalItemDLQClient:
    def __init__(self, rabbitmq_url: str, dlq_name: str):
        self.rabbitmq_url = rabbitmq_url
        self.dlq_name = dlq_name
        self._conn: Any | None = None
        self._channel: Any | None = None

    async def connect(self) -> None:
        import aio_pika

        self._conn = await aio_pika.connect_robust(self.rabbitmq_url)
        self._channel = await self._conn.channel()
        await self._channel.declare_queue(self.dlq_name, durable=True)

    async def _dlq_queue(self) -> Any:
        if self._channel is None:
            await self.connect()
        assert self._channel is not None
        return await self._channel.declare_queue(self.dlq_name, durable=True)

    async def list(self, limit: int) -> list[DLQEntry]:
        queue = await self._dlq_queue()
        entries: list[DLQEntry] = []
        messages: list[Any] = []
        for index in range(limit):
            message = await queue.get(fail=False, no_ack=False)
            if message is None:
                break
            messages.append(message)
            entries.append(build_dlq_entry(index, message, self.dlq_name))
        for message in messages:
            await message.nack(requeue=True)
        return entries

    async def take(
        self, *, item_id: str | None, index: int | None, scan_limit: int
    ) -> TakenDLQMessage | None:
        queue = await self._dlq_queue()
        for current_index in range(scan_limit):
            message = await queue.get(fail=False, no_ack=False)
            if message is None:
                return None
            entry = build_dlq_entry(current_index, message, self.dlq_name)
            matches_index = index is not None and current_index == index
            matches_item = (
                item_id is not None
                and entry.payload is not None
                and entry.payload.get("item_id") == item_id
            )
            if matches_index or matches_item:
                await message.ack()
                return TakenDLQMessage(entry=entry, message=message)
            await message.nack(requeue=True)
        return None

    async def close(self) -> None:
        if self._conn is not None:
            await self._conn.close()
```

- [ ] **Step 6: Run broker tests**

Run:

```bash
PYTHONPATH=services pytest services/eval/tests/test_broker.py -v
```

Expected: all broker tests pass.

- [ ] **Step 7: Commit broker DLQ primitives**

```bash
git add services/eval/app/broker.py services/eval/tests/test_broker.py
git commit -m "feat(eval): add DLQ broker primitives"
```

## Task 3: Eval API DLQ Endpoints

**Files:**
- Modify: `services/eval/app/models.py`
- Modify: `services/eval/app/main.py`
- Modify: `services/eval/app/metrics.py`
- Test: `services/eval/tests/test_main.py`

- [ ] **Step 1: Add failing endpoint tests for operator-only access and listing redaction**

Append to `services/eval/tests/test_main.py`:

```python
@patch("app.main.get_dlq_client")
@patch("app.main.get_db")
def test_list_eval_item_dlq_requires_operator(mock_get_db, mock_get_dlq_client):
    response = client.get("/evaluations/items/dlq")

    assert response.status_code == 403
    mock_get_dlq_client.assert_not_called()


@patch("app.main.get_dlq_client")
@patch("app.main.get_db")
def test_operator_lists_eval_item_dlq_with_safe_evidence(mock_get_db, mock_get_dlq_client):
    mock_db = AsyncMock()
    mock_db.get_evaluation_item.return_value = {
        "id": "item-1",
        "evaluation_id": "eval-1",
        "item_index": 0,
        "query": "secret query",
        "expected_answer": "secret answer",
        "expected_sources": [],
        "status": "failed",
        "attempt_count": 3,
        "max_attempts": 3,
        "last_error": {"error_type": "TimeoutError", "retryable": False},
        "replay_count": 0,
        "last_replayed_at": None,
    }
    mock_db.get_evaluation.return_value = {
        "id": "eval-1",
        "status": "completed_with_failures",
        "collection": "documents",
        "created_at": "2026-05-20T00:00:00+00:00",
        "completed_at": "2026-05-20T00:01:00+00:00",
    }
    mock_get_db.return_value = mock_db
    entry = MagicMock()
    entry.index = 0
    entry.delivery_tag = "7"
    entry.redelivered = False
    entry.payload = {
        "message_version": 1,
        "evaluation_id": "eval-1",
        "item_id": "item-1",
        "item_index": 0,
        "attempt": 3,
    }
    entry.routing.__dict__ = {
        "exchange": "",
        "routing_key": "eval.item.requested",
        "queue": "eval.item.requested.dlq",
        "death_count": 1,
        "death_reason": "rejected",
    }
    entry.invalid_payload = None
    dlq_client = AsyncMock()
    dlq_client.list.return_value = [entry]
    mock_get_dlq_client.return_value = dlq_client

    response = client.get(
        "/evaluations/items/dlq",
        headers={"Authorization": "Bearer operator-token"},
    )

    assert response.status_code == 200
    body = response.json()
    encoded = json.dumps(body)
    assert body["entries"][0]["payload"]["item_id"] == "item-1"
    assert body["entries"][0]["item"]["last_error"]["error_type"] == "TimeoutError"
    assert body["indexes_are_transient"] is True
    assert "secret query" not in encoded
    assert "secret answer" not in encoded
```

In the test fixture setup, monkeypatch auth so `"operator-token"` resolves to `AuthContext(subject="op", email=None, tier="operator")`. Follow existing rate-limit fixture patterns in the file.

- [ ] **Step 2: Run list endpoint tests and verify they fail**

Run:

```bash
PYTHONPATH=services pytest services/eval/tests/test_main.py::test_list_eval_item_dlq_requires_operator services/eval/tests/test_main.py::test_operator_lists_eval_item_dlq_with_safe_evidence -v
```

Expected: fail because the endpoint and operator dependency do not exist.

- [ ] **Step 3: Add Pydantic models**

In `services/eval/app/models.py`, add:

```python
class DLQPayload(BaseModel):
    message_version: int
    evaluation_id: str
    item_id: str
    item_index: int
    attempt: int


class DLQRouting(BaseModel):
    exchange: str
    routing_key: str
    queue: str
    death_count: int
    death_reason: str


class DLQItemEvidence(BaseModel):
    evaluation_id: str
    item_id: str
    item_index: int
    status: str
    attempt_count: int
    max_attempts: int
    last_error: dict[str, Any] | None = None
    replay_count: int = 0
    last_replayed_at: str | None = None


class DLQEvaluationEvidence(BaseModel):
    status: str
    collection: str | None = None
    created_at: str
    completed_at: str | None = None


class DLQEntryResponse(BaseModel):
    index: int
    delivery_tag: str
    redelivered: bool
    payload: DLQPayload | None = None
    routing: DLQRouting
    item: DLQItemEvidence | None = None
    evaluation: DLQEvaluationEvidence | None = None
    invalid_payload: str | None = None


class DLQListResponse(BaseModel):
    entries: list[DLQEntryResponse]
    indexes_are_transient: bool = True


class ReplayDLQItemRequest(BaseModel):
    item_id: str | None = None
    index: int | None = Field(default=None, ge=0)

    @model_validator(mode="after")
    def exactly_one_selector(self) -> "ReplayDLQItemRequest":
        selectors = [self.item_id is not None, self.index is not None]
        if selectors.count(True) != 1:
            raise ValueError("provide exactly one of item_id or index")
        return self


class ReplayDLQItemResponse(BaseModel):
    evaluation_id: str
    item_id: str
    item_index: int
    status: str
    replay_count: int
    message_published: bool
```

- [ ] **Step 4: Add metrics and operator dependency**

In `services/eval/app/metrics.py`, add:

```python
eval_item_dlq_inspections_total = Counter(
    "eval_item_dlq_inspections_total",
    "Evaluation item DLQ inspections by outcome",
    ["outcome"],
)

eval_item_dlq_replays_total = Counter(
    "eval_item_dlq_replays_total",
    "Evaluation item DLQ replays by outcome",
    ["outcome"],
)
```

In `services/eval/app/main.py`, add:

```python
async def enforce_eval_operator(request: Request) -> AuthContext:
    context = await _enforce_eval_rate_limit("eval_write", request)
    if context.tier != "operator":
        raise HTTPException(status_code=403, detail="operator access required")
    return context
```

- [ ] **Step 5: Add DLQ client factory and safe evidence helper**

In `services/eval/app/main.py`, import `EvalItemDLQClient` and add:

```python
_dlq_client: EvalItemDLQClient | None = None


async def get_dlq_client() -> EvalItemDLQClient:
    global _dlq_client
    if _dlq_client is None:
        if not settings.rabbitmq_url:
            raise HTTPException(status_code=503, detail="eval queue is not configured")
        _dlq_client = EvalItemDLQClient(
            rabbitmq_url=settings.rabbitmq_url,
            dlq_name=settings.eval_item_dlq,
        )
    return _dlq_client
```

Close `_dlq_client` in `shutdown()`.

Add helper:

```python
async def _hydrate_dlq_entry(db: EvalDB, entry) -> dict:
    payload = entry.payload
    item_evidence = None
    eval_evidence = None
    if payload is not None:
        item = await db.get_evaluation_item(payload["item_id"])
        if item is not None:
            item_evidence = {
                "evaluation_id": item["evaluation_id"],
                "item_id": item["id"],
                "item_index": item["item_index"],
                "status": item["status"],
                "attempt_count": item["attempt_count"],
                "max_attempts": item["max_attempts"],
                "last_error": item["last_error"],
                "replay_count": item.get("replay_count", 0),
                "last_replayed_at": item.get("last_replayed_at"),
            }
        evaluation = await db.get_evaluation(payload["evaluation_id"])
        if evaluation is not None:
            eval_evidence = {
                "status": evaluation["status"],
                "collection": evaluation["collection"],
                "created_at": evaluation["created_at"],
                "completed_at": evaluation["completed_at"],
            }
    return {
        "index": entry.index,
        "delivery_tag": entry.delivery_tag,
        "redelivered": entry.redelivered,
        "payload": payload,
        "routing": entry.routing.__dict__,
        "item": item_evidence,
        "evaluation": eval_evidence,
        "invalid_payload": entry.invalid_payload,
    }
```

- [ ] **Step 6: Add DLQ listing endpoint**

In `services/eval/app/main.py`, add before `/evaluations/{eval_id}` routes so static path matching wins:

```python
@app.get(
    "/evaluations/items/dlq",
    response_model=DLQListResponse,
    dependencies=[Depends(enforce_eval_operator)],
)
async def list_eval_item_dlq(request: Request, limit: int = 20):
    bounded_limit = min(max(limit, 1), 200)
    db = await get_db()
    dlq = await get_dlq_client()
    entries = await dlq.list(limit=bounded_limit)
    outcome = "empty" if not entries else "success"
    eval_item_dlq_inspections_total.labels(outcome=outcome).inc()
    return {
        "entries": [await _hydrate_dlq_entry(db, entry) for entry in entries],
        "indexes_are_transient": True,
    }
```

- [ ] **Step 7: Run list endpoint tests**

Run:

```bash
PYTHONPATH=services pytest services/eval/tests/test_main.py::test_list_eval_item_dlq_requires_operator services/eval/tests/test_main.py::test_operator_lists_eval_item_dlq_with_safe_evidence -v
```

Expected: both tests pass.

- [ ] **Step 8: Add failing replay endpoint tests**

Append to `services/eval/tests/test_main.py`:

```python
@patch("app.main.publish_evaluation_items", new_callable=AsyncMock)
@patch("app.main.get_dlq_client")
@patch("app.main.get_db")
def test_operator_replays_dlq_item_by_item_id(mock_get_db, mock_get_dlq_client, mock_publish):
    mock_db = AsyncMock()
    mock_db.get_evaluation_item.return_value = {"id": "item-1", "status": "failed"}
    mock_db.requeue_failed_item_for_replay.return_value = {
        "id": "item-1",
        "evaluation_id": "eval-1",
        "item_index": 0,
        "status": "queued",
        "attempt_count": 3,
        "replay_count": 1,
    }
    mock_get_db.return_value = mock_db
    entry = MagicMock()
    entry.payload = {
        "message_version": 1,
        "evaluation_id": "eval-1",
        "item_id": "item-1",
        "item_index": 0,
        "attempt": 3,
    }
    entry.routing.routing_key = "eval.item.requested"
    dlq = AsyncMock()
    dlq.take.return_value = MagicMock(entry=entry)
    mock_get_dlq_client.return_value = dlq

    response = client.post(
        "/evaluations/items/dlq/replay",
        json={"item_id": "item-1"},
        headers={"Authorization": "Bearer operator-token"},
    )

    assert response.status_code == 200
    assert response.json()["item_id"] == "item-1"
    mock_db.requeue_failed_item_for_replay.assert_awaited_once_with("item-1")
    mock_publish.assert_awaited_once_with(
        "eval-1",
        [{"id": "item-1", "item_index": 0, "attempt_count": 3}],
    )


@patch("app.main.get_dlq_client")
@patch("app.main.get_db")
def test_replay_rejects_non_failed_item(mock_get_db, mock_get_dlq_client):
    mock_db = AsyncMock()
    mock_db.get_evaluation_item.return_value = {"id": "item-1", "status": "completed"}
    mock_get_db.return_value = mock_db
    entry = MagicMock()
    entry.payload = {
        "message_version": 1,
        "evaluation_id": "eval-1",
        "item_id": "item-1",
        "item_index": 0,
        "attempt": 3,
    }
    dlq = AsyncMock()
    dlq.take.return_value = MagicMock(entry=entry)
    mock_get_dlq_client.return_value = dlq

    response = client.post(
        "/evaluations/items/dlq/replay",
        json={"item_id": "item-1"},
        headers={"Authorization": "Bearer operator-token"},
    )

    assert response.status_code == 409
    assert response.json()["detail"] == "evaluation item is not failed"
```

- [ ] **Step 9: Implement replay endpoint**

In `services/eval/app/main.py`, add:

```python
@app.post(
    "/evaluations/items/dlq/replay",
    response_model=ReplayDLQItemResponse,
    dependencies=[Depends(enforce_eval_operator)],
)
async def replay_eval_item_dlq(request: Request, body: ReplayDLQItemRequest):
    db = await get_db()
    dlq = await get_dlq_client()
    taken = await dlq.take(
        item_id=body.item_id,
        index=body.index,
        scan_limit=200,
    )
    if taken is None:
        eval_item_dlq_replays_total.labels(outcome="not_found").inc()
        raise HTTPException(status_code=404, detail="DLQ message not found")
    entry = taken.entry
    if entry.payload is None:
        eval_item_dlq_replays_total.labels(outcome="invalid_payload").inc()
        raise HTTPException(status_code=400, detail="invalid DLQ payload")
    item_id = entry.payload["item_id"]
    item = await db.get_evaluation_item(item_id)
    if item is None:
        eval_item_dlq_replays_total.labels(outcome="not_found").inc()
        raise HTTPException(status_code=404, detail="evaluation item not found")
    if item["status"] != "failed":
        eval_item_dlq_replays_total.labels(outcome="not_failed").inc()
        raise HTTPException(status_code=409, detail="evaluation item is not failed")
    replayed = await db.requeue_failed_item_for_replay(item_id)
    if replayed is None:
        eval_item_dlq_replays_total.labels(outcome="not_failed").inc()
        raise HTTPException(status_code=409, detail="evaluation item is not failed")
    try:
        await publish_evaluation_items(
            replayed["evaluation_id"],
            [
                {
                    "id": replayed["id"],
                    "item_index": replayed["item_index"],
                    "attempt_count": replayed["attempt_count"],
                }
            ],
        )
    except Exception as exc:
        eval_item_dlq_replays_total.labels(outcome="publish_failed").inc()
        logger.warning(
            "eval_item_dlq_replay_publish_failed eval_id=%s item_id=%s item_index=%s",
            replayed["evaluation_id"],
            replayed["id"],
            replayed["item_index"],
        )
        raise HTTPException(status_code=503, detail="unable to publish replay") from exc
    eval_item_dlq_replays_total.labels(outcome="success").inc()
    logger.info(
        "eval_item_dlq_replayed eval_id=%s item_id=%s item_index=%s selector=%s routing_key=%s",
        replayed["evaluation_id"],
        replayed["id"],
        replayed["item_index"],
        "item_id" if body.item_id is not None else "index",
        entry.routing.routing_key,
    )
    return {
        "evaluation_id": replayed["evaluation_id"],
        "item_id": replayed["id"],
        "item_index": replayed["item_index"],
        "status": replayed["status"],
        "replay_count": replayed["replay_count"],
        "message_published": True,
    }
```

Update `publish_evaluation_items` in the same file so replayed items publish the next explicit attempt value:

```python
attempt=item.get("attempt_count", 0) + 1
```

- [ ] **Step 10: Run endpoint tests**

Run:

```bash
PYTHONPATH=services pytest services/eval/tests/test_main.py::test_operator_replays_dlq_item_by_item_id services/eval/tests/test_main.py::test_replay_rejects_non_failed_item -v
```

Expected: both tests pass.

- [ ] **Step 11: Run Python eval service tests**

Run:

```bash
PYTHONPATH=services pytest services/eval/tests -v
```

Expected: all eval service tests pass.

- [ ] **Step 12: Commit eval API DLQ endpoints**

```bash
git add services/eval/app/main.py services/eval/app/metrics.py services/eval/app/models.py services/eval/tests/test_main.py
git commit -m "feat(eval): expose item DLQ triage replay API"
```

## Task 4: Go Eval API Client Methods

**Files:**
- Modify: `go/eval-mcp-service/internal/evalapi/client.go`
- Test: `go/eval-mcp-service/internal/evalapi/client_test.go`

- [ ] **Step 1: Add failing client tests**

Append to `go/eval-mcp-service/internal/evalapi/client_test.go`:

```go
func TestListEvalItemDLQ(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/evaluations/items/dlq" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if r.URL.Query().Get("limit") != "5" {
			t.Fatalf("limit = %q", r.URL.Query().Get("limit"))
		}
		_ = json.NewEncoder(w).Encode(DLQListResponse{
			IndexesAreTransient: true,
			Entries: []DLQEntry{{
				Index: 0,
				Payload: &DLQPayload{
					MessageVersion: 1,
					EvaluationID:   "eval-1",
					ItemID:         "item-1",
					ItemIndex:      0,
					Attempt:        3,
				},
			}},
		})
	}))
	defer server.Close()

	client := New(server.URL, "", server.Client())
	got, err := client.ListEvalItemDLQ(context.Background(), 5)
	if err != nil {
		t.Fatalf("ListEvalItemDLQ error: %v", err)
	}
	if len(got.Entries) != 1 || got.Entries[0].Payload.ItemID != "item-1" || !got.IndexesAreTransient {
		t.Fatalf("response = %#v", got)
	}
}

func TestReplayEvalItemDLQByItemID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/evaluations/items/dlq/replay" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var body ReplayDLQItemRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body.ItemID != "item-1" || body.Index != nil {
			t.Fatalf("body = %#v", body)
		}
		_ = json.NewEncoder(w).Encode(ReplayDLQItemResponse{
			EvaluationID:      "eval-1",
			ItemID:            "item-1",
			ItemIndex:         0,
			Status:            "queued",
			ReplayCount:       1,
			MessagePublished:  true,
		})
	}))
	defer server.Close()

	client := New(server.URL, "", server.Client())
	got, err := client.ReplayEvalItemDLQ(context.Background(), ReplayDLQItemRequest{ItemID: "item-1"})
	if err != nil {
		t.Fatalf("ReplayEvalItemDLQ error: %v", err)
	}
	if got.ItemID != "item-1" || !got.MessagePublished {
		t.Fatalf("response = %#v", got)
	}
}
```

- [ ] **Step 2: Run client tests and verify they fail**

Run:

```bash
cd go/eval-mcp-service && go test ./internal/evalapi -run 'Test(ListEvalItemDLQ|ReplayEvalItemDLQByItemID)' -v
```

Expected: fail because DTOs and methods do not exist.

- [ ] **Step 3: Add Go DTOs and client methods**

In `go/eval-mcp-service/internal/evalapi/client.go`, add structs near the other DTOs:

```go
type DLQPayload struct {
	MessageVersion int    `json:"message_version"`
	EvaluationID   string `json:"evaluation_id"`
	ItemID         string `json:"item_id"`
	ItemIndex      int    `json:"item_index"`
	Attempt        int    `json:"attempt"`
}

type DLQRouting struct {
	Exchange    string `json:"exchange"`
	RoutingKey  string `json:"routing_key"`
	Queue       string `json:"queue"`
	DeathCount  int    `json:"death_count"`
	DeathReason string `json:"death_reason"`
}

type DLQItemEvidence struct {
	EvaluationID   string         `json:"evaluation_id"`
	ItemID         string         `json:"item_id"`
	ItemIndex      int            `json:"item_index"`
	Status         string         `json:"status"`
	AttemptCount   int            `json:"attempt_count"`
	MaxAttempts    int            `json:"max_attempts"`
	LastError      map[string]any `json:"last_error"`
	ReplayCount    int            `json:"replay_count"`
	LastReplayedAt *string        `json:"last_replayed_at"`
}

type DLQEvaluationEvidence struct {
	Status      string  `json:"status"`
	Collection *string `json:"collection"`
	CreatedAt   string  `json:"created_at"`
	CompletedAt *string `json:"completed_at"`
}

type DLQEntry struct {
	Index          int                    `json:"index"`
	DeliveryTag    string                 `json:"delivery_tag"`
	Redelivered    bool                   `json:"redelivered"`
	Payload        *DLQPayload            `json:"payload"`
	Routing        DLQRouting             `json:"routing"`
	Item           *DLQItemEvidence       `json:"item"`
	Evaluation     *DLQEvaluationEvidence `json:"evaluation"`
	InvalidPayload *string                `json:"invalid_payload"`
}

type DLQListResponse struct {
	Entries             []DLQEntry `json:"entries"`
	IndexesAreTransient bool       `json:"indexes_are_transient"`
}

type ReplayDLQItemRequest struct {
	ItemID string `json:"item_id,omitempty"`
	Index  *int   `json:"index,omitempty"`
}

type ReplayDLQItemResponse struct {
	EvaluationID     string `json:"evaluation_id"`
	ItemID           string `json:"item_id"`
	ItemIndex        int    `json:"item_index"`
	Status           string `json:"status"`
	ReplayCount      int    `json:"replay_count"`
	MessagePublished bool   `json:"message_published"`
}
```

Add methods:

```go
func (c *Client) ListEvalItemDLQ(ctx context.Context, limit int) (DLQListResponse, error) {
	values := url.Values{}
	if limit > 0 {
		values.Set("limit", fmt.Sprintf("%d", limit))
	}
	path := "/evaluations/items/dlq"
	if encoded := values.Encode(); encoded != "" {
		path += "?" + encoded
	}
	var response DLQListResponse
	if err := c.do(ctx, http.MethodGet, path, nil, &response); err != nil {
		return DLQListResponse{}, err
	}
	return response, nil
}

func (c *Client) ReplayEvalItemDLQ(ctx context.Context, body ReplayDLQItemRequest) (ReplayDLQItemResponse, error) {
	var response ReplayDLQItemResponse
	if err := c.do(ctx, http.MethodPost, "/evaluations/items/dlq/replay", body, &response); err != nil {
		return ReplayDLQItemResponse{}, err
	}
	return response, nil
}
```

- [ ] **Step 4: Run eval API client tests**

Run:

```bash
cd go/eval-mcp-service && go test ./internal/evalapi -v
```

Expected: all eval API client tests pass.

- [ ] **Step 5: Commit Go eval API client**

```bash
git add go/eval-mcp-service/internal/evalapi/client.go go/eval-mcp-service/internal/evalapi/client_test.go
git commit -m "feat(eval-mcp): add DLQ eval API client"
```

## Task 5: Go Workflow And MCP Tools

**Files:**
- Modify: `go/eval-mcp-service/internal/evalworkflow/service.go`
- Modify: `go/eval-mcp-service/internal/evalworkflow/service_test.go`
- Modify: `go/eval-mcp-service/internal/mcpserver/server.go`
- Modify: `go/eval-mcp-service/internal/mcpserver/server_test.go`

- [ ] **Step 1: Add failing workflow tests**

In `go/eval-mcp-service/internal/evalworkflow/service_test.go`, extend `fakeAPI` with:

```go
dlqListRequestLimit int
dlqListResponse     evalapi.DLQListResponse
dlqReplayRequest    evalapi.ReplayDLQItemRequest
dlqReplayResponse   evalapi.ReplayDLQItemResponse
```

Add methods:

```go
func (f *fakeAPI) ListEvalItemDLQ(_ context.Context, limit int) (evalapi.DLQListResponse, error) {
	f.dlqListRequestLimit = limit
	return f.dlqListResponse, nil
}

func (f *fakeAPI) ReplayEvalItemDLQ(_ context.Context, in evalapi.ReplayDLQItemRequest) (evalapi.ReplayDLQItemResponse, error) {
	f.dlqReplayRequest = in
	return f.dlqReplayResponse, nil
}
```

Add tests:

```go
func TestListEvalItemDLQDelegatesToAPI(t *testing.T) {
	api := &fakeAPI{dlqListResponse: evalapi.DLQListResponse{IndexesAreTransient: true}}
	svc := New(api, nil, nil, time.Second, time.Minute)

	got, err := svc.ListEvalItemDLQ(context.Background(), 7)
	if err != nil {
		t.Fatalf("ListEvalItemDLQ error: %v", err)
	}
	if api.dlqListRequestLimit != 7 || !got.IndexesAreTransient {
		t.Fatalf("limit=%d response=%#v", api.dlqListRequestLimit, got)
	}
}

func TestReplayEvalItemDLQDelegatesToAPI(t *testing.T) {
	api := &fakeAPI{dlqReplayResponse: evalapi.ReplayDLQItemResponse{ItemID: "item-1", MessagePublished: true}}
	svc := New(api, nil, nil, time.Second, time.Minute)

	got, err := svc.ReplayEvalItemDLQ(context.Background(), evalapi.ReplayDLQItemRequest{ItemID: "item-1"})
	if err != nil {
		t.Fatalf("ReplayEvalItemDLQ error: %v", err)
	}
	if api.dlqReplayRequest.ItemID != "item-1" || !got.MessagePublished {
		t.Fatalf("request=%#v response=%#v", api.dlqReplayRequest, got)
	}
}
```

- [ ] **Step 2: Update workflow API interface and methods**

In `go/eval-mcp-service/internal/evalworkflow/service.go`, add to the eval API interface:

```go
ListEvalItemDLQ(context.Context, int) (evalapi.DLQListResponse, error)
ReplayEvalItemDLQ(context.Context, evalapi.ReplayDLQItemRequest) (evalapi.ReplayDLQItemResponse, error)
```

Add service methods:

```go
func (s *Service) ListEvalItemDLQ(ctx context.Context, limit int) (evalapi.DLQListResponse, error) {
	return s.api.ListEvalItemDLQ(ctx, limit)
}

func (s *Service) ReplayEvalItemDLQ(ctx context.Context, in evalapi.ReplayDLQItemRequest) (evalapi.ReplayDLQItemResponse, error) {
	return s.api.ReplayEvalItemDLQ(ctx, in)
}
```

- [ ] **Step 3: Run workflow tests**

Run:

```bash
cd go/eval-mcp-service && go test ./internal/evalworkflow -run 'Test(ListEvalItemDLQDelegatesToAPI|ReplayEvalItemDLQDelegatesToAPI)' -v
```

Expected: both tests pass.

- [ ] **Step 4: Add failing MCP server validation tests**

In `go/eval-mcp-service/internal/mcpserver/server_test.go`, extend `fakeEvalService` with:

```go
listDLQLimit      int
listDLQResponse   evalapi.DLQListResponse
replayDLQRequest  evalapi.ReplayDLQItemRequest
replayDLQResponse evalapi.ReplayDLQItemResponse
```

Add methods to `fakeEvalService`:

```go
func (f *fakeEvalService) ListEvalItemDLQ(_ context.Context, limit int) (evalapi.DLQListResponse, error) {
	f.listDLQLimit = limit
	return f.listDLQResponse, nil
}

func (f *fakeEvalService) ReplayEvalItemDLQ(_ context.Context, in evalapi.ReplayDLQItemRequest) (evalapi.ReplayDLQItemResponse, error) {
	f.replayDLQRequest = in
	return f.replayDLQResponse, nil
}
```

Update `TestServerRegistersPromptResourceAndTools` expected tool names to include the new tools in sorted order:

```go
wantTools := []string{
	"attach_eval_run",
	"compare_eval_runs",
	"create_eval_dataset",
	"get_eval_experiment",
	"get_eval_run",
	"get_eval_run_evidence",
	"get_rag_collection_config",
	"get_worst_eval_cases",
	"list_eval_dataset_fixtures",
	"list_eval_datasets",
	"list_eval_experiments",
	"list_eval_item_dlq",
	"list_rag_collections",
	"record_eval_experiment_conclusion",
	"replay_eval_item_dlq",
	"start_eval_experiment",
	"start_eval_run",
	"summarize_eval_experiment",
	"triage_rag_regression",
	"wait_for_eval_run",
}
```

Add these handler tests:

```go
func TestReplayEvalItemDLQRejectsMissingSelector(t *testing.T) {
	service := &fakeEvalService{}
	result, err := replayEvalItemDLQHandler(service)(context.Background(), callReq(map[string]any{}))
	if err != nil {
		t.Fatalf("handler returned transport error: %v", err)
	}

	if !result.IsError {
		t.Fatalf("expected MCP tool error")
	}
	if got := textResult(t, result); !strings.Contains(got, "provide exactly one of item_id or index") {
		t.Fatalf("result = %#v", result)
	}
}

func TestReplayEvalItemDLQRejectsBothSelectors(t *testing.T) {
	service := &fakeEvalService{}
	result, err := replayEvalItemDLQHandler(service)(context.Background(), callReq(map[string]any{
		"item_id": "item-1",
		"index": 0,
	}))
	if err != nil {
		t.Fatalf("handler returned transport error: %v", err)
	}

	if !result.IsError {
		t.Fatalf("expected MCP tool error")
	}
	if got := textResult(t, result); !strings.Contains(got, "provide exactly one of item_id or index") {
		t.Fatalf("result = %#v", result)
	}
}

func TestReplayEvalItemDLQCallsServiceByItemID(t *testing.T) {
	service := &fakeEvalService{
		replayDLQResponse: evalapi.ReplayDLQItemResponse{ItemID: "item-1", MessagePublished: true},
	}
	result, err := replayEvalItemDLQHandler(service)(context.Background(), callReq(map[string]any{
		"item_id": "item-1",
	}))
	if err != nil {
		t.Fatalf("handler returned transport error: %v", err)
	}
	if result.IsError {
		t.Fatalf("handler returned MCP error: %#v", result)
	}

	if service.replayDLQRequest.ItemID != "item-1" || !strings.Contains(textResult(t, result), "item-1") {
		t.Fatalf("request=%#v result=%#v", service.replayDLQRequest, result)
	}
}
```

- [ ] **Step 5: Register MCP interface methods and tools**

In `go/eval-mcp-service/internal/mcpserver/server.go`, add to `EvalService`:

```go
ListEvalItemDLQ(context.Context, int) (evalapi.DLQListResponse, error)
ReplayEvalItemDLQ(context.Context, evalapi.ReplayDLQItemRequest) (evalapi.ReplayDLQItemResponse, error)
```

In `New()`, register tools:

```go
addTool(srv, "list_eval_item_dlq", "List eval item DLQ messages without removing them.", listEvalItemDLQSchema(), listEvalItemDLQHandler(service))
addTool(srv, "replay_eval_item_dlq", "Explicitly replay one selected eval item DLQ message. This is mutating.", replayEvalItemDLQSchema(), replayEvalItemDLQHandler(service))
```

Add schemas:

```go
func listEvalItemDLQSchema() json.RawMessage {
	return json.RawMessage(`{
		"type":"object",
		"properties":{"limit":{"type":"integer","minimum":1,"maximum":200}},
		"additionalProperties":false
	}`)
}

func replayEvalItemDLQSchema() json.RawMessage {
	return json.RawMessage(`{
		"type":"object",
		"properties":{
			"item_id":{"type":"string","minLength":1},
			"index":{"type":"integer","minimum":0}
		},
		"additionalProperties":false
	}`)
}
```

Add handlers:

```go
func listEvalItemDLQHandler(service EvalService) sdkmcp.ToolHandler {
	return func(ctx context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		var args struct {
			Limit int `json:"limit,omitempty"`
		}
		if err := decodeArgs(req, &args); err != nil {
			return toolError(err.Error()), nil
		}
		result, err := service.ListEvalItemDLQ(ctx, args.Limit)
		return resultOrError(result, err), nil
	}
}

func replayEvalItemDLQHandler(service EvalService) sdkmcp.ToolHandler {
	return func(ctx context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		var args struct {
			ItemID string `json:"item_id,omitempty"`
			Index  *int   `json:"index,omitempty"`
		}
		if err := decodeArgs(req, &args); err != nil {
			return toolError(err.Error()), nil
		}
		hasItemID := strings.TrimSpace(args.ItemID) != ""
		hasIndex := args.Index != nil
		if hasItemID == hasIndex {
			return toolError("provide exactly one of item_id or index"), nil
		}
		result, err := service.ReplayEvalItemDLQ(ctx, evalapi.ReplayDLQItemRequest{
			ItemID: strings.TrimSpace(args.ItemID),
			Index:  args.Index,
		})
		return resultOrError(result, err), nil
	}
}
```

- [ ] **Step 6: Run MCP service tests**

Run:

```bash
cd go/eval-mcp-service && go test ./internal/evalworkflow ./internal/mcpserver -v
```

Expected: workflow and MCP server tests pass.

- [ ] **Step 7: Commit MCP tools**

```bash
git add go/eval-mcp-service/internal/evalworkflow/service.go go/eval-mcp-service/internal/evalworkflow/service_test.go go/eval-mcp-service/internal/mcpserver/server.go go/eval-mcp-service/internal/mcpserver/server_test.go
git commit -m "feat(eval-mcp): expose eval item DLQ tools"
```

## Task 6: Final Verification

**Files:**
- Verify all changed files from previous tasks.

- [ ] **Step 1: Run targeted Python tests**

Run:

```bash
PYTHONPATH=services pytest services/eval/tests -v
```

Expected: all eval Python tests pass.

- [ ] **Step 2: Run targeted Go tests**

Run:

```bash
cd go/eval-mcp-service && go test ./... -v
```

Expected: all eval MCP Go tests pass.

- [ ] **Step 3: Run required preflights before final commit or PR**

Run from repo root:

```bash
make preflight-python
make preflight-go
make preflight-security
```

Expected: all required preflights pass. If a preflight is blocked by a local tool or platform issue, capture the exact error and leave that verification to CI.

- [ ] **Step 4: Review redaction and payload safety**

Run:

```bash
rg -n "query|expected_answer|answer|contexts|api_key|secret" services/eval/app/broker.py services/eval/app/main.py go/eval-mcp-service/internal
```

Expected: matches are either existing eval result code or explicit redaction/validation code. DLQ list/replay response types and logs must not include raw query text, expected answers, generated answers, contexts, API keys, secret names, or model secret values.

- [ ] **Step 5: Check git status and recent commits**

Run:

```bash
git status --short
git log --oneline -6
```

Expected: only intended changes are present, and task commits are visible.
