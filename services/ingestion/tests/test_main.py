import io
from unittest.mock import AsyncMock, MagicMock, patch

import httpx
from app.main import app
from fastapi.testclient import TestClient

client = TestClient(app)


@patch("app.main._embedding_provider")
@patch("app.main.QdrantClient")
def test_health(mock_qdrant_cls, mock_provider):
    mock_qdrant = MagicMock()
    mock_qdrant.get_collections.return_value = True
    mock_qdrant_cls.return_value = mock_qdrant

    mock_provider.check_health = AsyncMock(return_value=True)

    response = client.get("/health")
    assert response.status_code == 200
    data = response.json()
    assert data["status"] == "healthy"
    assert data["qdrant"] == "connected"
    assert data["llm"] == "connected"


@patch("app.main._embedding_provider")
@patch("app.main.QdrantClient")
def test_health_qdrant_down(mock_qdrant_cls, mock_provider):
    mock_qdrant = MagicMock()
    mock_qdrant.get_collections.side_effect = Exception("Connection refused")
    mock_qdrant_cls.return_value = mock_qdrant

    mock_provider.check_health = AsyncMock(return_value=True)

    response = client.get("/health")
    assert response.status_code == 503
    data = response.json()
    assert data["status"] == "degraded"
    assert data["qdrant"] == "disconnected"
    assert data["llm"] == "connected"


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
def test_ingest_returns_503_when_ollama_unreachable(
    mock_extract, mock_embed, mock_get_store
):
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
    assert "Connection refused" not in response.json()["detail"]
    assert response.json()["detail"] == "Embedding service unavailable"


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


@patch("app.main.get_store")
def test_delete_document_success(mock_get_store):
    mock_store = MagicMock()
    mock_store.delete_document.return_value = 5
    mock_get_store.return_value = mock_store

    response = client.delete("/documents/abc-123")
    assert response.status_code == 200
    data = response.json()
    assert data["status"] == "deleted"
    assert data["document_id"] == "abc-123"
    assert data["chunks_deleted"] == 5


@patch("app.main.get_store")
def test_delete_document_not_found(mock_get_store):
    mock_store = MagicMock()
    mock_store.delete_document.return_value = 0
    mock_get_store.return_value = mock_store

    response = client.delete("/documents/nonexistent")
    assert response.status_code == 404


@patch("app.main.get_store")
def test_delete_collection_success(mock_get_store):
    mock_store = MagicMock()
    mock_get_store.return_value = mock_store

    response = client.delete("/collections/e2e-test")
    assert response.status_code == 200
    data = response.json()
    assert data["status"] == "deleted"
    assert data["collection"] == "e2e-test"


@patch("app.main.get_store")
def test_delete_collection_not_found(mock_get_store):
    mock_store = MagicMock()
    mock_store.delete_collection.side_effect = ValueError(
        "Collection nonexistent not found"
    )
    mock_get_store.return_value = mock_store

    response = client.delete("/collections/nonexistent")
    assert response.status_code == 404


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


@patch("app.main.QdrantStore")
@patch("app.main.embed_texts", new_callable=AsyncMock)
@patch("app.main.extract_pages")
def test_ingest_with_custom_collection(mock_extract, mock_embed, mock_qdrant_store_cls):
    mock_extract.return_value = [
        {"page_number": 1, "text": "Hello world. " * 100},
    ]
    mock_embed.return_value = [[0.1] * 768] * 2
    mock_store = MagicMock()
    mock_qdrant_store_cls.return_value = mock_store

    pdf_content = b"%PDF-1.4 fake content"
    response = client.post(
        "/ingest?collection=e2e-test",
        files={"file": ("test.pdf", io.BytesIO(pdf_content), "application/pdf")},
    )

    assert response.status_code == 200
    mock_store.upsert.assert_called_once()


def test_ingest_rejects_invalid_collection_name():
    pdf_content = b"%PDF-1.4 fake content"
    response = client.post(
        "/ingest?collection=DROP%20TABLE%20users",
        files={"file": ("test.pdf", io.BytesIO(pdf_content), "application/pdf")},
    )
    assert response.status_code == 422
    assert "Invalid collection name" in response.json()["detail"]


def test_ingest_rejects_too_long_collection_name():
    pdf_content = b"%PDF-1.4 fake content"
    long_name = "a" * 101
    response = client.post(
        f"/ingest?collection={long_name}",
        files={"file": ("test.pdf", io.BytesIO(pdf_content), "application/pdf")},
    )
    assert response.status_code == 422
    assert "Invalid collection name" in response.json()["detail"]


def test_ingest_rejects_empty_collection_name():
    pdf_content = b"%PDF-1.4 fake content"
    response = client.post(
        "/ingest?collection=",
        files={"file": ("test.pdf", io.BytesIO(pdf_content), "application/pdf")},
    )
    assert response.status_code == 422
    assert "Invalid collection name" in response.json()["detail"]


@patch("app.main.get_store")
def test_delete_collection_rejects_invalid_name(mock_get_store):
    response = client.delete("/collections/DROP TABLE users")
    assert response.status_code == 422
    assert "Invalid collection name" in response.json()["detail"]


@patch("app.main.get_store")
def test_list_collections(mock_get_store):
    mock_store = MagicMock()
    mock_store.list_collections.return_value = [
        {"name": "documents", "point_count": 150},
        {"name": "debug-myproject", "point_count": 42},
    ]
    mock_get_store.return_value = mock_store

    response = client.get("/collections")
    assert response.status_code == 200
    data = response.json()
    assert len(data["collections"]) == 2
    assert data["collections"][0]["name"] == "documents"


@patch("app.main.get_meta_db")
def test_get_collection_config_returns_metadata(mock_get_meta_db):
    mock_db = AsyncMock()
    mock_db.get.return_value = {
        "chunk_size": 1000,
        "chunk_overlap": 200,
        "embedding_model": "nomic-embed-text",
    }
    mock_get_meta_db.return_value = mock_db

    response = client.get("/collections/documents/config")
    assert response.status_code == 200
    body = response.json()
    assert body == {
        "chunk_size": 1000,
        "chunk_overlap": 200,
        "embedding_model": "nomic-embed-text",
    }


@patch("app.main.get_meta_db")
def test_get_collection_config_404_when_unknown(mock_get_meta_db):
    mock_db = AsyncMock()
    mock_db.get.return_value = None
    mock_get_meta_db.return_value = mock_db

    response = client.get("/collections/nope/config")
    assert response.status_code == 404
    assert "not found" in response.json()["detail"]


@patch("app.main.get_meta_db")
@patch("app.main.get_store")
@patch("app.main.embed_texts", new_callable=AsyncMock)
@patch("app.main.extract_pages")
def test_ingest_persists_collection_metadata(
    mock_extract, mock_embed, mock_get_store, mock_get_meta_db
):
    mock_extract.return_value = [{"page_number": 1, "text": "Hello world. " * 100}]
    mock_embed.return_value = [[0.1] * 768] * 2
    mock_store = MagicMock()
    mock_get_store.return_value = mock_store
    mock_db = AsyncMock()
    mock_get_meta_db.return_value = mock_db

    pdf_content = b"%PDF-1.4 fake content"
    response = client.post(
        "/ingest",
        files={"file": ("test.pdf", io.BytesIO(pdf_content), "application/pdf")},
    )
    assert response.status_code == 200

    from app.config import settings

    mock_db.upsert.assert_awaited_once_with(
        collection=settings.collection_name,
        chunk_size=settings.chunk_size,
        chunk_overlap=settings.chunk_overlap,
        embedding_model=settings.embedding_model,
    )
