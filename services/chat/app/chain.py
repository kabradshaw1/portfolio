import asyncio
import time
from collections.abc import AsyncGenerator

import structlog
from llm.base import EmbeddingProvider, LLMProvider
from rag.sparse import SparseVectorEncoder
from shared.llm.admission import embed_limiter, generate_limiter, rerank_limiter

from app.config import settings
from app.metrics import (
    EMBEDDING_DURATION,
    OLLAMA_EVAL_DURATION,
    OLLAMA_REQUEST_DURATION,
    OLLAMA_TOKENS,
    RAG_PIPELINE_DURATION,
    RERANK_FALLBACKS,
)
from app.prompt import SYSTEM_PROMPT, build_rag_prompt
from app.reranker import rerank_chunks
from app.retriever import QdrantRetriever, RetrievalResult

logger = structlog.get_logger()
_sparse_encoder: SparseVectorEncoder | None = None


def get_sparse_encoder() -> SparseVectorEncoder:
    global _sparse_encoder
    if _sparse_encoder is None:
        _sparse_encoder = SparseVectorEncoder(
            model_name=settings.sparse_model,
            batch_size=settings.sparse_batch_size,
        )
    return _sparse_encoder


def _search_legacy_semantic_fallback(
    retriever: QdrantRetriever,
    query_vector: list[float],
    top_k: int,
    collection_name: str,
    error: Exception,
) -> RetrievalResult:
    logger.warning(
        "semantic_legacy_fallback",
        error=str(error),
        error_type=error.__class__.__name__,
        collection=collection_name,
        exc_info=True,
    )
    return retriever.search_semantic(
        query_vector=query_vector,
        top_k=top_k,
        legacy_vector=True,
        fallback=True,
    )


def _rerank_candidate_limit(top_k: int) -> int:
    return min(
        max(top_k, settings.rerank_candidate_limit),
        settings.rerank_max_candidates,
    )


def _with_rerank_metadata(
    retrieval: RetrievalResult,
    metadata: dict,
) -> RetrievalResult:
    merged = dict(retrieval.metadata)
    merged.update(metadata)
    return RetrievalResult(chunks=retrieval.chunks, metadata=merged)


async def embed_texts(
    texts: list[str],
    provider: EmbeddingProvider,
    model: str,
) -> list[list[float]]:
    if not texts:
        return []

    start = time.perf_counter()
    permit = await embed_limiter.acquire()
    try:
        embeddings = await provider.embed(texts)
    finally:
        permit.release()
    EMBEDDING_DURATION.labels(service="chat", model=model).observe(
        time.perf_counter() - start
    )

    return embeddings


async def get_embedding(
    text: str,
    provider: EmbeddingProvider,
    model: str,
) -> list[float]:
    embeddings = await embed_texts(texts=[text], provider=provider, model=model)
    return embeddings[0]


async def stream_response(
    prompt: str,
    model: str,
    provider: LLMProvider,
) -> AsyncGenerator[dict, None]:
    start = time.perf_counter()
    permit = await generate_limiter.acquire()
    try:
        async for event in provider.generate(prompt=prompt, system=SYSTEM_PROMPT):
            if "token" in event:
                yield {"token": event["token"]}
            if event.get("done"):
                duration = time.perf_counter() - start
                OLLAMA_REQUEST_DURATION.labels(
                    service="chat", model=model, operation="generate"
                ).observe(duration)
                prompt_tokens = event.get("prompt_eval_count", 0)
                completion_tokens = event.get("eval_count", 0)
                if prompt_tokens:
                    OLLAMA_TOKENS.labels(
                        service="chat", model=model, kind="prompt"
                    ).inc(prompt_tokens)
                if completion_tokens:
                    OLLAMA_TOKENS.labels(
                        service="chat", model=model, kind="completion"
                    ).inc(completion_tokens)
                eval_ns = event.get("eval_duration", 0)
                if eval_ns:
                    OLLAMA_EVAL_DURATION.labels(service="chat", model=model).observe(
                        eval_ns / 1e9
                    )
                break
    finally:
        permit.release()


