import pytest
from app.models import (
    EvidenceItem,
    Findings,
    Incident,
    IRReport,
    Observable,
    TriageResult,
    ValidationVerdict,
)
from pydantic import ValidationError


def test_incident_round_trips():
    inc = Incident(
        id="INC-001",
        source="email-gw",
        title="Suspicious login after phishing email",
        raw={"user": "jdoe"},
        observables=[Observable(kind="user", value="jdoe")],
    )
    assert inc.id == "INC-001"
    assert inc.observables[0].kind == "user"


def test_triage_rejects_bad_severity():
    with pytest.raises(ValidationError):
        TriageResult(
            severity="catastrophic",  # not in the Literal
            category="phishing",
            confidence=0.9,
            rationale="x",
        )


def test_triage_rejects_out_of_range_confidence():
    with pytest.raises(ValidationError):
        TriageResult(
            severity="high",
            category="phishing",
            confidence=1.5,
            rationale="x",
        )


def test_findings_and_verdict_and_report_construct():
    ev = EvidenceItem(
        id="search_alerts-0",
        source_tool="search_alerts",
        query="jdoe",
        content="alert text",
    )
    f = Findings(
        summary="s",
        hypothesis="h",
        evidence_refs=["search_alerts-0"],
        iocs=["1.2.3.4"],
        affected_assets=["host-1"],
    )
    v = ValidationVerdict(
        grounded=True, unsupported_claims=[], gaps=[], needs_more_investigation=False
    )
    r = IRReport(
        executive_summary="e",
        timeline=["t1"],
        severity="high",
        iocs=["1.2.3.4"],
        mitre_attack=["T1566"],
        recommended_containment=["isolate host-1"],
        confidence=0.8,
    )
    assert ev.id in f.evidence_refs
    assert v.grounded is True
    assert r.severity == "high"
