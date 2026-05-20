from __future__ import annotations

import os
from dataclasses import dataclass


@dataclass(frozen=True)
class AnswerModelOverride:
    tier: str | None
    provider: str
    base_url: str | None
    model: str
    api_key_secret: str | None


@dataclass(frozen=True)
class ResolvedAnswerModelOverride:
    tier: str | None
    provider: str
    base_url: str | None
    model: str
    api_key_secret: str | None
    api_key: str

    def safe_dict(self) -> dict:
        return {
            "tier": self.tier,
            "provider": self.provider,
            "base_url": self.base_url,
            "model": self.model,
            "api_key_secret": self.api_key_secret,
        }


def resolve_answer_model_override(
    override: AnswerModelOverride | None,
) -> ResolvedAnswerModelOverride | None:
    if override is None:
        return None

    api_key = ""
    if override.api_key_secret:
        api_key = os.getenv(override.api_key_secret, "")
        if not api_key:
            raise ValueError(
                f"answer model secret {override.api_key_secret} is not configured"
            )
    elif override.provider in {"openai", "anthropic"}:
        raise ValueError(
            "answer_api_key_secret is required when answer_provider is "
            f"{override.provider}"
        )

    return ResolvedAnswerModelOverride(
        tier=override.tier,
        provider=override.provider,
        base_url=override.base_url,
        model=override.model,
        api_key_secret=override.api_key_secret,
        api_key=api_key,
    )
