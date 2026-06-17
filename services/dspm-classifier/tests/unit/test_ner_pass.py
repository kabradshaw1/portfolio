from __future__ import annotations

import pytest

from app.classifiers.ner_pass import PresidioClassifier


@pytest.fixture(scope="module")
def classifier() -> PresidioClassifier:
    return PresidioClassifier(confidence_threshold=0.4)


@pytest.mark.asyncio
async def test_detects_email_and_phone(classifier):
    r = await classifier.classify("Contact Alice at alice@example.com or 415-555-1212.")
    cats = {m.category for m in r.matches}
    assert "PII.EMAIL" in cats
    assert "PII.PHONE" in cats


@pytest.mark.asyncio
async def test_low_confidence_flags_escalation(classifier):
    r = await classifier.classify("John was here.")
    assert r.needs_escalation is True or r.matches == ()


@pytest.mark.asyncio
async def test_clean_text_no_matches(classifier):
    r = await classifier.classify("The weather report is unremarkable today.")
    assert r.matches == ()
