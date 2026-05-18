"""Tests for shared tier-aware FastAPI rate limiting."""

import pytest

from shared.auth import AuthContext
from shared.rate_limits import (
    FixedWindowRateLimiter,
    RateLimit,
    parse_rate_limit,
)


class FakeClock:
    def __init__(self, now: float):
        self.now = now

    def __call__(self) -> float:
        return self.now


def test_tiered_fixed_window_allows_operator_more_than_user():
    limiter = FixedWindowRateLimiter(
        clock=FakeClock(0.0),
        policies={
            "eval_read": {
                "operator": RateLimit(max_requests=3, window_seconds=60),
                "user": RateLimit(max_requests=1, window_seconds=60),
            }
        },
    )
    operator = AuthContext(subject="kyle", email="kyle@example.test", tier="operator")
    user = AuthContext(subject="u1", email="u1@example.test", tier="user")

    assert limiter.check("eval_read", operator).allowed
    assert limiter.check("eval_read", operator).allowed
    assert limiter.check("eval_read", operator).allowed
    assert not limiter.check("eval_read", operator).allowed
    assert limiter.check("eval_read", user).allowed
    denied = limiter.check("eval_read", user)
    assert not denied.allowed
    assert denied.retry_after_seconds == 60


def test_fixed_window_resets_after_window():
    clock = FakeClock(0.0)
    limiter = FixedWindowRateLimiter(
        clock=clock,
        policies={
            "chat_ask": {
                "user": RateLimit(max_requests=1, window_seconds=60),
            }
        },
    )
    user = AuthContext(subject="u1", email=None, tier="user")

    assert limiter.check("chat_ask", user).allowed
    assert not limiter.check("chat_ask", user).allowed
    clock.now = 60.0
    assert limiter.check("chat_ask", user).allowed


def test_zero_limit_always_denies_with_retry_after():
    limiter = FixedWindowRateLimiter(
        clock=FakeClock(10.0),
        policies={
            "chat_ask": {
                "anonymous": RateLimit(max_requests=0, window_seconds=60),
            }
        },
    )
    anonymous = AuthContext(subject="anonymous", email=None, tier="anonymous")

    denied = limiter.check("chat_ask", anonymous)

    assert not denied.allowed
    assert denied.retry_after_seconds == 60


@pytest.mark.parametrize(
    ("value", "expected"),
    [
        ("30/minute", RateLimit(max_requests=30, window_seconds=60)),
        ("240/minute", RateLimit(max_requests=240, window_seconds=60)),
        ("0/minute", RateLimit(max_requests=0, window_seconds=60)),
    ],
)
def test_parse_rate_limit_accepts_minute_limits(value, expected):
    assert parse_rate_limit(value) == expected


@pytest.mark.parametrize("value", ["abc/minute", "1/hour", "-1/minute", "1/second"])
def test_parse_rate_limit_rejects_invalid_strings(value):
    with pytest.raises(ValueError):
        parse_rate_limit(value)
