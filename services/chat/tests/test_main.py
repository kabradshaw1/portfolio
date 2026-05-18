import json
import time
from unittest.mock import AsyncMock, MagicMock, patch

import httpx
import jwt
import pytest
from app.main import app
from app.retriever import RetrievalResult
from fastapi.testclient import TestClient

client = TestClient(app)
SECRET = "chat-test-secret"


def _token(sub: str, email: str) -> str:
    return jwt.encode(
        {"sub": sub, "email": email, "exp": int(time.time()) + 3600},
        SECRET,
        algorithm="HS256",
    )


@pytest.fixture
def configured_chat_limits(monkeypatch):
    import app.main as main

    monkeypatch.setenv("RAG_OPERATOR_EMAILS", "operator@example.test")
    monkeypatch.setattr(main.settings, "jwt_secret", SECRET)
    monkeypatch.setattr(main.settings, "chat_rate_limit_ask_operator", "3/minute")
    monkeypatch.setattr(main.settings, "chat_rate_limit_ask_user", "1/minute")
    monkeypatch.setattr(main.settings, "chat_rate_limit_ask_anonymous", "0/minute")
    monkeypatch.setattr(
        main.settings, "chat_rate_limit_search_internal_eval", "3/minute"
    )
    monkeypatch.setattr(main.settings, "rag_internal_eval_token", "test-internal-token")
    main.chat_rate_limiter = main.build_chat_rate_limiter()
    main.chat_rate_limiter.enabled = True
    yield
    main.chat_rate_limiter = main.build_chat_rate_limiter()
    main.chat_rate_limiter.enabled = False


def retrieval_result(chunks: list[dict] | None = None) -> RetrievalResult:
    return RetrievalResult(
        chunks=chunks or [],
        metadata={
            "retrieval_mode": "hybrid",
            "retrieval_fallback": False,
            "fusion": "rrf",
        },
    )


def test_config_endpoint_returns_active_settings():
    from app.config import settings

    response = client.get("/config")
    assert response.status_code == 200
    body = response.json()
    assert body["llm_model"] == settings.get_llm_model()
    assert body["embedding_model"] == settings.embedding_model
    assert body["top_k"] == settings.top_k
    assert body["prompt_version"] == settings.prompt_version
    assert body["retrieval_mode"] == settings.retrieval_mode
    assert body["hybrid_prefetch_limit"] == settings.hybrid_prefetch_limit
    assert body["dense_vector_name"] == settings.dense_vector_name
    assert body["sparse_vector_name"] == settings.sparse_vector_name
    assert body["sparse_model"] == settings.sparse_model
    assert body["fusion"] == ("rrf" if settings.retrieval_mode == "hybrid" else None)


@patch("app.main.rag_query")
def test_chat_threads_settings_top_k_into_rag_query(mock_rag_query):
    captured = {}

    async def fake(**kwargs):
        captured.update(kwargs)
        yield {"done": True, "sources": [], "retrieval": {}}

    mock_rag_query.side_effect = fake

    from app.config import settings

    original = settings.top_k
    settings.top_k = 9
    try:
        response = client.post(
            "/chat",
            json={"question": "hi"},
            headers={"Accept": "application/json"},
        )
    finally:
        settings.top_k = original

    assert response.status_code == 200
    assert captured["top_k"] == 9


@patch("app.main.rag_query")
def test_chat_threads_retrieval_config_top_k_into_rag_query(mock_rag_query):
    captured = {}

    async def fake(**kwargs):
        captured.update(kwargs)
        yield {"done": True, "sources": [], "retrieval": {}}

    mock_rag_query.side_effect = fake

    response = client.post(
        "/chat",
        json={"question": "hi", "retrieval_config": {"top_k": 3}},
        headers={"Accept": "application/json"},
    )

    assert response.status_code == 200
    assert captured["top_k"] == 3


@patch("app.main.rag_query")
def test_chat_stream_threads_retrieval_config_top_k_into_rag_query(mock_rag_query):
    from sse_starlette.sse import AppStatus

    captured = {}

    async def fake(**kwargs):
        captured.update(kwargs)
        yield {"done": True, "sources": [], "retrieval": {}}

    mock_rag_query.side_effect = fake

    try:
        response = client.post(
            "/chat",
            json={"question": "hi", "retrieval_config": {"top_k": 4}},
        )
    finally:
        AppStatus.should_exit_event = None

    assert response.status_code == 200
    assert captured["top_k"] == 4


def test_chat_rejects_invalid_retrieval_config_top_k():
    response = client.post(
        "/chat",
        json={"question": "hi", "retrieval_config": {"top_k": 0}},
        headers={"Accept": "application/json"},
    )

    assert response.status_code == 422


