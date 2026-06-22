from __future__ import annotations

import json

import httpx
import pytest

from app.classifiers.llm_pass import LLMClassifier, LLMUnavailable


def _ok_response(payload: dict) -> httpx.Response:
    body = {"response": json.dumps(payload)}
    return httpx.Response(200, json=body)


@pytest.mark.asyncio
async def test_happy_path_returns_matches():
    payload = {
        "category": "PII",
        "sensitivity": "high",
        "reasoning": "Names + address",
        "entities": [{"type": "PII.PERSON", "text": "Jane Doe", "confidence": 0.9}],
    }

    def handler(req: httpx.Request) -> httpx.Response:
        return _ok_response(payload)

    transport = httpx.MockTransport(handler)
    c = LLMClassifier(base_url="http://x", model="m", transport=transport)

    r = await c.classify("Jane Doe lives at 1 Main St.")
    assert any(m.category == "PII.PERSON" for m in r.matches)
    assert r.confidence == pytest.approx(0.9)


@pytest.mark.asyncio
async def test_malformed_json_retries_then_gives_up():
    calls = {"n": 0}

    def handler(req: httpx.Request) -> httpx.Response:
        calls["n"] += 1
        return httpx.Response(200, json={"response": "not json at all"})

    transport = httpx.MockTransport(handler)
    c = LLMClassifier(base_url="http://x", model="m", transport=transport, max_parse_retries=2)

    with pytest.raises(LLMUnavailable):
        await c.classify("anything")
    assert calls["n"] == 3  # initial + 2 retries


@pytest.mark.asyncio
async def test_5xx_raises_unavailable():
    def handler(req: httpx.Request) -> httpx.Response:
        return httpx.Response(503, text="Service Unavailable")

    transport = httpx.MockTransport(handler)
    c = LLMClassifier(base_url="http://x", model="m", transport=transport)

    with pytest.raises(LLMUnavailable):
        await c.classify("anything")
