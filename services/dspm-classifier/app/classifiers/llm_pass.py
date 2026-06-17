"""LLM classifier targeting an Ollama-compatible JSON endpoint.

Asks the model for a structured JSON object with category, sensitivity, and a
list of entities. Re-prompts up to `max_parse_retries` times on malformed JSON,
then raises `LLMUnavailable` so the pipeline can persist a partial finding.
"""

from __future__ import annotations

import json
from typing import Any

import httpx

from app.classifiers.base import (
    ClassificationResult,
    Match,
)

_PROMPT_TEMPLATE = (
    "You are a sensitive-data classifier. Read the CONTENT and return a SINGLE "
    "JSON object with this exact shape and no prose:\n"
    '{{"category": "PII|FINANCIAL|HEALTH|SECRETS|NONE", '
    '"sensitivity": "none|low|medium|high", '
    '"reasoning": "...", '
    '"entities": [{{"type": "PII.PERSON|PII.EMAIL|...", "text": "...", "confidence": 0-1}}]}}\n'
    "CONTENT:\n{content}\n"
)


_SENSITIVITY_TO_CONF = {"none": 0.0, "low": 0.4, "medium": 0.7, "high": 0.95}


class LLMUnavailable(Exception):
    """Raised when the LLM returns 5xx, times out, or yields unparseable output."""


class LLMClassifier:
    def __init__(
        self,
        *,
        base_url: str,
        model: str,
        timeout_seconds: float = 15.0,
        max_parse_retries: int = 2,
        transport: httpx.AsyncBaseTransport | None = None,
    ) -> None:
        self._url = f"{base_url.rstrip('/')}/api/generate"
        self._model = model
        self._client = httpx.AsyncClient(timeout=timeout_seconds, transport=transport)
        self._max_parse_retries = max_parse_retries

    async def classify(self, content: str) -> ClassificationResult:
        last_error: Exception | None = None
        for _ in range(self._max_parse_retries + 1):
            try:
                resp = await self._client.post(
                    self._url,
                    json={
                        "model": self._model,
                        "prompt": _PROMPT_TEMPLATE.format(content=content[:8000]),
                        "stream": False,
                        "format": "json",
                    },
                )
            except httpx.HTTPError as e:
                raise LLMUnavailable(str(e)) from e

            if resp.status_code >= 500 or resp.status_code == 429:
                raise LLMUnavailable(f"upstream {resp.status_code}")
            resp.raise_for_status()

            try:
                payload = self._parse(resp.json())
            except (ValueError, KeyError, TypeError) as e:
                last_error = e
                continue
            return _to_result(payload)

        raise LLMUnavailable(f"unparseable output after retries: {last_error}")

    @staticmethod
    def _parse(envelope: dict[str, Any]) -> dict[str, Any]:
        raw = envelope.get("response", "")
        parsed = json.loads(raw)
        if not isinstance(parsed, dict):
            raise ValueError("expected object")
        return parsed

    async def aclose(self) -> None:
        await self._client.aclose()


def _to_result(payload: dict[str, Any]) -> ClassificationResult:
    sensitivity = str(payload.get("sensitivity", "none")).lower()
    entities = payload.get("entities") or []
    matches: list[Match] = []
    for e in entities:
        matches.append(
            Match(
                category=str(e.get("type", "OTHER")),
                span=(0, 0),  # LLM doesn't give us reliable offsets
                text=str(e.get("text", "")),
                confidence=float(e.get("confidence", _SENSITIVITY_TO_CONF.get(sensitivity, 0.5))),
            )
        )
    top_conf = max(
        (m.confidence for m in matches),
        default=_SENSITIVITY_TO_CONF.get(sensitivity, 0.0),
    )
    return ClassificationResult(
        matches=tuple(matches),
        confidence=top_conf,
        needs_escalation=False,
    )
