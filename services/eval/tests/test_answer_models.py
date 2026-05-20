import pytest
from app.answer_models import AnswerModelOverride, resolve_answer_model_override


def test_resolve_answer_model_override_uses_secret_reference(monkeypatch):
    monkeypatch.setenv("OPENAI_API_KEY", "test-key")

    override = resolve_answer_model_override(
        AnswerModelOverride(
            tier="efficient",
            provider="openai",
            base_url="https://api.openai.com/v1",
            model="gpt-5.4-mini",
            api_key_secret="OPENAI_API_KEY",
        )
    )

    assert override.tier == "efficient"
    assert override.provider == "openai"
    assert override.base_url == "https://api.openai.com/v1"
    assert override.model == "gpt-5.4-mini"
    assert override.api_key == "test-key"
    assert override.safe_dict() == {
        "tier": "efficient",
        "provider": "openai",
        "base_url": "https://api.openai.com/v1",
        "model": "gpt-5.4-mini",
        "api_key_secret": "OPENAI_API_KEY",
    }


def test_resolve_answer_model_override_allows_ollama_without_secret():
    override = resolve_answer_model_override(
        AnswerModelOverride(
            tier="local",
            provider="ollama",
            base_url="http://ollama:11434",
            model="qwen2.5:14b",
            api_key_secret=None,
        )
    )

    assert override.api_key == ""


def test_resolve_answer_model_override_rejects_missing_secret(monkeypatch):
    monkeypatch.delenv("OPENAI_API_KEY", raising=False)

    with pytest.raises(ValueError, match="OPENAI_API_KEY"):
        resolve_answer_model_override(
            AnswerModelOverride(
                tier="efficient",
                provider="openai",
                base_url="https://api.openai.com/v1",
                model="gpt-5.4-mini",
                api_key_secret="OPENAI_API_KEY",
            )
        )
