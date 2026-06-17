from __future__ import annotations

import pytest

from app.classifiers.base import ClassificationResult, Match
from app.classifiers.pipeline import Pipeline
from app.models.events import Sensitivity


class _Stub:
    def __init__(self, result: ClassificationResult) -> None:
        self._result = result
        self.called = 0

    async def classify(self, content: str) -> ClassificationResult:
        self.called += 1
        return self._result


@pytest.mark.asyncio
async def test_regex_hit_skips_ner_and_llm():
    regex = _Stub(
        ClassificationResult(
            matches=(Match("PII.SSN", (0, 11), "123-45-6789", 1.0),),
            confidence=1.0,
            needs_escalation=False,
        )
    )
    ner = _Stub(ClassificationResult())
    llm = _Stub(ClassificationResult())

    p = Pipeline(regex=regex, ner=ner, llm=llm, escalate_to_llm=True)
    out = await p.run("My SSN is 123-45-6789")

    assert regex.called == 1
    assert ner.called == 0
    assert llm.called == 0
    assert out.sensitivity == int(Sensitivity.HIGH)
    assert "PII" in out.categories


@pytest.mark.asyncio
async def test_ner_low_conf_escalates_to_llm():
    regex = _Stub(ClassificationResult())
    ner = _Stub(
        ClassificationResult(
            matches=(Match("PII.PERSON", (0, 4), "John", 0.3),),
            confidence=0.3,
            needs_escalation=True,
        )
    )
    llm = _Stub(
        ClassificationResult(
            matches=(Match("PII.PERSON", (0, 0), "John Doe", 0.92),),
            confidence=0.92,
            needs_escalation=False,
        )
    )

    p = Pipeline(regex=regex, ner=ner, llm=llm, escalate_to_llm=True)
    out = await p.run("John was here.")

    assert llm.called == 1
    assert out.sensitivity >= int(Sensitivity.MEDIUM)
    assert "llm" in out.stages_run


@pytest.mark.asyncio
async def test_escalate_disabled_does_not_call_llm():
    regex = _Stub(ClassificationResult())
    ner = _Stub(
        ClassificationResult(
            matches=(Match("PII.PERSON", (0, 4), "John", 0.3),),
            needs_escalation=True,
        )
    )
    llm = _Stub(ClassificationResult())

    p = Pipeline(regex=regex, ner=ner, llm=llm, escalate_to_llm=False)
    out = await p.run("John was here.")

    assert llm.called == 0
    assert "llm" not in out.stages_run


@pytest.mark.asyncio
async def test_dedupes_overlapping_spans():
    regex = _Stub(
        ClassificationResult(
            matches=(Match("PII.SSN", (10, 21), "123-45-6789", 1.0),),
        )
    )
    ner = _Stub(
        ClassificationResult(
            matches=(Match("PII.SSN", (10, 21), "123-45-6789", 0.8),),
        )
    )
    llm = _Stub(ClassificationResult())

    p = Pipeline(regex=regex, ner=ner, llm=llm, escalate_to_llm=False)
    out = await p.run("My SSN is 123-45-6789")

    assert sum(1 for m in out.matches if m.category == "PII.SSN") == 1


@pytest.mark.asyncio
async def test_no_matches_is_sensitivity_none():
    regex = _Stub(ClassificationResult())
    ner = _Stub(ClassificationResult())
    llm = _Stub(ClassificationResult())

    p = Pipeline(regex=regex, ner=ner, llm=llm, escalate_to_llm=True)
    out = await p.run("Just an ordinary paragraph.")

    assert out.sensitivity == int(Sensitivity.NONE)
    assert out.matches == ()
