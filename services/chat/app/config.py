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
    collection_name: str = "documents"
    allowed_origins: str = "https://kylebradshaw.dev"
    jwt_secret: str = ""

    # Retrieval tuning — number of chunks pulled from Qdrant per query.
    top_k: int = 5

    # Prompt versioning — selects which template in app.prompt.PROMPTS is active.
    # Validated in self.validate() to fail fast on typos.
    prompt_version: str = "v1-baseline"

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

    def validate(self) -> None:
        """Fail fast if provider-required secrets are missing or prompt unknown."""
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
        # Lazy import: app.prompt imports settings, so a top-level import here
        # would create a cycle.
        from app.prompt import PROMPTS

        if self.prompt_version not in PROMPTS:
            raise ValueError(
                f"prompt_version '{self.prompt_version}' is not in the registry "
                f"(known: {sorted(PROMPTS)})"
            )


settings = Settings()
settings.validate()
