import json
from unittest.mock import AsyncMock, MagicMock, patch

from app.main import app
from fastapi.testclient import TestClient

client = TestClient(app)


@patch("app.main._llm_provider")
@patch("app.main.QdrantClient")
def test_health_ok(mock_qdrant_cls, mock_provider):
    mock_qdrant = MagicMock()
    mock_qdrant.get_collections.return_value = True
    mock_qdrant_cls.return_value = mock_qdrant

    mock_provider.check_health = AsyncMock(return_value=True)

    response = client.get("/health")
    assert response.status_code == 200
    assert response.json()["status"] == "healthy"
    assert response.json()["qdrant"] == "connected"
    assert response.json()["llm"] == "connected"


@patch("app.main._llm_provider")
@patch("app.main.QdrantClient")
def test_health_qdrant_down(mock_qdrant_cls, mock_provider):
    mock_qdrant = MagicMock()
    mock_qdrant.get_collections.side_effect = Exception("connection refused")
    mock_qdrant_cls.return_value = mock_qdrant

    mock_provider.check_health = AsyncMock(return_value=True)

    response = client.get("/health")
    assert response.status_code == 503


@patch("app.main._llm_provider")
@patch("app.main.QdrantClient")
def test_health_ollama_down(mock_qdrant_cls, mock_provider):
    mock_qdrant = MagicMock()
    mock_qdrant.get_collections.return_value = True
    mock_qdrant_cls.return_value = mock_qdrant

    mock_provider.check_health = AsyncMock(return_value=False)

    response = client.get("/health")
    assert response.status_code == 503


@patch("app.main.os.path.isdir", return_value=True)
@patch("app.main.index_project", new_callable=AsyncMock)
def test_index_success(mock_index, mock_isdir):
    mock_index.return_value = {
        "collection": "debug-myproject",
        "files_indexed": 5,
        "chunks": 42,
    }
    response = client.post("/index", json={"path": "/mock/myproject"})
    assert response.status_code == 200
    data = response.json()
    assert data["collection"] == "debug-myproject"
    assert data["files_indexed"] == 5
    assert data["chunks"] == 42


def test_index_missing_path():
    response = client.post("/index", json={})
    assert response.status_code == 422


@patch("app.main.index_project", new_callable=AsyncMock)
def test_index_nonexistent_path(mock_index):
    mock_index.side_effect = FileNotFoundError("path not found")
    response = client.post("/index", json={"path": "/nonexistent"})
    assert response.status_code == 400


@patch("app.main.run_agent_loop")
def test_debug_streams_sse_events(mock_agent):
    from app.main import _project_paths

    _project_paths["debug-test"] = "/mock/project"

    async def fake_events(*args, **kwargs):
        yield {"event": "thinking", "data": {"step": 1, "content": "Analyzing..."}}
        yield {
            "event": "diagnosis",
            "data": {"step": 2, "content": "Root cause: missing check"},
        }
        yield {"event": "done", "data": {}}

    mock_agent.return_value = fake_events()

    response = client.post(
        "/debug", json={"collection": "debug-test", "description": "upload fails"}
    )
    assert response.status_code == 200
    assert "text/event-stream" in response.headers["content-type"]

    events = []
    for line in response.text.strip().split("\n"):
        if line.startswith("data: "):
            events.append(json.loads(line[6:]))
    assert len(events) >= 2

    _project_paths.pop("debug-test", None)


def test_debug_missing_collection():
    response = client.post("/debug", json={"description": "bug"})
    assert response.status_code == 422


def test_debug_missing_description():
    response = client.post("/debug", json={"collection": "test"})
    assert response.status_code == 422
