from typing import TypedDict

from app.models import (
    EvidenceItem,
    Findings,
    Incident,
    IRReport,
    TriageResult,
    ValidationVerdict,
)


class IRState(TypedDict, total=False):
    incident: Incident
    triage: TriageResult | None
    evidence: list[EvidenceItem]
    findings: Findings | None
    verdict: ValidationVerdict | None
    report: IRReport | None
    investigate_attempts: int
