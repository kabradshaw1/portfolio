"""Per-run token, cost, and latency accounting.

The nodes wrap every model call in ``with_structured_output``/``bind_tools``,
whose ``.invoke()`` returns the parsed object and discards token usage. A
callback handler taps the *raw* model call instead, so usage is captured without
threading telemetry through node return values. The handler is attached to the
graph run via config callbacks; LangGraph propagates it to every node's model
invocation and tags each with ``metadata["langgraph_node"]`` — which equals the
role name (triage/investigate/validate/report) — giving per-role attribution.
"""

from __future__ import annotations

import time
from collections import defaultdict

from langchain_core.callbacks import BaseCallbackHandler

from app import metrics
from app.pricing import cost_usd

# Node names in the graph are identical to role names.
_ROLES = ("triage", "investigate", "validate", "report")
_OPUS = "claude-opus-4-8"


def _extract_usage(response) -> tuple[int, int]:
    """Pull (input_tokens, output_tokens) from an LLMResult."""
    for gens in getattr(response, "generations", None) or []:
        for gen in gens:
            usage = getattr(getattr(gen, "message", None), "usage_metadata", None)
            if usage:
                return int(usage.get("input_tokens", 0)), int(
                    usage.get("output_tokens", 0)
                )
    llm_output = getattr(response, "llm_output", None) or {}
    usage = llm_output.get("usage") or {}
    return (
        int(usage.get("input_tokens", 0) or usage.get("prompt_tokens", 0) or 0),
        int(usage.get("output_tokens", 0) or usage.get("completion_tokens", 0) or 0),
    )


class RunAccountant(BaseCallbackHandler):
    """Aggregates per-role token usage, cost, and latency for one investigation."""

    def __init__(self, role_models: dict[str, str]):
        self._role_models = dict(role_models)
        self._input: dict[str, int] = defaultdict(int)
        self._output: dict[str, int] = defaultdict(int)
        self._calls: dict[str, int] = defaultdict(int)
        self._seconds: dict[str, float] = defaultdict(float)
        self._pending: dict = {}

    # --- callback hooks -------------------------------------------------
    def on_chat_model_start(
        self, serialized, messages, *, run_id, metadata=None, **kwargs
    ):
        self._pending[run_id] = (
            (metadata or {}).get("langgraph_node"),
            time.perf_counter(),
        )

    def on_llm_start(self, serialized, prompts, *, run_id, metadata=None, **kwargs):
        self._pending.setdefault(
            run_id, ((metadata or {}).get("langgraph_node"), time.perf_counter())
        )

    def on_llm_end(self, response, *, run_id, **kwargs):
        node, started = self._pending.pop(run_id, (None, None))
        elapsed = time.perf_counter() - started if started is not None else 0.0
        input_tokens, output_tokens = _extract_usage(response)
        if node in self._role_models:
            self.record(
                node,
                input_tokens=input_tokens,
                output_tokens=output_tokens,
                seconds=elapsed,
            )

    # --- core accounting ------------------------------------------------
    def record(self, role, *, input_tokens, output_tokens, seconds=0.0):
        self._input[role] += input_tokens
        self._output[role] += output_tokens
        self._calls[role] += 1
        self._seconds[role] += seconds
        metrics.LLM_TOKENS.labels(role=role, kind="input").inc(input_tokens)
        metrics.LLM_TOKENS.labels(role=role, kind="output").inc(output_tokens)
        if seconds:
            metrics.NODE_DURATION.labels(node=role).observe(seconds)

    def summary(self) -> dict:
        per_role: dict[str, dict] = {}
        total_in = total_out = 0
        total_seconds = tiered_cost = opus_cost = 0.0

        for role in _ROLES:
            if self._calls[role] == 0:
                continue
            model = self._role_models.get(role, _OPUS)
            i, o = self._input[role], self._output[role]
            cost = cost_usd(model, i, o)
            per_role[role] = {
                "model": model,
                "input_tokens": i,
                "output_tokens": o,
                "calls": self._calls[role],
                "cost_usd": round(cost, 4),
                "seconds": round(self._seconds[role], 2),
            }
            total_in += i
            total_out += o
            total_seconds += self._seconds[role]
            tiered_cost += cost
            opus_cost += cost_usd(_OPUS, i, o)

        return {
            "per_role": per_role,
            "totals": {
                "input_tokens": total_in,
                "output_tokens": total_out,
                "total_tokens": total_in + total_out,
                "cost_usd": round(tiered_cost, 4),
                "seconds": round(total_seconds, 2),
            },
            "comparison": {
                "tiered_cost_usd": round(tiered_cost, 4),
                "opus_everywhere_cost_usd": round(opus_cost, 4),
                "savings_factor": round(opus_cost / tiered_cost, 2)
                if tiered_cost
                else 0.0,
            },
        }
