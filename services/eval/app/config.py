from pydantic_settings import BaseSettings


class Settings(BaseSettings):
    # Chat service URL (for calling /search and /chat)
    chat_service_url: str = "http://chat:8000"

    # Ingestion service URL (for snapshotting per-collection chunk params at run start)
    ingestion_service_url: str = "http://ingestion:8000"

    # LLM config for RAGAS judge calls
    llm_provider: str = "ollama"
    llm_base_url: str = "http://host.docker.internal:11434"
    llm_api_key: str = ""
    llm_model: str = "qwen2.5:14b"

    # SQLite database path
    db_path: str = "data/eval.db"

    # Auth
    jwt_secret: str = ""

    # CORS
    allowed_origins: str = "https://kylebradshaw.dev"


settings = Settings()
