import time
from collections.abc import AsyncGenerator

from llm.base import EmbeddingProvider, LLMProvider

from app.metrics import (
    EMBEDDING_DURATION,
    OLLAMA_EVAL_DURATION,
    OLLAMA_REQUEST_DURATION,
    OLLAMA_TOKENS,
    RAG_PIPELINE_DURATION,
)
from app.prompt import SYSTEM_PROMPT, build_rag_prompt
from app.retriever import QdrantRetriever


async def embed_texts(
    texts: list[str],
    provider: EmbeddingProvider,
    model: str,
) -> list[list[float]]:
    if not texts:
        return []

    start = time.perf_counter()
    embeddings = await provider.embed(texts)
    EMBEDDING_DURATION.labels(service="chat", model=model).observe(
        time.perf_counter() - start
    )

    return embeddings


async def stream_response(
    prompt: str,
    model: str,
    provider: LLMProvider,
) -> AsyncGenerator[dict, None]:
    start = time.perf_counter()
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
                OLLAMA_TOKENS.labels(service="chat", model=model, kind="prompt").inc(
                    prompt_tokens
                )
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
) -> AsyncGenerator[dict, None]:
    # Embed the question
    retrieve_start = time.perf_counter()
    vectors = await embed_texts(
        texts=[question],
        provider=embedding_provider,
        model=embedding_model,
    )
    query_vector = vectors[0]

    # Retrieve relevant chunks
    retriever = QdrantRetriever(
        host=qdrant_host, port=qdrant_port, collection_name=collection_name
    )
    chunks = retriever.search(query_vector=query_vector, top_k=top_k)
    RAG_PIPELINE_DURATION.labels(stage="retrieve").observe(
        time.perf_counter() - retrieve_start
    )

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

    yield {"done": True, "sources": sources}
