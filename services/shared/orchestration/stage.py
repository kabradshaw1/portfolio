"""Stage protocols — the unit of work in a pipeline."""

from __future__ import annotations

from collections.abc import AsyncIterator
from typing import Protocol, runtime_checkable

from shared.orchestration.context import PipelineContext


@runtime_checkable
class Stage(Protocol):
    """A transform stage: reads and writes context, returns it."""

    name: str

    async def run(self, ctx: PipelineContext) -> PipelineContext: ...


@runtime_checkable
class StreamingStage(Protocol):
    """A stage that yields progressive events (e.g. token deltas)."""

    name: str

    def stream(self, ctx: PipelineContext) -> AsyncIterator[dict]: ...
