import asyncio
import json
import time
from unittest.mock import AsyncMock, MagicMock, patch

import httpx
import jwt
import pytest
from app.config import settings
from app.main import _run_evaluation_task, app, recover_stale_evaluations
from fastapi import HTTPException
from fastapi.testclient import TestClient
from shared.auth import AuthContext

client = TestClient(app)
SECRET = "eval-test-secret-at-least-32-bytes"


def _token(sub: str, email: str) -> str:
    return jwt.encode(
        {"sub": sub, "email": email, "exp": int(time.time()) + 3600},
        SECRET,
        algorithm="HS256",
    )


@pytest.fixture
def configured_eval_limits(monkeypatch):
    import app.main as main

    monkeypatch.setenv("RAG_OPERATOR_EMAILS", "operator@example.test")
    monkeypatch.setattr(main.settings, "jwt_secret", SECRET)
    monkeypatch.setattr(main.settings, "eval_rate_limit_read_operator", "2/minute")
    monkeypatch.setattr(main.settings, "eval_rate_limit_read_user", "1/minute")
    monkeypatch.setattr(main.settings, "eval_rate_limit_run_create_user", "1/minute")
    main.eval_rate_limiter = main.build_eval_rate_limiter()
    main.eval_rate_limiter.enabled = True
    yield
    main.eval_rate_limiter = main.build_eval_rate_limiter()
    main.eval_rate_limiter.enabled = False


@pytest.fixture(autouse=True)
def fake_eval_item_publisher(monkeypatch):
    import app.main as main

    publisher = AsyncMock()
    monkeypatch.setattr(main, "get_item_publisher", AsyncMock(return_value=publisher))
    yield publisher
    main._item_publisher = None


@pytest.fixture
def dlq_operator_auth(monkeypatch):
    import app.main as main

    async def fake_auth_context(request):
        if request.headers.get("Authorization") == "Bearer operator-token":
            return AuthContext(subject="op", email=None, tier="operator")
        return AuthContext(subject="anonymous", email=None, tier="anonymous")

    monkeypatch.setattr(main, "_resolve_auth_context", fake_auth_context)


def test_metrics_contains_eval_observability_metrics():
    response = client.get("/metrics")

    assert response.status_code == 200
    body = response.text
    assert "eval_run_duration_seconds" in body
    assert "eval_item_duration_seconds" in body
    assert "eval_upstream_request_duration_seconds" in body
    assert "eval_upstream_failures_total" in body
    assert "eval_runs_total" in body
    assert "eval_items_total" in body
    assert "eval_queue_publish_total" in body
    assert "eval_stale_running_runs" in body


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
        {
            "id": "ds-1",
            "name": "ds1",
            "created_at": "2026-04-16T00:00:00Z",
            "item_count": 1,
        },
        {
            "id": "ds-2",
            "name": "ds2",
            "created_at": "2026-04-16T01:00:00Z",
            "item_count": 2,
        },
    ]
    mock_get_db.return_value = mock_db

    response = client.get("/datasets")

    assert response.status_code == 200
    assert response.json() == {
        "datasets": [
            {
                "id": "ds-1",
                "name": "ds1",
                "created_at": "2026-04-16T00:00:00Z",
                "item_count": 1,
            },
            {
                "id": "ds-2",
                "name": "ds2",
                "created_at": "2026-04-16T01:00:00Z",
                "item_count": 2,
            },
        ]
    }


# --- Evaluation endpoints ---


@patch("app.main.validate_collection_exists", new_callable=AsyncMock)
@patch("app.main.get_db")
def test_start_evaluation(mock_get_db, mock_validate_collection):
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


@patch("app.main.validate_collection_exists", new_callable=AsyncMock)
@patch("app.main.get_db")
def test_start_evaluation_logs_run_context(
    mock_get_db, mock_validate_collection, caplog
):
    mock_db = AsyncMock()
    mock_db.get_dataset.return_value = {
        "id": "ds-123",
        "name": "test",
        "items": [{"query": "q", "expected_answer": "a", "expected_sources": []}],
        "created_at": "2026-04-16T00:00:00Z",
    }
    mock_db.create_evaluation.return_value = "eval-456"
    mock_get_db.return_value = mock_db

    with caplog.at_level("INFO", logger="app.main"):
        response = client.post(
            "/evaluations",
            json={"dataset_id": "ds-123", "collection": "documents", "rerank": True},
        )

    assert response.status_code == 202
    assert "evaluation_start_accepted" in caplog.text
    assert "eval-456" in caplog.text
    assert "documents" in caplog.text


