from __future__ import annotations

import pytest

from app.classifiers.regex_pass import RegexClassifier


@pytest.mark.asyncio
async def test_detects_ssn():
    c = RegexClassifier()
    r = await c.classify("My SSN is 123-45-6789.")
    cats = {m.category for m in r.matches}
    assert "PII.SSN" in cats


@pytest.mark.asyncio
async def test_rejects_ssn_shaped_but_invalid():
    # SSN cannot start with 000, 666, or 9.
    c = RegexClassifier()
    r = await c.classify("ID number 000-12-3456 is not an SSN.")
    assert all(m.category != "PII.SSN" for m in r.matches)


@pytest.mark.asyncio
async def test_detects_credit_card_with_luhn():
    # Test Visa, passes Luhn.
    c = RegexClassifier()
    r = await c.classify("Card 4111 1111 1111 1111 declined.")
    cats = {m.category for m in r.matches}
    assert "FINANCIAL.CREDIT_CARD" in cats


@pytest.mark.asyncio
async def test_rejects_credit_card_failing_luhn():
    c = RegexClassifier()
    r = await c.classify("Card 4111 1111 1111 1112 invalid.")  # last digit changed
    assert all(m.category != "FINANCIAL.CREDIT_CARD" for m in r.matches)


@pytest.mark.asyncio
async def test_detects_jwt():
    jwt = "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxIn0.dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
    c = RegexClassifier()
    r = await c.classify(f"Authorization: Bearer {jwt}")
    assert any(m.category == "SECRETS.JWT" for m in r.matches)


@pytest.mark.asyncio
async def test_detects_aws_access_key():
    c = RegexClassifier()
    r = await c.classify("AKIAIOSFODNN7EXAMPLE is the prod key.")
    assert any(m.category == "SECRETS.AWS_KEY" for m in r.matches)


@pytest.mark.asyncio
async def test_no_matches_on_clean_text():
    c = RegexClassifier()
    r = await c.classify("This is an ordinary paragraph about nothing in particular.")
    assert r.matches == ()
    assert r.needs_escalation is False
