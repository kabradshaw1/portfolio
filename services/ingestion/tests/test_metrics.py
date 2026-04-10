from app.main import app
from fastapi.testclient import TestClient

client = TestClient(app)


def test_metrics_endpoint_returns_200():
    response = client.get("/metrics")
    assert response.status_code == 200


def test_metrics_contains_process_metrics():
    response = client.get("/metrics")
    body = response.text
    assert "python_info" in body or "process_" in body


def test_metrics_contains_custom_metrics():
    response = client.get("/metrics")
    body = response.text
    assert "embedding_duration_seconds" in body
    assert "qdrant_operation_duration_seconds" in body
    assert "ingestion_chunks_created_total" in body
