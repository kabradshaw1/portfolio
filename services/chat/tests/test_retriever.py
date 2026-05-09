from unittest.mock import MagicMock, patch

import pytest
from app.retriever import QdrantRetriever
from qdrant_client.models import Fusion, FusionQuery, SparseVector


@pytest.fixture
def mock_qdrant_client():
    with patch("app.retriever.QdrantClient") as MockClient:
        client = MagicMock()
        MockClient.return_value = client
        yield client


def _hit(score=0.95, text="relevant chunk"):
    return MagicMock(
        score=score,
        payload={
            "text": text,
            "page_number": 1,
            "filename": "doc.pdf",
            "document_id": "abc",
        },
    )


def test_hybrid_search_uses_prefetch_and_rrf(mock_qdrant_client):
    mock_qdrant_client.query_points.return_value = MagicMock(points=[_hit()])
    retriever = QdrantRetriever(host="localhost", port=6333, collection_name="test")
    query_vector = [0.1] * 768
    sparse_vector = SparseVector(indices=[1], values=[0.7])

    result = retriever.search_hybrid(
        query_vector=query_vector,
        sparse_vector=sparse_vector,
        top_k=5,
        prefetch_limit=20,
    )

    assert result.metadata == {
        "retrieval_mode": "hybrid",
        "retrieval_fallback": False,
        "fusion": "rrf",
    }
    assert result.chunks[0]["text"] == "relevant chunk"
    call_args = mock_qdrant_client.query_points.call_args
    assert call_args.kwargs["collection_name"] == "test"
    assert call_args.kwargs["limit"] == 5
    assert call_args.kwargs["with_payload"] is True
    assert call_args.kwargs["query"] == FusionQuery(fusion=Fusion.RRF)

    dense_prefetch, sparse_prefetch = call_args.kwargs["prefetch"]
    assert dense_prefetch.query == query_vector
    assert dense_prefetch.using == "dense"
    assert dense_prefetch.limit == 20
    assert sparse_prefetch.query == sparse_vector
    assert sparse_prefetch.using == "sparse"
    assert sparse_prefetch.limit == 20


def test_hybrid_search_defaults_prefetch_limit_from_settings(
    mock_qdrant_client, monkeypatch
):
    mock_qdrant_client.query_points.return_value = MagicMock(points=[])
    monkeypatch.setattr("app.retriever.settings.hybrid_prefetch_limit", 37)
    retriever = QdrantRetriever(host="localhost", port=6333, collection_name="test")

    retriever.search_hybrid(
        query_vector=[0.1] * 768,
        sparse_vector=SparseVector(indices=[1], values=[0.7]),
        top_k=5,
    )

    dense_prefetch, sparse_prefetch = mock_qdrant_client.query_points.call_args.kwargs[
        "prefetch"
    ]
    assert dense_prefetch.limit == 37
    assert sparse_prefetch.limit == 37


def test_semantic_search_uses_named_dense_vector(mock_qdrant_client):
    mock_qdrant_client.query_points.return_value = MagicMock(points=[_hit()])
    retriever = QdrantRetriever(host="localhost", port=6333, collection_name="test")
    query_vector = [0.1] * 768

    result = retriever.search_semantic(query_vector=query_vector, top_k=3)

    assert result.metadata == {
        "retrieval_mode": "semantic",
        "retrieval_fallback": False,
        "fusion": None,
    }
    assert result.chunks[0]["text"] == "relevant chunk"
    assert mock_qdrant_client.search.call_count == 0
    call_args = mock_qdrant_client.query_points.call_args
    assert call_args.kwargs["collection_name"] == "test"
    assert call_args.kwargs["query"] == query_vector
    assert call_args.kwargs["using"] == "dense"
    assert call_args.kwargs["limit"] == 3
    assert call_args.kwargs["with_payload"] is True


def test_search_returns_plain_list_from_query_points(mock_qdrant_client):
    mock_qdrant_client.query_points.return_value = MagicMock(points=[])
    retriever = QdrantRetriever(host="localhost", port=6333, collection_name="test")

    result = retriever.search(query_vector=[0.1] * 768, top_k=3)

    assert result == []
    assert mock_qdrant_client.search.call_count == 0
    call_args = mock_qdrant_client.query_points.call_args
    assert call_args.kwargs["query"] == [0.1] * 768
    assert call_args.kwargs["using"] == "dense"
    assert call_args.kwargs["limit"] == 3
    assert call_args.kwargs["with_payload"] is True


def test_legacy_semantic_search_uses_unnamed_vector(mock_qdrant_client):
    mock_qdrant_client.query_points.return_value = MagicMock(points=[_hit()])
    retriever = QdrantRetriever(host="localhost", port=6333, collection_name="test")
    query_vector = [0.1] * 768

    result = retriever.search_semantic(
        query_vector=query_vector,
        top_k=3,
        legacy_vector=True,
        fallback=True,
    )

    assert result.metadata == {
        "retrieval_mode": "semantic",
        "retrieval_fallback": True,
        "fusion": None,
    }
    assert mock_qdrant_client.search.call_count == 0
    call_args = mock_qdrant_client.query_points.call_args
    assert call_args.kwargs["collection_name"] == "test"
    assert call_args.kwargs["query"] == query_vector
    assert call_args.kwargs.get("using") is None
    assert call_args.kwargs["limit"] == 3
    assert call_args.kwargs["with_payload"] is True