@patch("app.main.get_db")
def test_start_evaluation_dataset_not_found(mock_get_db):
    mock_db = AsyncMock()
    mock_db.get_dataset.return_value = None
    mock_get_db.return_value = mock_db

    response = client.post(
        "/evaluations",
        json={"dataset_id": "nonexistent", "baseline_eval_id": "eval-prev"},
    )
    assert response.status_code == 404
    assert response.json()["detail"] == "Dataset not found"
    mock_db.get_evaluation.assert_not_awaited()


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


@patch("app.main.get_dlq_client")
@patch("app.main.get_db")
def test_list_eval_item_dlq_requires_operator(
    mock_get_db, mock_get_dlq_client, dlq_operator_auth
):
    response = client.get("/evaluations/items/dlq")

    assert response.status_code == 403
    mock_get_dlq_client.assert_not_called()


@patch("app.main.get_dlq_client")
@patch("app.main.get_db")
def test_operator_lists_eval_item_dlq_with_safe_evidence(
    mock_get_db, mock_get_dlq_client, dlq_operator_auth
):
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


@patch("app.main.publish_evaluation_items", new_callable=AsyncMock)
@patch("app.main.get_dlq_client")
@patch("app.main.get_db")
def test_operator_replays_dlq_item_by_item_id(
    mock_get_db, mock_get_dlq_client, mock_publish, dlq_operator_auth
):
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
def test_replay_rejects_non_failed_item(
    mock_get_db, mock_get_dlq_client, dlq_operator_auth
):
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


