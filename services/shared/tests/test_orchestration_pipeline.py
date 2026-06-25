from collections.abc import AsyncIterator

from shared.orchestration.context import PipelineContext
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
