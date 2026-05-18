"""Tests for shared async LLM admission control."""

import pytest

from shared.llm.admission import AdmissionRejected, AsyncAdmissionLimiter


@pytest.mark.anyio
async def test_admission_rejects_after_queue_timeout():
    limiter = AsyncAdmissionLimiter(max_in_flight=1, queue_timeout_seconds=0.01)
    first = await limiter.acquire()
    with pytest.raises(AdmissionRejected) as exc:
        await limiter.acquire()
    assert exc.value.retry_after_seconds >= 1
    first.release()


@pytest.mark.anyio
async def test_admission_releases_permit():
    limiter = AsyncAdmissionLimiter(max_in_flight=1, queue_timeout_seconds=0.01)
    first = await limiter.acquire()
    first.release()
    second = await limiter.acquire()
    second.release()


@pytest.mark.parametrize(
    ("max_in_flight", "queue_timeout_seconds"),
    [(0, 1.0), (-1, 1.0), (1, 0), (1, -0.1)],
)
def test_admission_validates_config(max_in_flight, queue_timeout_seconds):
    with pytest.raises(ValueError):
        AsyncAdmissionLimiter(
            max_in_flight=max_in_flight,
            queue_timeout_seconds=queue_timeout_seconds,
        )