@patch("app.main.get_db")
def test_get_evaluation(mock_get_db):
    mock_db = AsyncMock()
    mock_db.get_evaluation.return_value = {
        "id": "eval-456",
        "dataset_id": "ds-123",
        "status": "completed",
        "collection": "documents",
        "aggregate_scores": {"faithfulness": 0.87, "answer_relevancy": 0.92},
        "results": [
            {
                "query": "q",
                "answer": "a",
                "contexts": [],
                "scores": {"faithfulness": 0.87},
            }
        ],
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
    assert response.json()["item_counts"] == {
        "queued": 1,
        "running": 1,
        "completed": 2,
        "failed": 0,
        "total": 4,
    }
    assert response.json()["item_summary"] == {
        "queued": 1,
        "running": 1,
        "completed": 2,
        "failed": 0,
        "total": 4,
    }


@patch("app.main.get_db")
def test_get_evaluation_not_found(mock_get_db):
    mock_db = AsyncMock()
    mock_db.get_evaluation.return_value = None
    mock_get_db.return_value = mock_db

    response = client.get("/evaluations/nonexistent")
    assert response.status_code == 404


@patch("app.main.get_db")
def test_operator_can_poll_more_than_normal_user(mock_get_db, configured_eval_limits):
    mock_db = AsyncMock()
    mock_db.get_evaluation.return_value = _baseline_run()
    mock_get_db.return_value = mock_db
    operator_headers = {
        "Authorization": f"Bearer {_token('op-1', 'operator@example.test')}"
    }
    user_headers = {"Authorization": f"Bearer {_token('u-1', 'user@example.test')}"}

    first_operator_response = client.get(
        "/evaluations/eval-1", headers=operator_headers
    )
    second_operator_response = client.get(
        "/evaluations/eval-1", headers=operator_headers
    )
    assert first_operator_response.status_code == 200
    assert second_operator_response.status_code == 200
    assert client.get("/evaluations/eval-1", headers=user_headers).status_code == 200
    denied = client.get("/evaluations/eval-1", headers=user_headers)

    assert denied.status_code == 429
    assert int(denied.headers["Retry-After"]) > 0


@patch("app.main.validate_collection_exists", new_callable=AsyncMock)
@patch("app.main.get_db")
def test_start_evaluation_uses_run_create_quota(
    mock_get_db, mock_validate_collection, configured_eval_limits
):
    mock_db = AsyncMock()
    mock_db.get_dataset.return_value = {
        "id": "ds-123",
        "name": "test",
        "items": [{"query": "q", "expected_answer": "a", "expected_sources": []}],
        "created_at": "2026-04-16T00:00:00Z",
    }
    mock_db.create_evaluation.return_value = "eval-456"
    mock_get_db.return_value = mock_db
    headers = {"Authorization": f"Bearer {_token('u-create', 'user@example.test')}"}

    first = client.post("/evaluations", json={"dataset_id": "ds-123"}, headers=headers)
    denied = client.post("/evaluations", json={"dataset_id": "ds-123"}, headers=headers)

    assert first.status_code == 202
    assert denied.status_code == 429
    assert int(denied.headers["Retry-After"]) > 0


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


@patch("app.main.validate_collection_exists", new_callable=AsyncMock)
@patch("app.main.get_db")
def test_start_evaluation_persists_notes_and_baseline(
    mock_get_db, mock_validate_collection
):
    mock_db = AsyncMock()
    mock_db.get_dataset.return_value = {
        "id": "ds-123",
        "name": "test",
        "items": [{"query": "q", "expected_answer": "a", "expected_sources": []}],
        "created_at": "2026-04-16T00:00:00Z",
    }
    mock_db.get_evaluation.return_value = _baseline_run()
    mock_db.create_evaluation.return_value = "eval-789"
    mock_get_db.return_value = mock_db

    response = client.post(
        "/evaluations",
        json={
            "dataset_id": "ds-123",
            "notes": "bumped chunk overlap to 300",
            "baseline_eval_id": "eval-prev",
        },
    )
    assert response.status_code == 202
    mock_db.get_evaluation.assert_awaited_once_with("eval-prev")
    mock_db.create_evaluation.assert_awaited_once_with(
        dataset_id="ds-123",
        collection="documents",
        notes="bumped chunk overlap to 300",
        baseline_eval_id="eval-prev",
        status="queued",
    )


@patch("app.main.validate_collection_exists", new_callable=AsyncMock)
@patch("app.main.get_db")
def test_start_evaluation_allows_completed_with_failures_baseline(
    mock_get_db, mock_validate_collection
):
    mock_db = AsyncMock()
    mock_db.get_dataset.return_value = {
        "id": "ds-123",
        "name": "test",
        "items": [{"query": "q", "expected_answer": "a", "expected_sources": []}],
        "created_at": "2026-04-16T00:00:00Z",
    }
    mock_db.get_evaluation.return_value = _baseline_run(
        status="completed_with_failures"
    )
    mock_db.create_evaluation.return_value = "eval-789"
    mock_get_db.return_value = mock_db

    response = client.post(
        "/evaluations",
        json={"dataset_id": "ds-123", "baseline_eval_id": "eval-prev"},
    )

    assert response.status_code == 202
    mock_db.create_evaluation.assert_awaited_once()


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
    mock_db.create_evaluation.return_value = "eval-new"
    mock_get_db.return_value = mock_db

    response = client.post(
        "/evaluations",
        json={"dataset_id": "ds-123", "baseline_eval_id": "missing-eval"},
    )

    assert response.status_code == 404
    assert response.json()["detail"] == "Baseline evaluation not found"
    mock_db.get_evaluation.assert_awaited_once_with("missing-eval")
    mock_db.create_evaluation.assert_not_awaited()


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
    mock_db.create_evaluation.return_value = "eval-new"
    mock_get_db.return_value = mock_db

    response = client.post(
        "/evaluations",
        json={"dataset_id": "ds-123", "baseline_eval_id": "eval-prev"},
    )

    assert response.status_code == 400
    assert response.json()["detail"] == "Baseline evaluation must be completed"
    mock_db.create_evaluation.assert_not_awaited()


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
    mock_db.create_evaluation.return_value = "eval-new"
    mock_get_db.return_value = mock_db

    response = client.post(
        "/evaluations",
        json={"dataset_id": "ds-123", "baseline_eval_id": "eval-prev"},
    )

    assert response.status_code == 400
    assert response.json()["detail"] == "Baseline evaluation must be completed"
    mock_db.create_evaluation.assert_not_awaited()


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
    mock_db.create_evaluation.return_value = "eval-new"
    mock_get_db.return_value = mock_db

    response = client.post(
        "/evaluations",
        json={"dataset_id": "ds-123", "baseline_eval_id": "eval-prev"},
    )

    assert response.status_code == 400
    assert response.json()["detail"] == "Baseline evaluation must use the same dataset"
    mock_db.create_evaluation.assert_not_awaited()


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
    mock_db.create_evaluation.return_value = "eval-new"
    mock_get_db.return_value = mock_db

    response = client.post(
        "/evaluations",
        json={"dataset_id": "ds-123", "baseline_eval_id": "eval-prev"},
    )

    assert response.status_code == 400
    assert (
        response.json()["detail"] == "Baseline evaluation must use the same collection"
    )
    mock_db.create_evaluation.assert_not_awaited()


@patch("app.main.validate_collection_exists", new_callable=AsyncMock)
@patch("app.main.get_db")
def test_start_evaluation_accepts_valid_baseline_for_custom_collection(
    mock_get_db, mock_validate_collection
):
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
        status="queued",
    )


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


