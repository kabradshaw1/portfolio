import time
from collections.abc import AsyncGenerator

import httpx

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
    ollama_base_url: str,
    model: str,
) -> list[list[float]]:
    if not texts:
        return []

    start = time.perf_counter()
    async with httpx.AsyncClient() as client:
        response = await client.post(
            f"{ollama_base_url}/api/embed",
            json={"model": model, "input": texts},
            timeout=120.0,
        )
        response.raise_for_status()
    EMBEDDING_DURATION.labels(service="chat", model=model).observe(
        time.perf_counter() - start
    )

    return response.json()["embeddings"]


async def stream_ollama_response(
    prompt: str,
    model: str,
    base_url: str,
) -> AsyncGenerator[dict, None]:
    import json

    start = time.perf_counter()
    async with httpx.AsyncClient() as client:
        async with client.stream(
            "POST",
            f"{base_url}/api/generate",
            json={
                "model": model,
                "prompt": prompt,
                "system": SYSTEM_PROMPT,
                "stream": True,
            },
            timeout=300.0,
        ) as response:
            response.raise_for_status()

            async for line in response.aiter_lines():
                if line.strip():
                    data = json.loads(line)
                    if data.get("response"):
                        yield {"token": data["response"]}
                    if data.get("done"):
                        # Extract token metrics from final chunk
                        duration = time.perf_counter() - start
                        OLLAMA_REQUEST_DURATION.labels(
                            service="chat", model=model, operation="generate"
                        ).observe(duration)
                        prompt_tokens = data.get("prompt_eval_count", 0)
                        completion_tokens = data.get("eval_count", 0)
                        if prompt_tokens:
                            OLLAMA_TOKENS.labels(
                                service="chat", model=model, kind="prompt"
                            ).inc(prompt_tokens)
                        if completion_tokens:
                            OLLAMA_TOKENS.labels(
                                service="chat", model=model, kind="completion"
                            ).inc(completion_tokens)
                        eval_ns = data.get("eval_duration", 0)
                        if eval_ns:
                            OLLAMA_EVAL_DURATION.labels(
                                service="chat", model=model
                            ).observe(eval_ns / 1e9)
                        break


async def rag_query(
    question: str,
    ollama_base_url: str,
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
        ollama_base_url=ollama_base_url,
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

    # Stream response (generate stage timing is inside stream_ollama_response)
    generate_start = time.perf_counter()
    async for event in stream_ollama_response(
        prompt=prompt, model=chat_model, base_url=ollama_base_url
    ):
        yield event
    RAG_PIPELINE_DURATION.labels(stage="generate").observe(
        time.perf_counter() - generate_start
    )

    yield {"done": True, "sources": sources}
