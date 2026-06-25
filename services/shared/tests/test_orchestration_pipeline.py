from collections.abc import AsyncIterator

import pytest

from shared.orchestration.context import PipelineContext
from shared.orchestration.errors import StageError
from shared.orchestration.metrics import STAGE_DURATION
from shared.orchestration.pipeline import Pipeline
from shared.orchestration.stage import Stage, StreamingStage


class _AddStage:
    name = "add"

    async def run(self, ctx: PipelineContext) -> PipelineContext:
        ctx.state["n"] += 1
        return ctx


class _EmitStage:
    name = "emit"

    async def stream(self, ctx: PipelineContext) -> AsyncIterator[dict]:
        yield {"token": "a"}
        yield {"token": "b"}


def test_add_stage_satisfies_stage_protocol():
    assert isinstance(_AddStage(), Stage)


def test_emit_stage_satisfies_streaming_protocol():
    assert isinstance(_EmitStage(), StreamingStage)


def test_plain_object_is_not_a_stage():
    assert not isinstance(object(), Stage)


def _stage_count(stage: str, status: str) -> float:
    """Get the observation count for a labeled histogram metric."""
    labeled = STAGE_DURATION.labels(pipeline="test", stage=stage, status=status)
    # Extract count from the histogram samples
    samples = labeled._samples()
    count_samples = [s for s in samples if s.name == "_count"]
    assert len(count_samples) == 1, f"expected one _count sample, got {count_samples!r}"
    return count_samples[0].value


class _FailStage:
    name = "fail"

    async def run(self, ctx):
        raise ValueError("boom")


@pytest.mark.asyncio
async def test_run_executes_stages_in_order():
    pipe = Pipeline("test")
    ctx = PipelineContext(state={"n": 0})
    ctx = await pipe.run([_AddStage(), _AddStage()], ctx)
    assert ctx.state["n"] == 2


@pytest.mark.asyncio
async def test_execute_records_ok_metric():
    pipe = Pipeline("test")
    before = _stage_count("add", "ok")
    await pipe.execute(_AddStage(), PipelineContext(state={"n": 0}))
    assert _stage_count("add", "ok") == before + 1


@pytest.mark.asyncio
async def test_execute_classifies_and_records_error():
    pipe = Pipeline("test")
    before = _stage_count("fail", "error")
    with pytest.raises(StageError) as exc:
        await pipe.execute(_FailStage(), PipelineContext(state=None))
    assert exc.value.stage == "fail"
    assert exc.value.retryable is False
    assert _stage_count("fail", "error") == before + 1


@pytest.mark.asyncio
async def test_execute_stream_yields_events():
    pipe = Pipeline("test")
    events = [
        e async for e in pipe.execute_stream(_EmitStage(), PipelineContext(state=None))
    ]
    assert events == [{"token": "a"}, {"token": "b"}]
