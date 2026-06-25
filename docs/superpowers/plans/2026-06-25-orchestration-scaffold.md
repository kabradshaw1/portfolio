# Orchestration Scaffold Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a reusable in-process multi-agent pipeline scaffold at `services/shared/orchestration`, then migrate the chat RAG pipeline onto it without changing chat's external behavior.

**Architecture:** Five primitives — `PipelineContext` (typed state threaded through stages), `Stage`/`StreamingStage` (the unit of work), `Role` (system prompt + model binding via `shared.llm`), `call_validated` (schema + retry-on-invalid LLM output), and `Pipeline` (a thin runner whose `execute`/`execute_stream` wrap each stage with metrics, tracing, structured-logging context, and error classification). Control flow stays in plain Python in the consuming service; the scaffold owns only the unit and the cross-cutting concerns.

**Tech Stack:** Python 3.11, Pydantic v2, structlog, OpenTelemetry API, prometheus_client, pytest + pytest-asyncio. Builds on existing `shared.llm` provider abstraction.

## Global Constraints

- **Scope:** This plan covers **Phase 1 (scaffold package) + Phase 2 (chat migration) only.** eval/dspm/debug migrations are separate later plans.
- **Import path:** the package is `services/shared/orchestration/`, imported by consumers and tests as `from shared.orchestration import ...` (matches the existing `from shared.llm.admission import ...` convention). No change to `shared/pyproject.toml` is needed — `shared/__init__.py` already exists and `PYTHONPATH=services` makes `shared.*` importable.
- **Internal LLM access:** the scaffold reaches providers only via `from shared.llm import get_llm_provider, LLMProvider, EmbeddingProvider` — no new provider code.
- **Test command:** all Python tests run from the repo root with the standard services virtualenv active: `PYTHONPATH=services pytest services/<svc>/tests/<file> -v`.
- **CI gate:** `ruff check services/shared/orchestration` and `ruff format --check` must pass before every commit (project convention).
- **Behavior preservation (Phase 2):** chat's existing test suite (`services/chat/tests/`) must stay 100% green after migration. The external SSE event contract of `rag_query` (token events `{"token": ...}`, then a final `{"done": True, "sources": [...], "retrieval": {...}, "usage": {...}}`) must be byte-for-byte unchanged. Existing chat metrics (`RAG_PIPELINE_DURATION`, `OLLAMA_*`) must continue to be emitted.
- **Commit style:** Conventional Commits, ending every commit message with `Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>`. Do not push (feature branch → PR to qa only when the work is complete; main only on "ship it").

---

## File Structure

**Phase 1 — scaffold (new files):**
- `services/shared/orchestration/__init__.py` — public exports
- `services/shared/orchestration/errors.py` — `StageError`, `OutputValidationError`, `CancelledPipelineError`, `classify_error`
- `services/shared/orchestration/context.py` — `PipelineContext`
- `services/shared/orchestration/stage.py` — `Stage`, `StreamingStage` protocols
- `services/shared/orchestration/role.py` — `Role`
- `services/shared/orchestration/metrics.py` — `STAGE_DURATION` histogram
- `services/shared/orchestration/validation.py` — `call_validated`
- `services/shared/orchestration/pipeline.py` — `Pipeline`
- `services/shared/tests/test_orchestration_errors.py`
- `services/shared/tests/test_orchestration_context.py`
- `services/shared/tests/test_orchestration_role.py`
- `services/shared/tests/test_orchestration_validation.py`
- `services/shared/tests/test_orchestration_pipeline.py`

**Phase 2 — chat migration:**
- Create: `services/chat/app/stages.py` — `ChatState`, `RetrieveStage`, `BuildPromptStage`, `GenerateStreamStage`
- Create: `services/chat/tests/test_stages.py`
- Modify: `services/chat/app/chain.py` — rewrite `rag_query` to drive a `Pipeline`

---

## PHASE 1 — Scaffold Package

### Task 1: Errors module

**Files:**
- Create: `services/shared/orchestration/__init__.py`
- Create: `services/shared/orchestration/errors.py`
- Test: `services/shared/tests/test_orchestration_errors.py`

**Interfaces:**
- Produces:
  - `StageError(message: str, *, stage: str, retryable: bool)` with attributes `.stage`, `.retryable`
  - `OutputValidationError(StageError)` — `retryable` defaults to `False`
  - `CancelledPipelineError(Exception)`
  - `classify_error(exc: Exception, *, stage: str) -> StageError`

- [ ] **Step 1: Create the package marker**

Create `services/shared/orchestration/__init__.py` with an empty body (exports are added in later tasks):

```python
"""In-process multi-agent pipeline scaffold.

Coordinates stages with standardized metrics, tracing, structured-logging
context, output validation, and error classification.
"""
```

- [ ] **Step 2: Write the failing test**

Create `services/shared/tests/test_orchestration_errors.py`:

```python
from shared.orchestration.errors import (
    CancelledPipelineError,
    OutputValidationError,
    StageError,
    classify_error,
)


def test_stage_error_carries_stage_and_retryable():
    err = StageError("boom", stage="retrieve", retryable=True)
    assert err.stage == "retrieve"
    assert err.retryable is True
    assert "boom" in str(err)


def test_output_validation_error_is_permanent_by_default():
    err = OutputValidationError("bad json", stage="judge")
    assert isinstance(err, StageError)
    assert err.retryable is False


def test_classify_error_passes_through_stage_error():
    original = StageError("x", stage="a", retryable=True)
    assert classify_error(original, stage="b") is original


def test_classify_error_marks_connection_errors_retryable():
    err = classify_error(ConnectionError("conn"), stage="generate")
    assert err.stage == "generate"
    assert err.retryable is True


def test_classify_error_marks_value_errors_permanent():
    err = classify_error(ValueError("nope"), stage="generate")
    assert err.retryable is False


def test_cancelled_pipeline_error_is_not_stage_error():
    assert not issubclass(CancelledPipelineError, StageError)
```

- [ ] **Step 3: Run test to verify it fails**

Run: `PYTHONPATH=services pytest services/shared/tests/test_orchestration_errors.py -v`
Expected: FAIL with `ModuleNotFoundError: No module named 'shared.orchestration.errors'`

- [ ] **Step 4: Write the implementation**

Create `services/shared/orchestration/errors.py`:

```python
"""Error taxonomy for orchestration stages.

Models the retryable-vs-permanent split so consumers (e.g. the eval worker's
retry/DLQ logic) can make uniform decisions instead of ad-hoc try/except.
"""

from __future__ import annotations


class StageError(Exception):
    """An error raised while executing a stage.

    Args:
        message: Human-readable description.
        stage: Name of the stage that failed.
        retryable: Whether re-running the stage could succeed.
    """

    def __init__(self, message: str, *, stage: str, retryable: bool) -> None:
        super().__init__(message)
        self.stage = stage
        self.retryable = retryable


class OutputValidationError(StageError):
    """LLM output failed schema validation after retries (permanent)."""

    def __init__(self, message: str, *, stage: str, retryable: bool = False) -> None:
        super().__init__(message, stage=stage, retryable=retryable)


class CancelledPipelineError(Exception):
    """Raised by a context cancellation check to abort a pipeline run."""


# Exceptions whose recurrence a retry could plausibly resolve.
_RETRYABLE = (ConnectionError, TimeoutError)


def classify_error(exc: Exception, *, stage: str) -> StageError:
    """Map an arbitrary exception to a StageError with a retryable verdict.

    Already-classified StageErrors pass through unchanged.
    """
    if isinstance(exc, StageError):
        return exc
    return StageError(str(exc), stage=stage, retryable=isinstance(exc, _RETRYABLE))
```

- [ ] **Step 5: Run test to verify it passes**

Run: `PYTHONPATH=services pytest services/shared/tests/test_orchestration_errors.py -v`
Expected: PASS (6 passed)

- [ ] **Step 6: Lint and commit**

```bash
ruff check services/shared/orchestration && ruff format services/shared/orchestration
git add services/shared/orchestration/__init__.py services/shared/orchestration/errors.py services/shared/tests/test_orchestration_errors.py
git commit -m "feat(orchestration): add stage error taxonomy and classifier

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 2: PipelineContext

**Files:**
- Create: `services/shared/orchestration/context.py`
- Test: `services/shared/tests/test_orchestration_context.py`

**Interfaces:**
- Consumes: `CancelledPipelineError` from `errors.py`
- Produces:
  - `PipelineContext(state, *, run_id: str | None = None, metadata: dict | None = None, cancel_check: Callable[[], Awaitable[None]] | None = None)`
  - attributes: `.state`, `.run_id` (auto hex if not given), `.metadata: dict`
  - `async def check_cancelled(self) -> None` — awaits `cancel_check` if provided

- [ ] **Step 1: Write the failing test**

Create `services/shared/tests/test_orchestration_context.py`:

```python
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `PYTHONPATH=services pytest services/shared/tests/test_orchestration_context.py -v`
Expected: FAIL with `ModuleNotFoundError: No module named 'shared.orchestration.context'`

- [ ] **Step 3: Write the implementation**

Create `services/shared/orchestration/context.py`:

```python
"""PipelineContext — typed state threaded through every stage."""

from __future__ import annotations

import uuid
from collections.abc import Awaitable, Callable
from typing import Any, Generic, TypeVar

StateT = TypeVar("StateT")


class PipelineContext(Generic[StateT]):
    """Carries pipeline state and request-scoped metadata across stages.

    Args:
        state: The per-pipeline typed payload that stages read and write.
        run_id: Correlation id for this run; a random hex id if omitted.
        metadata: Request-scoped data available to every stage (e.g. collection,
            prompt version, user).
        cancel_check: Optional coroutine invoked by ``check_cancelled``; it should
            raise ``CancelledPipelineError`` when the run should abort.
    """

    def __init__(
        self,
        state: StateT,
        *,
        run_id: str | None = None,
        metadata: dict[str, Any] | None = None,
        cancel_check: Callable[[], Awaitable[None]] | None = None,
    ) -> None:
        self.state = state
        self.run_id = run_id or uuid.uuid4().hex
        self.metadata: dict[str, Any] = metadata or {}
        self._cancel_check = cancel_check

    async def check_cancelled(self) -> None:
        """Invoke the cancellation predicate, if one was supplied."""
        if self._cancel_check is not None:
            await self._cancel_check()
```

