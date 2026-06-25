"""Pipeline — thin runner that wraps each stage with cross-cutting concerns.

execute()/execute_stream() own per-stage metrics, an OpenTelemetry span, bound
structured-logging context, and error classification, so individual stages and
consumers do not reimplement them. Control flow stays in the consumer.
"""

from __future__ import annotations

import time
from collections.abc import AsyncIterator

import structlog
from opentelemetry import trace

from shared.orchestration.context import PipelineContext
from shared.orchestration.errors import classify_error
from shared.orchestration.metrics import STAGE_DURATION
from shared.orchestration.stage import Stage, StreamingStage

logger = structlog.get_logger()
_tracer = trace.get_tracer(__name__)


class Pipeline:
    """Coordinates stage execution with standardized observability."""

    def __init__(self, name: str) -> None:
        self.name = name

    async def execute(self, stage: Stage, ctx: PipelineContext) -> PipelineContext:
        """Run one transform stage with metrics, tracing, log context, and
        error classification."""
        start = time.perf_counter()
        status = "ok"
        with _tracer.start_as_current_span(f"stage.{stage.name}"):
            tokens = structlog.contextvars.bind_contextvars(
                pipeline=self.name, stage=stage.name
            )
            try:
                return await stage.run(ctx)
            except Exception as exc:
                status = "error"
                err = classify_error(exc, stage=stage.name)
                logger.warning(
                    "stage_error",
                    error=str(err),
                    retryable=err.retryable,
                    exc_info=True,
                )
                raise err from exc
            finally:
                STAGE_DURATION.labels(self.name, stage.name, status).observe(
                    time.perf_counter() - start
                )
                structlog.contextvars.reset_contextvars(**tokens)

    async def run(self, stages: list[Stage], ctx: PipelineContext) -> PipelineContext:
        """Execute stages in order (the linear case)."""
        for stage in stages:
            ctx = await self.execute(stage, ctx)
        return ctx

    async def execute_stream(
        self, stage: StreamingStage, ctx: PipelineContext
    ) -> AsyncIterator[dict]:
        """Run a streaming stage, yielding its events, with the same
        cross-cutting wrapping as execute()."""
        start = time.perf_counter()
        status = "ok"
        with _tracer.start_as_current_span(f"stage.{stage.name}"):
            tokens = structlog.contextvars.bind_contextvars(
                pipeline=self.name, stage=stage.name
            )
            try:
                async for event in stage.stream(ctx):
                    yield event
            except Exception as exc:
                status = "error"
                err = classify_error(exc, stage=stage.name)
                logger.warning("stage_error", error=str(err), exc_info=True)
                raise err from exc
            finally:
                STAGE_DURATION.labels(self.name, stage.name, status).observe(
                    time.perf_counter() - start
                )
                structlog.contextvars.reset_contextvars(**tokens)
