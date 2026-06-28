"""Map a role to its configured chat model.

Keeping model selection in one place is the per-role tiering decision: cheap
models for classification, strong models for reasoning and validation.
"""

from __future__ import annotations

from typing import Literal

from langchain_anthropic import ChatAnthropic

from app.config import settings

Role = Literal["triage", "investigate", "validate", "report"]

_MODEL_BY_ROLE: dict[Role, str] = {
    "triage": settings.triage_model,
    "investigate": settings.investigate_model,
    "validate": settings.validate_model,
    "report": settings.report_model,
}


def model_for(role: Role, *, builder=ChatAnthropic):
    """Build the chat model for a role. `builder` is injectable for tests."""
    return builder(
        model=_MODEL_BY_ROLE[role],
        api_key=settings.anthropic_api_key,
        timeout=settings.request_timeout_seconds,
        max_retries=2,
    )
