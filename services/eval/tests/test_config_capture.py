import httpx
import pytest
from app.config_capture import capture_run_config


def _patch_async_transport(monkeypatch, transport: httpx.MockTransport) -> None:
    class MockedAsyncClient(httpx.AsyncClient):
        def __init__(self, *args, **kwargs):
            kwargs["transport"] = transport
            super().__init__(*args, **kwargs)

    monkeypatch.setattr("app.config_capture.httpx.AsyncClient", MockedAsyncClient)


def _transport(routes: dict[str, httpx.Response | Exception]) -> httpx.MockTransport:
    def handler(request: httpx.Request) -> httpx.Response:
        value = routes[str(request.url)]
        if isinstance(value, Exception):
            raise value
        return value

    return httpx.MockTransport(handler)


@pytest.mark.asyncio
async def test_capture_merges_chat_and_collection(monkeypatch):
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

    _patch_async_transport(
        monkeypatch,
        _transport(
            {
                "http://chat/config": httpx.Response(200, json=expected_chat_config),
                "http://ingestion/collections/documents/config": httpx.Response(
                    200,
                    json={
                        "chunk_size": 1000,
                        "chunk_overlap": 200,
                        "embedding_model": "nomic-embed-text",
                    },
                ),
            }
        ),
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
async def test_capture_records_baseline_rerank_intent(monkeypatch):
    _patch_async_transport(
        monkeypatch,
        _transport(
            {
                "http://chat/config": httpx.Response(
                    200, json={"rerank_enabled": True}
                ),
                "http://ingestion/collections/documents/config": httpx.Response(
                    404, json={"detail": "not found"}
                ),
            }
        ),
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
async def test_capture_records_requested_and_effective_retrieval_config(monkeypatch):
    _patch_async_transport(
        monkeypatch,
        _transport(
            {
                "http://chat/config": httpx.Response(200, json={"top_k": 5}),
                "http://ingestion/collections/documents/config": httpx.Response(
                    200, json={"chunk_size": 1000}
                ),
            }
        ),
    )

    cfg = await capture_run_config(
        chat_url="http://chat",
        ingestion_url="http://ingestion",
        collection="documents",
        requested_rerank=False,
        requested_retrieval_config={"top_k": 3},
    )

    assert cfg["requested_retrieval_config"] == {"top_k": 3}
    assert cfg["effective_retrieval_config"] == {"top_k": 3}
    assert cfg["chat"]["top_k"] == 5


@pytest.mark.asyncio
async def test_capture_records_empty_requested_retrieval_config_for_default_run(
    monkeypatch,
):
    _patch_async_transport(
        monkeypatch,
        _transport(
            {
                "http://chat/config": httpx.Response(200, json={"top_k": 5}),
                "http://ingestion/collections/documents/config": httpx.Response(
                    200, json={"chunk_size": 1000}
                ),
            }
        ),
    )

    cfg = await capture_run_config(
        chat_url="http://chat",
        ingestion_url="http://ingestion",
        collection="documents",
        requested_rerank=False,
    )

    assert cfg["requested_retrieval_config"] == {}
    assert cfg["effective_retrieval_config"] == {"top_k": 5}


@pytest.mark.asyncio
async def test_capture_records_error_when_chat_fails(monkeypatch):
    _patch_async_transport(
        monkeypatch,
        _transport(
            {
                "http://chat/config": httpx.ConnectError("boom"),
                "http://ingestion/collections/documents/config": httpx.Response(
                    200,
                    json={
                        "chunk_size": 1000,
                        "chunk_overlap": 200,
                        "embedding_model": "nomic-embed-text",
                    },
                ),
            }
        ),
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
async def test_capture_records_error_when_collection_unknown(monkeypatch):
    _patch_async_transport(
        monkeypatch,
        _transport(
            {
                "http://chat/config": httpx.Response(
                    200,
                    json={
                        "llm_model": "qwen2.5:14b",
                        "embedding_model": "nomic-embed-text",
                        "top_k": 5,
                        "prompt_version": "v1-baseline",
                    },
                ),
                "http://ingestion/collections/nope/config": httpx.Response(
                    404, json={"detail": "not found"}
                ),
            }
        ),
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
async def test_capture_records_both_errors_when_both_fail(monkeypatch):
    _patch_async_transport(
        monkeypatch,
        _transport(
            {
                "http://chat/config": httpx.ConnectError("boom"),
                "http://ingestion/collections/documents/config": httpx.Response(
                    500, json={"detail": "internal"}
                ),
            }
        ),
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