@patch("app.main.get_db")
def test_create_experiment_persists_focus_metric(mock_get_db):
    mock_db = AsyncMock()
    mock_db.get_dataset.return_value = {
        "id": "ds-123",
        "name": "test",
        "items": [{"query": "q", "expected_answer": "a", "expected_sources": []}],
        "created_at": "2026-04-16T00:00:00Z",
    }
    mock_db.create_experiment.return_value = "exp-1"
    mock_db.get_experiment.return_value = {
        "id": "exp-1",
        "name": "precision tuning",
        "hypothesis": "Reranking improves context precision",
        "dataset_id": "ds-123",
        "collection": "documents",
        "baseline_eval_id": None,
        "focus_metric": "context_precision",
        "status": "running",
        "decision": None,
        "conclusion": None,
        "evidence": None,
        "notes": None,
        "created_at": "2026-05-15T00:00:00Z",
        "updated_at": "2026-05-15T00:00:00Z",
        "runs": [],
    }
    mock_get_db.return_value = mock_db

    response = client.post(
        "/experiments",
        json={
            "name": "precision tuning",
            "hypothesis": "Reranking improves context precision",
            "dataset_id": "ds-123",
            "collection": "documents",
            "focus_metric": "context_precision",
            "status": "running",
        },
    )

    assert response.status_code == 201
    mock_db.create_experiment.assert_awaited_once_with(
        name="precision tuning",
        hypothesis="Reranking improves context precision",
        dataset_id="ds-123",
        collection="documents",
        baseline_eval_id=None,
        focus_metric="context_precision",
        status="running",
        notes=None,
    )
    assert response.json()["focus_metric"] == "context_precision"


@patch("app.main.get_db")
def test_update_experiment_can_complete_with_evidence(mock_get_db):
    mock_db = AsyncMock()
    mock_db.get_experiment.side_effect = [
        {
            "id": "exp-1",
            "name": "precision tuning",
            "hypothesis": "Reranking improves context precision",
            "dataset_id": "ds-123",
            "collection": "documents",
            "baseline_eval_id": None,
            "focus_metric": "context_precision",
            "status": "running",
            "decision": None,
            "conclusion": None,
            "evidence": None,
            "notes": None,
            "created_at": "2026-05-15T00:00:00Z",
            "updated_at": "2026-05-15T00:00:00Z",
            "runs": [],
        },
        {
            "id": "exp-1",
            "name": "precision tuning",
            "hypothesis": "Reranking improves context precision",
            "dataset_id": "ds-123",
            "collection": "documents",
            "baseline_eval_id": None,
            "focus_metric": "context_precision",
            "status": "completed",
            "decision": "keep",
            "conclusion": "Keep reranking.",
            "evidence": {
                "baseline_eval_id": "eval-base",
                "candidate_eval_ids": ["eval-candidate"],
            },
            "notes": None,
            "created_at": "2026-05-15T00:00:00Z",
            "updated_at": "2026-05-15T00:05:00Z",
            "runs": [],
        },
    ]
    mock_get_db.return_value = mock_db

    evidence = {
        "baseline_eval_id": "eval-base",
        "candidate_eval_ids": ["eval-candidate"],
    }
    response = client.patch(
        "/experiments/exp-1",
        json={
            "status": "completed",
            "decision": "keep",
            "conclusion": "Keep reranking.",
            "evidence": evidence,
        },
    )

    assert response.status_code == 200
    mock_db.update_experiment.assert_awaited_once_with(
        "exp-1",
        hypothesis=None,
        baseline_eval_id=None,
        focus_metric=None,
        status="completed",
        decision="keep",
        conclusion="Keep reranking.",
        evidence=evidence,
        notes=None,
    )
    assert response.json()["evidence"] == evidence


@patch("app.main.get_db")
def test_update_experiment_rejects_completed_without_decision_conclusion_or_evidence(
    mock_get_db,
):
    mock_db = AsyncMock()
    mock_db.get_experiment.return_value = {
        "id": "exp-1",
        "name": "precision tuning",
        "hypothesis": "Reranking improves context precision",
        "dataset_id": "ds-123",
        "collection": "documents",
        "baseline_eval_id": None,
        "focus_metric": "context_precision",
        "status": "running",
        "decision": None,
        "conclusion": None,
        "evidence": None,
        "notes": None,
        "created_at": "2026-05-15T00:00:00Z",
        "updated_at": "2026-05-15T00:00:00Z",
        "runs": [],
    }
    mock_get_db.return_value = mock_db

    response = client.patch("/experiments/exp-1", json={"status": "completed"})

    assert response.status_code == 400
    assert response.json()["detail"] == "completed experiments require a decision"
    mock_db.update_experiment.assert_not_awaited()

    response = client.patch(
        "/experiments/exp-1",
        json={"status": "completed", "decision": "keep"},
    )

    assert response.status_code == 400
    assert response.json()["detail"] == "completed experiments require a conclusion"
    mock_db.update_experiment.assert_not_awaited()

    response = client.patch(
        "/experiments/exp-1",
        json={
            "status": "completed",
            "decision": "keep",
            "conclusion": "Keep reranking.",
        },
    )

    assert response.status_code == 400
    assert response.json()["detail"] == "completed experiments require evidence"
    mock_db.update_experiment.assert_not_awaited()


