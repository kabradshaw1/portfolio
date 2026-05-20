from pydantic_settings import BaseSettings


class Settings(BaseSettings):
    # Chat service URL (for calling /search and /chat)
    chat_service_url: str = "http://chat:8000"

    # Ingestion service URL (for snapshotting per-collection chunk params at run start)
    ingestion_service_url: str = "http://ingestion:8000"

    # LLM config for first-party evaluation judge calls
    llm_provider: str = "ollama"
    llm_base_url: str = "http://host.docker.internal:11434"
    llm_api_key: str = ""
    llm_model: str = "qwen2.5:14b"

    # SQLite database path
    db_path: str = "data/eval.db"

    # Evaluation runtime guardrails
    eval_run_max_seconds: float = 900.0
    eval_stale_grace_seconds: float = 300.0

    # RabbitMQ eval item worker
    rabbitmq_url: str = ""
    eval_item_queue: str = "eval.item.requested"
    eval_item_dlq: str = "eval.item.requested.dlq"
    eval_worker_prefetch: int = 2
    eval_worker_concurrency: int = 2
    eval_item_max_attempts: int = 3
    eval_item_lease_seconds: float = 300.0
    eval_stale_item_seconds: float = 900.0

    # Auth
    jwt_secret: str = ""

    # Tiered route quotas.
    eval_rate_limit_run_create_operator: str = "20/minute"
    eval_rate_limit_run_create_user: str = "5/minute"
    eval_rate_limit_run_create_anonymous: str = "0/minute"
    eval_rate_limit_read_operator: str = "240/minute"
    eval_rate_limit_read_user: str = "30/minute"
    eval_rate_limit_read_anonymous: str = "10/minute"
    eval_rate_limit_write_operator: str = "30/minute"
    eval_rate_limit_write_user: str = "10/minute"
    eval_rate_limit_write_anonymous: str = "0/minute"
    rag_internal_eval_token: str = ""

    # CORS
    allowed_origins: str = "https://kylebradshaw.dev"


settings = Settings()
