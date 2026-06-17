"""Presidio + spaCy NER pass.

Maps Presidio entity types to our taxonomy. Sets `needs_escalation=True` when:
- Any returned entity confidence falls below `confidence_threshold`, OR
- All matches are NER-ambiguous types (PERSON/LOCATION) with no corroborating
  pattern-based PII signals — these require LLM confirmation to avoid false
  positives.

Unmapped entity types (e.g. DATE_TIME) are dropped; they are noise, not PII.
"""

from __future__ import annotations

import asyncio
from typing import ClassVar

from presidio_analyzer import AnalyzerEngine

from app.classifiers.base import ClassificationResult, Match

_PRESIDIO_TO_CATEGORY: dict[str, str] = {
    "EMAIL_ADDRESS": "PII.EMAIL",
    "PHONE_NUMBER": "PII.PHONE",
    "PERSON": "PII.PERSON",
    "LOCATION": "PII.LOCATION",
    "IP_ADDRESS": "PII.IP",
    "CREDIT_CARD": "FINANCIAL.CREDIT_CARD",
    "US_SSN": "PII.SSN",
    "IBAN_CODE": "FINANCIAL.IBAN",
    "MEDICAL_LICENSE": "HEALTH.MEDICAL_LICENSE",
}

# Entity types detected primarily via NLP rather than regex/patterns.
# Without corroborating PII signals they are ambiguous and need LLM review.
_NER_AMBIGUOUS: frozenset[str] = frozenset({"PII.PERSON", "PII.LOCATION"})


class PresidioClassifier:
    """Implements Classifier."""

    _SHARED_ENGINE: ClassVar[AnalyzerEngine | None] = None

    def __init__(self, *, confidence_threshold: float = 0.6, language: str = "en") -> None:
        if PresidioClassifier._SHARED_ENGINE is None:
            PresidioClassifier._SHARED_ENGINE = AnalyzerEngine()
        self._engine = PresidioClassifier._SHARED_ENGINE
        self._threshold = confidence_threshold
        self._language = language

    async def classify(self, content: str) -> ClassificationResult:
        results = await asyncio.to_thread(
            self._engine.analyze, text=content, language=self._language
        )

        matches: list[Match] = []
        low_conf = False
        for r in results:
            category = _PRESIDIO_TO_CATEGORY.get(r.entity_type)
            if category is None:
                # Drop unmapped entity types (DATE_TIME, NRP, etc.) — not PII.
                continue
            matches.append(
                Match(
                    category=category,
                    span=(r.start, r.end),
                    text=content[r.start : r.end],
                    confidence=float(r.score),
                )
            )
            if r.score < self._threshold:
                low_conf = True

        # Escalate when all matches are NER-ambiguous (e.g. only PERSON/LOCATION)
        # with no pattern-anchored PII signals; LLM tier confirms or rejects.
        only_ambiguous = bool(matches) and all(m.category in _NER_AMBIGUOUS for m in matches)

        return ClassificationResult(
            matches=tuple(matches),
            confidence=min((m.confidence for m in matches), default=1.0),
            needs_escalation=low_conf or only_ambiguous,
        )
