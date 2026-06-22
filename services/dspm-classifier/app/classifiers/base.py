"""Classifier Protocol and shared dataclasses."""

from __future__ import annotations

from dataclasses import dataclass, field
from typing import Protocol


@dataclass(frozen=True)
class Match:
    category: str  # e.g. "PII.SSN", "SECRETS.AWS_KEY", "PII.EMAIL"
    span: tuple[int, int]  # (start, end) char offsets
    text: str
    confidence: float  # 0..1


@dataclass(frozen=True)
class ClassificationResult:
    matches: tuple[Match, ...] = ()
    confidence: float = 1.0  # classifier's confidence in its own output
    needs_escalation: bool = False

    @property
    def has_matches(self) -> bool:
        return len(self.matches) > 0


@dataclass(frozen=True)
class PipelineOutput:
    matches: tuple[Match, ...]
    categories: tuple[str, ...]
    sensitivity: int  # Sensitivity int value
    llm_failed: bool = False
    reason: str | None = None
    stages_run: tuple[str, ...] = field(default_factory=tuple)


class Classifier(Protocol):
    async def classify(self, content: str) -> ClassificationResult: ...
