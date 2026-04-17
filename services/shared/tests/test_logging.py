"""Tests for shared.logging structured logging configuration."""

import json
import logging
from io import StringIO

import pytest

from shared.logging import configure_logging


def test_configure_logging_json_format():
    """JSON mode produces valid JSON with expected fields."""
    stream = StringIO()
    handler = logging.StreamHandler(stream)

    configure_logging(service_name="test-service", log_format="json", log_level="DEBUG")

    # Replace the root logger's handlers with our capturing handler
    root_logger = logging.getLogger()
    # Store original handlers and replace
    original_handlers = root_logger.handlers[:]
    for h in original_handlers:
        root_logger.removeHandler(h)

    # Re-run configure to get a fresh formatter on our handler
    # Instead, apply the formatter from configure_logging to our test handler
    configure_logging(service_name="test-service", log_format="json", log_level="DEBUG")
    if root_logger.handlers:
        formatter = root_logger.handlers[0].formatter
        handler.setFormatter(formatter)

    root_logger.addHandler(handler)

    try:
        logger = logging.getLogger("test.json")
        logger.info("hello json world")

        output = stream.getvalue().strip()
        assert output, "Expected log output, got nothing"

        parsed = json.loads(output)
        assert parsed["event"] == "hello json world"
        assert parsed["level"] == "info"
        assert "timestamp" in parsed
        assert parsed["service"] == "test-service"
    finally:
        root_logger.removeHandler(handler)
        for h in original_handlers:
            root_logger.addHandler(h)


def test_configure_logging_text_format():
    """Text mode produces human-readable output (not JSON)."""
    stream = StringIO()
    handler = logging.StreamHandler(stream)

    configure_logging(service_name="test-service", log_format="text", log_level="DEBUG")

    root_logger = logging.getLogger()
    original_handlers = root_logger.handlers[:]
    for h in original_handlers:
        root_logger.removeHandler(h)

    # Re-configure and grab the formatter
    configure_logging(service_name="test-service", log_format="text", log_level="DEBUG")
    if root_logger.handlers:
        formatter = root_logger.handlers[0].formatter
        handler.setFormatter(formatter)

    root_logger.addHandler(handler)

    try:
        logger = logging.getLogger("test.text")
        logger.info("hello text world")

        output = stream.getvalue().strip()
        assert output, "Expected log output, got nothing"

        # Text mode should NOT be valid JSON
        with pytest.raises((json.JSONDecodeError, ValueError)):
            json.loads(output)

        # But it should contain the event name
        assert "hello text world" in output
    finally:
        root_logger.removeHandler(handler)
        for h in original_handlers:
            root_logger.addHandler(h)
