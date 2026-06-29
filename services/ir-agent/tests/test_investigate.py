from app.models import Findings, Incident
from app.nodes.investigate import make_investigate_node
from langchain_core.messages import AIMessage


def _tool_call(name, args, call_id):
    return {"name": name, "args": args, "id": call_id, "type": "tool_call"}


def test_investigate_runs_tools_then_produces_findings(make_fake_model):
    # First model turn: ask to search alerts; second turn: no tool calls (stop).
    tool_script = [
        AIMessage(
            content="",
            tool_calls=[_tool_call("search_alerts", {"query": "login"}, "c1")],
        ),
        AIMessage(content="done"),
    ]
    findings = Findings(
        summary="creds phished",
        hypothesis="account takeover",
        evidence_refs=["search_alerts-0"],
        iocs=["203.0.113.42"],
        affected_assets=["jdoe"],
    )
    model = make_fake_model(structured=findings, tool_script=tool_script)
    node = make_investigate_node(model, max_tool_steps=4)

    state = {"incident": Incident(id="INC-PHISH-001", source="email-gw", title="t")}
    out = node(state)

    assert out["findings"] == findings
    assert out["investigate_attempts"] == 1
    # one tool call ran -> one evidence item, with a stable id
    assert len(out["evidence"]) == 1
    assert out["evidence"][0].id == "search_alerts-0"
    assert out["evidence"][0].source_tool == "search_alerts"


def test_investigate_records_tool_calls_metric(make_fake_model):
    from prometheus_client import REGISTRY

    tool_script = [
        AIMessage(
            content="",
            tool_calls=[_tool_call("search_alerts", {"query": "login"}, "c1")],
        ),
        AIMessage(content="done"),
    ]
    model = make_fake_model(
        structured=Findings(summary="s", hypothesis="h"), tool_script=tool_script
    )
    node = make_investigate_node(model, max_tool_steps=4)

    before = (
        REGISTRY.get_sample_value("ir_tool_calls_total", {"tool": "search_alerts"})
        or 0.0
    )
    node({"incident": Incident(id="INC-PHISH-001", source="email-gw", title="t")})
    after = REGISTRY.get_sample_value("ir_tool_calls_total", {"tool": "search_alerts"})
    assert after == before + 1


def test_investigate_increments_attempts_and_keeps_prior_evidence(make_fake_model):
    findings = Findings(summary="s", hypothesis="h")
    model = make_fake_model(
        structured=findings, tool_script=[AIMessage(content="done")]
    )
    node = make_investigate_node(model, max_tool_steps=4)

    from app.models import EvidenceItem

    prior = [
        EvidenceItem(
            id="search_alerts-0", source_tool="search_alerts", query="x", content="old"
        )
    ]
    state = {
        "incident": Incident(id="INC-PHISH-001", source="email-gw", title="t"),
        "evidence": prior,
        "investigate_attempts": 1,
    }
    out = node(state)
    assert out["investigate_attempts"] == 2
    assert out["evidence"][0].id == "search_alerts-0"  # prior preserved
