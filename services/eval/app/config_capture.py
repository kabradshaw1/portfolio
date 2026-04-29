"""Snapshot the RAG configuration in effect for an evaluation run.

Calls the chat service's /config and the ingestion service's
/collections/{name}/config in parallel and merges the responses.
Failures never raise — they're recorded under `_capture_error` so the
evaluation still proceeds (quality data must not be blocked by metadata
gaps).
"""

from __future__ import annotations

import asyncio
from datetime import datetime, timezone

import httpx

_UTC = timezone.utc  # noqa: UP017


async def _fetch_json(client: httpx.AsyncClient, url: str) -> dict:
    resp = await client.get(url, timeout=5.0)
    resp.raise_for_status()
    return resp.json()


async def capture_run_config(
    chat_url: str,
    ingestion_url: str,
    collection: str,
) -> dict:
    """Return a merged RAG config snapshot. Always returns a dict.

    On partial or full upstream failure, populates `_capture_error` with a
    short reason while preserving any sub-result that did succeed.
    """
    captured_at = datetime.now(_UTC).isoformat()

    async with httpx.AsyncClient() as client:
        chat_res, coll_res = await asyncio.gather(
            _fetch_json(client, f"{chat_url}/config"),
            _fetch_json(client, f"{ingestion_url}/collections/{collection}/config"),
            return_exceptions=True,
        )

    out: dict = {"captured_at": captured_at}
    errors: list[str] = []

    if isinstance(chat_res, Exception):
        errors.append(f"chat: {chat_res!s}")
    else:
        out["chat"] = chat_res

    if isinstance(coll_res, Exception):
        errors.append(f"collection: {coll_res!s}")
    else:
        out["collection"] = coll_res

    if errors:
        out["_capture_error"] = "; ".join(errors)
    return out