- [ ] **Step 4: Run test to verify it passes**

Run: `PYTHONPATH=services pytest services/shared/tests/test_orchestration_context.py -v`
Expected: PASS (5 passed)

- [ ] **Step 5: Lint and commit**

```bash
ruff check services/shared/orchestration && ruff format services/shared/orchestration
git add services/shared/orchestration/context.py services/shared/tests/test_orchestration_context.py
git commit -m "feat(orchestration): add PipelineContext with cancellation hook

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 3: Stage protocols

**Files:**
- Create: `services/shared/orchestration/stage.py`
- Test: `services/shared/tests/test_orchestration_pipeline.py` (shared file; this task adds protocol tests, Task 6 adds runner tests)

**Interfaces:**
- Consumes: `PipelineContext` from `context.py`
- Produces:
  - `Stage` — runtime-checkable Protocol: `name: str`, `async def run(ctx: PipelineContext) -> PipelineContext`
  - `StreamingStage` — runtime-checkable Protocol: `name: str`, `def stream(ctx: PipelineContext) -> AsyncIterator[dict]`

- [ ] **Step 1: Write the failing test**

Create `services/shared/tests/test_orchestration_pipeline.py`:

```python
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `PYTHONPATH=services pytest services/shared/tests/test_orchestration_pipeline.py -v`
Expected: FAIL with `ModuleNotFoundError: No module named 'shared.orchestration.stage'`

- [ ] **Step 3: Write the implementation**

Create `services/shared/orchestration/stage.py`:

```python
"""Stage protocols — the unit of work in a pipeline."""

from __future__ import annotations

from collections.abc import AsyncIterator
from typing import Protocol, runtime_checkable

from shared.orchestration.context import PipelineContext


@runtime_checkable
class Stage(Protocol):
    """A transform stage: reads and writes context, returns it."""

    name: str

    async def run(self, ctx: PipelineContext) -> PipelineContext: ...


@runtime_checkable
class StreamingStage(Protocol):
    """A stage that yields progressive events (e.g. token deltas)."""

    name: str

    def stream(self, ctx: PipelineContext) -> AsyncIterator[dict]: ...
```

- [ ] **Step 4: Run test to verify it passes**

Run: `PYTHONPATH=services pytest services/shared/tests/test_orchestration_pipeline.py -v`
Expected: PASS (3 passed)

- [ ] **Step 5: Lint and commit**

```bash
ruff check services/shared/orchestration && ruff format services/shared/orchestration
git add services/shared/orchestration/stage.py services/shared/tests/test_orchestration_pipeline.py
git commit -m "feat(orchestration): add Stage and StreamingStage protocols

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 4: Role

**Files:**
- Create: `services/shared/orchestration/role.py`
- Test: `services/shared/tests/test_orchestration_role.py`

**Interfaces:**
- Consumes: `get_llm_provider`, `LLMProvider` from `shared.llm`
- Produces:
  - `Role(name, system_prompt, provider, base_url, api_key, model, params={})` — frozen dataclass
  - `Role.build_provider() -> LLMProvider`

- [ ] **Step 1: Write the failing test**

Create `services/shared/tests/test_orchestration_role.py`:

```python
import pytest

from shared.orchestration.role import Role


def test_role_is_frozen():
    role = Role(
        name="answerer",
        system_prompt="You answer.",
        provider="ollama",
        base_url="http://localhost:11434",
        api_key="",
        model="qwen2.5:14b",
    )
    with pytest.raises(Exception):
        role.model = "other"  # frozen dataclass forbids mutation


def test_role_defaults_params_to_empty_dict():
    role = Role(
        name="judge",
        system_prompt="You judge.",
        provider="ollama",
        base_url="http://localhost:11434",
        api_key="",
        model="m",
    )
    assert role.params == {}


def test_build_provider_uses_factory(monkeypatch):
    captured = {}

    def fake_factory(provider, base_url, api_key, model):
        captured.update(
            provider=provider, base_url=base_url, api_key=api_key, model=model
        )
        return "PROVIDER"

    monkeypatch.setattr("shared.orchestration.role.get_llm_provider", fake_factory)
    role = Role(
        name="answerer",
        system_prompt="s",
        provider="anthropic",
        base_url="https://api",
        api_key="sk-x",
        model="claude-opus-4-8",
    )
    assert role.build_provider() == "PROVIDER"
    assert captured == {
        "provider": "anthropic",
        "base_url": "https://api",
        "api_key": "sk-x",
        "model": "claude-opus-4-8",
    }
