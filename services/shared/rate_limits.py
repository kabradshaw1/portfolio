"""Tier-aware fixed-window rate limiting for FastAPI RAG services."""

from __future__ import annotations

import math
import time
from collections.abc import Awaitable, Callable
from dataclasses import dataclass

from fastapi import HTTPException, Request

from shared.auth import AuthContext


@dataclass(frozen=True)
class RateLimit:
    max_requests: int
    window_seconds: int


@dataclass(frozen=True)
class RateLimitDecision:
    allowed: bool
    retry_after_seconds: int


class FixedWindowRateLimiter:
    def __init__(
        self,
        policies: dict[str, dict[str, RateLimit]],
        clock: Callable[[], float] = time.time,
    ):
        self._policies = policies
        self._clock = clock
        self._counters: dict[tuple[str, str, str], tuple[float, int]] = {}

    def check(self, group: str, context: AuthContext) -> RateLimitDecision:
        policy = self._policies.get(group, {}).get(context.tier)
        if policy is None:
            raise ValueError(f"missing rate-limit policy for {group}/{context.tier}")
        now = self._clock()
        retry_after = policy.window_seconds
        if policy.max_requests <= 0:
            return RateLimitDecision(allowed=False, retry_after_seconds=retry_after)

        key = (group, context.tier, context.subject)
        window_start, count = self._counters.get(key, (now, 0))
        elapsed = now - window_start
        if elapsed >= policy.window_seconds:
            window_start = now
            count = 0
            elapsed = 0

        remaining = max(1, math.ceil(policy.window_seconds - elapsed))
        if count >= policy.max_requests:
            return RateLimitDecision(allowed=False, retry_after_seconds=remaining)

        self._counters[key] = (window_start, count + 1)
        return RateLimitDecision(allowed=True, retry_after_seconds=0)


def parse_rate_limit(value: str) -> RateLimit:
    count_text, separator, unit = value.partition("/")
    if separator != "/" or unit != "minute":
        raise ValueError(f"invalid rate limit {value!r}")
    try:
        max_requests = int(count_text)
    except ValueError as exc:
        raise ValueError(f"invalid rate limit {value!r}") from exc
    if max_requests < 0:
        raise ValueError(f"invalid rate limit {value!r}")
    return RateLimit(max_requests=max_requests, window_seconds=60)


def policies_from_settings(
    groups: dict[str, dict[str, str]],
) -> dict[str, dict[str, RateLimit]]:
    return {
        group: {tier: parse_rate_limit(limit) for tier, limit in tiers.items()}
        for group, tiers in groups.items()
    }


def rate_limit_dependency(
    group: str,
    limiter: FixedWindowRateLimiter,
    auth_context_dependency: Callable[..., Awaitable[AuthContext]],
):
    async def enforce(request: Request) -> AuthContext:
        context = await auth_context_dependency(request, None)
        decision = limiter.check(group, context)
        if not decision.allowed:
            raise HTTPException(
                status_code=429,
                detail="Rate limit exceeded",
                headers={"Retry-After": str(decision.retry_after_seconds)},
            )
        return context

    return enforce
