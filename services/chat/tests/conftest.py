# Shared fixtures for chat service tests

import pytest


@pytest.fixture(autouse=True)
def disable_rate_limiting():
    """Disable rate limiting in tests to prevent 429 interference."""
    from app.main import chat_rate_limiter, limiter

    limiter.enabled = False
    chat_rate_limiter.enabled = False
    yield
    limiter.enabled = True
    chat_rate_limiter.enabled = True