async def retrieve_chunks(
    question: str,
    embedding_provider: EmbeddingProvider,
    embedding_model: str,
    qdrant_host: str,
    qdrant_port: int,
    collection_name: str,
    top_k: int = 5,
    rerank: bool = False,
) -> RetrievalResult:
    """Embed question and retrieve ranked chunks from Qdrant."""
    retrieve_start = time.perf_counter()
    query_vector = await get_embedding(
        text=question,
        provider=embedding_provider,
        model=embedding_model,
    )

    retriever = QdrantRetriever(
        host=qdrant_host, port=qdrant_port, collection_name=collection_name
    )
    retrieval_top_k = _rerank_candidate_limit(top_k) if rerank else top_k
    if settings.retrieval_mode == "hybrid":
        try:
            sparse_vectors = await asyncio.to_thread(
                get_sparse_encoder().embed, [question]
            )
            sparse_vector = sparse_vectors[0]
            result = retriever.search_hybrid(
                query_vector=query_vector,
                sparse_vector=sparse_vector,
                top_k=retrieval_top_k,
                prefetch_limit=max(settings.hybrid_prefetch_limit, retrieval_top_k),
            )
        except Exception as e:
            logger.warning(
                "hybrid_retrieval_fallback",
                error=str(e),
                error_type=e.__class__.__name__,
                collection=collection_name,
                exc_info=True,
            )
            try:
                result = retriever.search_semantic(
                    query_vector=query_vector,
                    top_k=retrieval_top_k,
                    fallback=True,
                )
            except Exception as semantic_error:
                result = _search_legacy_semantic_fallback(
                    retriever=retriever,
                    query_vector=query_vector,
                    top_k=retrieval_top_k,
                    collection_name=collection_name,
                    error=semantic_error,
                )
    else:
        try:
            result = retriever.search_semantic(
                query_vector=query_vector, top_k=retrieval_top_k
            )
        except Exception as semantic_error:
            result = _search_legacy_semantic_fallback(
                retriever=retriever,
                query_vector=query_vector,
                top_k=retrieval_top_k,
                collection_name=collection_name,
                error=semantic_error,
            )

    result = _with_rerank_metadata(result, {"top_k": top_k})

    if rerank and not settings.rerank_enabled:
        result = _with_rerank_metadata(
            result,
            {
                "rerank_requested": True,
                "rerank_enabled": False,
                "rerank_applied": False,
                "rerank_model": settings.rerank_model,
                "rerank_candidate_count": len(result.chunks),
                "rerank_returned_count": min(len(result.chunks), top_k),
                "rerank_fallback": False,
            },
        )
        result = RetrievalResult(chunks=result.chunks[:top_k], metadata=result.metadata)
    elif rerank:
        try:
            permit = await rerank_limiter.acquire()
            try:
                rerank_result = rerank_chunks(
                    query=question,
                    chunks=result.chunks,
                    top_k=top_k,
                    model_name=settings.rerank_model,
                    device=settings.rerank_device,
                )
            finally:
                permit.release()
            metadata = {
                "rerank_requested": True,
                "rerank_enabled": True,
                "rerank_fallback": False,
            }
            metadata.update(rerank_result.metadata)
            result = RetrievalResult(
                chunks=rerank_result.chunks,
                metadata={**result.metadata, **metadata},
            )
            RAG_PIPELINE_DURATION.labels(stage="rerank").observe(
                time.perf_counter() - retrieve_start
            )
        except Exception as e:
            logger.warning(
                "rerank_fallback",
                error=str(e),
                error_type=e.__class__.__name__,
                collection=collection_name,
                candidate_count=len(result.chunks),
                exc_info=True,
            )
            RERANK_FALLBACKS.labels(reason=e.__class__.__name__).inc()
            result = RetrievalResult(
                chunks=result.chunks[:top_k],
                metadata={
                    **result.metadata,
                    "rerank_requested": True,
                    "rerank_enabled": True,
                    "rerank_applied": False,
                    "rerank_model": settings.rerank_model,
                    "rerank_candidate_count": len(result.chunks),
                    "rerank_returned_count": min(len(result.chunks), top_k),
                    "rerank_fallback": True,
                    "rerank_error": e.__class__.__name__,
                },
            )
    else:
        result = _with_rerank_metadata(
            result,
            {
                "rerank_requested": False,
                "rerank_enabled": settings.rerank_enabled,
                "rerank_applied": False,
                "rerank_fallback": False,
            },
        )

    RAG_PIPELINE_DURATION.labels(stage="retrieve").observe(
        time.perf_counter() - retrieve_start
    )
    return result


async def rag_query(
    question: str,
    llm_provider: LLMProvider,
    embedding_provider: EmbeddingProvider,
    chat_model: str,
    embedding_model: str,
    qdrant_host: str,
    qdrant_port: int,
    collection_name: str,
    top_k: int = 5,
    rerank: bool = False,
) -> AsyncGenerator[dict, None]:
    # Retrieve relevant chunks
    retrieval = await retrieve_chunks(
        question=question,
        embedding_provider=embedding_provider,
        embedding_model=embedding_model,
        qdrant_host=qdrant_host,
        qdrant_port=qdrant_port,
        collection_name=collection_name,
        top_k=top_k,
        rerank=rerank,
    )
    chunks = retrieval.chunks

    # Build prompt
    build_start = time.perf_counter()
    prompt = build_rag_prompt(question=question, chunks=chunks)
    RAG_PIPELINE_DURATION.labels(stage="build_prompt").observe(
        time.perf_counter() - build_start
    )

    # Collect unique sources
    seen = set()
    sources = []
    for chunk in chunks:
        key = (chunk["filename"], chunk["page_number"])
        if key not in seen:
            seen.add(key)
            sources.append({"file": chunk["filename"], "page": chunk["page_number"]})

    # Stream response (generate stage timing is inside stream_response)
    generate_start = time.perf_counter()
    async for event in stream_response(
        prompt=prompt, model=chat_model, provider=llm_provider
    ):
        yield event
    RAG_PIPELINE_DURATION.labels(stage="generate").observe(
        time.perf_counter() - generate_start
    )

    yield {"done": True, "sources": sources, "retrieval": retrieval.metadata}
