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
    assert "agent_loop_iterations" in body
    assert "agent_tool_calls_total" in body
    assert "agent_tool_duration_seconds" in body
