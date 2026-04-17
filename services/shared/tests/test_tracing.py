"""Tests for shared.tracing OpenTelemetry configuration."""

import os
from unittest.mock import patch

from opentelemetry.sdk.trace import TracerProvider


def test_configure_tracing_returns_none_without_endpoint():
    """Without OTEL_EXPORTER_OTLP_ENDPOINT, returns None."""
    env = {k: v for k, v in os.environ.items() if k != "OTEL_EXPORTER_OTLP_ENDPOINT"}
    with patch.dict(os.environ, env, clear=True):
        from shared.tracing import configure_tracing

        result = configure_tracing(service_name="test")
        assert result is None


def test_configure_tracing_sets_service_name_with_endpoint():
    """With endpoint set, returns a TracerProvider with correct service name."""
    with patch.dict(
        os.environ, {"OTEL_EXPORTER_OTLP_ENDPOINT": "http://localhost:4317"}
    ):
        from shared.tracing import configure_tracing

        provider = configure_tracing(service_name="my-service")
        assert isinstance(provider, TracerProvider)
        attrs = dict(provider.resource.attributes)
        assert attrs.get("service.name") == "my-service"


def test_configure_tracing_no_endpoint_no_side_effects():
    """Without endpoint, calling configure_tracing is safe and has no side effects."""
    env = {k: v for k, v in os.environ.items() if k != "OTEL_EXPORTER_OTLP_ENDPOINT"}
    with patch.dict(os.environ, env, clear=True):
        from shared.tracing import configure_tracing

        # Should not raise anything
        result = configure_tracing(service_name="no-op-service")
        assert result is None


def test_instrument_app_does_not_raise():
    """instrument_app can be called without raising on a bare FastAPI app."""
    from fastapi import FastAPI

    from shared.tracing import instrument_app

    app = FastAPI()
    # Should not raise
    instrument_app(app)
