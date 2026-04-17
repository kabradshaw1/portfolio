from prometheus_client import Counter, Gauge, Histogram

eval_run_duration_seconds = Histogram(
    "eval_run_duration_seconds",
    "Duration of a full evaluation run",
    buckets=[10, 30, 60, 120, 300, 600, 1200],
)

eval_ragas_score = Gauge(
    "eval_ragas_score",
    "Latest RAGAS metric score",
    ["metric"],
)

eval_queries_total = Counter(
    "eval_queries_total",
    "Total number of queries evaluated",
)
