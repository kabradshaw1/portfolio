"""Validate node: adversarial grounding check + loop routing."""

from __future__ import annotations

from collections.abc import Callable

from langchain_core.messages import HumanMessage, SystemMessage

from app.models import ValidationVerdict
from app.prompts import VALIDATE_SYSTEM
from app.state import IRState


def make_validate_node(model) -> Callable[[IRState], dict]:
    structured = model.with_structured_output(ValidationVerdict)

    def validate_node(state: IRState) -> dict:
        findings = state["findings"]
        evidence = state.get("evidence") or []
        catalog = "\n".join(f"{e.id}: {e.content}" for e in evidence)
        prompt = (
            f"Findings:\n{findings.model_dump_json()}\n\n"
            f"Evidence available to the investigator:\n{catalog or '(none)'}"
        )
        verdict: ValidationVerdict = structured.invoke(
            [
                SystemMessage(content=VALIDATE_SYSTEM),
                HumanMessage(content=prompt),
            ]
        )
        return {"verdict": verdict}

    return validate_node


def route_after_validate(state: IRState, *, max_attempts: int) -> str:
    """Decide whether to re-investigate or proceed to the report."""
    verdict = state["verdict"]
    attempts = state.get("investigate_attempts", 0)
    needs_more = (not verdict.grounded) or verdict.needs_more_investigation
    if needs_more and attempts < max_attempts:
        return "investigate"
    return "report"