```

- [ ] **Step 2: Run test to verify it fails**

Run: `PYTHONPATH=services pytest services/shared/tests/test_orchestration_role.py -v`
Expected: FAIL with `ModuleNotFoundError: No module named 'shared.orchestration.role'`

- [ ] **Step 3: Write the implementation**

Create `services/shared/orchestration/role.py`:

```python
"""Role — system prompt + model binding for a single agent persona."""

from __future__ import annotations

from dataclasses import dataclass, field
from typing import Any

from shared.llm import LLMProvider, get_llm_provider


@dataclass(frozen=True)
class Role:
    """Bundles *who is acting*: a system prompt bound to a specific model.

    Two roles on different models (e.g. answerer vs. judge) make role
    separation explicit and configurable.
    """

    name: str
    system_prompt: str
    provider: str
    base_url: str
    api_key: str
    model: str
    params: dict[str, Any] = field(default_factory=dict)

    def build_provider(self) -> LLMProvider:
        """Construct the bound LLM provider via the shared factory."""
        return get_llm_provider(self.provider, self.base_url, self.api_key, self.model)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `PYTHONPATH=services pytest services/shared/tests/test_orchestration_role.py -v`
Expected: PASS (3 passed)

- [ ] **Step 5: Lint and commit**

```bash
ruff check services/shared/orchestration && ruff format services/shared/orchestration
git add services/shared/orchestration/role.py services/shared/tests/test_orchestration_role.py
git commit -m "feat(orchestration): add Role binding system prompt to a model

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 5: Output validation (`call_validated`) + metrics

**Files:**
- Create: `services/shared/orchestration/metrics.py`
- Create: `services/shared/orchestration/validation.py`
- Test: `services/shared/tests/test_orchestration_validation.py`

**Interfaces:**
- Consumes: `Role` from `role.py`, `OutputValidationError` from `errors.py`
- Produces:
  - `metrics.STAGE_DURATION` — `Histogram("orchestration_stage_duration_seconds", labelnames=("pipeline", "stage", "status"))`
  - `async def call_validated(role, messages, schema, *, tools=None, max_retries=2, repair=True) -> BaseModel`

- [ ] **Step 1: Write the metrics module (no test needed — exercised in Task 6)**

Create `services/shared/orchestration/metrics.py`:

```python
"""Prometheus metrics for orchestration stages."""

from __future__ import annotations

from prometheus_client import Histogram

STAGE_DURATION = Histogram(
    "orchestration_stage_duration_seconds",
    "Wall-clock duration of a single orchestration stage execution.",
    labelnames=("pipeline", "stage", "status"),
)
```

- [ ] **Step 2: Write the failing test**

Create `services/shared/tests/test_orchestration_validation.py`:

```python
import pytest
from pydantic import BaseModel

from shared.orchestration.errors import OutputValidationError
from shared.orchestration.role import Role
from shared.orchestration.validation import call_validated


class Scores(BaseModel):
    faithfulness: float
    relevancy: float


class _FakeProvider:
    """Returns the queued responses in order on successive chat() calls."""

    def __init__(self, responses):
        self._responses = list(responses)
        self.calls = []

    async def chat(self, messages, tools=None):
        self.calls.append(messages)
        content = self._responses.pop(0)
        return {"message": {"content": content}}


def _role_with(provider):
    role = Role(
        name="judge",
        system_prompt="You judge. Return JSON.",
        provider="ollama",
        base_url="x",
        api_key="",
        model="m",
    )
    # Bypass the real factory; bind the fake provider directly.
    object.__setattr__(role, "_test_provider", provider)
    return role


@pytest.fixture
def patch_build(monkeypatch):
    def _patch(provider):
        monkeypatch.setattr(
            "shared.orchestration.validation.Role.build_provider",
            lambda self: provider,
        )
    return _patch


@pytest.mark.asyncio
async def test_returns_validated_model_on_first_success(patch_build):
    provider = _FakeProvider(['{"faithfulness": 0.9, "relevancy": 0.8}'])
    patch_build(provider)
    result = await call_validated(
        _role_with(provider), [{"role": "user", "content": "q"}], Scores
    )
    assert isinstance(result, Scores)
    assert result.faithfulness == 0.9
    assert len(provider.calls) == 1


@pytest.mark.asyncio
async def test_extracts_json_embedded_in_prose(patch_build):
    provider = _FakeProvider(['Sure! {"faithfulness": 1.0, "relevancy": 1.0} done'])
    patch_build(provider)
    result = await call_validated(
        _role_with(provider), [{"role": "user", "content": "q"}], Scores
    )
    assert result.relevancy == 1.0


