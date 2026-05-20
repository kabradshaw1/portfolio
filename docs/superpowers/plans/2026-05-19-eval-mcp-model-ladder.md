# Eval MCP Model Ladder Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Enable MCP-launched RAG eval runs to compare local, efficient API, and premium API answer-model tiers with fixed judging and persisted provenance.

**Architecture:** Add a small answer-model override contract to eval, chat, and the Go eval MCP. Eval stores requested model metadata, resolves configured secret references, passes a secured override to chat for internal eval requests, and captures model, retrieval, usage, and latency evidence for each run.

**Tech Stack:** Python/FastAPI eval and chat services, shared Python LLM providers, Go eval MCP service, Pydantic v2, pytest, Go tests, Kubernetes ConfigMaps for non-secret defaults.

---

## File Structure

- Modify `services/eval/app/models.py`: add answer-model override fields to `StartEvaluationRequest`.
- Create `services/eval/app/answer_models.py`: validate provider/model/tier inputs and resolve API key secret references from environment.
- Modify `services/eval/app/rag_client.py`: forward answer model overrides to chat `/chat` calls.
- Modify `services/eval/app/evaluator.py`: carry answer model overrides into dataset building and preserve usage/latency metadata in per-query results.
- Modify `services/eval/app/main.py`: validate overrides before creating a run, pass overrides through the background task, and capture model metadata.
- Modify `services/eval/app/config_capture.py`: include requested answer model, effective answer model, judge model, and usage metadata shell in config capture.
- Modify `services/eval/tests/test_models.py`, `services/eval/tests/test_main.py`, `services/eval/tests/test_evaluator.py`, and add `services/eval/tests/test_answer_models.py`.
- Modify `services/chat/app/main.py`: accept an internal-only `answer_model` override in `ChatRequest`, instantiate an override LLM provider, and return usage metadata in JSON responses.
- Modify `services/chat/app/chain.py`: include token and latency metadata in the final `done` event.
- Modify `services/chat/tests/test_main.py` and `services/chat/tests/test_chain.py`.
- Modify `go/eval-mcp-service/internal/evalapi/client.go`: add answer model fields to `StartEvaluationRequest`.
- Modify `go/eval-mcp-service/internal/evalworkflow/service.go`: add answer model fields to `StartRunInput` and forward them.
- Modify `go/eval-mcp-service/internal/mcpserver/server.go`: expose, validate, and document answer model inputs.
- Modify `go/eval-mcp-service/internal/evalapi/client_test.go`: cover API request serialization.
- Modify `go/eval-mcp-service/internal/evalworkflow/service_test.go`: cover workflow forwarding.
- Modify `go/eval-mcp-service/internal/mcpserver/server_test.go`: cover schema, validation, forwarding, and workflow copy.
- Modify `go/eval-mcp-service/README.md`: document model-ladder usage and fixed-judge rule.

## Task 0: Prepare Feature Worktree

**Files:**
- No code files.

- [ ] **Step 1: Create an isolated feature worktree**

Run from the original repo root:

```bash
git branch --show-current
git worktree add .codex/worktrees/eval-mcp-model-ladder -b feat/eval-mcp-model-ladder
```

Expected: worktree created at `.codex/worktrees/eval-mcp-model-ladder`.

- [ ] **Step 2: Confirm all implementation work is inside the worktree**

Run:

```bash
cd .codex/worktrees/eval-mcp-model-ladder
pwd
git branch --show-current
git rev-parse --show-toplevel
git status --short
```

Expected:

```text
/Users/kylebradshaw/repos/gen_ai_engineer/.codex/worktrees/eval-mcp-model-ladder
feat/eval-mcp-model-ladder
/Users/kylebradshaw/repos/gen_ai_engineer/.codex/worktrees/eval-mcp-model-ladder
```

The status may show copied local changes only if they are already present in the new branch. Do not edit task files from the original repo root after this step.

## Task 1: Add Eval Answer Model Contract

**Files:**
- Modify: `services/eval/app/models.py`
- Create: `services/eval/app/answer_models.py`
- Test: `services/eval/tests/test_models.py`
- Test: `services/eval/tests/test_answer_models.py`

- [ ] **Step 1: Write failing model validation tests**

Add these tests to `services/eval/tests/test_models.py`:

