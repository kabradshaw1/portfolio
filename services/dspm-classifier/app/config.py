"""Environment-driven settings for the dspm-classifier service."""

from __future__ import annotations

from pydantic_settings import BaseSettings, SettingsConfigDict


class Settings(BaseSettings):
    model_config = SettingsConfigDict(env_file=None, case_sensitive=False)

    postgres_dsn: str

    s3_endpoint_url: str | None = None
    s3_region: str = "us-east-1"

    llm_base_url: str
    llm_model: str = "llama3.1:8b"
    llm_timeout_seconds: float = 15.0
    llm_concurrency: int = 8

    max_object_bytes: int = 10 * 1024 * 1024
    pipeline_version: int = 1
    ner_confidence_threshold: float = 0.6
    escalate_to_llm: bool = True

    log_level: str = "INFO"
    metrics_port: int = 9100