@patch("app.main.run_evaluation", new_callable=AsyncMock)
@patch("app.main.capture_run_config", new_callable=AsyncMock)
@patch("app.main.validate_collection_exists", new_callable=AsyncMock)
@patch("app.main.get_db")
def test_run_persists_config_snapshot(
    mock_get_db, mock_validate_collection, mock_capture, mock_run_evaluation
):
    mock_db = AsyncMock()
    mock_db.get_dataset.return_value = {
        "id": "ds-cfg",
        "name": "test",
        "items": [{"query": "q", "expected_answer": "a", "expected_sources": []}],
        "created_at": "2026-04-16T00:00:00Z",
    }
    mock_db.create_evaluation.return_value = "eval-cfg-1"
    mock_get_db.return_value = mock_db

    captured_config = {
        "chat": {"llm_model": "qwen2.5:14b", "top_k": 5},
        "collection": {"chunk_size": 1000, "chunk_overlap": 200},
        "captured_at": "2026-04-28T00:00:00+00:00",
    }
    mock_capture.return_value = captured_config
    mock_run_evaluation.return_value = ({"faithfulness": 0.9}, [])

    response = client.post(
        "/evaluations",
        json={"dataset_id": "ds-cfg", "collection": "documents"},
    )
    assert response.status_code == 202

    mock_capture.assert_awaited_once()
    call_kwargs = mock_capture.await_args.kwargs
    assert call_kwargs["collection"] == "documents"
    mock_db.set_evaluation_config.assert_awaited_once_with(
        "eval-cfg-1", captured_config
    )


@patch("app.main.run_evaluation", new_callable=AsyncMock)
@patch("app.main.capture_run_config", new_callable=AsyncMock)
@patch("app.main.validate_collection_exists", new_callable=AsyncMock)
@patch("app.main.get_db")
def test_run_uses_default_collection_when_none_provided(
    mock_get_db, mock_validate_collection, mock_capture, mock_run_evaluation
):
    mock_db = AsyncMock()
    mock_db.get_dataset.return_value = {
        "id": "ds-d",
        "name": "test",
        "items": [{"query": "q", "expected_answer": "a", "expected_sources": []}],
        "created_at": "2026-04-16T00:00:00Z",
    }
    mock_db.create_evaluation.return_value = "eval-d"
    mock_get_db.return_value = mock_db
    mock_capture.return_value = {"captured_at": "x"}
    mock_run_evaluation.return_value = ({"faithfulness": 0.5}, [])

    client.post("/evaluations", json={"dataset_id": "ds-d"})

    assert mock_capture.await_args.kwargs["collection"] == "documents"


@patch("app.main.run_evaluation", new_callable=AsyncMock)
@patch("app.main.capture_run_config", new_callable=AsyncMock)
@patch("app.main.validate_collection_exists", new_callable=AsyncMock)
@patch("app.main.get_db")
def test_start_evaluation_passes_rerank_to_background_run(
    mock_get_db, mock_validate_collection, mock_capture, mock_run_evaluation
):
    mock_db = AsyncMock()
    mock_db.get_dataset.return_value = {
        "id": "ds-rerank",
        "name": "test",
        "items": [{"query": "q", "expected_answer": "a", "expected_sources": []}],
        "created_at": "2026-04-16T00:00:00Z",
    }
    mock_db.create_evaluation.return_value = "eval-rerank"
    mock_get_db.return_value = mock_db
    mock_capture.return_value = {"captured_at": "x"}
    mock_run_evaluation.return_value = ({"faithfulness": 0.8}, [])

    response = client.post(
        "/evaluations",
        json={"dataset_id": "ds-rerank", "rerank": True},
    )

    assert response.status_code == 202
    assert mock_capture.await_args.kwargs["requested_rerank"] is True
    assert mock_capture.await_args.kwargs["collection"] == "documents"
    mock_run_evaluation.assert_not_awaited()


@patch("app.main.run_evaluation", new_callable=AsyncMock)
@patch("app.main.capture_run_config", new_callable=AsyncMock)
@patch("app.main.validate_collection_exists", new_callable=AsyncMock)
@patch("app.main.get_db")
def test_start_evaluation_passes_retrieval_config_to_background_run(
    mock_get_db, mock_validate_collection, mock_capture, mock_run_evaluation
):
    mock_db = AsyncMock()
    mock_db.get_dataset.return_value = {
        "id": "ds-top-k",
        "name": "test",
        "items": [{"query": "q", "expected_answer": "a", "expected_sources": []}],
        "created_at": "2026-04-16T00:00:00Z",
    }
    mock_db.create_evaluation.return_value = "eval-top-k"
    mock_get_db.return_value = mock_db
    mock_capture.return_value = {
        "captured_at": "x",
        "chat": {"top_k": 5},
        "effective_retrieval_config": {"top_k": 3},
    }
    mock_run_evaluation.return_value = ({"faithfulness": 0.8}, [])

    response = client.post(
        "/evaluations",
        json={"dataset_id": "ds-top-k", "retrieval_config": {"top_k": 3}},
    )

    assert response.status_code == 202
    assert mock_capture.await_args.kwargs["requested_retrieval_config"] == {"top_k": 3}
    mock_run_evaluation.assert_not_awaited()


