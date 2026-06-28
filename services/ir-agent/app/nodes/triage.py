"""Triage node: classify the incident with structured output."""

from __future__ import annotations

from collections.abc import Callable

from langchain_core.messages import HumanMessage, SystemMessage

from app.models import TriageResult
from app.prompts import TRIAGE_SYSTEM
from app.state import IRState


def make_triage_node(model) -> Callable[[IRState], dict]:
    structured = model.with_structured_output(TriageResult)

    def triage_node(state: IRState) -> dict:
        incident = state["incident"]
        messages = [
            SystemMessage(content=TRIAGE_SYSTEM),
            HumanMessage(content=incident.model_dump_json()),
        ]
        result: TriageResult = structured.invoke(messages)
        return {"triage": result}

    return triage_node
