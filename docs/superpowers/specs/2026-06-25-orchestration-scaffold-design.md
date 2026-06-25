# Orchestration Scaffold — Multi-Agent Pipeline Coordination Layer

**Date:** 2026-06-25
**Status:** Design — pending review
**Author:** Kyle Bradshaw (with Claude)

## Motivation

The portfolio backs a resume bullet:

> *Architect the scaffold layer that coordinates multi-agent pipelines, including role
> separation, context injection, and output validation.*

An audit of the Python services found that **three of the four sub-claims are already
demonstrated, but the headline noun is not.** Role separation, context injection, and
output validation each have strong, concrete examples — but they live in four independent,
copy-pasted implementations. There is no shared "scaffold layer": each service hand-rolls
its own stage sequencing, metrics, tracing, error handling, and LLM-output parsing.

This design extracts a reusable in-process orchestration package,
`services/shared/orchestration`, that makes the three named features first-class and
reusable, then migrates existing services onto it. The result makes the bullet literally
true (one scaffold, multiple consumers) and produces a defensible portfolio artifact.

### What exists today (evidence)

| Capability | Where it lives today | Shape |
| --- | --- | --- |
| Linear RAG pipeline | `chat/app/chain.py` (`rag_query`) | embed → retrieve → rerank → prompt → generate (streaming) |
| Conditional cascade | `dspm-classifier/app/classifiers/pipeline.py` | regex → NER → LLM, with early-exit + escalation |
| Multi-agent generate→judge | `eval/app/evaluator.py` | search → answerer → judge (distinct model) → score |
| Agentic ReAct loop | `debug/app/agent.py` | while-loop tool calling with message memory |
| Provider abstraction | `shared/llm/` (`base.py`, `factory.py`) | `LLMProvider` Protocol, multi-provider factory |
| Structured logging + tracing | `shared/logging.py`, `shared/tracing.py` | structlog + OTel, auto trace/span injection |

### The gap

- No shared abstraction for "a stage" or "a pipeline" — each service reimplements sequencing.
- No request-scoped context propagated *across* stages in a uniform way.
- Output validation (schema + retry-on-invalid) is hand-written separately in
  `dspm/llm_pass.py` and `eval/parse_judge_scores()`.
- Error classification (retryable vs. permanent) exists only in the eval worker.
- Per-stage metrics/tracing are ad-hoc, with inconsistent labels.

## Goals

1. A reusable `services/shared/orchestration` package built on five primitives.
2. Make **role separation**, **context injection**, and **output validation** first-class,
   reusable concepts — not per-service reimplementations.
3. Fit all four existing pipeline shapes (linear, conditional, multi-agent, looping) without
   contortion.
4. Preserve existing service behavior exactly — proven by existing test suites staying green.
5. Lean on existing `shared/` investments (`llm`, `logging`, `tracing`) rather than replacing
   them.

## Non-Goals

- No declarative graph engine or external framework (LangGraph / LlamaIndex). Control flow
  stays in plain Python.
- No new LLM provider code — `Role` binds to the existing `shared/llm` factory.
- No change to any service's external HTTP API or SSE event contract.
- No cross-service distributed orchestration — this is an in-process library; services remain
  independent and communicate over HTTP as they do today.

## Approach

