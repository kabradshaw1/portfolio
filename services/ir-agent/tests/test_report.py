from app.models import Findings, IRReport, TriageResult, ValidationVerdict
from app.nodes.report import make_report_node


def test_report_node_sets_report(make_fake_model):
    report = IRReport(
        executive_summary="e",
        timeline=["t"],
        severity="high",
        iocs=["203.0.113.42"],
        mitre_attack=["T1566"],
        recommended_containment=["disable jdoe"],
        confidence=0.85,
    )
    model = make_fake_model(structured=report)
    node = make_report_node(model)
    state = {
        "triage": TriageResult(
            severity="high", category="phishing", confidence=0.9, rationale="r"
        ),
        "findings": Findings(summary="s", hypothesis="h"),
        "verdict": ValidationVerdict(grounded=True, needs_more_investigation=False),
    }
    out = node(state)
    assert out["report"] == report
