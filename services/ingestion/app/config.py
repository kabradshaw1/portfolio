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
    jwt_secret: str = ""

    # Per-collection metadata is stored in a tiny SQLite file so the eval
    # service can read back the chunk params and embedding model that
    # produced a collection. Qdrant has no first-class collection metadata.
    collection_meta_db_path: str = "data/collection_meta.db"

    def get_embedding_base_url(self) -> str:
        if self.embedding_provider == "ollama":
            return self.embedding_base_url or self.ollama_base_url
        return self.embedding_base_url

    def validate(self) -> None:
        """Fail fast if provider-required secrets are missing."""
        api_key_providers = ("openai", "anthropic")
        if self.llm_provider in api_key_providers and not self.llm_api_key:
            raise ValueError(
                f"llm_api_key is required when llm_provider is '{self.llm_provider}'"
            )
        if self.embedding_provider in api_key_providers and not self.embedding_api_key:
            raise ValueError(
                f"embedding_api_key is required when embedding_provider is "
                f"'{self.embedding_provider}'"
            )


settings = Settings()
settings.validate()
