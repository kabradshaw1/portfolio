from unittest.mock import AsyncMock, MagicMock, patch

import pytest
from app.chain import rag_query, retrieve_chunks
from app.retriever import RetrievalResult


def hybrid_result(chunks: list[dict] | None = None) -> RetrievalResult:
    return RetrievalResult(
        chunks=chunks or [],
        metadata={
            "retrieval_mode": "hybrid",
            "retrieval_fallback": False,
            "fusion": "rrf",
        },
    )


def semantic_fallback_result(chunks: list[dict] | None = None) -> RetrievalResult:
    return RetrievalResult(
        chunks=chunks or [],
        metadata={
            "retrieval_mode": "semantic",
            "retrieval_fallback": True,
            "fusion": None,
        },
    )


@patch("app.chain.SparseVectorEncoder", create=True)
@patch("app.chain.QdrantRetriever")
@pytest.mark.asyncio
async def test_retrieve_chunks_returns_hybrid_metadata(
    mock_retriever_cls,
    mock_sparse_encoder_cls,
    monkeypatch,
):
    monkeypatch.setattr("app.chain.settings.retrieval_mode", "hybrid")
    embedding_provider = AsyncMock()
    embedding_provider.embed.return_value = [[0.1] * 768]

    sparse_vector = MagicMock()
    mock_sparse_encoder = MagicMock()
    mock_sparse_encoder.embed.return_value = [sparse_vector]
    mock_sparse_encoder_cls.return_value = mock_sparse_encoder

    retriever = MagicMock()
    retriever.search_hybrid.return_value = hybrid_result(
        [
            {
                "text": "RFC 7231",
                "page_number": 1,
                "filename": "http.pdf",
                "document_id": "doc",
                "score": 0.9,
            }
        ]
    )
    mock_retriever_cls.return_value = retriever

    result = await retrieve_chunks(
        question="What is RFC 7231?",
        embedding_provider=embedding_provider,
        embedding_model="nomic-embed-text",
        qdrant_host="localhost",
        qdrant_port=6333,
        collection_name="documents",
        top_k=5,
    )

    assert result.metadata["retrieval_mode"] == "hybrid"
    assert result.metadata["retrieval_fallback"] is False
    assert result.chunks[0]["text"] == "RFC 7231"


@patch("app.chain.logger", create=True)
@patch("app.chain.get_sparse_encoder", create=True)
@patch("app.chain.QdrantRetriever")
@pytest.mark.asyncio
async def test_retrieve_chunks_falls_back_when_sparse_generation_fails(
    mock_retriever_cls,
    mock_sparse_encoder,
    mock_logger,
    monkeypatch,
):
    monkeypatch.setattr("app.chain.settings.retrieval_mode", "hybrid")
    embedding_provider = AsyncMock()
    embedding_provider.embed.return_value = [[0.1] * 768]
    sparse_error = RuntimeError("sparse unavailable")
    mock_sparse_encoder.return_value.embed.side_effect = sparse_error

    retriever = MagicMock()
    retriever.search_semantic.return_value = semantic_fallback_result()
    mock_retriever_cls.return_value = retriever

    result = await retrieve_chunks(
        question="What is RFC 7231?",
        embedding_provider=embedding_provider,
        embedding_model="nomic-embed-text",
        qdrant_host="localhost",
        qdrant_port=6333,
        collection_name="documents",
        top_k=5,
    )

    assert result.metadata["retrieval_mode"] == "semantic"
    assert result.metadata["retrieval_fallback"] is True
    assert result.metadata["fusion"] is None
    retriever.search_hybrid.assert_not_called()
    retriever.search_semantic.assert_called_once_with(
        query_vector=[0.1] * 768,
        top_k=5,
        fallback=True,
    )
    mock_logger.warning.assert_called_once_with(
        "hybrid_retrieval_fallback",
        error="sparse unavailable",
        error_type="RuntimeError",
        collection="documents",
        exc_info=True,
    )


