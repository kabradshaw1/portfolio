from unittest.mock import AsyncMock, patch

import pytest
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
