from __future__ import annotations

import httpx


class RAGClient:
    """HTTP client for the chat service's /search and /chat endpoints."""

    def __init__(
        self,
        base_url: str,
        transport: httpx.AsyncBaseTransport | None = None,
    ):
        client_kwargs = {"base_url": base_url, "timeout": 60.0}
        if transport:
            client_kwargs["transport"] = transport
        self._client = httpx.AsyncClient(**client_kwargs)

    async def search(
        self, query: str, collection: str | None, limit: int
    ) -> list[dict]:
        """Call POST /search for retrieval-only results."""
        body: dict = {"query": query, "limit": limit}
        if collection:
            body["collection"] = collection

        resp = await self._client.post("/search", json=body)
        resp.raise_for_status()
        return resp.json()["results"]

    async def ask(self, question: str, collection: str | None) -> dict:
        """Call POST /chat with Accept: application/json for a full RAG response."""
        body: dict = {"question": question}
        if collection:
            body["collection"] = collection

        resp = await self._client.post(
            "/chat", json=body, headers={"Accept": "application/json"}
        )
        resp.raise_for_status()
        return resp.json()

    async def close(self):
        await self._client.aclose()