@patch("app.chain.logger", create=True)
@patch("app.chain.get_sparse_encoder", create=True)
@patch("app.chain.QdrantRetriever")
@pytest.mark.asyncio
async def test_retrieve_chunks_falls_back_when_hybrid_search_fails(
    mock_retriever_cls,
    mock_sparse_encoder,
    mock_logger,
    monkeypatch,
):
    monkeypatch.setattr("app.chain.settings.retrieval_mode", "hybrid")
    embedding_provider = AsyncMock()
    embedding_provider.embed.return_value = [[0.1] * 768]
    sparse_vector = MagicMock()
    mock_sparse_encoder.return_value.embed.return_value = [sparse_vector]

    retriever = MagicMock()
    retriever.search_hybrid.side_effect = RuntimeError("hybrid unavailable")
    retriever.search_semantic.return_value = semantic_fallback_result()
    mock_retriever_cls.return_value = retriever

    result = await retrieve_chunks(
        question="What is RFC 7231?",
        embedding_provider=embedding_provider,
        embedding_model="nomic-embed-text",
        qdrant_host="localhost",
        qdrant_port=6333,
        collection_name="documents",
        top_k=5,
    )

    assert result.metadata["retrieval_mode"] == "semantic"
    assert result.metadata["retrieval_fallback"] is True
    assert result.metadata["fusion"] is None
    retriever.search_hybrid.assert_called_once_with(
        query_vector=[0.1] * 768,
        sparse_vector=sparse_vector,
        top_k=5,
        prefetch_limit=20,
    )
    retriever.search_semantic.assert_called_once_with(
        query_vector=[0.1] * 768,
        top_k=5,
        fallback=True,
    )
    mock_logger.warning.assert_called_once_with(
        "hybrid_retrieval_fallback",
        error="hybrid unavailable",
        error_type="RuntimeError",
        collection="documents",
        exc_info=True,
    )


@patch("app.chain.logger", create=True)
@patch("app.chain.get_sparse_encoder", create=True)
@patch("app.chain.QdrantRetriever")
@pytest.mark.asyncio
async def test_retrieve_chunks_falls_back_to_legacy_when_named_semantic_fallback_fails(
    mock_retriever_cls,
    mock_sparse_encoder,
    mock_logger,
    monkeypatch,
):
    monkeypatch.setattr("app.chain.settings.retrieval_mode", "hybrid")
    embedding_provider = AsyncMock()
    embedding_provider.embed.return_value = [[0.1] * 768]
    sparse_vector = MagicMock()
    mock_sparse_encoder.return_value.embed.return_value = [sparse_vector]

    retriever = MagicMock()
    retriever.search_hybrid.side_effect = RuntimeError("hybrid unavailable")
    retriever.search_semantic.side_effect = [
        RuntimeError("named vector missing"),
        semantic_fallback_result(
            [
                {
                    "text": "legacy chunk",
                    "page_number": 2,
                    "filename": "legacy.pdf",
                    "document_id": "legacy-doc",
                    "score": 0.8,
                }
            ]
        ),
    ]
    mock_retriever_cls.return_value = retriever

    result = await retrieve_chunks(
        question="What is RFC 7231?",
        embedding_provider=embedding_provider,
        embedding_model="nomic-embed-text",
        qdrant_host="localhost",
        qdrant_port=6333,
        collection_name="documents",
        top_k=5,
    )

    assert result.metadata["retrieval_mode"] == "semantic"
    assert result.metadata["retrieval_fallback"] is True
    assert result.chunks[0]["text"] == "legacy chunk"
    retriever.search_hybrid.assert_called_once_with(
        query_vector=[0.1] * 768,
        sparse_vector=sparse_vector,
        top_k=5,
        prefetch_limit=20,
    )
    assert retriever.search_semantic.call_args_list == [
        ((), {"query_vector": [0.1] * 768, "top_k": 5, "fallback": True}),
        (
            (),
            {
                "query_vector": [0.1] * 768,
                "top_k": 5,
                "legacy_vector": True,
                "fallback": True,
            },
        ),
    ]
    assert mock_logger.warning.call_args_list[0] == (
        ("hybrid_retrieval_fallback",),
        {
            "error": "hybrid unavailable",
            "error_type": "RuntimeError",
            "collection": "documents",
            "exc_info": True,
        },
    )
    assert mock_logger.warning.call_args_list[1] == (
        ("semantic_legacy_fallback",),
        {
            "error": "named vector missing",
            "error_type": "RuntimeError",
            "collection": "documents",
            "exc_info": True,
        },
    )


