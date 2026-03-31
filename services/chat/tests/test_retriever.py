from unittest.mock import MagicMock, patch

import pytest
from app.retriever import QdrantRetriever


@pytest.fixture
def mock_qdrant_client():
    with patch("app.retriever.QdrantClient") as MockClient:
        client = MagicMock()
        MockClient.return_value = client
        yield client


def test_search_returns_chunks_with_scores(mock_qdrant_client):
    mock_qdrant_client.search.return_value = [
        MagicMock(
            score=0.95,
            payload={
                "text": "relevant chunk",
                "page_number": 1,
                "filename": "doc.pdf",
                "document_id": "abc",
            },
        ),
        MagicMock(
            score=0.85,
            payload={
                "text": "another chunk",
                "page_number": 2,
                "filename": "doc.pdf",
                "document_id": "abc",
            },
        ),
    ]

    retriever = QdrantRetriever(host="localhost", port=6333, collection_name="test")
    results = retriever.search(query_vector=[0.1] * 768, top_k=5)

    assert len(results) == 2
    assert results[0]["text"] == "relevant chunk"
    assert results[0]["score"] == 0.95
    assert results[0]["page_number"] == 1
    assert results[0]["filename"] == "doc.pdf"


def test_search_respects_top_k(mock_qdrant_client):
    mock_qdrant_client.search.return_value = []
    retriever = QdrantRetriever(host="localhost", port=6333, collection_name="test")
    retriever.search(query_vector=[0.1] * 768, top_k=3)

    call_args = mock_qdrant_client.search.call_args
    assert call_args.kwargs["limit"] == 3


def test_search_empty_results(mock_qdrant_client):
    mock_qdrant_client.search.return_value = []
    retriever = QdrantRetriever(host="localhost", port=6333, collection_name="test")
    results = retriever.search(query_vector=[0.1] * 768, top_k=5)
    assert results == []
