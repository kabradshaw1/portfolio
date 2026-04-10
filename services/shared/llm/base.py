"""Protocol classes defining the LLM and embedding provider interfaces."""

from __future__ import annotations

from collections.abc import AsyncIterator
from typing import Protocol, runtime_checkable


@runtime_checkable
class EmbeddingProvider(Protocol):
    async def embed(self, texts: list[str]) -> list[list[float]]:
        """Embed a list of texts into vectors."""
        ...

    async def check_health(self) -> bool:
        """Return True if the provider is reachable."""
        ...


@runtime_checkable
class LLMProvider(Protocol):
    async def generate(
        self,
        prompt: str,
        system: str,
        *,
        stream: bool = True,
    ) -> AsyncIterator[dict]:
        """Stream tokens as {"token": "..."} dicts, ending with metrics."""
        ...

    async def chat(
        self,
        messages: list[dict],
        tools: list[dict] | None = None,
    ) -> dict:
        """Non-streaming chat with optional tool calling.

        Returns a dict with "message" key containing "content" and
        optionally "tool_calls". Also includes token metrics.
        """
        ...

    async def check_health(self) -> bool:
        """Return True if the provider is reachable."""
        ...
