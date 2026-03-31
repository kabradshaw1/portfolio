from typing import AsyncGenerator

import httpx

from app.prompt import SYSTEM_PROMPT, build_rag_prompt
from app.retriever import QdrantRetriever


async def embed_texts(
    texts: list[str],
    ollama_base_url: str,
    model: str,
) -> list[list[float]]:
    if not texts:
        return []
    async with httpx.AsyncClient() as client:
        response = await client.post(
            f"{ollama_base_url}/api/embed",
            json={"model": model, "input": texts},
            timeout=120.0,
        )
        response.raise_for_status()
        return response.json()["embeddings"]


async def stream_ollama_response(
    prompt: str,
    model: str,
    base_url: str,
) -> AsyncGenerator[dict, None]:
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
            import json

            async for line in response.aiter_lines():
                if line.strip():
                    data = json.loads(line)
                    if data.get("response"):
                        yield {"token": data["response"]}
                    if data.get("done"):
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

    # Build prompt
    prompt = build_rag_prompt(question=question, chunks=chunks)

    # Collect unique sources
    seen = set()
    sources = []
    for chunk in chunks:
        key = (chunk["filename"], chunk["page_number"])
        if key not in seen:
            seen.add(key)
            sources.append({"file": chunk["filename"], "page": chunk["page_number"]})

    # Stream response
    async for event in stream_ollama_response(
        prompt=prompt, model=chat_model, base_url=ollama_base_url
    ):
        yield event

    yield {"done": True, "sources": sources}
