import io
import httpx
import pytest
from unittest.mock import patch, AsyncMock, MagicMock
from fastapi.testclient import TestClient
from app.main import app


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
def test_health_qdrant_down(mock_qdrant_cls, mock_httpx_cls):
    mock_qdrant = MagicMock()
    mock_qdrant.get_collections.side_effect = Exception("Connection refused")
    mock_qdrant_cls.return_value = mock_qdrant

    mock_client = AsyncMock()
    mock_client.get.return_value = AsyncMock(status_code=200)
    mock_httpx_cls.return_value.__aenter__ = AsyncMock(return_value=mock_client)
    mock_httpx_cls.return_value.__aexit__ = AsyncMock(return_value=False)

    response = client.get("/health")
    assert response.status_code == 503
    data = response.json()
    assert data["status"] == "degraded"
    assert data["qdrant"] == "disconnected"
    assert data["ollama"] == "connected"


@patch("app.main.get_store")
@patch("app.main.embed_texts", new_callable=AsyncMock)
@patch("app.main.extract_pages")
def test_ingest_pdf_success(mock_extract, mock_embed, mock_get_store):
    mock_extract.return_value = [
        {"page_number": 1, "text": "Hello world. " * 100},
    ]
    mock_embed.return_value = [[0.1] * 768] * 2
    mock_store = MagicMock()
    mock_get_store.return_value = mock_store

    pdf_content = b"%PDF-1.4 fake content"
    response = client.post(
        "/ingest",
        files={"file": ("test.pdf", io.BytesIO(pdf_content), "application/pdf")},
    )

    assert response.status_code == 200
    data = response.json()
    assert data["status"] == "success"
    assert data["filename"] == "test.pdf"
    assert "document_id" in data
    assert "chunks_created" in data


@patch("app.main.get_store")
def test_ingest_rejects_non_pdf(mock_get_store):
    response = client.post(
        "/ingest",
        files={"file": ("test.txt", io.BytesIO(b"hello"), "text/plain")},
    )
    assert response.status_code == 422


@patch("app.main.get_store")
@patch("app.main.embed_texts", new_callable=AsyncMock)
@patch("app.main.extract_pages")
def test_ingest_returns_503_when_ollama_unreachable(mock_extract, mock_embed, mock_get_store):
    mock_extract.return_value = [
        {"page_number": 1, "text": "Hello world. " * 100},
    ]
    mock_embed.side_effect = httpx.ConnectError("Connection refused")
    mock_store = MagicMock()
    mock_get_store.return_value = mock_store

    pdf_content = b"%PDF-1.4 fake content"
    response = client.post(
        "/ingest",
        files={"file": ("test.pdf", io.BytesIO(pdf_content), "application/pdf")},
    )

    assert response.status_code == 503


@patch("app.main.get_store")
def test_documents_list(mock_get_store):
    mock_store = MagicMock()
    mock_store.list_documents.return_value = [
        {"document_id": "abc", "filename": "test.pdf", "chunks": 5},
    ]
    mock_get_store.return_value = mock_store

    response = client.get("/documents")
    assert response.status_code == 200
    data = response.json()
    assert len(data["documents"]) == 1
    assert data["documents"][0]["filename"] == "test.pdf"