```python
def test_start_evaluation_accepts_answer_model_override():
    req = StartEvaluationRequest(
        dataset_id="ds-1",
        answer_tier="efficient",
        answer_provider="openai",
        answer_base_url="https://api.openai.com/v1",
        answer_model="gpt-5.4-mini",
        answer_api_key_secret="OPENAI_API_KEY",
    )

    assert req.answer_tier == "efficient"
    assert req.answer_provider == "openai"
    assert req.answer_base_url == "https://api.openai.com/v1"
    assert req.answer_model == "gpt-5.4-mini"
    assert req.answer_api_key_secret == "OPENAI_API_KEY"


def test_start_evaluation_rejects_unknown_answer_provider():
    with pytest.raises(ValueError, match="answer_provider"):
        StartEvaluationRequest(
            dataset_id="ds-1",
            answer_provider="unknown",
            answer_model="model",
        )


def test_start_evaluation_rejects_raw_answer_secret_value():
    with pytest.raises(ValueError, match="answer_api_key_secret"):
        StartEvaluationRequest(
            dataset_id="ds-1",
            answer_provider="openai",
            answer_model="gpt-5.4-mini",
            answer_api_key_secret="sk-test-secret-value",
        )
```

Create `services/eval/tests/test_answer_models.py`:

```python
import pytest

from app.answer_models import AnswerModelOverride, resolve_answer_model_override


def test_resolve_answer_model_override_uses_secret_reference(monkeypatch):
    monkeypatch.setenv("OPENAI_API_KEY", "test-key")

    override = resolve_answer_model_override(
        AnswerModelOverride(
            tier="efficient",
            provider="openai",
            base_url="https://api.openai.com/v1",
            model="gpt-5.4-mini",
            api_key_secret="OPENAI_API_KEY",
        )
    )

    assert override.tier == "efficient"
    assert override.provider == "openai"
    assert override.base_url == "https://api.openai.com/v1"
    assert override.model == "gpt-5.4-mini"
    assert override.api_key == "test-key"
    assert override.safe_dict() == {
        "tier": "efficient",
        "provider": "openai",
        "base_url": "https://api.openai.com/v1",
        "model": "gpt-5.4-mini",
        "api_key_secret": "OPENAI_API_KEY",
    }


def test_resolve_answer_model_override_allows_ollama_without_secret():
    override = resolve_answer_model_override(
        AnswerModelOverride(
            tier="local",
            provider="ollama",
            base_url="http://ollama:11434",
            model="qwen2.5:14b",
            api_key_secret=None,
        )
    )

    assert override.api_key == ""


def test_resolve_answer_model_override_rejects_missing_secret(monkeypatch):
    monkeypatch.delenv("OPENAI_API_KEY", raising=False)

    with pytest.raises(ValueError, match="OPENAI_API_KEY"):
        resolve_answer_model_override(
            AnswerModelOverride(
                tier="efficient",
                provider="openai",
                base_url="https://api.openai.com/v1",
                model="gpt-5.4-mini",
                api_key_secret="OPENAI_API_KEY",
            )
        )
```

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
PYTHONPATH=services/eval:services pytest \
  services/eval/tests/test_models.py::test_start_evaluation_accepts_answer_model_override \
  services/eval/tests/test_models.py::test_start_evaluation_rejects_unknown_answer_provider \
  services/eval/tests/test_models.py::test_start_evaluation_rejects_raw_answer_secret_value \
  services/eval/tests/test_answer_models.py -q
```

Expected: failures because `answer_*` fields and `app.answer_models` do not exist.

- [ ] **Step 3: Implement the contract**

In `services/eval/app/models.py`, add imports:

```python
from pydantic import BaseModel, ConfigDict, Field, field_validator, model_validator
```

Update `StartEvaluationRequest`:

```python
class StartEvaluationRequest(BaseModel):
    dataset_id: str
    collection: str | None = Field(default=None, pattern=r"^[a-zA-Z0-9_-]{1,100}$")
    notes: str | None = Field(default=None, max_length=500)
    baseline_eval_id: str | None = None
    rerank: bool = False
    retrieval_config: RetrievalConfig | None = None
    experiment_id: str | None = None
    experiment_label: str | None = Field(
        default=None, min_length=1, max_length=100, pattern=r"^[a-zA-Z0-9_-]+$"
    )
    answer_tier: str | None = Field(
        default=None, pattern=r"^[a-zA-Z0-9_-]{1,50}$"
    )
    answer_provider: str | None = None
    answer_base_url: str | None = Field(default=None, max_length=300)
    answer_model: str | None = Field(default=None, max_length=100)
    answer_api_key_secret: str | None = Field(
        default=None, pattern=r"^[A-Z][A-Z0-9_]{1,100}$"
    )

    @field_validator("answer_provider")
    @classmethod
    def validate_answer_provider(cls, value: str | None) -> str | None:
        if value is None:
            return None
        if value not in {"ollama", "openai", "anthropic"}:
            raise ValueError("answer_provider must be 'ollama', 'openai', or 'anthropic'")
        return value

    @field_validator("answer_api_key_secret")
    @classmethod
    def reject_secret_values(cls, value: str | None) -> str | None:
        if value is None:
            return None
        lowered = value.lower()
        if lowered.startswith(("sk-", "sk_", "bearer ", "api-")):
            raise ValueError("answer_api_key_secret must name an environment variable")
        return value

    @model_validator(mode="after")
    def validate_answer_override(self) -> "StartEvaluationRequest":
        fields = [
            self.answer_tier,
            self.answer_provider,
            self.answer_base_url,
            self.answer_model,
            self.answer_api_key_secret,
        ]
        if any(value is not None for value in fields):
            if not self.answer_provider:
                raise ValueError("answer_provider is required with answer override")
            if not self.answer_model:
                raise ValueError("answer_model is required with answer override")
        return self
