from unittest.mock import AsyncMock, patch

import pytest
from app.embedder import embed_texts


@pytest.mark.asyncio
async def test_embed_texts_returns_list_of_vectors():
    mock_response = {"embeddings": [[0.1] * 768, [0.2] * 768]}
    with patch("app.embedder.httpx.AsyncClient") as MockClient:
        client_instance = AsyncMock()
        client_instance.post.return_value = AsyncMock(
            status_code=200,
            json=lambda: mock_response,
            raise_for_status=lambda: None,
        )
        MockClient.return_value.__aenter__ = AsyncMock(return_value=client_instance)
        MockClient.return_value.__aexit__ = AsyncMock(return_value=False)

        vectors = await embed_texts(
            texts=["hello", "world"],
            ollama_base_url="http://localhost:11434",
            model="nomic-embed-text",
        )

    assert len(vectors) == 2
    assert len(vectors[0]) == 768


@pytest.mark.asyncio
async def test_embed_texts_calls_ollama_api():
    mock_response = {"embeddings": [[0.1] * 768]}
    with patch("app.embedder.httpx.AsyncClient") as MockClient:
        client_instance = AsyncMock()
        client_instance.post.return_value = AsyncMock(
            status_code=200,
            json=lambda: mock_response,
            raise_for_status=lambda: None,
        )
        MockClient.return_value.__aenter__ = AsyncMock(return_value=client_instance)
        MockClient.return_value.__aexit__ = AsyncMock(return_value=False)

        await embed_texts(
            texts=["hello"],
            ollama_base_url="http://localhost:11434",
            model="nomic-embed-text",
        )

    client_instance.post.assert_called_once_with(
        "http://localhost:11434/api/embed",
        json={"model": "nomic-embed-text", "input": ["hello"]},
        timeout=120.0,
    )


@pytest.mark.asyncio
async def test_embed_texts_empty_list_returns_empty():
    vectors = await embed_texts(
        texts=[],
        ollama_base_url="http://localhost:11434",
        model="nomic-embed-text",
    )
    assert vectors == []
