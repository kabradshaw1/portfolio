from app.main import app
from fastapi.testclient import TestClient

client = TestClient(app)


def test_metrics_endpoint_returns_200():
    response = client.get("/metrics")
    assert response.status_code == 200


def test_metrics_contains_custom_metrics():
    response = client.get("/metrics")
    body = response.text
    assert "ollama_request_duration_seconds" in body
    assert "ollama_tokens_total" in body
    assert "embedding_duration_seconds" in body
    assert "qdrant_search_duration_seconds" in body
    assert "rag_pipeline_duration_seconds" in body
