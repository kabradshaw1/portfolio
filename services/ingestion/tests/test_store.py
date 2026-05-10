from unittest.mock import MagicMock, patch

import pytest
from app.store import QdrantStore
from qdrant_client.models import Distance, SparseVector, SparseVectorParams


@pytest.fixture
def mock_qdrant_client():
    with patch("app.store.QdrantClient") as MockClient:
        client = MagicMock()
        MockClient.return_value = client
        yield client


def test_store_init_creates_collection_if_not_exists(mock_qdrant_client):
    mock_qdrant_client.collection_exists.return_value = False
    QdrantStore(host="localhost", port=6333, collection_name="test")
    mock_qdrant_client.create_collection.assert_called_once()


def test_store_init_skips_creation_if_exists(mock_qdrant_client):
    mock_qdrant_client.collection_exists.return_value = True
    QdrantStore(host="localhost", port=6333, collection_name="test")
    mock_qdrant_client.create_collection.assert_not_called()


def test_upsert_vectors(mock_qdrant_client):
    mock_qdrant_client.collection_exists.return_value = True
    store = QdrantStore(host="localhost", port=6333, collection_name="test")

    chunks = [
        {"text": "hello", "page_number": 1, "chunk_index": 0},
        {"text": "world", "page_number": 1, "chunk_index": 1},
    ]
    vectors = [[0.1] * 768, [0.2] * 768]

    store.upsert(
        chunks=chunks,
        vectors=vectors,
        sparse_vectors=[
            SparseVector(indices=[1], values=[0.1]),
            SparseVector(indices=[2], values=[0.2]),
        ],
        document_id="doc-123",
        filename="test.pdf",
    )

    mock_qdrant_client.upsert.assert_called_once()
    call_args = mock_qdrant_client.upsert.call_args
    assert call_args.kwargs["collection_name"] == "test"
    points = call_args.kwargs["points"]
    assert len(points) == 2
    assert points[0].payload["filename"] == "test.pdf"
    assert points[0].payload["document_id"] == "doc-123"
    assert points[0].payload["page_number"] == 1
    assert points[0].payload["text"] == "hello"


def test_list_documents(mock_qdrant_client):
    mock_qdrant_client.collection_exists.return_value = True
    store = QdrantStore(host="localhost", port=6333, collection_name="test")

    mock_qdrant_client.scroll.return_value = (
        [
            MagicMock(
                payload={
                    "document_id": "doc-1",
                    "filename": "a.pdf",
                    "page_number": 1,
                    "chunk_index": 0,
                }
            ),
            MagicMock(
                payload={
                    "document_id": "doc-1",
                    "filename": "a.pdf",
                    "page_number": 1,
                    "chunk_index": 1,
                }
            ),
            MagicMock(
                payload={
                    "document_id": "doc-2",
                    "filename": "b.pdf",
                    "page_number": 1,
                    "chunk_index": 0,
                }
            ),
        ],
        None,
    )

    docs = store.list_documents()
    assert len(docs) == 2
    assert docs[0]["document_id"] == "doc-1"
    assert docs[0]["chunks"] == 2
    assert docs[1]["document_id"] == "doc-2"
    assert docs[1]["chunks"] == 1


def test_delete_document(mock_qdrant_client):
    mock_qdrant_client.collection_exists.return_value = True
    store = QdrantStore(host="localhost", port=6333, collection_name="test")

    mock_qdrant_client.scroll.return_value = (
        [
            MagicMock(
                payload={
                    "document_id": "doc-1",
                    "filename": "a.pdf",
                    "page_number": 1,
                    "chunk_index": 0,
                }
            ),
            MagicMock(
                payload={
                    "document_id": "doc-1",
                    "filename": "a.pdf",
                    "page_number": 1,
                    "chunk_index": 1,
                }
            ),
            MagicMock(
                payload={
                    "document_id": "doc-1",
                    "filename": "a.pdf",
                    "page_number": 2,
                    "chunk_index": 2,
                }
            ),
        ],
        None,
    )

    count = store.delete_document("doc-1")
    assert count == 3
    mock_qdrant_client.delete.assert_called_once()
    call_args = mock_qdrant_client.delete.call_args
    assert call_args.kwargs["collection_name"] == "test"


