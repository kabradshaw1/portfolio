"""Prometheus metrics for the IR agent service."""

from prometheus_client import Counter, Histogram
from prometheus_fastapi_instrumentator import Instrumentator

SERVICE = "ir-agent"

instrumentator = Instrumentator(
    should_group_status_codes=False,
    excluded_handlers=["/health", "/metrics"],
)

NODE_DURATION = Histogram(
    "ir_node_duration_seconds",
    "Wall-clock time per graph node",
    ["node"],
    buckets=(0.1, 0.5, 1.0, 2.5, 5.0, 10.0, 30.0, 60.0, 120.0, 300.0),
)

TOOL_CALLS = Counter(
    "ir_tool_calls_total",
    "Total evidence-tool calls by the investigator",
    ["tool"],
)

INVESTIGATE_ATTEMPTS = Histogram(
    "ir_investigate_attempts",
    "Number of investigate passes per incident (validator loop)",
    buckets=(1, 2, 3, 4, 5),
)

LLM_TOKENS = Counter(
    "ir_llm_tokens_total",
    "Tokens used per role",
    ["role", "kind"],
)