@pytest.mark.parametrize("top_k", [True, 3.0, "3"])
def test_chat_rejects_non_integer_retrieval_config_top_k(top_k):
    response = client.post(
        "/chat",
        json={"question": "hi", "retrieval_config": {"top_k": top_k}},
        headers={"Accept": "application/json"},
    )

    assert response.status_code == 422


def test_chat_rejects_unknown_retrieval_config_fields():
    response = client.post(
        "/chat",
        json={"question": "hi", "retrieval_config": {"top_k": 3, "score": 0.8}},
        headers={"Accept": "application/json"},
    )

    assert response.status_code == 422


def test_config_endpoint_omits_secrets():
    response = client.get("/config")
    body = response.json()
    # Sanity: never leak base URLs or API keys.
    for key in body:
        assert "key" not in key.lower()
        assert "secret" not in key.lower()
        assert "url" not in key.lower()


@patch("app.main._llm_provider")
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


@patch("app.main._llm_provider")
@patch("app.main.QdrantClient")
def test_health_ollama_down(mock_qdrant_cls, mock_provider):
    mock_qdrant = MagicMock()
    mock_qdrant.get_collections.return_value = True
    mock_qdrant_cls.return_value = mock_qdrant

    mock_provider.check_health = AsyncMock(return_value=False)

    response = client.get("/health")
    assert response.status_code == 503
    data = response.json()
    assert data["status"] == "degraded"
    assert data["llm"] == "disconnected"


@patch("app.main.rag_query")
def test_chat_streams_response(mock_rag_query):
    async def fake_rag_query(**kwargs):
        yield {"token": "Hello"}
        yield {"token": " world"}
        yield {
            "done": True,
            "sources": [{"file": "test.pdf", "page": 1}],
            "retrieval": {"retrieval_mode": "hybrid"},
        }

    mock_rag_query.return_value = fake_rag_query()

    response = client.post(
        "/chat",
        json={"question": "What is this?", "collection": "default"},
    )
    assert response.status_code == 200
    assert "text/event-stream" in response.headers["content-type"]

    events = []
    for line in response.text.strip().split("\n"):
        if line.startswith("data: "):
            events.append(json.loads(line[6:]))

    tokens = [e["token"] for e in events if "token" in e]
    assert "Hello" in tokens
    done_events = [e for e in events if e.get("done")]
    assert len(done_events) == 1
    assert done_events[0]["sources"][0]["file"] == "test.pdf"
    assert done_events[0]["retrieval"]["retrieval_mode"] == "hybrid"


def test_chat_requires_question():
    response = client.post("/chat", json={})
    assert response.status_code == 422


def test_chat_rejects_too_long_question():
    response = client.post(
        "/chat",
        json={"question": "x" * 2001},
    )
    assert response.status_code == 422


def test_chat_rejects_invalid_collection_name():
    response = client.post(
        "/chat",
        json={"question": "What is this?", "collection": "DROP TABLE users"},
    )
    assert response.status_code == 422


@patch("app.main.rag_query")
def test_chat_accepts_valid_collection_name(mock_rag_query):
    """Verify valid collection names pass Pydantic validation."""

    async def fake_rag_query(**kwargs):
        yield {"done": True, "sources": [], "retrieval": {}}

    mock_rag_query.return_value = fake_rag_query()

    with TestClient(app, raise_server_exceptions=False) as c:
        response = c.post(
            "/chat",
            json={"question": "Hello", "collection": "my-collection_123"},
        )
    assert response.status_code == 200


@patch("app.main.rag_query")
def test_chat_returns_error_when_backend_unreachable(mock_rag_query):
    async def failing_rag_query(**kwargs):
        raise httpx.ConnectError("Connection refused")
        yield  # make it a generator

    mock_rag_query.return_value = failing_rag_query()

    with TestClient(app, raise_server_exceptions=False) as c:
        response = c.post(
            "/chat",
            json={"question": "What is this?"},
        )
    # SSE endpoint should still return 200 but with an error event in the stream
    assert response.status_code == 200
    assert "error" in response.text.lower() or "unavailable" in response.text.lower()
    assert "Connection refused" not in response.text