**Composable Stage protocol + typed Context, with imperative control flow** (chosen over a
declarative DAG engine or adopting LangGraph). The scaffold standardizes the *unit of work*
and the *cross-cutting concerns* around each unit; it does not own the graph topology. This
is the only model that cleanly fits a streaming ReAct loop and a conditional cascade under
the same abstraction, and it keeps the architecture authored in-repo (the strongest "I
architected the scaffold" story).

## Architecture

```
services/shared/orchestration/
  __init__.py     # public exports
  context.py      # PipelineContext: typed state + event emission + cancellation
  stage.py        # Stage protocol — the unit of work
  role.py         # Role: system prompt + model + params, bound to shared/llm
  validation.py   # call_validated(): schema + retry-on-invalid + repair nudge
  pipeline.py     # Pipeline.execute(stage, ctx) wrapper + run([stages]) + stream()
  errors.py       # StageError taxonomy + retryable/permanent classifier
  metrics.py      # standardized per-stage Prometheus histogram
  tests/          # unit tests for every primitive
```

### Primitive 1 — `PipelineContext` (context injection)

A typed object threaded through every stage. It accumulates state and carries request-scoped
data so any stage can read what earlier stages produced.

```python
StateT = TypeVar("StateT")

class PipelineContext(Generic[StateT]):
    state: StateT                       # typed per-pipeline payload (dataclass/Pydantic)
    run_id: str                         # correlation id for this pipeline run
    metadata: dict[str, Any]            # request-scoped: collection, user, prompt_version...
    async def check_cancelled(self) -> None  # raises CancelledPipelineError if cancelled
```

- **State** is a per-pipeline typed payload (e.g. a `ChatState` dataclass holding `question`,
  `chunks`, `answer`). Stages read and write it — this is how the output of one stage becomes
  the context of the next.
- **`check_cancelled`** generalizes eval's existing cancellation checkpoints; the hook is a
  no-op unless a consumer supplies a cancellation predicate.
- Context injection helpers (assembling prompts from `state`, appending to a message history)
  live as small functions stages call; the scaffold does not dictate prompt format.

### Primitive 2 — `Stage` (role separation at the pipeline level)

```python
@runtime_checkable
class Stage(Protocol):
    name: str
    async def run(self, ctx: PipelineContext) -> PipelineContext: ...

@runtime_checkable
class StreamingStage(Protocol):
    name: str
    def stream(self, ctx: PipelineContext) -> AsyncIterator[dict]: ...
```

- One method, one responsibility. Dependencies (retriever, reranker, an LLM `Role`) are
  injected via the stage's constructor, so each stage is unit-testable with fakes.
- A transform stage mutates `ctx.state` and returns the context (return enables clean
  composition and testing). Stages are the explicit role boundaries: `RetrieveStage`,
  `RerankStage`, `GenerateStage`, `JudgeStage`, etc.
- A `StreamingStage` yields progressive events (token deltas, ReAct step events) instead of
  returning a context — used for the generate stage in chat and the agent loop in debug.

### Primitive 3 — `Role` (role separation at the agent level)

```python
@dataclass(frozen=True)
class Role:
    name: str            # "answerer", "judge", "classifier"
    system_prompt: str
    provider: str        # "ollama" | "openai" | "anthropic"
    base_url: str
    api_key: str
    model: str
    params: dict[str, Any] = field(default_factory=dict)  # temperature, etc.

    def build_provider(self) -> LLMProvider:
        return get_llm_provider(self.provider, self.base_url, self.api_key, self.model)
```

- A `Role` bundles *who is acting*: system prompt + model binding + params, built via the
  existing `get_llm_provider` factory and the existing `LLMProvider` Protocol
  (`chat(messages, tools)` / `generate(prompt, system, stream=)`).
- eval's **answerer vs. judge become two `Role`s on different models** — the clearest
  role-separation showcase. No new provider code is introduced.

### Primitive 4 — `call_validated` (output validation)

```python
async def call_validated(
    role: Role,
    messages: list[dict],
    schema: type[BaseModelT],
    *,
    tools: list[dict] | None = None,
    max_retries: int = 2,
    repair: bool = True,
) -> BaseModelT:
    """Call the role's LLM, parse the response against `schema`, and retry on
    invalid output. When `repair` is set, a corrective nudge describing the
    parse/validation error is appended to the messages before the next attempt.
    Raises OutputValidationError after retries are exhausted."""
```

- Generalizes the hand-written retry/parse logic in `dspm/llm_pass.py` and
  `eval/parse_judge_scores()` into one reusable primitive.
- Parses LLM text output into a Pydantic model (the project's existing validation idiom),
  clamping/validating fields via the schema. Tool-calling schemas pass through `tools`.
- The repair nudge is the differentiated production touch: on a parse/validation failure it
  feeds the error back to the model and retries, rather than failing immediately.

### Primitive 5 — `Pipeline` (the runner / cross-cutting concerns)

```python
class Pipeline(Generic[StateT]):
    def __init__(self, name: str): ...

    async def execute(self, stage: Stage[StateT], ctx) -> PipelineContext[StateT]:
        """Run one stage wrapped with: OTel span (stage.<name>), structlog
        contextvar binding (stage=<name>), per-stage duration+status metric,
        and error classification. Re-raises classified errors."""

    async def run(self, stages: list[Stage], ctx) -> PipelineContext:
        """Convenience: execute stages in order (the linear case)."""

    async def execute_stream(self, stage: StreamingStage, ctx) -> AsyncIterator[dict]:
        """Run a streaming stage, yielding its events, wrapped with the same
        metrics/tracing/log-context/error handling as execute()."""
```

- **The key to fitting all four shapes:** consumers call `pipeline.execute(stage, ctx)` inside
  *their own* Python control flow — a `for` (chat), an `if`/early-exit (dspm), a `while`
  (debug). `run([stages])` is sugar for the linear case.
- `execute` is where the scaffold earns its name: it owns metrics, tracing, logging context,
  and error classification so individual stages don't reimplement them.

### Cross-cutting concerns standardized by `execute`

- **Metrics** (`metrics.py`): one histogram `orchestration_stage_duration_seconds{pipeline,
  stage, status}`, replacing the ad-hoc per-service stage timers.
- **Tracing**: opens `tracer.start_as_current_span(f"stage.{stage.name}")`. Because
  `shared/logging.py`'s `_add_otel_context` reads the active span, every log line inside a
  stage automatically carries `trace_id`/`span_id` — no manual plumbing. (Confirmed against
  `shared/tracing.py` + `shared/logging.py`.)
- **Logging context**: binds `stage=<name>` to structlog contextvars for the stage's duration.
- **Error classification** (`errors.py`): promotes eval's `classify_item_error`
  (retryable vs. permanent) into a shared taxonomy used uniformly across consumers.

## Data Flow — how each consumer maps

| Consumer | Flow on the scaffold | Proves |
| --- | --- | --- |
| **chat** | `Embed → Retrieve(hybrid + semantic fallback) → Rerank? → BuildPrompt → Generate(emit deltas)` via `run`/`stream` | linear + streaming + context injection |
| **eval** | `Search → Generate(role=answerer) → Judge(role=judge, call_validated[JudgeScores]) → Score(deterministic)` | multi-agent roles + output validation |
| **dspm** | `Regex → [stop if high-conf] → NER → [if escalate] LLM(call_validated)` in plain `if` flow | conditional routing / early-exit |
| **debug** | `while step < max: AgentStep(tools) → if calls: ToolDispatch; else emit diagnosis, break` | looping / agentic + streaming |

## Error Handling

- `errors.py` defines a small taxonomy: `StageError` (base), with `retryable: bool`, plus a
  `classify(exc) -> StageError` helper seeded from eval's existing logic.
- `Pipeline.execute` catches exceptions, records a `status="error"` metric for the stage,
  logs with the bound stage context, and re-raises a classified error.
- Consumers decide policy: the eval worker keeps its retry/DLQ behavior; dspm keeps its
  best-effort tolerance (LLM-stage failure does not block a result); chat/debug surface errors
  as SSE error events. The scaffold standardizes *classification*, not *policy*.

## Streaming

- Progressive output is produced by a `StreamingStage` whose `stream(ctx)` is an async
  generator (matching the existing `stream_response` shape in chat).
- `Pipeline.execute_stream(stage, ctx)` wraps the streaming stage with the same
  metrics/tracing/log-context/error handling as `execute`, yielding events straight to the
  FastAPI `StreamingResponse`. This preserves the exact SSE event contracts of chat and debug.
- Non-streaming consumers (eval, dspm) call `run`/`execute` directly.
- *(Considered and deferred: a `ctx.emit()` event queue draining concurrently with a control-flow
  runner. The `StreamingStage` protocol is simpler and fits the existing async-generator code;
  the queue can return later if a consumer needs to fan out events from mid-stage.)*

## Testing Strategy

- **Per-primitive unit tests** (`orchestration/tests/`):
  - `Stage` fakes prove `execute` wraps metrics/tracing/error handling.
  - `call_validated` tested with a fake `LLMProvider` returning bad-then-good JSON — proves
    retry + repair nudge.
  - `PipelineContext` tested for event emission ordering and cancellation.
- **Behavior-preservation gate (the safety net):** each migrated service's *existing* test
  suite must stay green. A green suite is the regression proof. No behavior change is allowed
  to land with a red suite.
- CI: `ruff` clean before every commit (project convention).

## Implementation Phasing

The design covers all four consumers so the abstraction is correct, but implementation lands
incrementally — each phase is its own plan + PR to `qa`:

1. **Phase 1 — Scaffold package + its own tests** (no consumers). Establishes the five
   primitives and the cross-cutting `execute` wrapper.
2. **Phase 2 — Migrate `chat`** onto the scaffold (linear + streaming proof). Existing chat
   tests stay green.
3. *(Later plans)* **Phase 3 — eval** (multi-agent + validation), **Phase 4 — dspm**
   (conditional), **Phase 5 — debug** (looping).

**This spec's first implementation plan covers Phases 1 + 2 only.** Phases 3–5 get their own
specs/plans once the abstraction is validated against a real consumer.

## Open Questions / Risks

- **Streaming queue back-pressure:** the event queue should be bounded; confirm sizing during
  Phase 2 against chat's token throughput.
- **Generic typing ergonomics:** `PipelineContext[StateT]` generics must stay readable; if they
  fight `ruff`/type-checking, fall back to a per-pipeline `state` dataclass without parameterizing
  the context class.
- **`call_validated` for tool-calling vs. text-JSON:** Phase 1 targets text→Pydantic parsing
  (dspm/eval idiom); native tool-call schema validation (debug) is finalized in Phase 5.