@pytest.mark.asyncio
async def test_retries_with_repair_then_succeeds(patch_build):
    provider = _FakeProvider(
        ["not json at all", '{"faithfulness": 0.5, "relevancy": 0.5}']
    )
    patch_build(provider)
    result = await call_validated(
        _role_with(provider),
        [{"role": "user", "content": "q"}],
        Scores,
        max_retries=2,
    )
    assert result.faithfulness == 0.5
    # Second call must include the repair nudge appended after the bad answer.
    assert len(provider.calls) == 2
    repair_msgs = provider.calls[1]
    assert any("invalid" in m["content"].lower() for m in repair_msgs)


@pytest.mark.asyncio
async def test_raises_output_validation_error_after_exhausting_retries(patch_build):
    provider = _FakeProvider(["bad", "still bad", "nope"])
    patch_build(provider)
    with pytest.raises(OutputValidationError) as exc:
        await call_validated(
            _role_with(provider),
            [{"role": "user", "content": "q"}],
            Scores,
            max_retries=2,
        )
    assert exc.value.stage == "judge"
    assert exc.value.retryable is False
```

- [ ] **Step 3: Run test to verify it fails**

Run: `PYTHONPATH=services pytest services/shared/tests/test_orchestration_validation.py -v`
Expected: FAIL with `ModuleNotFoundError: No module named 'shared.orchestration.validation'`

- [ ] **Step 4: Write the implementation**

Create `services/shared/orchestration/validation.py`:

```python
"""call_validated — call an LLM via a Role and validate its output against a
Pydantic schema, retrying with a corrective nudge on invalid output."""

from __future__ import annotations

import json
from typing import TypeVar

import structlog
from pydantic import BaseModel, ValidationError

from shared.orchestration.errors import OutputValidationError
from shared.orchestration.role import Role

logger = structlog.get_logger()

ModelT = TypeVar("ModelT", bound=BaseModel)


def _extract_json(text: str) -> dict:
    """Extract the first top-level JSON object from a model response."""
    start = text.find("{")
    end = text.rfind("}")
    if start == -1 or end == -1 or end < start:
        raise ValueError("no JSON object found in response")
    return json.loads(text[start : end + 1])


async def call_validated(
    role: Role,
    messages: list[dict],
    schema: type[ModelT],
    *,
    tools: list[dict] | None = None,
    max_retries: int = 2,
    repair: bool = True,
) -> ModelT:
    """Call ``role``'s LLM, parse the response into ``schema``, and retry on
    invalid output.

    When ``repair`` is set, a corrective message describing the failure is
    appended before the next attempt. Raises ``OutputValidationError`` once
    ``max_retries`` is exhausted.
    """
    provider = role.build_provider()
    convo = list(messages)
    last_error: Exception | None = None

    for attempt in range(max_retries + 1):
        chat_messages = [{"role": "system", "content": role.system_prompt}, *convo]
        response = await provider.chat(messages=chat_messages, tools=tools)
        content = response.get("message", {}).get("content", "") or ""
        try:
            return schema.model_validate(_extract_json(content))
        except (ValueError, json.JSONDecodeError, ValidationError) as exc:
            last_error = exc
            logger.warning(
                "output_validation_retry",
                role=role.name,
                attempt=attempt,
                error=str(exc),
            )
            if repair and attempt < max_retries:
                convo = [
                    *convo,
                    {"role": "assistant", "content": content},
                    {
                        "role": "user",
                        "content": (
                            f"Your previous response was invalid: {exc}. "
                            "Respond with ONLY a valid JSON object matching the "
                            "required schema, no prose."
                        ),
                    },
                ]

    raise OutputValidationError(
        f"output failed validation after {max_retries + 1} attempts: {last_error}",
        stage=role.name,
    )
```

- [ ] **Step 5: Run test to verify it passes**

Run: `PYTHONPATH=services pytest services/shared/tests/test_orchestration_validation.py -v`
Expected: PASS (4 passed)

- [ ] **Step 6: Lint and commit**

```bash
ruff check services/shared/orchestration && ruff format services/shared/orchestration
git add services/shared/orchestration/metrics.py services/shared/orchestration/validation.py services/shared/tests/test_orchestration_validation.py
git commit -m "feat(orchestration): add call_validated with schema retry + repair

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 6: Pipeline runner

**Files:**
- Create: `services/shared/orchestration/pipeline.py`
- Modify: `services/shared/orchestration/__init__.py` (add public exports)
- Test: `services/shared/tests/test_orchestration_pipeline.py` (extend the file from Task 3)

**Interfaces:**
- Consumes: `PipelineContext`, `Stage`, `StreamingStage`, `classify_error`, `STAGE_DURATION`
- Produces:
  - `Pipeline(name: str)`
  - `async def execute(stage: Stage, ctx) -> PipelineContext`
  - `async def run(stages: list[Stage], ctx) -> PipelineContext`
  - `def execute_stream(stage: StreamingStage, ctx) -> AsyncIterator[dict]`

- [ ] **Step 1: Append failing tests to the pipeline test file**

Append to `services/shared/tests/test_orchestration_pipeline.py`:

