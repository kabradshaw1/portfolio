"""Deterministic regex-based detectors for high-precision categories."""

from __future__ import annotations

import re

from app.classifiers.base import ClassificationResult, Match

# --- Patterns ----------------------------------------------------------------

_SSN_RE = re.compile(r"\b(?!000|666|9\d{2})\d{3}-(?!00)\d{2}-(?!0000)\d{4}\b")
_CC_RE = re.compile(r"\b(?:\d[ -]?){13,19}\b")
_JWT_RE = re.compile(r"\beyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\b")
_AWS_KEY_RE = re.compile(r"\b(AKIA|ASIA)[0-9A-Z]{16}\b")
_GENERIC_API_KEY_RE = re.compile(r"(?i)(?:api[_-]?key|secret|token)[\"'\s:=]+([A-Za-z0-9_\-]{24,})")


def _luhn_ok(digits: str) -> bool:
    total = 0
    for i, ch in enumerate(reversed(digits)):
        n = int(ch)
        if i % 2 == 1:
            n *= 2
            if n > 9:
                n -= 9
        total += n
    return total % 10 == 0


class RegexClassifier:
    """Implements Classifier."""

    async def classify(self, content: str) -> ClassificationResult:
        matches: list[Match] = []

        for m in _SSN_RE.finditer(content):
            matches.append(Match("PII.SSN", m.span(), m.group(), 1.0))

        for m in _CC_RE.finditer(content):
            digits = re.sub(r"[ -]", "", m.group())
            if 13 <= len(digits) <= 19 and _luhn_ok(digits):
                matches.append(Match("FINANCIAL.CREDIT_CARD", m.span(), m.group(), 1.0))

        for m in _JWT_RE.finditer(content):
            matches.append(Match("SECRETS.JWT", m.span(), m.group(), 0.95))

        for m in _AWS_KEY_RE.finditer(content):
            matches.append(Match("SECRETS.AWS_KEY", m.span(), m.group(), 1.0))

        for m in _GENERIC_API_KEY_RE.finditer(content):
            matches.append(Match("SECRETS.GENERIC", m.span(1), m.group(1), 0.7))

        return ClassificationResult(
            matches=tuple(matches),
            confidence=1.0,
            needs_escalation=False,
        )