```

Create `services/eval/app/answer_models.py`:

```python
from __future__ import annotations

import os
from dataclasses import dataclass


@dataclass(frozen=True)
class AnswerModelOverride:
    tier: str | None
    provider: str
    base_url: str | None
    model: str
    api_key_secret: str | None


@dataclass(frozen=True)
class ResolvedAnswerModelOverride:
    tier: str | None
    provider: str
    base_url: str | None
    model: str
    api_key_secret: str | None
    api_key: str

    def safe_dict(self) -> dict:
        return {
            "tier": self.tier,
            "provider": self.provider,
            "base_url": self.base_url,
            "model": self.model,
            "api_key_secret": self.api_key_secret,
        }


def resolve_answer_model_override(
    override: AnswerModelOverride | None,
) -> ResolvedAnswerModelOverride | None:
    if override is None:
        return None
    api_key = ""
    if override.api_key_secret:
        api_key = os.getenv(override.api_key_secret, "")
        if not api_key:
            raise ValueError(
                f"answer model secret {override.api_key_secret} is not configured"
            )
    elif override.provider in {"openai", "anthropic"}:
        raise ValueError(
            f"answer_api_key_secret is required when answer_provider is {override.provider}"
        )
    return ResolvedAnswerModelOverride(
        tier=override.tier,
        provider=override.provider,
        base_url=override.base_url,
        model=override.model,
        api_key_secret=override.api_key_secret,
        api_key=api_key,
    )
```

- [ ] **Step 4: Run tests to verify they pass**

Run:

```bash
PYTHONPATH=services/eval:services pytest \
  services/eval/tests/test_models.py::test_start_evaluation_accepts_answer_model_override \
  services/eval/tests/test_models.py::test_start_evaluation_rejects_unknown_answer_provider \
  services/eval/tests/test_models.py::test_start_evaluation_rejects_raw_answer_secret_value \
  services/eval/tests/test_answer_models.py -q
```

Expected: all selected tests pass.

- [ ] **Step 5: Commit**

Run:

```bash
git add services/eval/app/models.py services/eval/app/answer_models.py \
  services/eval/tests/test_models.py services/eval/tests/test_answer_models.py
git commit -m "Add eval answer model override contract"
```

## Task 2: Add Internal Chat Answer Overrides

**Files:**
- Modify: `services/chat/app/main.py`
- Modify: `services/chat/app/chain.py`
- Test: `services/chat/tests/test_main.py`
- Test: `services/chat/tests/test_chain.py`

- [ ] **Step 1: Write failing chat route tests**

Add to `services/chat/tests/test_main.py`:

```python
@patch("app.main.get_llm_provider")
@patch("app.main.rag_query")
def test_internal_eval_chat_can_override_answer_model(
    mock_rag_query, mock_get_llm_provider, configured_chat_limits
):
    captured = {}
    provider = object()
    mock_get_llm_provider.return_value = provider

    async def fake(**kwargs):
        captured.update(kwargs)
        yield {"done": True, "sources": [], "retrieval": {}, "usage": {}}

    mock_rag_query.side_effect = fake

    response = client.post(
        "/chat",
        json={
            "question": "hi",
            "answer_model": {
                "provider": "openai",
                "base_url": "https://api.openai.com/v1",
                "model": "gpt-5.4-mini",
                "api_key": "test-key",
                "tier": "efficient",
            },
        },
        headers={
            "Accept": "application/json",
            "X-RAG-Internal-Token": "test-internal-token",
        },
    )

    assert response.status_code == 200
    mock_get_llm_provider.assert_called_once_with(
        provider="openai",
        base_url="https://api.openai.com/v1",
        api_key="test-key",
        model="gpt-5.4-mini",
    )
    assert captured["llm_provider"] is provider
    assert captured["chat_model"] == "gpt-5.4-mini"


@patch("app.main.rag_query")
def test_public_chat_rejects_answer_model_override(mock_rag_query):
    response = client.post(
        "/chat",
        json={
            "question": "hi",
            "answer_model": {
                "provider": "openai",
                "model": "gpt-5.4-mini",
                "api_key": "test-key",
            },
        },
        headers={"Accept": "application/json"},
    )

    assert response.status_code == 403
    mock_rag_query.assert_not_called()
