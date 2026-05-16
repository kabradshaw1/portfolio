import httpx
import pytest
import respx
from app.collection_validation import validate_collection_exists
from fastapi import HTTPException


@pytest.mark.asyncio
@respx.mock
async def test_validate_collection_exists_accepts_existing_collection():
    respx.get("http://ingestion/collections").mock(
        return_value=httpx.Response(
            200,
            json={"collections": [{"name": "documents", "points_count": 15}]},
        )
    )

    await validate_collection_exists("http://ingestion", "documents")


@pytest.mark.asyncio
@respx.mock
async def test_validate_collection_exists_rejects_missing_collection():
    respx.get("http://ingestion/collections").mock(
        return_value=httpx.Response(
            200,
            json={"collections": [{"name": "documents", "points_count": 15}]},
        )
    )

    with pytest.raises(HTTPException) as exc:
        await validate_collection_exists("http://ingestion", "missing")

    assert exc.value.status_code == 422
    assert 'retrieval collection "missing" does not exist' in exc.value.detail


@pytest.mark.asyncio
@respx.mock
async def test_validate_collection_exists_reports_dependency_failure():
    respx.get("http://ingestion/collections").mock(
        side_effect=httpx.ConnectError("boom")
    )

    with pytest.raises(HTTPException) as exc:
        await validate_collection_exists("http://ingestion", "documents")

    assert exc.value.status_code == 503
    assert "unable to validate retrieval collection" in exc.value.detail
