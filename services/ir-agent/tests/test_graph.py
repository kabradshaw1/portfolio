from app.graph import build_graph
from app.models import (
    Findings,
    Incident,
    IRReport,
    TriageResult,
    ValidationVerdict,
)
from langchain_core.messages import AIMessage

from tests.conftest import FakeChatModel


def _models(*, verdicts):
    """Build a role->model map. `verdicts` is a list the validate model
    returns on successive invocations (drives the loop)."""

    class _SequencedValidate:
        def __init__(self, seq):
            self._seq = list(seq)

        def with_structured_output(self, _schema):
            outer = self

            class _R:
                def invoke(self, _m):
                    return outer._seq.pop(0)

            return _R()

    return {
        "triage": FakeChatModel(
            structured=TriageResult(
                severity="high", category="phishing", confidence=0.9, rationale="r"
            )
        ),
        "investigate": FakeChatModel(
            structured=Findings(
                summary="s", hypothesis="h", evidence_refs=["search_alerts-0"]
            ),
            tool_script=[
                AIMessage(
                    content="",
                    tool_calls=[
                        {
                            "name": "search_alerts",
                            "args": {"query": "login"},
                            "id": "c1",
                            "type": "tool_call",
                        }
                    ],
                ),
                AIMessage(content="done"),
            ],
        ),
        "validate": _SequencedValidate(verdicts),
        "report": FakeChatModel(
            structured=IRReport(executive_summary="e", severity="high", confidence=0.8)
        ),
    }


def _start_state():
    return {
        "incident": Incident(id="INC-PHISH-001", source="email-gw", title="t"),
        "evidence": [],
        "investigate_attempts": 0,
    }


def test_happy_path_reaches_report():
    grounded = ValidationVerdict(grounded=True, needs_more_investigation=False)
    app = build_graph(_models(verdicts=[grounded]), max_tool_steps=4, max_attempts=2)
    out = app.invoke(_start_state())
    assert out["report"].severity == "high"
    assert out["investigate_attempts"] == 1


def test_ungrounded_then_grounded_loops_once():
    bad = ValidationVerdict(
        grounded=False, needs_more_investigation=True, gaps=["no IOC checked"]
    )
    good = ValidationVerdict(grounded=True, needs_more_investigation=False)
    app = build_graph(_models(verdicts=[bad, good]), max_tool_steps=4, max_attempts=2)
    out = app.invoke(_start_state())
    assert out["report"] is not None
    assert out["investigate_attempts"] == 2  # investigated twice


def test_loop_is_bounded():
    bad = ValidationVerdict(grounded=False, needs_more_investigation=True)
    # always ungrounded; cap=2 means investigate runs at most twice, then report
    app = build_graph(
        _models(verdicts=[bad, bad, bad]), max_tool_steps=4, max_attempts=2
    )
    out = app.invoke(_start_state())
    assert out["report"] is not None
    assert out["investigate_attempts"] == 2
