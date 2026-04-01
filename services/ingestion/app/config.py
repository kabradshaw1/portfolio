from pydantic_settings import BaseSettings


class Settings(BaseSettings):
    ollama_base_url: str = "http://host.docker.internal:11434"
    embedding_model: str = "nomic-embed-text"
    qdrant_host: str = "qdrant"
    qdrant_port: int = 6333
    collection_name: str = "documents"
    chunk_size: int = 1000
    chunk_overlap: int = 200
    max_file_size_mb: int = 50
    allowed_origins: str = "https://kylebradshaw.dev"


settings = Settings()
