import json

import httpx
import pytest
from app.rag_client import RAGClient


@pytest.fixture
def mock_search_response():
    return {
        "results": [
            {
                "text": "Kubernetes is a container orchestration platform.",
                "filename": "k8s.pdf",
                "page_number": 1,
                "score": 0.95,
            },
            {
                "text": "Pods are the smallest deployable units.",
                "filename": "k8s.pdf",
                "page_number": 3,
                "score": 0.82,
            },
        ]
    }


@pytest.fixture
def mock_chat_response():
    answer_text = (
        "Kubernetes is a container orchestration platform for automating deployment."
    )
    return {
        "answer": answer_text,
        "sources": [{"file": "k8s.pdf", "page": 1}],
    }


@pytest.mark.asyncio
async def test_search(mock_search_response):
    async def mock_handler(request: httpx.Request) -> httpx.Response:
        assert request.url.path == "/search"
        body = json.loads(request.content)
        assert body["query"] == "what is kubernetes"
        assert body["limit"] == 5
        return httpx.Response(200, json=mock_search_response)

    transport = httpx.MockTransport(mock_handler)
    client = RAGClient(base_url="http://chat:8000", transport=transport)

    results = await client.search("what is kubernetes", collection=None, limit=5)
    assert len(results) == 2
    assert results[0]["text"] == "Kubernetes is a container orchestration platform."
    assert results[0]["score"] == 0.95


@pytest.mark.asyncio
async def test_search_with_collection(mock_search_response):
    async def mock_handler(request: httpx.Request) -> httpx.Response:
        body = json.loads(request.content)
        assert body["collection"] == "my-docs"
        return httpx.Response(200, json=mock_search_response)

    transport = httpx.MockTransport(mock_handler)
    client = RAGClient(base_url="http://chat:8000", transport=transport)

    results = await client.search("test", collection="my-docs", limit=5)
    assert len(results) == 2


@pytest.mark.asyncio
async def test_ask(mock_chat_response):
    async def mock_handler(request: httpx.Request) -> httpx.Response:
        assert request.url.path == "/chat"
        assert request.headers["accept"] == "application/json"
        body = json.loads(request.content)
        assert body["question"] == "what is kubernetes"
        return httpx.Response(200, json=mock_chat_response)

    transport = httpx.MockTransport(mock_handler)
    client = RAGClient(base_url="http://chat:8000", transport=transport)

    answer = await client.ask("what is kubernetes", collection=None)
    expected = (
        "Kubernetes is a container orchestration platform for automating deployment."
    )
    assert answer["answer"] == expected
    assert len(answer["sources"]) == 1


@pytest.mark.asyncio
async def test_search_server_error():
    async def mock_handler(request: httpx.Request) -> httpx.Response:
        return httpx.Response(500, json={"detail": "internal error"})

    transport = httpx.MockTransport(mock_handler)
    client = RAGClient(base_url="http://chat:8000", transport=transport)

    with pytest.raises(httpx.HTTPStatusError):
        await client.search("test", collection=None, limit=5)


@pytest.mark.asyncio
async def test_ask_timeout():
    async def mock_handler(request: httpx.Request) -> httpx.Response:
        raise httpx.ConnectTimeout("connection timed out")

    transport = httpx.MockTransport(mock_handler)
    client = RAGClient(base_url="http://chat:8000", transport=transport)

    with pytest.raises(httpx.ConnectTimeout):
        await client.ask("test", collection=None)