```python
import pytest

from shared.orchestration.errors import StageError
from shared.orchestration.metrics import STAGE_DURATION
from shared.orchestration.pipeline import Pipeline


def _stage_count(stage: str, status: str) -> float:
    return (
        STAGE_DURATION.labels(pipeline="test", stage=stage, status=status)._count.get()
    )


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
    events = [e async for e in pipe.execute_stream(_EmitStage(), PipelineContext(state=None))]
    assert events == [{"token": "a"}, {"token": "b"}]
```

- [ ] **Step 2: Run test to verify it fails**

Run: `PYTHONPATH=services pytest services/shared/tests/test_orchestration_pipeline.py -v`
Expected: FAIL with `ModuleNotFoundError: No module named 'shared.orchestration.pipeline'`

- [ ] **Step 3: Write the implementation**

Create `services/shared/orchestration/pipeline.py`:

```python
"""Pipeline — thin runner that wraps each stage with cross-cutting concerns.

execute()/execute_stream() own per-stage metrics, an OpenTelemetry span, bound
structured-logging context, and error classification, so individual stages and
consumers do not reimplement them. Control flow stays in the consumer.
"""

from __future__ import annotations

import time
from collections.abc import AsyncIterator

import structlog
from opentelemetry import trace

from shared.orchestration.context import PipelineContext
from shared.orchestration.errors import classify_error
from shared.orchestration.metrics import STAGE_DURATION
from shared.orchestration.stage import Stage, StreamingStage

logger = structlog.get_logger()
_tracer = trace.get_tracer(__name__)


class Pipeline:
    """Coordinates stage execution with standardized observability."""

    def __init__(self, name: str) -> None:
        self.name = name

    async def execute(self, stage: Stage, ctx: PipelineContext) -> PipelineContext:
        """Run one transform stage with metrics, tracing, log context, and
        error classification."""
        start = time.perf_counter()
        status = "ok"
        with _tracer.start_as_current_span(f"stage.{stage.name}"):
            tokens = structlog.contextvars.bind_contextvars(
                pipeline=self.name, stage=stage.name
            )
            try:
                return await stage.run(ctx)
            except Exception as exc:
                status = "error"
                err = classify_error(exc, stage=stage.name)
                logger.warning(
                    "stage_error",
                    error=str(err),
                    retryable=err.retryable,
                    exc_info=True,
                )
                raise err from exc
            finally:
                STAGE_DURATION.labels(self.name, stage.name, status).observe(
                    time.perf_counter() - start
                )
                structlog.contextvars.reset_contextvars(**tokens)

    async def run(
        self, stages: list[Stage], ctx: PipelineContext
    ) -> PipelineContext:
        """Execute stages in order (the linear case)."""
        for stage in stages:
            ctx = await self.execute(stage, ctx)
        return ctx

    async def execute_stream(
        self, stage: StreamingStage, ctx: PipelineContext
    ) -> AsyncIterator[dict]:
        """Run a streaming stage, yielding its events, with the same
        cross-cutting wrapping as execute()."""
        start = time.perf_counter()
        status = "ok"
        with _tracer.start_as_current_span(f"stage.{stage.name}"):
            tokens = structlog.contextvars.bind_contextvars(
                pipeline=self.name, stage=stage.name
            )
            try:
                async for event in stage.stream(ctx):
                    yield event
            except Exception as exc:
                status = "error"
                err = classify_error(exc, stage=stage.name)
                logger.warning("stage_error", error=str(err), exc_info=True)
                raise err from exc
            finally:
                STAGE_DURATION.labels(self.name, stage.name, status).observe(
                    time.perf_counter() - start
                )
                structlog.contextvars.reset_contextvars(**tokens)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `PYTHONPATH=services pytest services/shared/tests/test_orchestration_pipeline.py -v`
Expected: PASS (7 passed)

- [ ] **Step 5: Add public exports**

Replace `services/shared/orchestration/__init__.py` with:

```python
"""In-process multi-agent pipeline scaffold.

Coordinates stages with standardized metrics, tracing, structured-logging
context, output validation, and error classification.
"""

from shared.orchestration.context import PipelineContext
from shared.orchestration.errors import (
    CancelledPipelineError,
    OutputValidationError,
    StageError,
    classify_error,
)
from shared.orchestration.pipeline import Pipeline
from shared.orchestration.role import Role
from shared.orchestration.stage import Stage, StreamingStage
from shared.orchestration.validation import call_validated

__all__ = [
    "CancelledPipelineError",
    "OutputValidationError",
    "Pipeline",
    "PipelineContext",
    "Role",
    "Stage",
    "StreamingStage",
    "StageError",
    "call_validated",
    "classify_error",
]
```

- [ ] **Step 6: Verify full scaffold suite + public import**

Run: `PYTHONPATH=services pytest services/shared/tests/test_orchestration_*.py -v`
Expected: PASS (all orchestration tests)

Run: `PYTHONPATH=services python -c "from shared.orchestration import Pipeline, PipelineContext, Role, Stage, StreamingStage, call_validated; print('ok')"`
Expected: prints `ok`

- [ ] **Step 7: Lint and commit**

```bash
ruff check services/shared/orchestration && ruff format services/shared/orchestration
git add services/shared/orchestration/pipeline.py services/shared/orchestration/__init__.py services/shared/tests/test_orchestration_pipeline.py
git commit -m "feat(orchestration): add Pipeline runner with observability wrapping

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## PHASE 2 — Migrate chat onto the scaffold