```

- [ ] **Step 2: Write failing usage metadata chain test**

Add to `services/chat/tests/test_chain.py`:

```python
@pytest.mark.asyncio
async def test_rag_query_final_event_includes_answer_usage(monkeypatch):
    monkeypatch.setattr("app.chain.retrieve_chunks", AsyncMock(return_value=hybrid_result()))

    async def fake_stream_response(**kwargs):
        yield {"token": "hello"}
        yield {
            "done": True,
            "usage": {
                "prompt_tokens": 3,
                "completion_tokens": 2,
                "generation_seconds": 0.01,
            },
        }

    monkeypatch.setattr("app.chain.stream_response", fake_stream_response)

    events = [
        event
        async for event in rag_query(
            question="q",
            llm_provider=object(),
            embedding_provider=object(),
            chat_model="model-a",
            embedding_model="embed",
            qdrant_host="qdrant",
            qdrant_port=6333,
            collection_name="documents",
        )
    ]

    assert events[-1]["usage"]["answer_model"] == "model-a"
    assert events[-1]["usage"]["prompt_tokens"] == 3
    assert events[-1]["usage"]["completion_tokens"] == 2
```

- [ ] **Step 3: Run tests to verify they fail**

Run:

```bash
PYTHONPATH=services/chat:services pytest \
  services/chat/tests/test_main.py::test_internal_eval_chat_can_override_answer_model \
  services/chat/tests/test_main.py::test_public_chat_rejects_answer_model_override \
  services/chat/tests/test_chain.py::test_rag_query_final_event_includes_answer_usage -q
```

Expected: failures because `answer_model` and final usage metadata are not implemented.

- [ ] **Step 4: Implement chat override and usage metadata**

In `services/chat/app/main.py`, add this model near `RetrievalConfig`:

```python
class AnswerModelOverride(BaseModel):
    model_config = ConfigDict(extra="forbid")

    provider: str = Field(pattern=r"^(ollama|openai|anthropic)$")
    model: str = Field(min_length=1, max_length=100)
    base_url: str | None = Field(default=None, max_length=300)
    api_key: str = Field(default="", max_length=500)
    tier: str | None = Field(default=None, pattern=r"^[a-zA-Z0-9_-]{1,50}$")
```

Update `ChatRequest`:

```python
class ChatRequest(BaseModel):
    question: str = Field(max_length=2000)
    collection: str | None = Field(default=None, pattern=r"^[a-zA-Z0-9_-]{1,100}$")
    retrieval_config: RetrievalConfig | None = None
    rerank: bool = False
    answer_model: AnswerModelOverride | None = None
```

Add helper functions:

```python
def _is_internal_eval(auth_context: AuthContext) -> bool:
    return auth_context.subject == "internal-eval"


def _answer_provider_for_request(
    body: ChatRequest,
    auth_context: AuthContext,
) -> tuple[object, str, dict | None]:
    if body.answer_model is None:
        return _llm_provider, settings.get_llm_model(), None
    if not _is_internal_eval(auth_context):
        raise HTTPException(
            status_code=403,
            detail="answer_model override is only available to internal eval requests",
        )
    provider = get_llm_provider(
        provider=body.answer_model.provider,
        base_url=body.answer_model.base_url or settings.get_llm_base_url(),
        api_key=body.answer_model.api_key,
        model=body.answer_model.model,
    )
    safe = body.answer_model.model_dump(exclude={"api_key"})
    return provider, body.answer_model.model, safe
```

At the start of `chat`, after `effective_top_k`, add:

```python
    llm_provider, chat_model, answer_model_metadata = _answer_provider_for_request(
        body, auth_context
    )
```

Replace both `rag_query` calls in `chat`:

```python
                llm_provider=llm_provider,
                chat_model=chat_model,
```

When handling JSON events, initialize and return usage:

```python
            usage = {}
            async for event in rag_query(...):
                if "usage" in event:
                    usage = event["usage"]
            if answer_model_metadata is not None:
                usage["answer_model_override"] = answer_model_metadata
            return {
                "answer": "".join(tokens),
                "sources": sources,
                "retrieval": retrieval,
                "usage": usage,
            }
```

In `services/chat/app/chain.py`, update `stream_response` to yield usage:

```python
                yield {
                    "done": True,
                    "usage": {
                        "prompt_tokens": prompt_tokens,
                        "completion_tokens": completion_tokens,
                        "generation_seconds": duration,
                    },
                }
                break
