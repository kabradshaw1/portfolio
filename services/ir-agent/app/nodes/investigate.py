"""Investigate node: a bounded ReAct tool loop that gathers evidence, then a
structured call that produces grounded findings."""

from __future__ import annotations

from collections.abc import Callable

from langchain_core.messages import HumanMessage, SystemMessage, ToolMessage

from app.models import EvidenceItem, Findings
from app.prompts import INVESTIGATE_SYSTEM
from app.state import IRState
from app.tools import build_tools


def make_investigate_node(model, *, max_tool_steps: int) -> Callable[[IRState], dict]:
    def investigate_node(state: IRState) -> dict:
        incident = state["incident"]
        tools = {t.name: t for t in build_tools(incident.id)}
        bound = model.bind_tools(list(tools.values()))

        evidence: list[EvidenceItem] = list(state.get("evidence") or [])
        counter = len(evidence)

        prompt = f"Incident:\n{incident.model_dump_json()}"
        triage = state.get("triage")
        if triage is not None:
            prompt += f"\n\nTriage:\n{triage.model_dump_json()}"
        verdict = state.get("verdict")
        if verdict is not None and verdict.gaps:
            prompt += "\n\nPrior review found gaps to address:\n" + "\n".join(
                verdict.gaps
            )

        messages = [
            SystemMessage(content=INVESTIGATE_SYSTEM),
            HumanMessage(content=prompt),
        ]

        for _ in range(max_tool_steps):
            ai = bound.invoke(messages)
            messages.append(ai)
            tool_calls = getattr(ai, "tool_calls", None) or []
            if not tool_calls:
                break
            for call in tool_calls:
                name = call["name"]
                args = call.get("args", {})
                tool = tools.get(name)
                result = tool.invoke(args) if tool else f"Unknown tool: {name}"
                eid = f"{name}-{counter}"
                counter += 1
                evidence.append(
                    EvidenceItem(
                        id=eid,
                        source_tool=name,
                        query=", ".join(str(v) for v in args.values()),
                        content=str(result),
                    )
                )
                messages.append(
                    ToolMessage(
                        content=f"[{eid}] {result}",
                        tool_call_id=call["id"],
                    )
                )

        evidence_catalog = "\n".join(f"{e.id}: {e.content}" for e in evidence)
        findings: Findings = model.with_structured_output(Findings).invoke(
            messages
            + [
                HumanMessage(
                    content="Produce findings. Cite evidence by id. "
                    f"Available evidence ids:\n{evidence_catalog}"
                )
            ]
        )

        return {
            "findings": findings,
            "evidence": evidence,
            "investigate_attempts": state.get("investigate_attempts", 0) + 1,
        }

    return investigate_node
