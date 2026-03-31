from unittest.mock import AsyncMock, MagicMock, patch

import pytest
from app.chain import rag_query


@pytest.mark.asyncio
@patch("app.chain.stream_ollama_response")
@patch("app.chain.embed_texts", new_callable=AsyncMock)
@patch("app.chain.QdrantRetriever")
async def test_rag_query_returns_generator(MockRetriever, mock_embed, mock_stream):
    mock_embed.return_value = [[0.1] * 768]
    retriever_instance = MagicMock()
    retriever_instance.search.return_value = [
        {
            "text": "The answer is 42.",
            "filename": "doc.pdf",
            "page_number": 1,
            "document_id": "abc",
            "score": 0.9,
        },
    ]
    MockRetriever.return_value = retriever_instance

    async def fake_stream(prompt, model, base_url):
        yield {"token": "The"}
        yield {"token": " answer"}
        yield {"token": " is 42."}

    mock_stream.side_effect = fake_stream

    tokens = []
    sources = None
    async for event in rag_query(
        question="What is the answer?",
        ollama_base_url="http://localhost:11434",
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

    assert len(tokens) == 3
    assert sources is not None
    assert sources[0]["file"] == "doc.pdf"
    assert sources[0]["page"] == 1


@pytest.mark.asyncio
@patch("app.chain.stream_ollama_response")
@patch("app.chain.embed_texts", new_callable=AsyncMock)
@patch("app.chain.QdrantRetriever")
async def test_rag_query_no_results_still_responds(
    MockRetriever, mock_embed, mock_stream
):
    mock_embed.return_value = [[0.1] * 768]
    retriever_instance = MagicMock()
    retriever_instance.search.return_value = []
    MockRetriever.return_value = retriever_instance

    async def fake_stream(prompt, model, base_url):
        yield {"token": "I don't have any relevant context."}

    mock_stream.side_effect = fake_stream

    events = []
    async for event in rag_query(
        question="Unknown topic?",
        ollama_base_url="http://localhost:11434",
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
