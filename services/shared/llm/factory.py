"""Factory functions to create LLM and embedding providers from env vars."""

from __future__ import annotations

from llm.anthropic_provider import AnthropicLLMProvider
from llm.ollama import OllamaEmbeddingProvider, OllamaLLMProvider
from llm.openai_compat import OpenAIEmbeddingProvider, OpenAILLMProvider


def get_llm_provider(
    provider: str,
    base_url: str,
    api_key: str,
    model: str,
) -> OllamaLLMProvider | OpenAILLMProvider | AnthropicLLMProvider:
    """Create an LLM provider based on the provider name."""
    if provider == "ollama":
        return OllamaLLMProvider(base_url=base_url, model=model)
    if provider == "openai":
        return OpenAILLMProvider(base_url=base_url, api_key=api_key, model=model)
    if provider == "anthropic":
        return AnthropicLLMProvider(api_key=api_key, model=model)
    raise ValueError(
        f"Unknown LLM provider: {provider!r}. Use 'ollama', 'openai', or 'anthropic'."
    )


def get_embedding_provider(
    provider: str,
    base_url: str,
    api_key: str,
    model: str,
) -> OllamaEmbeddingProvider | OpenAIEmbeddingProvider:
    """Create an embedding provider based on the provider name."""
    if provider == "ollama":
        return OllamaEmbeddingProvider(base_url=base_url, model=model)
    if provider == "openai":
        return OpenAIEmbeddingProvider(base_url=base_url, api_key=api_key, model=model)
    raise ValueError(
        f"Unknown embedding provider: {provider!r}. Use 'ollama' or 'openai'."
    )
