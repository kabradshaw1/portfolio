from app.config import Settings


def test_default_models_are_tiered():
    s = Settings(anthropic_api_key="sk-test")
    assert s.triage_model == "claude-haiku-4-5"
    assert s.investigate_model == "claude-opus-4-8"
    assert s.validate_model == "claude-opus-4-8"
    assert s.report_model == "claude-sonnet-4-6"


def test_validate_requires_api_key():
    s = Settings(anthropic_api_key="")
    try:
        s.validate()
        raised = False
    except ValueError:
        raised = True
    assert raised


def test_loop_bounds_have_defaults():
    s = Settings(anthropic_api_key="sk-test")
    assert s.max_investigate_attempts == 2
    assert s.max_tool_steps == 6
