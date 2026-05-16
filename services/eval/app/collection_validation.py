from __future__ import annotations

import httpx
from fastapi import HTTPException


async def validate_collection_exists(ingestion_url: str, collection: str) -> None:
    """Raise HTTPException if the requested Qdrant collection is unavailable."""
    try:
        async with httpx.AsyncClient() as client:
            resp = await client.get(f"{ingestion_url}/collections", timeout=5.0)
            resp.raise_for_status()
    except httpx.HTTPError as exc:
        raise HTTPException(
            status_code=503,
            detail=f"unable to validate retrieval collection {collection!r}: {exc}",
        ) from exc

    collections = resp.json().get("collections", [])
    names = {item.get("name") for item in collections if isinstance(item, dict)}
    if collection not in names:
        raise HTTPException(
            status_code=422,
            detail=f'retrieval collection "{collection}" does not exist',
        )
