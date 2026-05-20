import httpx
import pytest
from app.collection_validation import validate_collection_exists
from fastapi import HTTPException


def _patch_async_transport(monkeypatch, transport: httpx.MockTransport) -> None:
    class MockedAsyncClient(httpx.AsyncClient):
        def __init__(self, *args, **kwargs):
            kwargs["transport"] = transport
            super().__init__(*args, **kwargs)

    monkeypatch.setattr(
        "app.collection_validation.httpx.AsyncClient", MockedAsyncClient
    )


@pytest.mark.asyncio
async def test_validate_collection_exists_accepts_existing_collection(monkeypatch):
    def handler(request: httpx.Request) -> httpx.Response:
        assert str(request.url) == "http://ingestion/collections"
        return httpx.Response(
            200,
            json={"collections": [{"name": "documents", "points_count": 15}]},
        )

    _patch_async_transport(monkeypatch, httpx.MockTransport(handler))

    await validate_collection_exists("http://ingestion", "documents")


@pytest.mark.asyncio
async def test_validate_collection_exists_rejects_missing_collection(monkeypatch):
    def handler(request: httpx.Request) -> httpx.Response:
        assert str(request.url) == "http://ingestion/collections"
        return httpx.Response(
            200,
            json={"collections": [{"name": "documents", "points_count": 15}]},
        )

    _patch_async_transport(monkeypatch, httpx.MockTransport(handler))

    with pytest.raises(HTTPException) as exc:
        await validate_collection_exists("http://ingestion", "missing")

    assert exc.value.status_code == 422
    assert 'retrieval collection "missing" does not exist' in exc.value.detail


@pytest.mark.asyncio
async def test_validate_collection_exists_reports_dependency_failure(monkeypatch):
    def handler(request: httpx.Request) -> httpx.Response:
        raise httpx.ConnectError("boom", request=request)

    _patch_async_transport(monkeypatch, httpx.MockTransport(handler))

    with pytest.raises(HTTPException) as exc:
        await validate_collection_exists("http://ingestion", "documents")

    assert exc.value.status_code == 503
    assert "unable to validate retrieval collection" in exc.value.detail
