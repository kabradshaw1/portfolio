from prometheus_client import Counter
from prometheus_fastapi_instrumentator import Instrumentator

instrumentator = Instrumentator(excluded_handlers=["/health", "/metrics"])

triage_requests_total = Counter(
    "rag_triage_requests_total",
    "RAG triage requests by endpoint and outcome",
    ["endpoint", "outcome"],
)
