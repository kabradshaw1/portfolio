"""Per-model token pricing.

USD per 1,000,000 tokens, from Anthropic's published rates. Opus 4.8 prices
apply across its full 1M context window (no long-context premium), so a single
flat rate per model is accurate for this service.
"""

from __future__ import annotations

from dataclasses import dataclass


@dataclass(frozen=True)
class ModelPrice:
    input_per_mtok: float
    output_per_mtok: float


MODEL_PRICING: dict[str, ModelPrice] = {
    "claude-haiku-4-5": ModelPrice(1.00, 5.00),
    "claude-sonnet-4-6": ModelPrice(3.00, 15.00),
    "claude-opus-4-8": ModelPrice(5.00, 25.00),
}

# Unknown models are assumed Opus-tier so cost is never silently understated.
_FALLBACK = MODEL_PRICING["claude-opus-4-8"]


def price_for(model: str) -> ModelPrice:
    return MODEL_PRICING.get(model, _FALLBACK)


def cost_usd(model: str, input_tokens: int, output_tokens: int) -> float:
    p = price_for(model)
    return (
        input_tokens / 1_000_000 * p.input_per_mtok
        + output_tokens / 1_000_000 * p.output_per_mtok
    )
