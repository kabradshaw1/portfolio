"""Prometheus metrics for orchestration stages."""

from __future__ import annotations

from prometheus_client import Histogram

STAGE_DURATION = Histogram(
    "orchestration_stage_duration_seconds",
    "Wall-clock duration of a single orchestration stage execution.",
    labelnames=("pipeline", "stage", "status"),
)
