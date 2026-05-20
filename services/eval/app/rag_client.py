from __future__ import annotations

import time

import httpx

from app.metrics import (
    eval_upstream_failures_total,
    eval_upstream_request_duration_seconds,
)


class RAGClient:
    """HTTP client for the chat service's /search and /chat endpoints."""

    def __init__(
        self,
        base_url: str,
        transport: httpx.AsyncBaseTransport | None = None,
        internal_token: str = "",
    ):
        client_kwargs = {"base_url": base_url, "timeout": 60.0}
        if transport:
            client_kwargs["transport"] = transport
        self._client = httpx.AsyncClient(**client_kwargs)
        self._internal_token = internal_token

    def _headers(self, extra: dict[str, str] | None = None) -> dict[str, str]:
        headers = dict(extra or {})
        if self._internal_token:
            headers["X-RAG-Internal-Token"] = self._internal_token
        return headers

    def _record_success(
        self, endpoint: str, rerank: bool, status_code: int, started_at: float
    ) -> None:
        eval_upstream_request_duration_seconds.labels(
            endpoint=endpoint,
            status=str(status_code),
            requested_rerank=str(rerank).lower(),
        ).observe(time.perf_counter() - started_at)

    def _record_failure(
        self, endpoint: str, rerank: bool, failure_type: str, started_at: float
    ) -> None:
        requested_rerank = str(rerank).lower()
        eval_upstream_failures_total.labels(
            endpoint=endpoint,
            failure_type=failure_type,
            requested_rerank=requested_rerank,
        ).inc()
        eval_upstream_request_duration_seconds.labels(
            endpoint=endpoint,
            status=failure_type,
            requested_rerank=requested_rerank,
        ).observe(time.perf_counter() - started_at)

    def _failure_type(self, exc: Exception) -> str:
        if isinstance(exc, httpx.HTTPStatusError):
            status_code = exc.response.status_code
            if status_code >= 500:
                return "http_5xx"
            if status_code >= 400:
                return "http_4xx"
            return f"http_{status_code}"
        if isinstance(exc, httpx.TimeoutException):
            return "timeout"
        if isinstance(exc, httpx.RequestError):
            return "request_error"
        return "unknown"

    async def search(
        self,
        query: str,
        collection: str | None,
        limit: int,
        rerank: bool = False,
    ) -> list[dict]:
        """Call POST /search for retrieval-only results."""
        body: dict = {"query": query, "limit": limit, "rerank": rerank}
        if collection:
            body["collection"] = collection

        started_at = time.perf_counter()
        try:
            resp = await self._client.post(
                "/search", json=body, headers=self._headers()
            )
            resp.raise_for_status()
        except Exception as exc:
            self._record_failure("search", rerank, self._failure_type(exc), started_at)
            raise
        self._record_success("search", rerank, resp.status_code, started_at)
        return resp.json()["results"]

    async def ask(
        self,
        question: str,
        collection: str | None,
        rerank: bool = False,
        retrieval_config: dict | None = None,
        answer_model: dict | None = None,
    ) -> dict:
        """Call POST /chat with Accept: application/json for a full RAG response."""
        body: dict = {"question": question, "rerank": rerank}
        if collection:
            body["collection"] = collection
        if retrieval_config:
            body["retrieval_config"] = retrieval_config
        if answer_model is not None:
            body["answer_model"] = answer_model

        started_at = time.perf_counter()
        try:
            resp = await self._client.post(
                "/chat",
                json=body,
                headers=self._headers({"Accept": "application/json"}),
            )
            resp.raise_for_status()
        except Exception as exc:
            self._record_failure("chat", rerank, self._failure_type(exc), started_at)
            raise
        self._record_success("chat", rerank, resp.status_code, started_at)
        return resp.json()

    async def close(self):
        await self._client.aclose()
