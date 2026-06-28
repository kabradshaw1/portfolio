"""End-to-end run against the real Anthropic API.

Skipped unless RUN_LIVE_LLM=1 and IR_ANTHROPIC_API_KEY are set. This is the
only test that incurs API cost — run it manually a handful of times.
"""

import os

import pytest

RUN_LIVE = os.getenv("RUN_LIVE_LLM") == "1"

pytestmark = pytest.mark.skipif(
    not RUN_LIVE, reason="set RUN_LIVE_LLM=1 to run the live end-to-end smoke test"
)


def test_full_investigation_against_real_api():
    from app import fixtures_store
    from app.main import _build_graph_app

    graph_app = _build_graph_app()
    incident = fixtures_store.load_incident("INC-PHISH-001")
    out = graph_app.invoke(
        {"incident": incident, "evidence": [], "investigate_attempts": 0}
    )
    assert out["report"] is not None
    assert out["report"].severity in {"low", "medium", "high", "critical"}
    assert out["triage"] is not None
    assert out["investigate_attempts"] >= 1