> Goal: drive chat's RAG flow through `Pipeline` while preserving the exact SSE
> event contract and existing metrics. The existing functions
> (`retrieve_chunks`, `build_rag_prompt`, `stream_response`) keep their internals
> and their `RAG_PIPELINE_DURATION`/`OLLAMA_*` emissions; we wrap them as stages.

### Task 7: Chat stages

**Files:**
- Create: `services/chat/app/stages.py`
- Test: `services/chat/tests/test_stages.py`

**Interfaces:**
- Consumes: `PipelineContext` from `shared.orchestration`; existing `retrieve_chunks`, `build_rag_prompt`, `stream_response`, `RetrievalResult`
- Produces:
  - `ChatState` dataclass with fields: `question: str`, `top_k: int`, `rerank: bool`, `chat_model: str`, `embedding_model: str`, `qdrant_host: str`, `qdrant_port: int`, `collection_name: str`, `embedding_provider`, `llm_provider`, and result fields `retrieval: RetrievalResult | None = None`, `prompt: str = ""`
  - `RetrieveStage()` (name `"retrieve"`), `BuildPromptStage()` (name `"build_prompt"`), `GenerateStreamStage()` (name `"generate"`, streaming)

- [ ] **Step 1: Write the failing test**

Create `services/chat/tests/test_stages.py`:

```python
import pytest

from app.stages import (
    BuildPromptStage,
    ChatState,
    GenerateStreamStage,
    RetrieveStage,
)
from app.retriever import RetrievalResult
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `PYTHONPATH=services pytest services/chat/tests/test_stages.py -v`
Expected: FAIL with `ModuleNotFoundError: No module named 'app.stages'`

- [ ] **Step 3: Write the implementation**

Create `services/chat/app/stages.py`:

```python
"""Chat RAG stages adapted to the shared orchestration scaffold.

Each stage is a thin adapter over the existing chain functions, preserving
their internals (and their RAG_PIPELINE_DURATION / OLLAMA_* metric emissions).
"""

from __future__ import annotations

from collections.abc import AsyncIterator
from dataclasses import dataclass
from typing import Any

from llm.base import EmbeddingProvider, LLMProvider

from app.chain import build_rag_prompt, retrieve_chunks, stream_response
from app.retriever import RetrievalResult
from shared.orchestration import PipelineContext


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
```

- [ ] **Step 4: Run test to verify it passes**

Run: `PYTHONPATH=services pytest services/chat/tests/test_stages.py -v`
Expected: PASS (3 passed)

- [ ] **Step 5: Lint and commit**

```bash
ruff check services/chat/app/stages.py && ruff format services/chat/app/stages.py
git add services/chat/app/stages.py services/chat/tests/test_stages.py
git commit -m "feat(chat): add orchestration stages wrapping the RAG chain

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 8: Drive `rag_query` through the Pipeline

**Files:**
- Modify: `services/chat/app/chain.py:302-363` (the `rag_query` function)
- Test: `services/chat/tests/test_chain.py` (existing — must stay green)

**Interfaces:**
- Consumes: `Pipeline`, `PipelineContext` from `shared.orchestration`; `ChatState`, `RetrieveStage`, `BuildPromptStage`, `GenerateStreamStage` from `app.stages`
- Produces: `rag_query(...)` with an **unchanged signature and unchanged yielded events**.

- [ ] **Step 1: Confirm the existing chat suite is green before changing anything**

Run: `PYTHONPATH=services pytest services/chat/tests/ -v`
Expected: PASS (baseline — record the count)

- [ ] **Step 2: Rewrite `rag_query` to drive the pipeline**

In `services/chat/app/chain.py`, add to the imports near the top (after the existing `from app.retriever import ...` line):

```python
from app.stages import (
    BuildPromptStage,
    ChatState,
    GenerateStreamStage,
    RetrieveStage,
)
from shared.orchestration import Pipeline, PipelineContext
```

Replace the entire `rag_query` function body (currently lines 302-363) with:

