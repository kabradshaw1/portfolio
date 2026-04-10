import time

from llm.base import EmbeddingProvider

from app.metrics import EMBEDDING_DURATION


async def embed_texts(
    texts: list[str],
    provider: EmbeddingProvider,
    model: str,
) -> list[list[float]]:
    """Embed a list of texts using the configured embedding provider.

    Returns a list of embedding vectors (list of floats).
    """
    if not texts:
        return []

    start = time.perf_counter()
    embeddings = await provider.embed(texts)
    EMBEDDING_DURATION.labels(service="ingestion", model=model).observe(
        time.perf_counter() - start
    )

    return embeddings
