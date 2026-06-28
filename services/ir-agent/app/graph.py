"""Assemble the IR investigation graph.

triage -> investigate -> validate -> (investigate | report) -> END
The validate->investigate edge is bounded by max_attempts (enforced in
route_after_validate against investigate_attempts).
"""

from __future__ import annotations

from functools import partial

from langgraph.graph import END, START, StateGraph

from app.nodes.investigate import make_investigate_node
from app.nodes.report import make_report_node
from app.nodes.triage import make_triage_node
from app.nodes.validate import make_validate_node, route_after_validate
from app.state import IRState


def build_graph(models: dict, *, max_tool_steps: int, max_attempts: int):
    graph = StateGraph(IRState)
    graph.add_node("triage", make_triage_node(models["triage"]))
    graph.add_node(
        "investigate",
        make_investigate_node(models["investigate"], max_tool_steps=max_tool_steps),
    )
    graph.add_node("validate", make_validate_node(models["validate"]))
    graph.add_node("report", make_report_node(models["report"]))

    graph.add_edge(START, "triage")
    graph.add_edge("triage", "investigate")
    graph.add_edge("investigate", "validate")
    graph.add_conditional_edges(
        "validate",
        partial(route_after_validate, max_attempts=max_attempts),
        {"investigate": "investigate", "report": "report"},
    )
    graph.add_edge("report", END)
    return graph.compile()
