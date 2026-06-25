import pytest
from pydantic import BaseModel

from shared.orchestration.errors import OutputValidationError
from shared.orchestration.role import Role
from shared.orchestration.validation import call_validated


class Scores(BaseModel):
    faithfulness: float
    relevancy: float


class _FakeProvider:
    """Returns the queued responses in order on successive chat() calls."""

    def __init__(self, responses):
        self._responses = list(responses)
        self.calls = []

    async def chat(self, messages, tools=None):
        self.calls.append(messages)
        content = self._responses.pop(0)
        return {"message": {"content": content}}


def _role_with(provider):
    role = Role(
        name="judge",
        system_prompt="You judge. Return JSON.",
        provider="ollama",
        base_url="x",
        api_key="",
        model="m",
    )
    # Bypass the real factory; bind the fake provider directly.
    object.__setattr__(role, "_test_provider", provider)
    return role


@pytest.fixture
def patch_build(monkeypatch):
    def _patch(provider):
        monkeypatch.setattr(
            "shared.orchestration.validation.Role.build_provider",
            lambda self: provider,
        )

    return _patch


@pytest.mark.asyncio
async def test_returns_validated_model_on_first_success(patch_build):
    provider = _FakeProvider(['{"faithfulness": 0.9, "relevancy": 0.8}'])
    patch_build(provider)
    result = await call_validated(
        _role_with(provider), [{"role": "user", "content": "q"}], Scores
    )
    assert isinstance(result, Scores)
    assert result.faithfulness == 0.9
    assert len(provider.calls) == 1


@pytest.mark.asyncio
async def test_extracts_json_embedded_in_prose(patch_build):
    provider = _FakeProvider(['Sure! {"faithfulness": 1.0, "relevancy": 1.0} done'])
    patch_build(provider)
    result = await call_validated(
        _role_with(provider), [{"role": "user", "content": "q"}], Scores
    )
    assert result.relevancy == 1.0


@pytest.mark.asyncio
async def test_retries_with_repair_then_succeeds(patch_build):
    provider = _FakeProvider(
        ["not json at all", '{"faithfulness": 0.5, "relevancy": 0.5}']
    )
    patch_build(provider)
    result = await call_validated(
        _role_with(provider),
        [{"role": "user", "content": "q"}],
        Scores,
        max_retries=2,
    )
    assert result.faithfulness == 0.5
    # Second call must include the repair nudge appended after the bad answer.
    assert len(provider.calls) == 2
    repair_msgs = provider.calls[1]
    assert any("invalid" in m["content"].lower() for m in repair_msgs)


@pytest.mark.asyncio
async def test_raises_output_validation_error_after_exhausting_retries(patch_build):
    provider = _FakeProvider(["bad", "still bad", "nope"])
    patch_build(provider)
    with pytest.raises(OutputValidationError) as exc:
        await call_validated(
            _role_with(provider),
            [{"role": "user", "content": "q"}],
            Scores,
            max_retries=2,
        )
    assert exc.value.stage == "judge"
    assert exc.value.retryable is False