@patch("app.main.retrieve_chunks", new_callable=AsyncMock)
def test_search_returns_chunks(mock_retrieve):
    mock_retrieve.return_value = retrieval_result(
        [
            {
                "text": "Hello world",
                "filename": "test.pdf",
                "page_number": 1,
                "document_id": "abc",
                "score": 0.92,
            },
            {
                "text": "Goodbye world",
                "filename": "test.pdf",
                "page_number": 2,
                "document_id": "abc",
                "score": 0.85,
            },
        ]
    )

    response = client.post("/search", json={"query": "hello", "limit": 5})
    assert response.status_code == 200
    data = response.json()
    assert len(data["results"]) == 2
    assert data["results"][0]["text"] == "Hello world"
    assert data["results"][0]["score"] == 0.92
    assert data["retrieval"]["retrieval_mode"] == "hybrid"


@patch("app.main.retrieve_chunks", new_callable=AsyncMock)
def test_search_threads_rerank_flag_into_retrieve_chunks(mock_retrieve):
    mock_retrieve.return_value = retrieval_result()

    response = client.post(
        "/search", json={"query": "hello", "limit": 5, "rerank": True}
    )

    assert response.status_code == 200
    assert mock_retrieve.await_args.kwargs["rerank"] is True


def test_search_requires_query():
    response = client.post("/search", json={})
    assert response.status_code == 422


def test_search_rejects_invalid_collection():
    response = client.post(
        "/search", json={"query": "hello", "collection": "DROP TABLE users"}
    )
    assert response.status_code == 422


@patch("app.main.rag_query")
def test_chat_json_mode(mock_rag_query):
    async def fake_rag_query(**kwargs):
        yield {"token": "Hello"}
        yield {"token": " world"}
        yield {
            "done": True,
            "sources": [{"file": "test.pdf", "page": 1}],
            "retrieval": {"retrieval_mode": "hybrid"},
        }

    mock_rag_query.return_value = fake_rag_query()

    response = client.post(
        "/chat",
        json={"question": "What is this?"},
        headers={"Accept": "application/json"},
    )
    assert response.status_code == 200
    data = response.json()
    assert data["answer"] == "Hello world"
    assert len(data["sources"]) == 1
    assert data["sources"][0]["file"] == "test.pdf"
    assert data["retrieval"]["retrieval_mode"] == "hybrid"


@patch("app.main.rag_query")
def test_chat_json_threads_rerank_flag_into_rag_query(mock_rag_query):
    captured = {}

    async def fake(**kwargs):
        captured.update(kwargs)
        yield {"done": True, "sources": [], "retrieval": {}}

    mock_rag_query.side_effect = fake

    response = client.post(
        "/chat",
        json={"question": "hi", "rerank": True},
        headers={"Accept": "application/json"},
    )

    assert response.status_code == 200
    assert captured["rerank"] is True


def test_config_endpoint_returns_rerank_settings():
    from app.config import settings

    response = client.get("/config")

    assert response.status_code == 200
    body = response.json()
    assert body["rerank_enabled"] == settings.rerank_enabled
    assert body["rerank_model"] == settings.rerank_model
    assert body["rerank_candidate_limit"] == settings.rerank_candidate_limit
    assert body["rerank_max_candidates"] == settings.rerank_max_candidates
    assert body["rerank_device"] == settings.rerank_device


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


@patch("app.main.rag_query")
def test_chat_operator_has_higher_ask_limit(mock_rag_query, configured_chat_limits):
    async def fake(**kwargs):
        yield {"done": True, "sources": [], "retrieval": {}}

    mock_rag_query.side_effect = fake
    operator_token = _token("operator-1", "operator@example.test")
    user_token = _token("user-1", "user@example.test")

    operator_headers = {
        "Authorization": f"Bearer {operator_token}",
        "Accept": "application/json",
    }
    user_headers = {
        "Authorization": f"Bearer {user_token}",
        "Accept": "application/json",
    }

    for _ in range(3):
        response = client.post(
            "/chat", json={"question": "hi"}, headers=operator_headers
        )
        assert response.status_code == 200

    first_user_response = client.post(
        "/chat", json={"question": "hi"}, headers=user_headers
    )
    assert first_user_response.status_code == 200
    denied = client.post("/chat", json={"question": "hi"}, headers=user_headers)

    assert denied.status_code == 429
    assert int(denied.headers["Retry-After"]) > 0


@patch("app.main.retrieve_chunks", new_callable=AsyncMock)
def test_search_internal_eval_token_uses_internal_eval_limit(
    mock_retrieve, configured_chat_limits
):
    mock_retrieve.return_value = retrieval_result()

    for _ in range(3):
        response = client.post(
            "/search",
            json={"query": "hello", "limit": 5},
            headers={"X-RAG-Internal-Token": "test-internal-token"},
        )
        assert response.status_code == 200

    wrong = client.post(
        "/search",
        json={"query": "hello", "limit": 5},
        headers={"X-RAG-Internal-Token": "wrong"},
    )

    assert wrong.status_code == 401