```

In `rag_query`, collect usage:

```python
    usage = {}
    async for event in stream_response(
        prompt=prompt, model=chat_model, provider=llm_provider
    ):
        if "token" in event:
            yield event
        if event.get("done"):
            usage = event.get("usage", {})
```

And update the final event:

```python
    usage = {**usage, "answer_model": chat_model}
    yield {"done": True, "sources": sources, "retrieval": retrieval.metadata, "usage": usage}
```

- [ ] **Step 5: Run tests to verify they pass**

Run:

```bash
PYTHONPATH=services/chat:services pytest \
  services/chat/tests/test_main.py::test_internal_eval_chat_can_override_answer_model \
  services/chat/tests/test_main.py::test_public_chat_rejects_answer_model_override \
  services/chat/tests/test_chain.py::test_rag_query_final_event_includes_answer_usage -q
```

Expected: all selected tests pass.

- [ ] **Step 6: Commit**

Run:

```bash
git add services/chat/app/main.py services/chat/app/chain.py \
  services/chat/tests/test_main.py services/chat/tests/test_chain.py
git commit -m "Allow internal eval answer model overrides"
```

## Task 3: Thread Overrides Through Eval Runs

**Files:**
- Modify: `services/eval/app/rag_client.py`
- Modify: `services/eval/app/evaluator.py`
- Modify: `services/eval/app/main.py`
- Modify: `services/eval/app/config_capture.py`
- Test: `services/eval/tests/test_evaluator.py`
- Test: `services/eval/tests/test_main.py`

- [ ] **Step 1: Write failing eval threading tests**

Add to `services/eval/tests/test_evaluator.py`:

```python
async def test_build_evaluation_dataset_passes_answer_model_override(
    golden_items, mock_search_results, mock_chat_answer
):
    rag_client = AsyncMock()
    rag_client.search.return_value = mock_search_results
    rag_client.ask.return_value = {**mock_chat_answer, "usage": {"prompt_tokens": 4}}
    answer_model = {
        "tier": "efficient",
        "provider": "openai",
        "base_url": "https://api.openai.com/v1",
        "model": "gpt-5.4-mini",
        "api_key": "test-key",
    }

    dataset = await build_evaluation_dataset(
        golden_items,
        rag_client,
        collection="documents",
        answer_model=answer_model,
    )

    assert rag_client.ask.call_args.kwargs["answer_model"] == answer_model
    assert dataset[0]["usage"] == {"prompt_tokens": 4}
```

Add to `services/eval/tests/test_main.py`:

```python
@patch("app.main.resolve_answer_model_override")
@patch("app.main.run_evaluation", new_callable=AsyncMock)
@patch("app.main.capture_run_config", new_callable=AsyncMock)
@patch("app.main.validate_collection_exists", new_callable=AsyncMock)
@patch("app.main.get_db")
def test_start_evaluation_resolves_and_captures_answer_model_override(
    mock_get_db,
    mock_validate_collection,
    mock_capture,
    mock_run_evaluation,
    mock_resolve,
):
    mock_db = AsyncMock()
    mock_db.get_dataset.return_value = {
        "id": "ds-model",
        "name": "test",
        "items": [{"query": "q", "expected_answer": "a", "expected_sources": []}],
        "created_at": "2026-04-16T00:00:00Z",
    }
    mock_db.create_evaluation.return_value = "eval-model"
    mock_get_db.return_value = mock_db
    resolved = MagicMock()
    resolved.safe_dict.return_value = {
        "tier": "efficient",
        "provider": "openai",
        "base_url": "https://api.openai.com/v1",
        "model": "gpt-5.4-mini",
        "api_key_secret": "OPENAI_API_KEY",
    }
    resolved.provider = "openai"
    resolved.base_url = "https://api.openai.com/v1"
    resolved.model = "gpt-5.4-mini"
    resolved.api_key = "test-key"
    mock_resolve.return_value = resolved
    mock_capture.return_value = {"captured_at": "x"}
    mock_run_evaluation.return_value = ({"faithfulness": 0.8}, [])

    response = client.post(
        "/evaluations",
        json={
            "dataset_id": "ds-model",
            "answer_tier": "efficient",
            "answer_provider": "openai",
            "answer_base_url": "https://api.openai.com/v1",
            "answer_model": "gpt-5.4-mini",
            "answer_api_key_secret": "OPENAI_API_KEY",
        },
    )

    assert response.status_code == 202
    assert mock_capture.await_args.kwargs["requested_answer_model"]["model"] == "gpt-5.4-mini"
    assert mock_run_evaluation.await_args.kwargs["answer_model"]["api_key"] == "test-key"
```

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
PYTHONPATH=services/eval:services pytest \
  services/eval/tests/test_evaluator.py::test_build_evaluation_dataset_passes_answer_model_override \
  services/eval/tests/test_main.py::test_start_evaluation_resolves_and_captures_answer_model_override -q
```

