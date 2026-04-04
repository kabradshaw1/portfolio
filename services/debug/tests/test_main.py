from unittest.mock import AsyncMock, MagicMock, patch

from app.main import app
from fastapi.testclient import TestClient

client = TestClient(app)


@patch("app.main.httpx.AsyncClient")
@patch("app.main.QdrantClient")
def test_health_ok(mock_qdrant_cls, mock_httpx_cls):
    mock_qdrant = MagicMock()
    mock_qdrant.get_collections.return_value = True
    mock_qdrant_cls.return_value = mock_qdrant

    mock_client = AsyncMock()
    mock_response = AsyncMock(status_code=200)
    mock_client.get.return_value = mock_response
    mock_httpx_cls.return_value.__aenter__ = AsyncMock(return_value=mock_client)
    mock_httpx_cls.return_value.__aexit__ = AsyncMock(return_value=False)

    response = client.get("/health")
    assert response.status_code == 200
    assert response.json()["status"] == "ok"


@patch("app.main.httpx.AsyncClient")
@patch("app.main.QdrantClient")
def test_health_qdrant_down(mock_qdrant_cls, mock_httpx_cls):
    mock_qdrant = MagicMock()
    mock_qdrant.get_collections.side_effect = Exception("connection refused")
    mock_qdrant_cls.return_value = mock_qdrant

    response = client.get("/health")
    assert response.status_code == 503
