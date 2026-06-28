"""Pydantic data contracts for the IR agent graph.

Every node emits one of these validated models — never free text — so the
graph state is type-checked end to end.
"""

from __future__ import annotations

from typing import Literal

from pydantic import BaseModel, Field

Severity = Literal["low", "medium", "high", "critical"]
Category = Literal["phishing", "malware", "lateral-movement", "data-exfil", "other"]


class Observable(BaseModel):
    kind: Literal["ip", "hash", "domain", "user", "host", "email"]
    value: str


class Incident(BaseModel):
    id: str
    source: Literal["EDR", "SIEM", "email-gw"]
    title: str
    raw: dict = Field(default_factory=dict)
    observables: list[Observable] = Field(default_factory=list)


class TriageResult(BaseModel):
    severity: Severity
    category: Category
    confidence: float = Field(ge=0.0, le=1.0)
    rationale: str


class EvidenceItem(BaseModel):
    id: str
    source_tool: str
    query: str
    content: str


class Findings(BaseModel):
    summary: str
    hypothesis: str
    evidence_refs: list[str] = Field(default_factory=list)
    iocs: list[str] = Field(default_factory=list)
    affected_assets: list[str] = Field(default_factory=list)


class ValidationVerdict(BaseModel):
    grounded: bool
    unsupported_claims: list[str] = Field(default_factory=list)
    gaps: list[str] = Field(default_factory=list)
    needs_more_investigation: bool


class IRReport(BaseModel):
    executive_summary: str
    timeline: list[str] = Field(default_factory=list)
    severity: Severity
    iocs: list[str] = Field(default_factory=list)
    mitre_attack: list[str] = Field(default_factory=list)
    recommended_containment: list[str] = Field(default_factory=list)
    confidence: float = Field(ge=0.0, le=1.0)
