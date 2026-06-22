"""Tiered classification pipeline: regex → NER → LLM."""

from __future__ import annotations

import logging
from collections.abc import Iterable

from app.classifiers.base import (
    Classifier,
    Match,
    PipelineOutput,
)
from app.classifiers.llm_pass import LLMUnavailable
from app.models.events import Sensitivity

_LOGGER = logging.getLogger(__name__)


_SENSITIVITY_FLOOR: dict[str, Sensitivity] = {
    "PII.SSN": Sensitivity.HIGH,
    "FINANCIAL.CREDIT_CARD": Sensitivity.HIGH,
    "SECRETS.JWT": Sensitivity.HIGH,
    "SECRETS.AWS_KEY": Sensitivity.HIGH,
    "SECRETS.GENERIC": Sensitivity.MEDIUM,
    "HEALTH.MEDICAL_LICENSE": Sensitivity.HIGH,
    "FINANCIAL.IBAN": Sensitivity.HIGH,
    "PII.EMAIL": Sensitivity.MEDIUM,
    "PII.PHONE": Sensitivity.MEDIUM,
    "PII.PERSON": Sensitivity.LOW,
    "PII.LOCATION": Sensitivity.LOW,
    "PII.IP": Sensitivity.LOW,
}


def _dedupe(matches: Iterable[Match]) -> tuple[Match, ...]:
    seen: dict[tuple[str, tuple[int, int]], Match] = {}
    for m in matches:
        key = (m.category, m.span)
        existing = seen.get(key)
        if existing is None or m.confidence > existing.confidence:
            seen[key] = m
    return tuple(seen.values())


def _top_category(category: str) -> str:
    return category.split(".", 1)[0]


def _sensitivity_from(matches: Iterable[Match]) -> Sensitivity:
    floor = Sensitivity.NONE
    for m in matches:
        bumped = _SENSITIVITY_FLOOR.get(m.category, Sensitivity.LOW)
        if bumped > floor:
            floor = bumped
    return floor


class Pipeline:
    def __init__(
        self,
        *,
        regex: Classifier,
        ner: Classifier,
        llm: Classifier,
        escalate_to_llm: bool = True,
    ) -> None:
        self._regex = regex
        self._ner = ner
        self._llm = llm
        self._escalate_to_llm = escalate_to_llm

    async def run(self, content: str) -> PipelineOutput:
        stages_run: list[str] = []
        all_matches: list[Match] = []
        llm_failed = False
        reason: str | None = None

        regex_result = await self._regex.classify(content)
        stages_run.append("regex")
        all_matches.extend(regex_result.matches)

        if _has_high_conf_match(regex_result.matches):
            return _output(
                all_matches, stages_run, llm_failed=False, reason=None, llm_confirmed=False
            )

        ner_result = await self._ner.classify(content)
        stages_run.append("ner")
        all_matches.extend(ner_result.matches)

        llm_confirmed = False
        if self._escalate_to_llm and ner_result.needs_escalation:
            try:
                llm_result = await self._llm.classify(content)
                stages_run.append("llm")
                all_matches.extend(llm_result.matches)
                llm_confirmed = llm_result.has_matches
            except LLMUnavailable as e:
                _LOGGER.warning("llm unavailable; persisting partial finding: %s", e)
                llm_failed = True
                reason = "llm_unavailable"

        return _output(
            all_matches,
            stages_run,
            llm_failed=llm_failed,
            reason=reason,
            llm_confirmed=llm_confirmed,
        )


def _has_high_conf_match(matches: Iterable[Match]) -> bool:
    return any(m.confidence >= 0.9 and m.category != "OTHER" for m in matches)


def _output(
    matches: list[Match],
    stages_run: list[str],
    *,
    llm_failed: bool,
    reason: str | None,
    llm_confirmed: bool = False,
) -> PipelineOutput:
    deduped = _dedupe(matches)
    sensitivity = _sensitivity_from(deduped)
    # When the LLM stage confirmed findings, ensure at least MEDIUM sensitivity
    # because escalation only happens when NER was uncertain; LLM confirmation
    # raises the confidence bar and warrants at minimum a MEDIUM classification.
    if llm_confirmed and sensitivity < Sensitivity.MEDIUM:
        sensitivity = Sensitivity.MEDIUM
    categories = tuple(sorted({_top_category(m.category) for m in deduped}))
    return PipelineOutput(
        matches=deduped,
        categories=categories,
        sensitivity=int(sensitivity),
        llm_failed=llm_failed,
        reason=reason,
        stages_run=tuple(stages_run),
    )
