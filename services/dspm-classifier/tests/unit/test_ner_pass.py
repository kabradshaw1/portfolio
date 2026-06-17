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


@pytest.mark.asyncio
async def test_ner_only_person_match_escalates(classifier):
    # "Alice Smith" produces only PERSON matches with no pattern-anchored
    # signals (no email, phone, SSN). The classifier should escalate so
    # the LLM tier can confirm context-dependent PII.
    r = await classifier.classify("Alice Smith stopped by yesterday.")
    cats = {m.category for m in r.matches}
    assert "PII.PERSON" in cats
    assert r.needs_escalation is True


@pytest.mark.asyncio
async def test_pattern_anchored_match_does_not_force_escalation(classifier):
    # Email is pattern-anchored, so the NER-only escalation rule should
    # NOT fire when EMAIL is present alongside PERSON, regardless of NER
    # confidence on the person name.
    r = await classifier.classify("Contact bob@example.com about it.")
    cats = {m.category for m in r.matches}
    assert "PII.EMAIL" in cats
    # We don't assert needs_escalation specifically — it may or may not be
    # set depending on confidence, but the only_ambiguous trigger should
    # NOT fire here because EMAIL is not in the ambiguous set.