Expected: failures because eval does not accept or forward `answer_model`.

- [ ] **Step 3: Implement eval threading**

In `services/eval/app/rag_client.py`, update `ask`:

```python
    async def ask(
        self,
        question: str,
        collection: str | None,
        rerank: bool = False,
        retrieval_config: dict | None = None,
        answer_model: dict | None = None,
    ) -> dict:
        body: dict = {"question": question, "rerank": rerank}
        if collection:
            body["collection"] = collection
        if retrieval_config:
            body["retrieval_config"] = retrieval_config
        if answer_model:
            body["answer_model"] = answer_model
```

In `services/eval/app/evaluator.py`, add `answer_model` parameters to `build_evaluation_dataset` and `run_evaluation`, pass it into `rag_client.ask`, and preserve usage:

```python
async def build_evaluation_dataset(
    items: list[dict],
    rag_client: RAGClient,
    collection: str | None,
    rerank: bool = False,
    top_k: int = 5,
    run_context: EvalRunContext | None = None,
    answer_model: dict | None = None,
) -> list[dict]:
```

```python
            chat_response = await rag_client.ask(
                query,
                collection=collection,
                rerank=rerank,
                retrieval_config={"top_k": top_k},
                answer_model=answer_model,
            )
```

```python
        if "usage" in chat_response:
            row["usage"] = chat_response["usage"]
```

```python
async def run_evaluation(
    ...
    answer_model: dict | None = None,
) -> tuple[dict, list[dict]]:
    raw_dataset = await build_evaluation_dataset(
        ...
        answer_model=answer_model,
    )
```

```python
        if "usage" in row:
            result["usage"] = row["usage"]
```

In `services/eval/app/main.py`, import:

```python
from app.answer_models import AnswerModelOverride, resolve_answer_model_override
```

Before creating the evaluation, resolve:

```python
    requested_answer_model = None
    resolved_answer_model = None
    if body.answer_provider or body.answer_model:
        requested_answer_model = AnswerModelOverride(
            tier=body.answer_tier,
            provider=body.answer_provider or "",
            base_url=body.answer_base_url,
            model=body.answer_model or "",
            api_key_secret=body.answer_api_key_secret,
        )
        try:
            resolved_answer_model = resolve_answer_model_override(requested_answer_model)
        except ValueError as exc:
            raise HTTPException(status_code=400, detail=str(exc)) from exc
```

Pass `resolved_answer_model` into `_run_evaluation_task`. In `_run_evaluation_task`, build:

```python
    answer_model_payload = None
    requested_answer_model = None
    if resolved_answer_model is not None:
        requested_answer_model = resolved_answer_model.safe_dict()
        answer_model_payload = {
            "tier": resolved_answer_model.tier,
            "provider": resolved_answer_model.provider,
            "base_url": resolved_answer_model.base_url,
            "model": resolved_answer_model.model,
            "api_key": resolved_answer_model.api_key,
        }
```

Pass `requested_answer_model` to `capture_run_config` and `answer_model_payload` to `run_evaluation`.

In `services/eval/app/config_capture.py`, update the signature:

```python
async def capture_run_config(
    chat_url: str,
    ingestion_url: str,
    collection: str,
    requested_rerank: bool,
    requested_retrieval_config: dict | None = None,
    requested_answer_model: dict | None = None,
    judge_model: dict | None = None,
) -> dict:
```

Add to `out`:

```python
        "requested_answer_model": requested_answer_model or {},
        "judge_model": judge_model or {},
        "usage": {"available": False},
```

- [ ] **Step 4: Run tests to verify they pass**

Run:

```bash
PYTHONPATH=services/eval:services pytest \
  services/eval/tests/test_evaluator.py::test_build_evaluation_dataset_passes_answer_model_override \
  services/eval/tests/test_main.py::test_start_evaluation_resolves_and_captures_answer_model_override \
  services/eval/tests/test_main.py::test_run_persists_config_snapshot \
  services/eval/tests/test_main.py::test_start_evaluation_passes_retrieval_config_to_background_run -q
```

Expected: all selected tests pass.

- [ ] **Step 5: Commit**

Run:

```bash
git add services/eval/app/rag_client.py services/eval/app/evaluator.py \
  services/eval/app/main.py services/eval/app/config_capture.py \
  services/eval/tests/test_evaluator.py services/eval/tests/test_main.py
git commit -m "Thread answer model overrides through eval runs"
```

## Task 4: Expose Answer Model Overrides In Eval MCP