@patch("app.main.resolve_answer_model_override")
@patch("app.main.run_evaluation", new_callable=AsyncMock)
@patch("app.main.capture_run_config", new_callable=AsyncMock)
@patch("app.main.validate_collection_exists", new_callable=AsyncMock)
@patch("app.main.get_db")
def test_start_evaluation_resolves_and_captures_answer_model_override(
    mock_get_db,
    mock_validate_collection,
    mock_capture,
    mock_run_evaluation,
    mock_resolve,
):
    mock_db = AsyncMock()
    mock_db.get_dataset.return_value = {
        "id": "ds-model",
        "name": "test",
        "items": [{"query": "q", "expected_answer": "a", "expected_sources": []}],
        "created_at": "2026-04-16T00:00:00Z",
    }
    mock_db.create_evaluation.return_value = "eval-model"
    mock_get_db.return_value = mock_db
    resolved = MagicMock()
    resolved.safe_dict.return_value = {
        "tier": "efficient",
        "provider": "openai",
        "base_url": "https://api.openai.com/v1",
        "model": "gpt-5.4-mini",
        "api_key_secret": "OPENAI_API_KEY",
    }
    resolved.tier = "efficient"
    resolved.provider = "openai"
    resolved.base_url = "https://api.openai.com/v1"
    resolved.model = "gpt-5.4-mini"
    resolved.api_key = "test-key"
    mock_resolve.return_value = resolved
    mock_capture.return_value = {"captured_at": "x"}
    mock_run_evaluation.return_value = ({"faithfulness": 0.8}, [])

    response = client.post(
        "/evaluations",
        json={
            "dataset_id": "ds-model",
            "answer_tier": "efficient",
            "answer_provider": "openai",
            "answer_base_url": "https://api.openai.com/v1",
            "answer_model": "gpt-5.4-mini",
            "answer_api_key_secret": "OPENAI_API_KEY",
        },
    )

    assert response.status_code == 202
    assert (
        mock_capture.await_args.kwargs["requested_answer_model"]["model"]
        == "gpt-5.4-mini"
    )
    mock_run_evaluation.assert_not_awaited()
    assert mock_capture.await_args.kwargs["judge_model"] == {
        "provider": settings.llm_provider,
        "base_url": settings.llm_base_url,
        "model": settings.llm_model,
    }


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


@pytest.mark.asyncio
@patch("app.main.RAGClient")
@patch("app.main.run_evaluation", new_callable=AsyncMock)
@patch("app.main.capture_run_config", new_callable=AsyncMock)
@patch("app.main.get_db")
async def test_run_evaluation_task_marks_overall_timeout_failed(
    mock_get_db, mock_capture, mock_run_evaluation, mock_rag_client, monkeypatch
):
    mock_db = AsyncMock()
    mock_get_db.return_value = mock_db
    mock_capture.return_value = {"captured_at": "x"}
    mock_rag_client.return_value.close = AsyncMock()

    async def slow_evaluation(**_kwargs):
        await asyncio.sleep(1)
        return {"faithfulness": 1.0}, []

    mock_run_evaluation.side_effect = slow_evaluation
    monkeypatch.setattr(settings, "eval_run_max_seconds", 0.001)

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


@pytest.mark.asyncio
@patch("app.main.get_db")
async def test_recover_stale_evaluations_uses_max_runtime_plus_grace(mock_get_db):
    mock_db = AsyncMock()
    mock_db.count_stale_running_evaluations.return_value = 2
    mock_db.fail_stale_running_evaluations.return_value = 2
    mock_get_db.return_value = mock_db

    await recover_stale_evaluations()

    expected_age = settings.eval_run_max_seconds + settings.eval_stale_grace_seconds
    mock_db.count_stale_running_evaluations.assert_awaited_once_with(expected_age)
    mock_db.fail_stale_running_evaluations.assert_awaited_once_with(expected_age)


@patch("app.main.validate_collection_exists", new_callable=AsyncMock)
@patch("app.main.get_db")
def test_start_evaluation_omits_optional_fields(mock_get_db, mock_validate_collection):
    mock_db = AsyncMock()
    mock_db.get_dataset.return_value = {
        "id": "ds-123",
        "name": "test",
        "items": [{"query": "q", "expected_answer": "a", "expected_sources": []}],
        "created_at": "2026-04-16T00:00:00Z",
    }
    mock_db.create_evaluation.return_value = "eval-noopt"
    mock_get_db.return_value = mock_db

    response = client.post("/evaluations", json={"dataset_id": "ds-123"})
    assert response.status_code == 202
    mock_db.create_evaluation.assert_awaited_once_with(
        dataset_id="ds-123",
        collection="documents",
        notes=None,
        baseline_eval_id=None,
        status="queued",
    )
    mock_validate_collection.assert_awaited_once_with(
        settings.ingestion_service_url, "documents"
    )


