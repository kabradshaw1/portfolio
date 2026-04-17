import pytest


@pytest.fixture(autouse=True)
def disable_rate_limiting():
    """Disable rate limiting in tests to prevent 429 interference."""
    try:
        from app.main import limiter

        limiter.enabled = False
        yield
        limiter.enabled = True
    except ImportError:
        # shared.auth not available in unit test environments without the full
        # service dependencies installed; rate limiting is not relevant for db tests.
        yield
