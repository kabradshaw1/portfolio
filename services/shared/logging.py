"""Structured logging configuration using structlog.

All Python services import and call configure_logging() at startup to get
consistent JSON (production) or text (development) log output with automatic
fields: timestamp, level, service name, and OpenTelemetry trace/span IDs.
"""

import logging
import sys
import time
import uuid
from typing import Any

import structlog
from starlette.middleware.base import BaseHTTPMiddleware
from starlette.requests import Request
from structlog.types import EventDict, WrappedLogger


def _add_otel_context(
    logger: WrappedLogger, method_name: str, event_dict: EventDict
) -> EventDict:
    """Add OpenTelemetry trace_id and span_id to every log record.

    Gracefully falls back to empty strings if opentelemetry is not installed
    or no active span exists.
    """
    try:
        from opentelemetry import trace

        span = trace.get_current_span()
        ctx = span.get_span_context()
        if ctx and ctx.is_valid:
            event_dict["trace_id"] = format(ctx.trace_id, "032x")
            event_dict["span_id"] = format(ctx.span_id, "016x")
        else:
            event_dict["trace_id"] = ""
            event_dict["span_id"] = ""
    except ImportError:
        event_dict["trace_id"] = ""
        event_dict["span_id"] = ""
    return event_dict


def _make_add_service_name(service_name: str):
    """Return a structlog processor that stamps every log with the service name."""

    def _add_service_name(
        logger: WrappedLogger, method_name: str, event_dict: EventDict
    ) -> EventDict:
        event_dict["service"] = service_name
        return event_dict

    return _add_service_name


def configure_logging(
    service_name: str,
    log_format: str = "json",
    log_level: str = "INFO",
) -> None:
    """Configure structlog and Python's root logger for structured logging.

    Args:
        service_name: Stamped onto every log record (e.g. "chat", "ingestion").
        log_format:   "json" for production JSON output; "text" for dev console.
        log_level:    Standard Python log level name (e.g. "INFO", "DEBUG").
    """
    renderer: Any
    if log_format == "json":
        renderer = structlog.processors.JSONRenderer()
    else:
        renderer = structlog.dev.ConsoleRenderer()

    # Step 1: configure structlog's internal pipeline.
    # The last processor must be wrap_for_formatter so that structlog hands
    # off the event dict to ProcessorFormatter (the stdlib bridge).
    structlog.configure(
        processors=[
            structlog.contextvars.merge_contextvars,
            structlog.processors.add_log_level,
            structlog.processors.TimeStamper(fmt="iso"),
            _make_add_service_name(service_name),
            _add_otel_context,
            structlog.processors.StackInfoRenderer(),
            structlog.processors.format_exc_info,
            structlog.stdlib.ProcessorFormatter.wrap_for_formatter,
        ],
        logger_factory=structlog.stdlib.LoggerFactory(),
        wrapper_class=structlog.stdlib.BoundLogger,
        cache_logger_on_first_use=True,
    )

    # Shared pre-chain processors applied to ALL log records (both structlog
    # and foreign stdlib records).  These run before the final renderer.
    shared_processors = [
        structlog.contextvars.merge_contextvars,
        structlog.processors.add_log_level,
        structlog.processors.TimeStamper(fmt="iso"),
        _make_add_service_name(service_name),
        _add_otel_context,
        structlog.processors.StackInfoRenderer(),
        structlog.processors.format_exc_info,
    ]

    # Step 2: build the stdlib formatter that handles final rendering.
    # foreign_pre_chain applies shared_processors to non-structlog log records
    # (i.e. any logging.getLogger(__name__).info(...) call).
    formatter = structlog.stdlib.ProcessorFormatter(
        processors=[
            structlog.stdlib.ProcessorFormatter.remove_processors_meta,
            renderer,
        ],
        foreign_pre_chain=shared_processors,
    )

    # Step 3: attach a StreamHandler with the structlog formatter to the root
    # logger so all logging.getLogger(__name__) calls flow through it.
    handler = logging.StreamHandler(sys.stdout)
    handler.setFormatter(formatter)

    root_logger = logging.getLogger()
    # Remove any existing handlers to avoid duplicate output on re-configure.
    root_logger.handlers.clear()
    root_logger.addHandler(handler)
    root_logger.setLevel(log_level.upper())

    # Step 4: quiet noisy third-party loggers.
    logging.getLogger("uvicorn.access").setLevel(logging.WARNING)


class RequestLoggingMiddleware(BaseHTTPMiddleware):
    """Starlette middleware that adds per-request structured logging context.

    For every incoming request:
    - Generates a UUID4 request_id and binds it (plus method + path) to
      structlog's contextvars so every log line emitted during the request
      automatically carries those fields — no manual passing needed.
    - Logs ``request_finished`` with status_code and duration_ms on success.
    - Logs ``request_failed`` with exc_info on unhandled exception, then
      re-raises so FastAPI's exception handlers still fire.
    - Sets the ``x-request-id`` response header for client correlation.
    """

    async def dispatch(self, request: Request, call_next):
        request_id = str(uuid.uuid4())
        structlog.contextvars.clear_contextvars()
        structlog.contextvars.bind_contextvars(
            request_id=request_id,
            method=request.method,
            path=request.url.path,
        )

        logger = structlog.get_logger()
        start = time.perf_counter()

        try:
            response = await call_next(request)
        except Exception:
            logger.error("request_failed", exc_info=True)
            raise
        else:
            duration_ms = round((time.perf_counter() - start) * 1000, 1)
            logger.info(
                "request_finished",
                status_code=response.status_code,
                duration_ms=duration_ms,
            )
            response.headers["x-request-id"] = request_id
            return response
