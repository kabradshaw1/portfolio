from app.models import Incident, TriageResult
from app.nodes.triage import make_triage_node


def test_triage_node_sets_triage(make_fake_model):
    expected = TriageResult(
        severity="high",
        category="phishing",
        confidence=0.9,
        rationale="login from RO after phish",
    )
    model = make_fake_model(structured=expected)
    node = make_triage_node(model)
    state = {"incident": Incident(id="INC-PHISH-001", source="email-gw", title="t")}
    out = node(state)
    assert out["triage"] == expected
