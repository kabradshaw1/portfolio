from pydantic_settings import BaseSettings


class Settings(BaseSettings):
    # LLM provider config
    llm_provider: str = "ollama"
    llm_base_url: str = "http://host.docker.internal:11434"
    llm_api_key: str = ""
    llm_model: str = ""

    # Embedding provider config
    embedding_provider: str = "ollama"
    embedding_base_url: str = "http://host.docker.internal:11434"
    embedding_api_key: str = ""
    embedding_model: str = "nomic-embed-text"

    # Legacy — used as fallback for embedding_base_url when provider is ollama
    ollama_base_url: str = "http://host.docker.internal:11434"

    qdrant_host: str = "qdrant"
    qdrant_port: int = 6333
    collection_name: str = "documents"
    chunk_size: int = 1000
    chunk_overlap: int = 200
    max_file_size_mb: int = 50
    allowed_origins: str = "https://kylebradshaw.dev"

    def get_embedding_base_url(self) -> str:
        if self.embedding_provider == "ollama":
            return self.embedding_base_url or self.ollama_base_url
        return self.embedding_base_url


settings = Settings()
