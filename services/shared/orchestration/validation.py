"""call_validated — call an LLM via a Role and validate its output against a
Pydantic schema, retrying with a corrective nudge on invalid output."""

from __future__ import annotations

import json
from typing import TypeVar

import structlog
from pydantic import BaseModel, ValidationError

from shared.orchestration.errors import OutputValidationError
from shared.orchestration.role import Role

logger = structlog.get_logger()

ModelT = TypeVar("ModelT", bound=BaseModel)


def _extract_json(text: str) -> dict:
    """Extract the first top-level JSON object from a model response."""
    start = text.find("{")
    end = text.rfind("}")
    if start == -1 or end == -1 or end < start:
        raise ValueError("no JSON object found in response")
    return json.loads(text[start : end + 1])


async def call_validated(
    role: Role,
    messages: list[dict],
    schema: type[ModelT],
    *,
    tools: list[dict] | None = None,
    max_retries: int = 2,
    repair: bool = True,
) -> ModelT:
    """Call ``role``'s LLM, parse the response into ``schema``, and retry on
    invalid output.

    When ``repair`` is set, a corrective message describing the failure is
    appended before the next attempt. Raises ``OutputValidationError`` once
    ``max_retries`` is exhausted.
    """
    provider = role.build_provider()
    convo = list(messages)
    last_error: Exception | None = None

    for attempt in range(max_retries + 1):
        chat_messages = [{"role": "system", "content": role.system_prompt}, *convo]
        response = await provider.chat(messages=chat_messages, tools=tools)
        content = response.get("message", {}).get("content", "") or ""
        try:
            return schema.model_validate(_extract_json(content))
        except (ValueError, json.JSONDecodeError, ValidationError) as exc:
            last_error = exc
            logger.warning(
                "output_validation_retry",
                role=role.name,
                attempt=attempt,
                error=str(exc),
            )
            if repair and attempt < max_retries:
                convo = [
                    *convo,
                    {"role": "assistant", "content": content},
                    {
                        "role": "user",
                        "content": (
                            f"Your previous response was invalid: {exc}. "
                            "Respond with ONLY a valid JSON object matching the "
                            "required schema, no prose."
                        ),
                    },
                ]

    raise OutputValidationError(
        f"output failed validation after {max_retries + 1} attempts: {last_error}",
        stage=role.name,
    )
