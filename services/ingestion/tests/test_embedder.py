from unittest.mock import AsyncMock

import pytest
from app.embedder import embed_texts


@pytest.mark.asyncio
async def test_embed_texts_returns_list_of_vectors():
    mock_provider = AsyncMock()
    mock_provider.embed.return_value = [[0.1] * 768, [0.2] * 768]

    vectors = await embed_texts(
        texts=["hello", "world"],
        provider=mock_provider,
        model="nomic-embed-text",
    )

    assert len(vectors) == 2
    assert len(vectors[0]) == 768


@pytest.mark.asyncio
async def test_embed_texts_calls_provider():
    mock_provider = AsyncMock()
    mock_provider.embed.return_value = [[0.1] * 768]

    await embed_texts(
        texts=["hello"],
        provider=mock_provider,
        model="nomic-embed-text",
    )

    mock_provider.embed.assert_called_once_with(["hello"])


@pytest.mark.asyncio
async def test_embed_texts_empty_list_returns_empty():
    mock_provider = AsyncMock()

    vectors = await embed_texts(
        texts=[],
        provider=mock_provider,
        model="nomic-embed-text",
    )
    assert vectors == []
