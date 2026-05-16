import httpx
import pytest
import respx
from app.config_capture import capture_run_config


@pytest.mark.asyncio
@respx.mock
async def test_capture_merges_chat_and_collection():
    expected_chat_config = {
        "llm_model": "qwen2.5:14b",
        "embedding_model": "nomic-embed-text",
        "top_k": 5,
        "prompt_version": "v1-baseline",
        "retrieval_mode": "hybrid",
        "hybrid_prefetch_limit": 20,
        "dense_vector_name": "dense",
        "sparse_vector_name": "sparse",
        "sparse_model": "Qdrant/bm25",
        "fusion": "rrf",
    }

    respx.get("http://chat/config").mock(
        return_value=httpx.Response(
            200,
            json=expected_chat_config,
        )
    )
    respx.get("http://ingestion/collections/documents/config").mock(
        return_value=httpx.Response(
            200,
            json={
                "chunk_size": 1000,
                "chunk_overlap": 200,
                "embedding_model": "nomic-embed-text",
            },
        )
    )

    cfg = await capture_run_config(
        chat_url="http://chat",
        ingestion_url="http://ingestion",
        collection="documents",
        requested_rerank=True,
    )

    assert cfg["chat"] == expected_chat_config
    assert cfg["collection"]["chunk_size"] == 1000
    assert cfg["requested_rerank"] is True
    assert cfg["effective_collection"] == "documents"
    assert "captured_at" in cfg
    assert "_capture_error" not in cfg


@pytest.mark.asyncio
@respx.mock
async def test_capture_records_baseline_rerank_intent():
    respx.get("http://chat/config").mock(
        return_value=httpx.Response(200, json={"rerank_enabled": True})
    )
    respx.get("http://ingestion/collections/documents/config").mock(
        return_value=httpx.Response(404, json={"detail": "not found"})
    )

    cfg = await capture_run_config(
        chat_url="http://chat",
        ingestion_url="http://ingestion",
        collection="documents",
        requested_rerank=False,
    )

    assert cfg["requested_rerank"] is False
    assert cfg["effective_collection"] == "documents"
    assert cfg["chat"]["rerank_enabled"] is True
    assert "_capture_error" in cfg


@pytest.mark.asyncio
@respx.mock
async def test_capture_records_error_when_chat_fails():
    respx.get("http://chat/config").mock(side_effect=httpx.ConnectError("boom"))
    respx.get("http://ingestion/collections/documents/config").mock(
        return_value=httpx.Response(
            200,
            json={
                "chunk_size": 1000,
                "chunk_overlap": 200,
                "embedding_model": "nomic-embed-text",
            },
        )
    )

    cfg = await capture_run_config(
        chat_url="http://chat",
        ingestion_url="http://ingestion",
        collection="documents",
        requested_rerank=False,
    )

    assert "_capture_error" in cfg
    assert "chat" in cfg["_capture_error"]
    # Partial data still recorded:
    assert cfg["collection"]["chunk_size"] == 1000


@pytest.mark.asyncio
@respx.mock
async def test_capture_records_error_when_collection_unknown():
    respx.get("http://chat/config").mock(
        return_value=httpx.Response(
            200,
            json={
                "llm_model": "qwen2.5:14b",
                "embedding_model": "nomic-embed-text",
                "top_k": 5,
                "prompt_version": "v1-baseline",
            },
        )
    )
    respx.get("http://ingestion/collections/nope/config").mock(
        return_value=httpx.Response(404, json={"detail": "not found"})
    )

    cfg = await capture_run_config(
        chat_url="http://chat",
        ingestion_url="http://ingestion",
        collection="nope",
        requested_rerank=False,
    )

    assert "_capture_error" in cfg
    assert "collection" in cfg["_capture_error"]
    assert cfg["chat"]["llm_model"] == "qwen2.5:14b"


@pytest.mark.asyncio
@respx.mock
async def test_capture_records_both_errors_when_both_fail():
    respx.get("http://chat/config").mock(side_effect=httpx.ConnectError("boom"))
    respx.get("http://ingestion/collections/documents/config").mock(
        return_value=httpx.Response(500, json={"detail": "internal"})
    )

    cfg = await capture_run_config(
        chat_url="http://chat",
        ingestion_url="http://ingestion",
        collection="documents",
        requested_rerank=False,
    )

    assert "_capture_error" in cfg
    assert "chat" in cfg["_capture_error"]
    assert "collection" in cfg["_capture_error"]
    assert "chat" not in cfg or "chat" == "chat"
    assert cfg.get("chat") is None or "chat" not in cfg
