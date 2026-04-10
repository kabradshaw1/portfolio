"""Prometheus metrics for the debug service."""

from prometheus_client import Counter, Histogram
from prometheus_fastapi_instrumentator import Instrumentator

SERVICE = "debug"

instrumentator = Instrumentator(
    should_group_status_codes=False,
    excluded_handlers=["/health", "/metrics"],
)

# --- Ollama metrics ---
OLLAMA_REQUEST_DURATION = Histogram(
    "ollama_request_duration_seconds",
    "Wall-clock time for Ollama API calls",
    ["service", "model", "operation"],
    buckets=(0.1, 0.5, 1.0, 2.5, 5.0, 10.0, 30.0, 60.0, 120.0, 300.0),
)

OLLAMA_TOKENS = Counter(
    "ollama_tokens_total",
    "Total tokens processed by Ollama",
    ["service", "model", "kind"],
)

OLLAMA_EVAL_DURATION = Histogram(
    "ollama_eval_duration_seconds",
    "Ollama model evaluation duration (from response metadata)",
    ["service", "model"],
    buckets=(0.1, 0.5, 1.0, 2.5, 5.0, 10.0, 30.0, 60.0, 120.0),
)

EMBEDDING_DURATION = Histogram(
    "embedding_duration_seconds",
    "Time spent calling Ollama /api/embed",
    ["service", "model"],
    buckets=(0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0, 30.0, 60.0, 120.0),
)

# --- Agent loop metrics ---
AGENT_LOOP_ITERATIONS = Histogram(
    "agent_loop_iterations",
    "Number of agent loop iterations per debug session",
    ["service"],
    buckets=(1, 2, 3, 4, 5, 6, 7, 8, 9, 10),
)

AGENT_TOOL_CALLS = Counter(
    "agent_tool_calls_total",
    "Total tool calls made by the debug agent",
    ["tool", "result"],
)

AGENT_TOOL_DURATION = Histogram(
    "agent_tool_duration_seconds",
    "Time spent executing agent tools",
    ["tool"],
    buckets=(0.01, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0, 30.0),
)
