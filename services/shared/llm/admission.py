"""Async admission control for local LLM and embedding workloads."""

from __future__ import annotations

import asyncio
import math
import os
from dataclasses import dataclass


class AdmissionRejected(RuntimeError):
    def __init__(self, retry_after_seconds: int):
        self.retry_after_seconds = retry_after_seconds
        super().__init__("LLM admission queue timeout")


@dataclass
class AdmissionPermit:
    _semaphore: asyncio.Semaphore
    _released: bool = False

    def release(self) -> None:
        if self._released:
            return
        self._released = True
        self._semaphore.release()


class AsyncAdmissionLimiter:
    def __init__(self, max_in_flight: int, queue_timeout_seconds: float):
        if max_in_flight <= 0:
            raise ValueError("max_in_flight must be positive")
        if queue_timeout_seconds <= 0:
            raise ValueError("queue_timeout_seconds must be positive")
        self.max_in_flight = max_in_flight
        self.queue_timeout_seconds = queue_timeout_seconds
        self._semaphore = asyncio.Semaphore(max_in_flight)

    @classmethod
    def from_env(
        cls,
        *,
        max_key: str,
        timeout_key: str,
        default_max: int,
        default_timeout_seconds: float,
    ) -> AsyncAdmissionLimiter:
        max_in_flight = int(os.getenv(max_key, str(default_max)))
        queue_timeout_seconds = float(
            os.getenv(timeout_key, str(default_timeout_seconds))
        )
        return cls(
            max_in_flight=max_in_flight,
            queue_timeout_seconds=queue_timeout_seconds,
        )

    async def acquire(self) -> AdmissionPermit:
        try:
            await asyncio.wait_for(
                self._semaphore.acquire(),
                timeout=self.queue_timeout_seconds,
            )
        except TimeoutError as exc:
            retry_after = max(1, math.ceil(self.queue_timeout_seconds))
            raise AdmissionRejected(retry_after_seconds=retry_after) from exc
        return AdmissionPermit(self._semaphore)


generate_limiter = AsyncAdmissionLimiter.from_env(
    max_key="OLLAMA_GENERATE_MAX_IN_FLIGHT",
    timeout_key="OLLAMA_ADMISSION_QUEUE_TIMEOUT",
    default_max=2,
    default_timeout_seconds=5.0,
)
embed_limiter = AsyncAdmissionLimiter.from_env(
    max_key="OLLAMA_EMBED_MAX_IN_FLIGHT",
    timeout_key="OLLAMA_ADMISSION_QUEUE_TIMEOUT",
    default_max=4,
    default_timeout_seconds=5.0,
)
rerank_limiter = AsyncAdmissionLimiter.from_env(
    max_key="RERANK_MAX_IN_FLIGHT",
    timeout_key="OLLAMA_ADMISSION_QUEUE_TIMEOUT",
    default_max=2,
    default_timeout_seconds=5.0,
)
