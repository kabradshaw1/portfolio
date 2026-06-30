"""Pricing table + per-run token/cost/latency accountant."""

from __future__ import annotations

import uuid

from app.pricing import cost_usd, price_for
from app.usage import RunAccountant


def test_cost_usd_haiku_one_million_each():
    # Haiku 4.5: $1.00 in / $5.00 out per 1M tokens.
    assert cost_usd("claude-haiku-4-5", 1_000_000, 1_000_000) == 6.0


def test_cost_usd_opus_input_only():
    # Opus 4.8: $5.00 in / $25.00 out per 1M tokens.
    assert cost_usd("claude-opus-4-8", 1_000_000, 0) == 5.0


def test_cost_usd_sonnet():
    # Sonnet 4.6: $3.00 in / $15.00 out per 1M tokens.
    assert cost_usd("claude-sonnet-4-6", 1_000_000, 1_000_000) == 18.0


def test_price_for_unknown_model_falls_back_to_opus_tier():
    assert price_for("some-future-model") == price_for("claude-opus-4-8")


_ROLE_MODELS = {
    "triage": "claude-haiku-4-5",
    "investigate": "claude-opus-4-8",
    "validate": "claude-opus-4-8",
    "report": "claude-sonnet-4-6",
}


def test_accountant_record_accumulates_per_role():
    acc = RunAccountant(_ROLE_MODELS)
    acc.record("triage", input_tokens=1000, output_tokens=300, seconds=0.5)
    acc.record("triage", input_tokens=500, output_tokens=100, seconds=0.5)

    summary = acc.summary()
    triage = summary["per_role"]["triage"]
    assert triage["input_tokens"] == 1500
    assert triage["output_tokens"] == 400
    assert triage["calls"] == 2
    assert triage["model"] == "claude-haiku-4-5"
    # (1500/1e6)*1 + (400/1e6)*5 = 0.0015 + 0.002 = 0.0035
    assert triage["cost_usd"] == 0.0035


def test_accountant_summary_compares_tiered_vs_opus_everywhere():
    acc = RunAccountant(_ROLE_MODELS)
    # Only triage runs (Haiku); opus-everywhere would price it at Opus rates.
    acc.record("triage", input_tokens=1_000_000, output_tokens=300_000)

    summary = acc.summary()
    comp = summary["comparison"]
    # tiered (Haiku): 1.0 + 1.5 = 2.5 ; opus: 5.0 + 7.5 = 12.5 ; factor 5.0
    assert comp["tiered_cost_usd"] == 2.5
    assert comp["opus_everywhere_cost_usd"] == 12.5
    assert comp["savings_factor"] == 5.0


def test_accountant_summary_skips_roles_with_no_calls():
    acc = RunAccountant(_ROLE_MODELS)
    acc.record("triage", input_tokens=100, output_tokens=10)
    summary = acc.summary()
    assert set(summary["per_role"].keys()) == {"triage"}
    assert summary["totals"]["total_tokens"] == 110


def test_accountant_callback_attributes_tokens_by_langgraph_node():
    from langchain_core.messages import AIMessage
    from langchain_core.outputs import ChatGeneration, LLMResult

    acc = RunAccountant(_ROLE_MODELS)
    run_id = uuid.uuid4()
    acc.on_chat_model_start(
        {}, [], run_id=run_id, metadata={"langgraph_node": "investigate"}
    )
    msg = AIMessage(
        content="x",
        usage_metadata={
            "input_tokens": 8000,
            "output_tokens": 2000,
            "total_tokens": 10000,
        },
    )
    result = LLMResult(generations=[[ChatGeneration(message=msg)]])
    acc.on_llm_end(result, run_id=run_id)

    inv = acc.summary()["per_role"]["investigate"]
    assert inv["input_tokens"] == 8000
    assert inv["output_tokens"] == 2000
    assert inv["model"] == "claude-opus-4-8"
