from pydantic_settings import BaseSettings


class Settings(BaseSettings):
    ollama_base_url: str = "http://host.docker.internal:11434"
    chat_model: str = "qwen2.5:14b"
    embedding_model: str = "nomic-embed-text"
    qdrant_host: str = "qdrant"
    qdrant_port: int = 6333
    collection_name: str = "documents"
    allowed_origins: str = "https://kylebradshaw.dev"


settings = Settings()
