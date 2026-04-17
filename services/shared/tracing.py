"""OpenTelemetry tracing configuration for all Python services."""

from __future__ import annotations

import os

from opentelemetry import trace
from opentelemetry.instrumentation.fastapi import FastAPIInstrumentor
from opentelemetry.instrumentation.httpx import HTTPXClientInstrumentor
from opentelemetry.propagate import set_global_textmap
from opentelemetry.propagators.composite import CompositePropagator
from opentelemetry.sdk.resources import Resource
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import BatchSpanProcessor
from opentelemetry.trace.propagation.tracecontext import TraceContextTextMapPropagator


def configure_tracing(service_name: str) -> TracerProvider | None:
    """Configure OpenTelemetry tracing for a service.

    Always sets up W3C trace context propagation so incoming ``traceparent``
    headers from the Go ai-service (sent via otelhttp.NewTransport) are
    extracted and made available to structlog's ``_add_otel_context`` processor.

    When ``OTEL_EXPORTER_OTLP_ENDPOINT`` is set, creates a ``TracerProvider``
    with a gRPC OTLP exporter and registers it globally.  When the env var is
    absent (local dev / CI), returns ``None`` — no exporter, zero overhead, but
    propagation still works so trace context flows through log lines.

    Args:
        service_name: Stamped as ``service.name`` in the OTel resource
                      (e.g. ``"chat"``, ``"ingestion"``).

    Returns:
        The configured ``TracerProvider`` when an endpoint is configured,
        ``None`` otherwise.
    """
    set_global_textmap(CompositePropagator([TraceContextTextMapPropagator()]))

    endpoint = os.environ.get("OTEL_EXPORTER_OTLP_ENDPOINT", "")
    if not endpoint:
        return None

    from opentelemetry.exporter.otlp.proto.grpc.trace_exporter import (
        OTLPSpanExporter,
    )

    resource = Resource.create({"service.name": service_name})
    provider = TracerProvider(resource=resource)
    exporter = OTLPSpanExporter(endpoint=endpoint, insecure=True)
    provider.add_span_processor(BatchSpanProcessor(exporter))
    trace.set_tracer_provider(provider)

    return provider


def instrument_app(app) -> None:
    """Instrument a FastAPI app with OpenTelemetry auto-instrumentation.

    - ``FastAPIInstrumentor``: creates spans for every incoming HTTP request,
      reading the incoming ``traceparent`` header to continue distributed traces.
    - ``HTTPXClientInstrumentor``: propagates trace context on outbound httpx
      requests (e.g. calls to Ollama), so the full request chain is linked.

    Args:
        app: The FastAPI application instance to instrument.
    """
    FastAPIInstrumentor.instrument_app(app)
    HTTPXClientInstrumentor().instrument()
