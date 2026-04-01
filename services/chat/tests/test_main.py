import json
from unittest.mock import AsyncMock, MagicMock, patch

import httpx
from app.main import app
from fastapi.testclient import TestClient

client = TestClient(app)


@patch("app.main.httpx.AsyncClient")
@patch("app.main.QdrantClient")
def test_health(mock_qdrant_cls, mock_httpx_cls):
    mock_qdrant = MagicMock()
    mock_qdrant.get_collections.return_value = True
    mock_qdrant_cls.return_value = mock_qdrant

    mock_client = AsyncMock()
    mock_client.get.return_value = AsyncMock(status_code=200)
    mock_httpx_cls.return_value.__aenter__ = AsyncMock(return_value=mock_client)
    mock_httpx_cls.return_value.__aexit__ = AsyncMock(return_value=False)

    response = client.get("/health")
    assert response.status_code == 200
    data = response.json()
    assert data["status"] == "healthy"
    assert data["qdrant"] == "connected"
    assert data["ollama"] == "connected"


@patch("app.main.httpx.AsyncClient")
@patch("app.main.QdrantClient")
def test_health_ollama_down(mock_qdrant_cls, mock_httpx_cls):
    mock_qdrant = MagicMock()
    mock_qdrant.get_collections.return_value = True
    mock_qdrant_cls.return_value = mock_qdrant

    mock_client = AsyncMock()
    mock_client.get.side_effect = Exception("Connection refused")
    mock_httpx_cls.return_value.__aenter__ = AsyncMock(return_value=mock_client)
    mock_httpx_cls.return_value.__aexit__ = AsyncMock(return_value=False)

    response = client.get("/health")
    assert response.status_code == 503
    data = response.json()
    assert data["status"] == "degraded"
    assert data["ollama"] == "disconnected"


@patch("app.main.rag_query")
def test_chat_streams_response(mock_rag_query):
    async def fake_rag_query(**kwargs):
        yield {"token": "Hello"}
        yield {"token": " world"}
        yield {"done": True, "sources": [{"file": "test.pdf", "page": 1}]}

    mock_rag_query.return_value = fake_rag_query()

    response = client.post(
        "/chat",
        json={"question": "What is this?", "collection": "default"},
    )
    assert response.status_code == 200
    assert "text/event-stream" in response.headers["content-type"]

    events = []
    for line in response.text.strip().split("\n"):
        if line.startswith("data: "):
            events.append(json.loads(line[6:]))

    tokens = [e["token"] for e in events if "token" in e]
    assert "Hello" in tokens
    done_events = [e for e in events if e.get("done")]
    assert len(done_events) == 1
    assert done_events[0]["sources"][0]["file"] == "test.pdf"


def test_chat_requires_question():
    response = client.post("/chat", json={})
    assert response.status_code == 422


@patch("app.main.rag_query")
def test_chat_returns_error_when_backend_unreachable(mock_rag_query):
    async def failing_rag_query(**kwargs):
        raise httpx.ConnectError("Connection refused")
        yield  # make it a generator

    mock_rag_query.return_value = failing_rag_query()

    with TestClient(app, raise_server_exceptions=False) as c:
        response = c.post(
            "/chat",
            json={"question": "What is this?"},
        )
    # SSE endpoint should still return 200 but with an error event in the stream
    assert response.status_code == 200
    assert "error" in response.text.lower() or "unavailable" in response.text.lower()


def test_cors_rejects_unknown_origin():
    response = client.options(
        "/health",
        headers={
            "Origin": "https://evil.example.com",
            "Access-Control-Request-Method": "GET",
        },
    )
    assert response.headers.get("access-control-allow-origin") != "*"
    assert "evil.example.com" not in response.headers.get(
        "access-control-allow-origin", ""
    )
