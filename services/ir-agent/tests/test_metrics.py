from app.metrics import (
    INVESTIGATE_ATTEMPTS,
    LLM_TOKENS,
    NODE_DURATION,
    SERVICE,
    TOOL_CALLS,
    instrumentator,
)


def test_service_label():
    assert SERVICE == "ir-agent"


def test_collectors_accept_labels():
    NODE_DURATION.labels(node="triage").observe(0.1)
    TOOL_CALLS.labels(tool="search_alerts").inc()
    INVESTIGATE_ATTEMPTS.observe(1)
    LLM_TOKENS.labels(role="triage", kind="prompt").inc(10)
    assert instrumentator is not None
