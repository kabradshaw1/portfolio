"""Test doubles so node/graph tests never call the real API.

FakeChatModel mimics the small slice of the LangChain chat-model interface the
nodes use: `.with_structured_output(schema)` and `.bind_tools(tools)`, both
returning an object with `.invoke(messages)`.
"""

import pytest
from langchain_core.messages import AIMessage


class _StructuredRunnable:
    def __init__(self, payload):
        self._payload = payload

    def invoke(self, _messages):
        return self._payload


class _ToolRunnable:
    def __init__(self, scripted):
        # scripted: list of AIMessage to return on successive invokes
        self._scripted = list(scripted)

    def invoke(self, _messages):
        return self._scripted.pop(0) if self._scripted else AIMessage(content="done")


class FakeChatModel:
    """Returns canned structured output and a scripted tool-call sequence."""

    def __init__(self, *, structured=None, tool_script=None):
        self._structured = structured
        self._tool_script = tool_script or []

    def with_structured_output(self, _schema):
        return _StructuredRunnable(self._structured)

    def bind_tools(self, _tools):
        return _ToolRunnable(self._tool_script)


@pytest.fixture
def make_fake_model():
    def _factory(*, structured=None, tool_script=None):
        return FakeChatModel(structured=structured, tool_script=tool_script)

    return _factory


@pytest.fixture(autouse=True)
def _disable_rate_limiting():
    """Disable rate limiting when main.py is imported (mirrors debug)."""
    try:
        from app.main import limiter
    except Exception:
        yield
        return
    limiter.enabled = False
    yield
    limiter.enabled = True
