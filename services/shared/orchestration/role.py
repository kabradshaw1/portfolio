"""Role — system prompt + model binding for a single agent persona."""

from __future__ import annotations

from dataclasses import dataclass, field
from typing import Any

from shared.llm import LLMProvider, get_llm_provider


@dataclass(frozen=True)
class Role:
    """Bundles *who is acting*: a system prompt bound to a specific model.

    Two roles on different models (e.g. answerer vs. judge) make role
    separation explicit and configurable.
    """

    name: str
    system_prompt: str
    provider: str
    base_url: str
    api_key: str
    model: str
    params: dict[str, Any] = field(default_factory=dict)

    def build_provider(self) -> LLMProvider:
        """Construct the bound LLM provider via the shared factory."""
        return get_llm_provider(self.provider, self.base_url, self.api_key, self.model)