@patch("app.main.validate_collection_exists", new_callable=AsyncMock)
@patch("app.main.get_db")
def test_start_evaluation_attaches_run_to_experiment(
    mock_get_db, mock_validate_collection
):
    mock_db = AsyncMock()
    mock_db.get_dataset.return_value = {
        "id": "ds-123",
        "name": "test",
        "items": [{"query": "q", "expected_answer": "a", "expected_sources": []}],
        "created_at": "2026-04-16T00:00:00Z",
    }
    mock_db.get_experiment.return_value = {
        **_stub_experiment(status="running"),
        "dataset_id": "ds-123",
    }
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
    detail = response.json()["detail"]
    assert detail == "experiment_label is required with experiment_id"
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
        "dataset_id": "ds-123",
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


# --- Compare endpoint ---


def _stub_run(run_id, dataset_id, scores):
    """Helper for compare/history fixtures."""
    return {
        "id": run_id,
        "dataset_id": dataset_id,
        "status": "completed",
        "collection": "documents",
        "aggregate_scores": scores,
        "results": None,
        "error": None,
        "created_at": "2026-04-28T00:00:00Z",
        "completed_at": "2026-04-28T00:01:00Z",
        "notes": None,
        "config": None,
        "baseline_eval_id": None,
    }


@patch("app.main.get_db")
def test_compare_happy_path(mock_get_db):
    mock_db = AsyncMock()
    mock_db.get_evaluations_by_ids.return_value = [
        _stub_run(
            "a",
            "ds-1",
            {
                "faithfulness": 0.80,
                "answer_relevancy": 0.70,
                "context_precision": 0.60,
                "context_recall": 0.50,
            },
        ),
        _stub_run(
            "b",
            "ds-1",
            {
                "faithfulness": 0.85,
                "answer_relevancy": 0.75,
                "context_precision": 0.65,
                "context_recall": 0.55,
            },
        ),
    ]
    mock_get_db.return_value = mock_db

    response = client.get("/evaluations/compare?ids=a,b")
    assert response.status_code == 200
    body = response.json()
    assert len(body["runs"]) == 2
    assert body["deltas"]["faithfulness"][0] == 0.0
    assert body["deltas"]["faithfulness"][1] == pytest.approx(0.05, abs=1e-6)
    assert body["deltas"]["answer_relevancy"][1] == pytest.approx(0.05, abs=1e-6)


@patch("app.main.get_db")
def test_compare_n_way_with_5_runs(mock_get_db):
    runs = [
        _stub_run(
            f"r{i}",
            "ds-1",
            {
                "faithfulness": 0.80 + i * 0.01,
                "answer_relevancy": 0.70,
                "context_precision": 0.60,
                "context_recall": 0.50,
            },
        )
        for i in range(5)
    ]
    mock_db = AsyncMock()
    mock_db.get_evaluations_by_ids.return_value = runs
    mock_get_db.return_value = mock_db

    response = client.get("/evaluations/compare?ids=r0,r1,r2,r3,r4")
    assert response.status_code == 200
    deltas = response.json()["deltas"]
    assert deltas["faithfulness"] == [0.0, 0.01, 0.02, 0.03, 0.04]


def test_compare_400_on_too_few_ids():
    response = client.get("/evaluations/compare?ids=only-one")
    assert response.status_code == 400
    assert "2-5 ids" in response.json()["detail"]


def test_compare_400_on_too_many_ids():
    response = client.get("/evaluations/compare?ids=a,b,c,d,e,f")
    assert response.status_code == 400
    assert "2-5 ids" in response.json()["detail"]


@patch("app.main.get_db")
def test_compare_400_on_mixed_datasets(mock_get_db):
    mock_db = AsyncMock()
    mock_db.get_evaluations_by_ids.return_value = [
        _stub_run("a", "ds-1", {"faithfulness": 0.8}),
        _stub_run("b", "ds-other", {"faithfulness": 0.9}),
    ]
    mock_get_db.return_value = mock_db

    response = client.get("/evaluations/compare?ids=a,b")
    assert response.status_code == 400
    assert "same dataset" in response.json()["detail"]


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


