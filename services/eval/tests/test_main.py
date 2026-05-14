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
    )


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


@patch("app.main.run_evaluation", new_callable=AsyncMock)
@patch("app.main.capture_run_config", new_callable=AsyncMock)
@patch("app.main.get_db")
def test_start_evaluation_passes_rerank_to_background_run(
    mock_get_db, mock_capture, mock_run_evaluation
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
    assert mock_run_evaluation.await_args.kwargs["rerank"] is True


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


@patch("app.main.get_db")
def test_start_evaluation_attaches_run_to_experiment(mock_get_db):
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


# --- Experiment endpoints ---


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
def test_health_degraded_when_chat_unreachable(mock_client_cls):
    mock_client = AsyncMock()
    mock_client.__aenter__ = AsyncMock(return_value=mock_client)
    mock_client.__aexit__ = AsyncMock(return_value=False)
    mock_client.get.side_effect = Exception("connection refused")
    mock_client_cls.return_value = mock_client

    response = client.get("/health")
    assert response.status_code == 503
    assert response.json()["status"] == "degraded"
