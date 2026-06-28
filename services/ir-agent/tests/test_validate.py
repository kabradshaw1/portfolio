from app.models import Findings, ValidationVerdict
from app.nodes.validate import make_validate_node, route_after_validate


def test_validate_node_sets_verdict(make_fake_model):
    verdict = ValidationVerdict(
        grounded=True, unsupported_claims=[], gaps=[], needs_more_investigation=False
    )
    model = make_fake_model(structured=verdict)
    node = make_validate_node(model)
    out = node({"findings": Findings(summary="s", hypothesis="h"), "evidence": []})
    assert out["verdict"] == verdict


def test_route_to_report_when_grounded():
    state = {
        "verdict": ValidationVerdict(grounded=True, needs_more_investigation=False),
        "investigate_attempts": 1,
    }
    assert route_after_validate(state, max_attempts=2) == "report"


def test_route_back_to_investigate_when_ungrounded_under_cap():
    state = {
        "verdict": ValidationVerdict(grounded=False, needs_more_investigation=True),
        "investigate_attempts": 1,
    }
    assert route_after_validate(state, max_attempts=2) == "investigate"


def test_route_to_report_when_cap_reached():
    state = {
        "verdict": ValidationVerdict(grounded=False, needs_more_investigation=True),
        "investigate_attempts": 2,
    }
    assert route_after_validate(state, max_attempts=2) == "report"