@patch("app.main.get_db")
def test_compare_allows_completed_with_failures_runs(mock_get_db):
    mock_db = AsyncMock()
    mock_db.get_evaluations_by_ids.return_value = [
        _stub_run("base", "ds-1", {"faithfulness": 0.8}) | {"status": "completed"},
        _stub_run("candidate", "ds-1", {"faithfulness": 0.7})
        | {"status": "completed_with_failures"},
    ]
    mock_get_db.return_value = mock_db

    response = client.get("/evaluations/compare?ids=base,candidate")

    assert response.status_code == 200
    assert response.json()["runs"][1]["status"] == "completed_with_failures"


@patch("app.main.get_db")
def test_compare_404_on_unknown_id(mock_get_db):
    mock_db = AsyncMock()
    mock_db.get_evaluations_by_ids.return_value = []
    mock_get_db.return_value = mock_db

    response = client.get("/evaluations/compare?ids=missing-1,missing-2")
    assert response.status_code == 404
    assert "unknown evaluation id" in response.json()["detail"]


@patch("app.main.get_db")
def test_compare_handles_missing_metric_scores(mock_get_db):
    mock_db = AsyncMock()
    mock_db.get_evaluations_by_ids.return_value = [
        _stub_run("a", "ds-1", {"faithfulness": 0.8}),  # other metrics absent
        _stub_run("b", "ds-1", {"faithfulness": 0.9}),
    ]
    mock_get_db.return_value = mock_db

    response = client.get("/evaluations/compare?ids=a,b")
    assert response.status_code == 200
    deltas = response.json()["deltas"]
    assert deltas["faithfulness"][1] == pytest.approx(0.1, abs=1e-6)
    # Missing metrics get 0.0 deltas (not NaN, not crash)
    assert deltas["answer_relevancy"] == [0.0, 0.0]


# --- History endpoint ---


@patch("app.main.get_db")
def test_history_returns_completed_runs(mock_get_db):
    mock_db = AsyncMock()
    mock_db.get_history.return_value = [
        _stub_run("r1", "ds-1", {"faithfulness": 0.7}),
        _stub_run("r2", "ds-1", {"faithfulness": 0.8}),
        _stub_run("r3", "ds-1", {"faithfulness": 0.9}),
    ]
    mock_get_db.return_value = mock_db

    response = client.get("/evaluations/history?dataset_id=ds-1&collection=documents")
    assert response.status_code == 200
    body = response.json()
    assert len(body["runs"]) == 3
    mock_db.get_history.assert_awaited_once_with(
        dataset_id="ds-1", collection="documents"
    )


def test_history_400_when_dataset_id_missing():
    response = client.get("/evaluations/history?collection=documents")
    assert response.status_code == 400
    assert "required" in response.json()["detail"]


def test_history_400_when_collection_missing():
    response = client.get("/evaluations/history?dataset_id=ds-1")
    assert response.status_code == 400
    assert "required" in response.json()["detail"]


@patch("app.main.get_db")
def test_history_empty_returns_200(mock_get_db):
    mock_db = AsyncMock()
    mock_db.get_history.return_value = []
    mock_get_db.return_value = mock_db

    response = client.get(
        "/evaluations/history?dataset_id=nonexistent&collection=documents"
    )
    assert response.status_code == 200
    assert response.json() == {"runs": []}


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


# --- Experiment endpoints ---


def _stub_experiment(exp_id="exp-1", status="running", runs=None):
    return {
        "id": exp_id,
        "name": "precision tuning",
        "hypothesis": "Reranking improves context precision",
        "dataset_id": "ds-1",
        "collection": "documents",
        "baseline_eval_id": None,
        "focus_metric": "context_precision",
        "status": status,
        "decision": None,
        "conclusion": None,
        "evidence": None,
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
        focus_metric="context_precision",
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
        json={
            "status": "completed",
            "decision": "keep",
            "conclusion": "Keep reranking.",
            "evidence": {"baseline_eval_id": "eval-base"},
            "notes": "rerank won",
        },
    )

    assert response.status_code == 200
    mock_db.update_experiment.assert_awaited_once_with(
        "exp-1",
        hypothesis=None,
        baseline_eval_id=None,
        focus_metric=None,
        status="completed",
        decision="keep",
        conclusion="Keep reranking.",
        evidence={"baseline_eval_id": "eval-base"},
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
                "evaluation": _stub_run("eval-1", "ds-1", {"context_precision": 0.42}),
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


# --- Health check ---


@patch("app.main.httpx.AsyncClient")
def test_health_returns_200_when_chat_degraded(mock_client_cls):
    mock_client = AsyncMock()
    mock_client.__aenter__ = AsyncMock(return_value=mock_client)
    mock_client.__aexit__ = AsyncMock(return_value=False)
    mock_client.get.side_effect = httpx.ConnectError("boom")
    mock_client_cls.return_value = mock_client

    response = client.get("/health")
    assert response.status_code == 200
    body = response.json()
    assert body["status"] == "healthy"
    assert body["chat_service"] == "unreachable"
