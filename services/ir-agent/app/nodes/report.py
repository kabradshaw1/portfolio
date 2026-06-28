"""Report node: produce the final structured IR report."""

from __future__ import annotations

from collections.abc import Callable

from langchain_core.messages import HumanMessage, SystemMessage

from app.models import IRReport
from app.prompts import REPORT_SYSTEM
from app.state import IRState


def make_report_node(model) -> Callable[[IRState], dict]:
    structured = model.with_structured_output(IRReport)

    def report_node(state: IRState) -> dict:
        parts = []
        triage = state.get("triage")
        if triage is not None:
            parts.append(f"Triage:\n{triage.model_dump_json()}")
        findings = state.get("findings")
        if findings is not None:
            parts.append(f"Findings:\n{findings.model_dump_json()}")
        verdict = state.get("verdict")
        if verdict is not None:
            parts.append(f"Validation:\n{verdict.model_dump_json()}")
        report: IRReport = structured.invoke(
            [
                SystemMessage(content=REPORT_SYSTEM),
                HumanMessage(content="\n\n".join(parts)),
            ]
        )
        return {"report": report}

    return report_node
