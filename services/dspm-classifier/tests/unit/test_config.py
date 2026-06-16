from __future__ import annotations

from app.config import Settings


def test_defaults_load_with_required_env(monkeypatch):
    monkeypatch.setenv("POSTGRES_DSN", "postgresql+asyncpg://u:p@h/db")
    monkeypatch.setenv("LLM_BASE_URL", "http://mock-ollama:11434")

    s = Settings()

    assert s.postgres_dsn == "postgresql+asyncpg://u:p@h/db"
    assert s.llm_base_url == "http://mock-ollama:11434"
    assert s.max_object_bytes == 10 * 1024 * 1024
    assert s.llm_concurrency == 8
    assert s.escalate_to_llm is True
    assert s.pipeline_version == 1
    assert s.s3_endpoint_url is None  # real AWS by default
    assert s.ner_confidence_threshold == 0.6


def test_env_overrides(monkeypatch):
    monkeypatch.setenv("POSTGRES_DSN", "postgresql+asyncpg://u:p@h/db")
    monkeypatch.setenv("LLM_BASE_URL", "http://x:1")
    monkeypatch.setenv("MAX_OBJECT_BYTES", "1024")
    monkeypatch.setenv("LLM_CONCURRENCY", "2")
    monkeypatch.setenv("ESCALATE_TO_LLM", "false")
    monkeypatch.setenv("S3_ENDPOINT_URL", "http://minio:9000")
    monkeypatch.setenv("PIPELINE_VERSION", "3")

    s = Settings()

    assert s.max_object_bytes == 1024
    assert s.llm_concurrency == 2
    assert s.escalate_to_llm is False
    assert s.s3_endpoint_url == "http://minio:9000"
    assert s.pipeline_version == 3
