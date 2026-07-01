"""Transcript capture: the shared event generator and the transcript builder.

These cover the logic that makes pre-recorded demo transcripts byte-faithful to
live ``/investigate`` runs — both the SSE endpoint and the capture script consume
``run_investigation_events`` so the wire shape can never drift between them.
"""

import os

from app.models import (
    EvidenceItem,
    Findings,
    IRReport,
    TriageResult,
    ValidationVerdict,
)


def _fake_graph(record=None):
    """A graph stand-in that streams one of each node update.

    If ``record`` is given it is invoked with the run accountant pulled from the
    stream config, so tests can simulate per-role token usage flowing through.
    """

    class _FakeGraph:
        def stream(self, _state, *, config=None, **_kwargs):
            if record is not None and config:
                record(config["callbacks"][0])
            yield {
                "triage": {
                    "triage": TriageResult(
                        severity="high",
                        category="phishing",
                        confidence=0.9,
                        rationale="r",
                    )
                }
            }
            yield {
                "investigate": {
                    "evidence": [
                        EvidenceItem(
                            id="search_alerts-0",
                            source_tool="search_alerts",
                            query="q",
                            content="c",
                        ),
                        EvidenceItem(
                            id="get_logs-1",
                            source_tool="get_logs",
                            query="q2",
                            content="c2",
                        ),
                    ],
                    "investigate_attempts": 2,
                }
            }
            yield {
                "investigate": {
                    "findings": Findings(summary="s", hypothesis="h", iocs=["x"])
                }
            }
            yield {
                "validate": {
                    "verdict": ValidationVerdict(
                        grounded=True, needs_more_investigation=False
                    )
                }
            }
            yield {
                "report": {
                    "report": IRReport(
                        executive_summary="e", severity="high", confidence=0.8
                    )
                }
            }

    return _FakeGraph()


def test_run_investigation_events_ordered_shape():
    from app.main import ROLE_MODELS, run_investigation_events
    from app.usage import RunAccountant

    accountant = RunAccountant(ROLE_MODELS)
    events = list(run_investigation_events(_fake_graph(), {}, accountant))

    names = [name for name, _ in events]
    assert names == [
        "triage",
        "evidence",
        "findings",
        "verdict",
        "report",
        "summary",
        "done",
    ]

    data = dict(events)
    # Pydantic payloads are reduced to plain JSON-able dicts.
    assert data["triage"]["severity"] == "high"
    assert isinstance(data["evidence"], list) and len(data["evidence"]) == 2
    assert data["evidence"][0]["source_tool"] == "search_alerts"
    assert data["report"]["executive_summary"] == "e"
    # Summary carries run accounting derived from the stream.
    assert data["summary"]["tool_calls"] == 2
    assert data["summary"]["investigate_attempts"] == 2
    assert "comparison" in data["summary"]
    assert data["done"] == {}


def test_run_investigation_events_summary_reflects_usage():
    from app.main import ROLE_MODELS, run_investigation_events
    from app.usage import RunAccountant

    def record(acc):
        acc.record("triage", input_tokens=1000, output_tokens=200, seconds=0.5)
        acc.record("report", input_tokens=2000, output_tokens=800, seconds=1.0)

    accountant = RunAccountant(ROLE_MODELS)
    events = dict(run_investigation_events(_fake_graph(record), {}, accountant))
    summary = events["summary"]

    assert summary["per_role"]["triage"]["input_tokens"] == 1000
    assert summary["per_role"]["report"]["output_tokens"] == 800
    assert summary["totals"]["total_tokens"] == 4000
    assert summary["comparison"]["savings_factor"] >= 1.0


def test_build_transcript_shape():
    from app.main import ROLE_MODELS
    from app.transcript_capture import build_transcript

    def record(acc):
        acc.record("triage", input_tokens=1000, output_tokens=200, seconds=0.5)

    transcript = build_transcript(
        _fake_graph(record),
        incident_id="INC-PHISH-001",
        title="Phishing credential theft",
        role_models=ROLE_MODELS,
        captured_at="2026-06-30T00:00:00Z",
    )

    assert transcript["incident_id"] == "INC-PHISH-001"
    assert transcript["title"] == "Phishing credential theft"
    assert transcript["captured_at"] == "2026-06-30T00:00:00Z"
    names = [e["event"] for e in transcript["events"]]
    assert names == [
        "triage",
        "evidence",
        "findings",
        "verdict",
        "report",
        "summary",
        "done",
    ]
    # Every event's data is JSON-able (already reduced from pydantic models).
    import json

    json.dumps(transcript)
    summary = next(e for e in transcript["events"] if e["event"] == "summary")["data"]
    assert summary["per_role"]["triage"]["input_tokens"] == 1000


def test_capture_script_resolves_imports_without_pythonpath():
    """The script must bootstrap sys.path itself so it runs from a plain shell.

    Runs it in a subprocess with no PYTHONPATH and no API key: it should get
    *past* the ``app``/``shared`` imports and fail only at the key check —
    proving the path bootstrapping works without any env setup.
    """
    import subprocess
    import sys
    from pathlib import Path

    service_root = Path(__file__).resolve().parent.parent
    script = service_root / "scripts" / "capture_transcripts.py"
    env = {
        k: v
        for k, v in os.environ.items()
        if k not in ("PYTHONPATH", "IR_ANTHROPIC_API_KEY", "ANTHROPIC_API_KEY")
    }
    proc = subprocess.run(
        [sys.executable, str(script)],
        cwd=str(service_root),
        env=env,
        capture_output=True,
        text=True,
    )
    assert proc.returncode != 0
    # Reached settings.validate(), i.e. imports resolved — not a ModuleNotFoundError.
    assert "ModuleNotFoundError" not in proc.stderr
    assert "IR_ANTHROPIC_API_KEY is required" in proc.stderr


def _load_script():
    import importlib.util
    from pathlib import Path

    path = Path(__file__).resolve().parent.parent / "scripts" / "capture_transcripts.py"
    spec = importlib.util.spec_from_file_location("capture_transcripts", path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def test_capture_script_writes_transcripts_and_manifest(tmp_path, monkeypatch):
    import json

    import app.main as main_module

    def record(acc):
        acc.record("triage", input_tokens=1000, output_tokens=200, seconds=0.5)

    monkeypatch.setattr(main_module, "_build_graph_app", lambda: _fake_graph(record))

    script = _load_script()
    monkeypatch.setattr(
        script.sys,
        "argv",
        [
            "capture_transcripts.py",
            "--out",
            str(tmp_path),
            "--incident",
            "INC-PHISH-001",
        ],
    )
    assert script.main() == 0

    transcript = json.loads((tmp_path / "INC-PHISH-001.json").read_text())
    assert transcript["incident_id"] == "INC-PHISH-001"
    assert [e["event"] for e in transcript["events"]][-2:] == ["summary", "done"]

    manifest = json.loads((tmp_path / "index.json").read_text())
    assert len(manifest) == 1
    assert manifest[0]["incident_id"] == "INC-PHISH-001"
    assert manifest[0]["file"] == "INC-PHISH-001.json"
    assert manifest[0]["title"]
