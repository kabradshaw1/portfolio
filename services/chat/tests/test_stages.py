import pytest
from app.retriever import RetrievalResult
from app.stages import (
    BuildPromptStage,
    ChatState,
    GenerateStreamStage,
    RetrieveStage,
)
from shared.orchestration import PipelineContext


def _state(**overrides):
    base = dict(
        question="what is X?",
        top_k=5,
        rerank=False,
        chat_model="m",
        embedding_model="e",
        qdrant_host="h",
        qdrant_port=6333,
        collection_name="docs",
        embedding_provider=object(),
        llm_provider=object(),
    )
    base.update(overrides)
    return ChatState(**base)


@pytest.mark.asyncio
async def test_retrieve_stage_populates_retrieval(monkeypatch):
    fake = RetrievalResult(
        chunks=[{"filename": "a.pdf", "page_number": 1, "text": "hi"}],
        metadata={"top_k": 5},
    )

    async def fake_retrieve(**kwargs):
        assert kwargs["question"] == "what is X?"
        return fake

    monkeypatch.setattr("app.stages.retrieve_chunks", fake_retrieve)
    ctx = PipelineContext(state=_state())
    ctx = await RetrieveStage().run(ctx)
    assert ctx.state.retrieval is fake


@pytest.mark.asyncio
async def test_build_prompt_stage_sets_prompt():
    ctx = PipelineContext(state=_state())
    ctx.state.retrieval = RetrievalResult(
        chunks=[{"filename": "a.pdf", "page_number": 2, "text": "body"}],
        metadata={},
    )
    ctx = await BuildPromptStage().run(ctx)
    assert "body" in ctx.state.prompt
    assert "a.pdf" in ctx.state.prompt


@pytest.mark.asyncio
async def test_generate_stream_stage_yields_tokens(monkeypatch):
    async def fake_stream(prompt, model, provider):
        yield {"token": "Hel"}
        yield {"token": "lo"}
        yield {"done": True, "usage": {"prompt_tokens": 3, "completion_tokens": 2}}

    monkeypatch.setattr("app.stages.stream_response", fake_stream)
    ctx = PipelineContext(state=_state())
    ctx.state.prompt = "p"
    events = [e async for e in GenerateStreamStage().stream(ctx)]
    assert {"token": "Hel"} in events
    assert events[-1]["done"] is True
