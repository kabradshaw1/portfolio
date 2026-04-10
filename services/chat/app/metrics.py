"""Prometheus metrics for the chat service."""

from prometheus_client import Counter, Histogram
from prometheus_fastapi_instrumentator import Instrumentator

SERVICE = "chat"

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

# --- Qdrant metrics ---
QDRANT_SEARCH_DURATION = Histogram(
    "qdrant_search_duration_seconds",
    "Time spent on Qdrant search operations",
    ["collection"],
    buckets=(0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0),
)

QDRANT_SEARCH_RESULTS = Histogram(
    "qdrant_search_results",
    "Number of results returned by Qdrant search",
    ["collection"],
    buckets=(0, 1, 2, 3, 5, 10, 20),
)

# --- RAG pipeline metrics ---
RAG_PIPELINE_DURATION = Histogram(
    "rag_pipeline_duration_seconds",
    "RAG pipeline stage durations",
    ["stage"],
    buckets=(0.01, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0, 30.0, 60.0),
)

RAG_PIPELINE_ERRORS = Counter(
    "rag_pipeline_errors_total",
    "RAG pipeline errors by stage",
    ["stage"],
)
