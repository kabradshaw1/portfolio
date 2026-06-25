import pytest

from shared.orchestration.role import Role


def test_role_is_frozen():
    role = Role(
        name="answerer",
        system_prompt="You answer.",
        provider="ollama",
        base_url="http://localhost:11434",
        api_key="",
        model="qwen2.5:14b",
    )
    with pytest.raises(Exception):
        role.model = "other"  # frozen dataclass forbids mutation


def test_role_defaults_params_to_empty_dict():
    role = Role(
        name="judge",
        system_prompt="You judge.",
        provider="ollama",
        base_url="http://localhost:11434",
        api_key="",
        model="m",
    )
    assert role.params == {}


def test_build_provider_uses_factory(monkeypatch):
    captured = {}

    def fake_factory(provider, base_url, api_key, model):
        captured.update(
            provider=provider, base_url=base_url, api_key=api_key, model=model
        )
        return "PROVIDER"

    monkeypatch.setattr("shared.orchestration.role.get_llm_provider", fake_factory)
    role = Role(
        name="answerer",
        system_prompt="s",
        provider="anthropic",
        base_url="https://api",
        api_key="sk-x",
        model="claude-opus-4-8",
    )
    assert role.build_provider() == "PROVIDER"
    assert captured == {
        "provider": "anthropic",
        "base_url": "https://api",
        "api_key": "sk-x",
        "model": "claude-opus-4-8",
    }
