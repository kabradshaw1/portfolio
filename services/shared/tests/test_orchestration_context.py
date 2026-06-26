import pytest

from shared.orchestration.context import PipelineContext
from shared.orchestration.errors import CancelledPipelineError


def test_context_holds_typed_state():
    ctx = PipelineContext(state={"question": "hi"})
    assert ctx.state["question"] == "hi"


def test_context_generates_run_id_when_absent():
    ctx = PipelineContext(state=None)
    assert isinstance(ctx.run_id, str)
    assert len(ctx.run_id) > 0


def test_context_uses_supplied_run_id_and_metadata():
    ctx = PipelineContext(state=None, run_id="abc", metadata={"collection": "docs"})
    assert ctx.run_id == "abc"
    assert ctx.metadata["collection"] == "docs"


@pytest.mark.asyncio
async def test_check_cancelled_is_noop_without_predicate():
    ctx = PipelineContext(state=None)
    await ctx.check_cancelled()  # must not raise


@pytest.mark.asyncio
async def test_check_cancelled_invokes_predicate():
    async def cancel():
        raise CancelledPipelineError()

    ctx = PipelineContext(state=None, cancel_check=cancel)
    with pytest.raises(CancelledPipelineError):
        await ctx.check_cancelled()
