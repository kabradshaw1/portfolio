"""Error taxonomy for orchestration stages.

Models the retryable-vs-permanent split so consumers (e.g. the eval worker's
retry/DLQ logic) can make uniform decisions instead of ad-hoc try/except.
"""

from __future__ import annotations


class StageError(Exception):
    """An error raised while executing a stage.

    Args:
        message: Human-readable description.
        stage: Name of the stage that failed.
        retryable: Whether re-running the stage could succeed.
    """

    def __init__(self, message: str, *, stage: str, retryable: bool) -> None:
        super().__init__(message)
        self.stage = stage
        self.retryable = retryable


class OutputValidationError(StageError):
    """LLM output failed schema validation after retries (permanent)."""

    def __init__(self, message: str, *, stage: str, retryable: bool = False) -> None:
        super().__init__(message, stage=stage, retryable=retryable)


class CancelledPipelineError(Exception):
    """Raised by a context cancellation check to abort a pipeline run."""


# Exceptions whose recurrence a retry could plausibly resolve.
_RETRYABLE = (ConnectionError, TimeoutError)


def classify_error(exc: Exception, *, stage: str) -> StageError:
    """Map an arbitrary exception to a StageError with a retryable verdict.

    Already-classified StageErrors pass through unchanged.
    """
    if isinstance(exc, StageError):
        return exc
    return StageError(str(exc), stage=stage, retryable=isinstance(exc, _RETRYABLE))
