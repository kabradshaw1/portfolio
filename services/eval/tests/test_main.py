from unittest.mock import AsyncMock, patch

from app.main import app
from fastapi.testclient import TestClient

client = TestClient(app)


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
        {"id": "ds-1", "name": "ds1", "created_at": "2026-04-16T00:00:00Z"},
        {"id": "ds-2", "name": "ds2", "created_at": "2026-04-16T01:00:00Z"},
    ]
    mock_get_db.return_value = mock_db

    response = client.get("/datasets")
    assert response.status_code == 200
    assert len(response.json()["datasets"]) == 2


# --- Evaluation endpoints ---


@patch("app.main.get_db")
def test_start_evaluation(mock_get_db):
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


@patch("app.main.get_db")
def test_start_evaluation_dataset_not_found(mock_get_db):
    mock_db = AsyncMock()
    mock_db.get_dataset.return_value = None
    mock_get_db.return_value = mock_db

    response = client.post(
        "/evaluations",
        json={"dataset_id": "nonexistent"},
    )
    assert response.status_code == 404


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
def test_get_evaluation_not_found(mock_get_db):
    mock_db = AsyncMock()
    mock_db.get_evaluation.return_value = None
    mock_get_db.return_value = mock_db

    response = client.get("/evaluations/nonexistent")
    assert response.status_code == 404


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


@patch("app.main.get_db")
def test_start_evaluation_persists_notes_and_baseline(mock_get_db):
    mock_db = AsyncMock()
    mock_db.get_dataset.return_value = {
        "id": "ds-123",
        "name": "test",
        "items": [{"query": "q", "expected_answer": "a", "expected_sources": []}],
        "created_at": "2026-04-16T00:00:00Z",
    }
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
    mock_db.create_evaluation.assert_awaited_once_with(
        dataset_id="ds-123",
        collection="documents",
        notes="bumped chunk overlap to 300",
        baseline_eval_id="eval-prev",
    )


@patch("app.main.run_evaluation", new_callable=AsyncMock)
@patch("app.main.capture_run_config", new_callable=AsyncMock)
@patch("app.main.get_db")
def test_run_persists_config_snapshot(mock_get_db, mock_capture, mock_run_evaluation):
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
@patch("app.main.get_db")
def test_run_uses_default_collection_when_none_provided(
    mock_get_db, mock_capture, mock_run_evaluation
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


@patch("app.main.get_db")
def test_start_evaluation_omits_optional_fields(mock_get_db):
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
    )


# --- Health check ---


@patch("app.main.httpx.AsyncClient")
def test_health_degraded_when_chat_unreachable(mock_client_cls):
    mock_client = AsyncMock()
    mock_client.__aenter__ = AsyncMock(return_value=mock_client)
    mock_client.__aexit__ = AsyncMock(return_value=False)
    mock_client.get.side_effect = Exception("connection refused")
    mock_client_cls.return_value = mock_client

    response = client.get("/health")
    assert response.status_code == 503
    assert response.json()["status"] == "degraded"
