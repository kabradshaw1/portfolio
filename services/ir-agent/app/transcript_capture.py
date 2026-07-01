"""Build a static, replayable transcript of one investigation run.

A transcript is the exact ordered event sequence the ``/investigate`` SSE endpoint
would stream — captured once against the real Anthropic API and served by the
frontend as a free, always-on demo (no key, no live cost). It reuses
``run_investigation_events`` so the captured wire shape is identical to a live run.
"""

from __future__ import annotations

from app import fixtures_store
from app.main import run_investigation_events
from app.usage import RunAccountant


def build_transcript(
    graph_app,
    *,
    incident_id: str,
    title: str,
    role_models: dict[str, str],
    captured_at: str,
) -> dict:
    """Run one incident through the graph and return its replayable transcript.

    The returned dict is JSON-serializable and self-describing: metadata plus an
    ordered ``events`` list of ``{"event": name, "data": <jsonable>}`` frames,
    ending with the ``summary`` (real token/cost/latency numbers) and ``done``.
    """
    accountant = RunAccountant(role_models)
    incident = fixtures_store.load_incident(incident_id)
    start_state = {
        "incident": incident,
        "evidence": [],
        "investigate_attempts": 0,
    }
    events = [
        {"event": name, "data": data}
        for name, data in run_investigation_events(graph_app, start_state, accountant)
    ]
    return {
        "incident_id": incident_id,
        "title": title,
        "captured_at": captured_at,
        "events": events,
    }
