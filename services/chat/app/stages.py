"""Chat RAG stages adapted to the shared orchestration scaffold.

Each stage is a thin adapter over the existing chain functions, preserving
their internals (and their RAG_PIPELINE_DURATION / OLLAMA_* metric emissions).
"""

from __future__ import annotations

from collections.abc import AsyncIterator
from dataclasses import dataclass
from typing import Any

from llm.base import EmbeddingProvider, LLMProvider
from shared.orchestration import PipelineContext

import app.chain as _chain
from app.prompt import build_rag_prompt
from app.retriever import RetrievalResult


async def retrieve_chunks(*args, **kwargs):
    """Shim resolving app.chain.retrieve_chunks at call time, not import time.
    Lets test_chain.py patch app.chain.* and test_stages.py patch app.stages.*
    independently — do NOT inline this or one test suite breaks."""
    return await _chain.retrieve_chunks(*args, **kwargs)


async def stream_response(*args, **kwargs):
    """Shim resolving app.chain.stream_response at call time, not import time.
    Lets test_chain.py patch app.chain.* and test_stages.py patch app.stages.*
    independently — do NOT inline this or one test suite breaks."""
    async for event in _chain.stream_response(*args, **kwargs):
        yield event


@dataclass
class ChatState:
    question: str
    top_k: int
    rerank: bool
    chat_model: str
    embedding_model: str
    qdrant_host: str
    qdrant_port: int
    collection_name: str
    embedding_provider: EmbeddingProvider
    llm_provider: LLMProvider
    retrieval: RetrievalResult | None = None
    prompt: str = ""


class RetrieveStage:
    name = "retrieve"

    async def run(self, ctx: PipelineContext) -> PipelineContext:
        s = ctx.state
        s.retrieval = await retrieve_chunks(
            question=s.question,
            embedding_provider=s.embedding_provider,
            embedding_model=s.embedding_model,
            qdrant_host=s.qdrant_host,
            qdrant_port=s.qdrant_port,
            collection_name=s.collection_name,
            top_k=s.top_k,
            rerank=s.rerank,
        )
        return ctx


class BuildPromptStage:
    name = "build_prompt"

    async def run(self, ctx: PipelineContext) -> PipelineContext:
        chunks = ctx.state.retrieval.chunks if ctx.state.retrieval else []
        ctx.state.prompt = build_rag_prompt(question=ctx.state.question, chunks=chunks)
        return ctx


class GenerateStreamStage:
    name = "generate"

    async def stream(self, ctx: PipelineContext) -> AsyncIterator[dict[str, Any]]:
        async for event in stream_response(
            prompt=ctx.state.prompt,
            model=ctx.state.chat_model,
            provider=ctx.state.llm_provider,
        ):
            yield event