**Files:**
- Modify: `go/eval-mcp-service/internal/evalapi/client.go`
- Modify: `go/eval-mcp-service/internal/evalworkflow/service.go`
- Modify: `go/eval-mcp-service/internal/mcpserver/server.go`
- Test: `go/eval-mcp-service/internal/evalapi/client_test.go`
- Test: `go/eval-mcp-service/internal/evalworkflow/service_test.go`
- Test: `go/eval-mcp-service/internal/mcpserver/server_test.go`

- [ ] **Step 1: Write failing Go tests**

In `go/eval-mcp-service/internal/evalapi/client_test.go`, extend `TestStartEvaluationSendsOptionalFields` expected request with:

```go
AnswerTier:         "efficient",
AnswerProvider:     "openai",
AnswerBaseURL:      "https://api.openai.com/v1",
AnswerModel:        "gpt-5.4-mini",
AnswerAPIKeySecret: "OPENAI_API_KEY",
```

In `go/eval-mcp-service/internal/mcpserver/server_test.go`, add:

```go
func TestStartEvalRunForwardsAnswerModelOverride(t *testing.T) {
	fake := &fakeEvalService{}
	result, err := startEvalRunHandler(fake)(context.Background(), callReq(map[string]any{
		"dataset_id":            "ds-1",
		"collection":            "documents",
		"answer_tier":           "efficient",
		"answer_provider":       "openai",
		"answer_base_url":       "https://api.openai.com/v1",
		"answer_model":          "gpt-5.4-mini",
		"answer_api_key_secret": "OPENAI_API_KEY",
	}))
	if err != nil {
		t.Fatalf("handler returned transport error: %v", err)
	}
	if result.IsError {
		t.Fatalf("handler returned MCP error: %#v", result)
	}
	if fake.startRunInput.AnswerModel != "gpt-5.4-mini" {
		t.Fatalf("answer model = %#v", fake.startRunInput)
	}
}

func TestStartEvalRunRejectsRawAnswerSecret(t *testing.T) {
	fake := &fakeEvalService{}
	result, err := startEvalRunHandler(fake)(context.Background(), callReq(map[string]any{
		"dataset_id":            "ds-1",
		"collection":            "documents",
		"answer_provider":       "openai",
		"answer_model":          "gpt-5.4-mini",
		"answer_api_key_secret": "sk-test-secret",
	}))
	if err != nil {
		t.Fatalf("handler returned transport error: %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected MCP tool error")
	}
	if got := textResult(t, result); !strings.Contains(got, "answer_api_key_secret must be an environment variable name") {
		t.Fatalf("error = %q", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
cd go/eval-mcp-service
go test ./internal/evalapi ./internal/evalworkflow ./internal/mcpserver
```

Expected: failures because answer model fields do not exist.

- [ ] **Step 3: Implement Go MCP forwarding**

In `go/eval-mcp-service/internal/evalapi/client.go`, extend `StartEvaluationRequest`:

```go
AnswerTier         string `json:"answer_tier,omitempty"`
AnswerProvider     string `json:"answer_provider,omitempty"`
AnswerBaseURL      string `json:"answer_base_url,omitempty"`
AnswerModel        string `json:"answer_model,omitempty"`
AnswerAPIKeySecret string `json:"answer_api_key_secret,omitempty"`
```

In `go/eval-mcp-service/internal/evalworkflow/service.go`, extend `StartRunInput` with the same fields and forward them in `StartEvaluationRequest`.

In `go/eval-mcp-service/internal/mcpserver/server.go`, extend `startEvalRunHandler` args and input:

```go
AnswerTier         string `json:"answer_tier,omitempty"`
AnswerProvider     string `json:"answer_provider,omitempty"`
AnswerBaseURL      string `json:"answer_base_url,omitempty"`
AnswerModel        string `json:"answer_model,omitempty"`
AnswerAPIKeySecret string `json:"answer_api_key_secret,omitempty"`
```

Add validation:

```go
func validateAnswerModelArgs(provider, model, secret string) error {
	if strings.TrimSpace(provider) == "" && strings.TrimSpace(model) == "" && strings.TrimSpace(secret) == "" {
		return nil
	}
	if provider != "ollama" && provider != "openai" && provider != "anthropic" {
		return fmt.Errorf("answer_provider must be ollama, openai, or anthropic")
	}
	if strings.TrimSpace(model) == "" {
		return fmt.Errorf("answer_model is required when answer_provider is set")
	}
	lowered := strings.ToLower(strings.TrimSpace(secret))
	if strings.HasPrefix(lowered, "sk-") || strings.HasPrefix(lowered, "sk_") || strings.HasPrefix(lowered, "bearer ") {
		return fmt.Errorf("answer_api_key_secret must be an environment variable name")
	}
	return nil
}
```

Call it before `StartRun`.

Update `startEvalRunSchema()` to include the five answer fields.

- [ ] **Step 4: Run tests to verify they pass**

Run:

```bash
cd go/eval-mcp-service
go test ./internal/evalapi ./internal/evalworkflow ./internal/mcpserver
```

Expected: all tests pass.

- [ ] **Step 5: Commit**

Run:

```bash
git add go/eval-mcp-service/internal/evalapi/client.go \
  go/eval-mcp-service/internal/evalworkflow/service.go \
  go/eval-mcp-service/internal/mcpserver/server.go \
  go/eval-mcp-service/internal/evalapi/client_test.go \
  go/eval-mcp-service/internal/evalworkflow/service_test.go \
  go/eval-mcp-service/internal/mcpserver/server_test.go
git commit -m "Expose answer model overrides in eval MCP"
```

## Task 5: Document The Model Ladder Workflow

**Files:**
- Modify: `go/eval-mcp-service/internal/mcpserver/server.go`
- Modify: `go/eval-mcp-service/README.md`
- Test: `go/eval-mcp-service/internal/mcpserver/server_test.go`

- [ ] **Step 1: Write failing workflow copy test**

In `go/eval-mcp-service/internal/mcpserver/server_test.go`, update the workflow resource and prompt tests to include:

```go
for _, want := range []string{
	"model ladder",
	"Keep the judge model fixed",
	"answer_tier",
	"answer_provider",
	"answer_model",
} {
	if !strings.Contains(got, want) {
		t.Fatalf("workflow missing %q: %s", want, got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:

```bash
cd go/eval-mcp-service
go test ./internal/mcpserver
```

Expected: workflow text test fails.

- [ ] **Step 3: Update workflow resource and README**

In `go/eval-mcp-service/internal/mcpserver/server.go`, update `evalPromptHandler` and `workflowResourceHandler` text with:

```text
For model ladder experiments, keep the judge model fixed and vary answer_tier,
answer_provider, answer_model, retrieval_config, and rerank. Start one run at a
time, wait for completion, compare only completed runs, inspect worst cases, and
record a conclusion only after the user approves it.
```

In `go/eval-mcp-service/README.md`, add a section:

```markdown
## Model Ladder Experiments

Use `start_eval_run` answer-model fields to compare local, efficient API, and
premium API tiers without manually redeploying chat between runs:

- `answer_tier`: `local`, `efficient`, or `premium`
- `answer_provider`: `ollama`, `openai`, or `anthropic`
- `answer_base_url`: provider endpoint when required
- `answer_model`: model name
- `answer_api_key_secret`: environment variable name containing the provider key

Keep the eval judge model fixed across all candidates. A useful portfolio sweep
is local baseline, efficient API tier, and premium API tier across baseline
retrieval and quality retrieval settings.
```

- [ ] **Step 4: Run test to verify it passes**

Run:

```bash
cd go/eval-mcp-service
go test ./internal/mcpserver
```

Expected: tests pass.

- [ ] **Step 5: Commit**

Run:

```bash
git add go/eval-mcp-service/internal/mcpserver/server.go \
  go/eval-mcp-service/internal/mcpserver/server_test.go \
  go/eval-mcp-service/README.md
git commit -m "Document eval MCP model ladder workflow"
```

## Task 6: Run Focused And Full Verification

**Files:**
- No new files.

- [ ] **Step 1: Run Python focused tests**

Run:

```bash
PYTHONPATH=services/eval:services pytest services/eval/tests/test_models.py services/eval/tests/test_answer_models.py services/eval/tests/test_main.py services/eval/tests/test_evaluator.py -q
PYTHONPATH=services/chat:services pytest services/chat/tests/test_main.py services/chat/tests/test_chain.py -q
```

Expected: all selected Python tests pass.

- [ ] **Step 2: Run Go focused tests**

Run:

```bash
cd go/eval-mcp-service
go test ./...
```

Expected: all eval MCP tests pass.

- [ ] **Step 3: Run repository preflights**

Run from the worktree root:

```bash
make preflight-python
make preflight-go
```

Expected: both preflights pass. If a preflight is blocked by local tooling, capture the exact missing tool or environment error in the final handoff.

- [ ] **Step 4: Inspect final diff**

Run:

```bash
git status --short
git log --oneline --max-count=6
git diff --stat qa...HEAD
```

Expected: only planned eval, chat, MCP, tests, and docs files changed.

- [ ] **Step 5: Push and open PR to `qa`**

Run:

```bash
git push -u origin feat/eval-mcp-model-ladder
gh pr create --base qa --head feat/eval-mcp-model-ladder \
  --title "Add eval MCP model ladder workflow" \
  --body "Adds per-run answer model overrides for MCP-driven RAG eval model ladder experiments, with fixed-judge provenance and workflow documentation."
```

Expected: PR created against `qa`. Do not watch CI unless Kyle asks.
