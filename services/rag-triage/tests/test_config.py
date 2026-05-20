from app.config import Settings


def test_default_settings():
    settings = Settings()

    assert settings.eval_api_url == "http://eval:8000"
    assert settings.request_timeout_seconds == 30.0
    assert settings.default_metric == "context_precision"
    assert settings.default_limit == 5
    assert settings.max_limit == 20


def test_metric_validation_rejects_unknown_default():
    try:
        Settings(default_metric="latency")
    except ValueError as exc:
        assert "default_metric must be one of" in str(exc)
    else:
        raise AssertionError("expected invalid metric to raise")
