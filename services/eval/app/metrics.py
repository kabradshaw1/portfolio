from prometheus_client import Counter, Gauge, Histogram

eval_run_duration_seconds = Histogram(
    "eval_run_duration_seconds",
    "Duration of a full evaluation run",
    buckets=[10, 30, 60, 120, 300, 600, 1200],
)

eval_runs_total = Counter(
    "eval_runs_total",
    "Evaluation runs by terminal status",
    ["status", "requested_rerank"],
)

eval_items_total = Counter(
    "eval_items_total",
    "Evaluation items by processing status",
    ["status", "requested_rerank"],
)

eval_item_duration_seconds = Histogram(
    "eval_item_duration_seconds",
    "Evaluation item stage duration",
    ["stage", "requested_rerank"],
    buckets=[0.1, 0.5, 1, 2.5, 5, 10, 30, 60],
)

eval_upstream_request_duration_seconds = Histogram(
    "eval_upstream_request_duration_seconds",
    "Evaluation upstream request duration",
    ["endpoint", "status", "requested_rerank"],
    buckets=[0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60],
)

eval_upstream_failures_total = Counter(
    "eval_upstream_failures_total",
    "Evaluation upstream request failures",
    ["endpoint", "failure_type", "requested_rerank"],
)

eval_stale_running_runs = Gauge(
    "eval_stale_running_runs",
    "Running evaluation rows older than the configured stale threshold",
)

eval_quality_score = Gauge(
    "eval_ragas_score",
    "Latest RAG evaluation metric score",
    ["metric"],
)

eval_queries_total = Counter(
    "eval_queries_total",
    "Total number of queries evaluated",
)