@patch("app.chain.logger", create=True)
@patch("app.chain.QdrantRetriever")
@pytest.mark.asyncio
async def test_retrieve_chunks_semantic_mode_falls_back_to_legacy_vector(
    mock_retriever_cls,
    mock_logger,
    monkeypatch,
):
    monkeypatch.setattr("app.chain.settings.retrieval_mode", "semantic")
    embedding_provider = AsyncMock()
    embedding_provider.embed.return_value = [[0.1] * 768]

    retriever = MagicMock()
    retriever.search_semantic.side_effect = [
        RuntimeError("named vector missing"),
        semantic_fallback_result(),
    ]
    mock_retriever_cls.return_value = retriever

    result = await retrieve_chunks(
        question="What is RFC 7231?",
        embedding_provider=embedding_provider,
        embedding_model="nomic-embed-text",
        qdrant_host="localhost",
        qdrant_port=6333,
        collection_name="documents",
        top_k=5,
    )

    assert result.metadata["retrieval_mode"] == "semantic"
    assert result.metadata["retrieval_fallback"] is True
    assert retriever.search_semantic.call_args_list == [
        ((), {"query_vector": [0.1] * 768, "top_k": 5}),
        (
            (),
            {
                "query_vector": [0.1] * 768,
                "top_k": 5,
                "legacy_vector": True,
                "fallback": True,
            },
        ),
    ]
    mock_logger.warning.assert_called_once_with(
        "semantic_legacy_fallback",
        error="named vector missing",
        error_type="RuntimeError",
        collection="documents",
        exc_info=True,
    )


@pytest.mark.asyncio
@patch("app.chain.stream_response")
@patch("app.chain.embed_texts", new_callable=AsyncMock)
@patch("app.chain.get_sparse_encoder", create=True)
@patch("app.chain.QdrantRetriever")
async def test_rag_query_returns_generator(
    MockRetriever, mock_sparse_encoder, mock_embed, mock_stream, monkeypatch
):
    monkeypatch.setattr("app.chain.settings.retrieval_mode", "hybrid")
    mock_embed.return_value = [[0.1] * 768]
    mock_sparse_encoder.return_value.embed.return_value = [MagicMock()]
    retriever_instance = MagicMock()
    retriever_instance.search_hybrid.return_value = hybrid_result(
        [
            {
                "text": "The answer is 42.",
                "filename": "doc.pdf",
                "page_number": 1,
                "document_id": "abc",
                "score": 0.9,
            },
        ]
    )
    MockRetriever.return_value = retriever_instance

    mock_llm_provider = AsyncMock()
    mock_embedding_provider = AsyncMock()

    async def fake_stream(prompt, model, provider):
        yield {"token": "The"}
        yield {"token": " answer"}
        yield {"token": " is 42."}

    mock_stream.side_effect = fake_stream

    tokens = []
    sources = None
    retrieval = None
    async for event in rag_query(
        question="What is the answer?",
        llm_provider=mock_llm_provider,
        embedding_provider=mock_embedding_provider,
        chat_model="mistral",
        embedding_model="nomic-embed-text",
        qdrant_host="localhost",
        qdrant_port=6333,
        collection_name="documents",
    ):
        if "token" in event:
            tokens.append(event["token"])
        if "sources" in event:
            sources = event["sources"]
        if "retrieval" in event:
            retrieval = event["retrieval"]

    assert len(tokens) == 3
    assert sources is not None
    assert sources[0]["file"] == "doc.pdf"
    assert sources[0]["page"] == 1
    assert retrieval["retrieval_mode"] == "hybrid"


@pytest.mark.asyncio
@patch("app.chain.stream_response")
@patch("app.chain.embed_texts", new_callable=AsyncMock)
@patch("app.chain.get_sparse_encoder", create=True)
@patch("app.chain.QdrantRetriever")
async def test_rag_query_no_results_still_responds(
    MockRetriever, mock_sparse_encoder, mock_embed, mock_stream, monkeypatch
):
    monkeypatch.setattr("app.chain.settings.retrieval_mode", "hybrid")
    mock_embed.return_value = [[0.1] * 768]
    mock_sparse_encoder.return_value.embed.return_value = [MagicMock()]
    retriever_instance = MagicMock()
    retriever_instance.search_hybrid.return_value = hybrid_result()
    MockRetriever.return_value = retriever_instance

    mock_llm_provider = AsyncMock()
    mock_embedding_provider = AsyncMock()

    async def fake_stream(prompt, model, provider):
        yield {"token": "I don't have any relevant context."}

    mock_stream.side_effect = fake_stream

    events = []
    async for event in rag_query(
        question="Unknown topic?",
        llm_provider=mock_llm_provider,
        embedding_provider=mock_embedding_provider,
        chat_model="mistral",
        embedding_model="nomic-embed-text",
        qdrant_host="localhost",
        qdrant_port=6333,
        collection_name="documents",
    ):
        events.append(event)

    # Should still produce token events (the "no context" response)
    assert any("token" in e for e in events)
    # Should have done/sources event
    assert any("done" in e for e in events)