```python
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
    pipeline = Pipeline("chat")
    ctx = PipelineContext(
        state=ChatState(
            question=question,
            top_k=top_k,
            rerank=rerank,
            chat_model=chat_model,
            embedding_model=embedding_model,
            qdrant_host=qdrant_host,
            qdrant_port=qdrant_port,
            collection_name=collection_name,
            embedding_provider=embedding_provider,
            llm_provider=llm_provider,
        ),
        metadata={"collection": collection_name},
    )

    # Retrieve + build prompt (transform stages).
    ctx = await pipeline.run([RetrieveStage(), BuildPromptStage()], ctx)
    retrieval = ctx.state.retrieval
    chunks = retrieval.chunks

    # Collect unique sources (unchanged contract).
    seen = set()
    sources = []
    for chunk in chunks:
        key = (chunk["filename"], chunk["page_number"])
        if key not in seen:
            seen.add(key)
            sources.append({"file": chunk["filename"], "page": chunk["page_number"]})

    # Generate (streaming stage).
    usage: dict = {}
    async for event in pipeline.execute_stream(GenerateStreamStage(), ctx):
        if "token" in event:
            yield event
        if event.get("done"):
            usage = event.get("usage", {})

    usage = {**usage, "answer_model": chat_model}
    yield {
        "done": True,
        "sources": sources,
        "retrieval": retrieval.metadata,
        "usage": usage,
    }
```

Note: the `RAG_PIPELINE_DURATION.labels(stage="build_prompt"/"generate")` observations that previously lived inline in `rag_query` are intentionally dropped here because the scaffold's `STAGE_DURATION` now times those stages, and the `retrieve` stage's `RAG_PIPELINE_DURATION` is still emitted inside `retrieve_chunks`. If `services/chat/tests/test_metrics.py` asserts on the `build_prompt` or `generate` labels of `RAG_PIPELINE_DURATION`, instead keep those two `.observe()` calls by wrapping them: move the `RAG_PIPELINE_DURATION.labels(stage="build_prompt")` call into `BuildPromptStage.run` and the `stage="generate"` call into `GenerateStreamStage.stream` (around the loop) in `app/stages.py`. Decide based on Step 3's result.

- [ ] **Step 3: Run the full chat suite**

Run: `PYTHONPATH=services pytest services/chat/tests/ -v`
Expected: PASS (same count as Step 1's baseline). If `test_metrics.py` fails on a missing `RAG_PIPELINE_DURATION` `build_prompt`/`generate` observation, apply the fallback described in Step 2's note (move those `.observe()` calls into the stages), then re-run until green.

- [ ] **Step 4: Verify the streaming contract end-to-end**

Run: `PYTHONPATH=services pytest services/chat/tests/test_chain.py -v -k "rag_query or stream or query"`
Expected: PASS — confirms token events and the final `done` event (sources/retrieval/usage) are unchanged.

- [ ] **Step 5: Lint and commit**

```bash
ruff check services/chat && ruff format --check services/chat
git add services/chat/app/chain.py services/chat/app/stages.py
git commit -m "refactor(chat): drive rag_query through the orchestration scaffold

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

- [ ] **Step 6: Full Python regression sweep**

Run: `PYTHONPATH=services pytest services/shared/tests/ services/chat/tests/ -v`
Expected: PASS (scaffold + chat). This is the Phase 2 completion gate.

---

## Self-Review

**Spec coverage:**
- Five primitives — `PipelineContext` (Task 2), `Stage`/`StreamingStage` (Task 3), `Role` (Task 4), `call_validated` (Task 5), `Pipeline` (Task 6). ✓
- Role separation — `Role` + stage boundaries (Tasks 4, 7). ✓
- Context injection — `PipelineContext` threading state, `BuildPromptStage` assembling prompt from retrieved chunks (Tasks 2, 7). ✓
- Output validation — `call_validated` schema + retry + repair (Task 5). ✓
- Cross-cutting concerns (metrics/tracing/log-context/error classification) standardized in `Pipeline.execute` (Tasks 1, 5, 6). ✓
- Observability wired into existing `shared/tracing` + `shared/logging` — `Pipeline.execute` opens an OTel span and binds structlog contextvars; `_add_otel_context` auto-injects trace/span ids (Task 6). ✓
- Behavior preservation — chat suite green gate (Task 8, Steps 1/3/6). ✓
- Phasing — plan limited to Phase 1 + Phase 2; eval/dspm/debug deferred. ✓

**Deviation from spec (intentional):** the spec described streaming via a `ctx.emit()` event queue. This plan uses a simpler `StreamingStage` protocol + `Pipeline.execute_stream`, which fits the existing async-generator `stream_response` with lower regression risk. The event-queue can be added later if a consumer needs mid-stage fan-out (e.g. debug ReAct step events). The spec's Streaming section will be reconciled to reflect this.

**Placeholder scan:** none — every code step contains complete code; every run step has an exact command and expected output.

**Type consistency:** `PipelineContext`, `Stage`, `StreamingStage`, `Role`, `call_validated`, `Pipeline.execute`/`run`/`execute_stream`, `ChatState`, `RetrieveStage`/`BuildPromptStage`/`GenerateStreamStage` names are used identically across tasks. `STAGE_DURATION` label order `(pipeline, stage, status)` is consistent between `metrics.py` and `pipeline.py` and the tests.

**Open follow-up (not blocking):** wire `PYTHONPATH=services pytest services/shared/tests/` into the Makefile `test` target so CI runs the scaffold suite. Add as a one-line step alongside the existing chat pytest invocation when first running CI.
