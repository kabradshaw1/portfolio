from app.config import Settings


def test_settings_include_rate_limit_and_internal_token_defaults():
    settings = Settings()

    assert settings.eval_rate_limit_run_create_operator == "20/minute"
    assert settings.eval_rate_limit_run_create_user == "5/minute"
    assert settings.eval_rate_limit_run_create_anonymous == "0/minute"
    assert settings.eval_rate_limit_read_operator == "240/minute"
    assert settings.eval_rate_limit_read_user == "30/minute"
    assert settings.eval_rate_limit_read_anonymous == "10/minute"
    assert settings.eval_rate_limit_write_operator == "30/minute"
    assert settings.eval_rate_limit_write_user == "10/minute"
    assert settings.eval_rate_limit_write_anonymous == "0/minute"
    assert settings.rag_internal_eval_token == ""