def test_delete_document_not_found(mock_qdrant_client):
    mock_qdrant_client.collection_exists.return_value = True
    store = QdrantStore(host="localhost", port=6333, collection_name="test")

    mock_qdrant_client.scroll.return_value = ([], None)

    count = store.delete_document("nonexistent")
    assert count == 0
    mock_qdrant_client.delete.assert_not_called()


def test_delete_collection(mock_qdrant_client):
    mock_qdrant_client.collection_exists.return_value = True
    store = QdrantStore(host="localhost", port=6333, collection_name="test")

    mock_qdrant_client.collection_exists.return_value = True
    store.delete_collection("e2e-test")
    mock_qdrant_client.delete_collection.assert_called_once_with(
        collection_name="e2e-test"
    )


def test_delete_collection_not_found(mock_qdrant_client):
    mock_qdrant_client.collection_exists.return_value = True
    store = QdrantStore(host="localhost", port=6333, collection_name="test")

    mock_qdrant_client.collection_exists.return_value = False
    with pytest.raises(ValueError, match="Collection nonexistent not found"):
        store.delete_collection("nonexistent")


def test_list_collections(mock_qdrant_client):
    store = QdrantStore(host="localhost", port=6333, collection_name="default")

    collection1 = MagicMock()
    collection1.name = "documents"
    collection2 = MagicMock()
    collection2.name = "debug-myproject"

    mock_qdrant_client.get_collections.return_value = MagicMock(
        collections=[collection1, collection2]
    )

    info1 = MagicMock()
    info1.points_count = 150
    info2 = MagicMock()
    info2.points_count = 42

    mock_qdrant_client.get_collection.side_effect = [info1, info2]

    result = store.list_collections()
    assert len(result) == 2
    assert result[0]["name"] == "documents"
    assert result[0]["point_count"] == 150
    assert result[1]["name"] == "debug-myproject"
    assert result[1]["point_count"] == 42


def test_store_init_creates_hybrid_collection(mock_qdrant_client):
    mock_qdrant_client.collection_exists.return_value = False

    QdrantStore(host="localhost", port=6333, collection_name="test")

    call_args = mock_qdrant_client.create_collection.call_args
    assert call_args.kwargs["collection_name"] == "test"
    assert sorted(call_args.kwargs["vectors_config"].keys()) == ["dense"]
    assert sorted(call_args.kwargs["sparse_vectors_config"].keys()) == ["sparse"]
    dense_config = call_args.kwargs["vectors_config"]["dense"]
    assert dense_config.size == 768
    assert dense_config.distance == Distance.COSINE
    assert isinstance(
        call_args.kwargs["sparse_vectors_config"]["sparse"], SparseVectorParams
    )


def test_upsert_writes_named_dense_and_sparse_vectors(mock_qdrant_client):
    mock_qdrant_client.collection_exists.return_value = True
    store = QdrantStore(host="localhost", port=6333, collection_name="test")

    chunks = [{"text": "RFC 7231", "page_number": 1, "chunk_index": 0}]
    dense_vectors = [[0.1] * 768]
    sparse_vectors = [SparseVector(indices=[10, 11], values=[0.8, 0.4])]

    store.upsert(
        chunks=chunks,
        vectors=dense_vectors,
        sparse_vectors=sparse_vectors,
        document_id="doc-123",
        filename="http.pdf",
    )

    point = mock_qdrant_client.upsert.call_args.kwargs["points"][0]
    assert point.vector["dense"] == dense_vectors[0]
    assert point.vector["sparse"] == sparse_vectors[0]
    assert point.payload["text"] == "RFC 7231"
    assert point.payload["page_number"] == 1
    assert point.payload["chunk_index"] == 0
    assert point.payload["document_id"] == "doc-123"
    assert point.payload["filename"] == "http.pdf"


def test_upsert_rejects_vector_count_mismatch(mock_qdrant_client):
    mock_qdrant_client.collection_exists.return_value = True
    store = QdrantStore(host="localhost", port=6333, collection_name="test")

    chunks = [{"text": "RFC 7231", "page_number": 1, "chunk_index": 0}]

    with pytest.raises(ValueError, match="matching lengths"):
        store.upsert(
            chunks=chunks,
            vectors=[],
            sparse_vectors=[SparseVector(indices=[1], values=[0.7])],
            document_id="doc-123",
            filename="http.pdf",
        )

    mock_qdrant_client.upsert.assert_not_called()
