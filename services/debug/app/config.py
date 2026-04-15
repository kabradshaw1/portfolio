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

    # Legacy — used as fallback for base URLs when provider is ollama
    ollama_base_url: str = "http://host.docker.internal:11434"
    chat_model: str = "qwen2.5:14b"

    qdrant_host: str = "qdrant"
    qdrant_port: int = 6333
    max_agent_steps: int = 10
    max_file_lines: int = 100
    max_grep_matches: int = 20
    test_timeout_seconds: int = 30
    allowed_origins: str = "https://kylebradshaw.dev"
    allowed_project_paths: str = ""  # Comma-separated list; empty = deny all

    def get_llm_base_url(self) -> str:
        if self.llm_provider == "ollama":
            return self.llm_base_url or self.ollama_base_url
        return self.llm_base_url

    def get_llm_model(self) -> str:
        return self.llm_model or self.chat_model

    def get_embedding_base_url(self) -> str:
        if self.embedding_provider == "ollama":
            return self.embedding_base_url or self.ollama_base_url
        return self.embedding_base_url


settings = Settings()
