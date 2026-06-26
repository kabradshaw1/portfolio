"""PipelineContext — typed state threaded through every stage."""

from __future__ import annotations

import uuid
from collections.abc import Awaitable, Callable
from typing import Any, Generic, TypeVar

StateT = TypeVar("StateT")


class PipelineContext(Generic[StateT]):
    """Carries pipeline state and request-scoped metadata across stages.

    Args:
        state: The per-pipeline typed payload that stages read and write.
        run_id: Correlation id for this run; a random hex id if omitted.
        metadata: Request-scoped data available to every stage (e.g. collection,
            prompt version, user).
        cancel_check: Optional coroutine invoked by ``check_cancelled``; it should
            raise ``CancelledPipelineError`` when the run should abort.
    """

    def __init__(
        self,
        state: StateT,
        *,
        run_id: str | None = None,
        metadata: dict[str, Any] | None = None,
        cancel_check: Callable[[], Awaitable[None]] | None = None,
    ) -> None:
        self.state = state
        self.run_id = run_id or uuid.uuid4().hex
        self.metadata: dict[str, Any] = metadata or {}
        self._cancel_check = cancel_check

    async def check_cancelled(self) -> None:
        """Invoke the cancellation predicate, if one was supplied."""
        if self._cancel_check is not None:
            await self._cancel_check()
