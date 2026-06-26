# Multi-Agent Pipeline Scaffold: Custom In-Process Design over LangGraph

- **Date:** 2026-06-25
- **Status:** Accepted

## Context

This work was scoped directly against a job requirement:

> *Architect the scaffold layer that coordinates multi-agent pipelines, including role
> separation, context injection, and output validation.*

An audit of the existing Python services found that three of the four sub-claims were
already demonstrated in scattered, copy-pasted form, but the headline noun — *the scaffold
layer* — did not exist. Each service hand-rolled its own stage sequencing, metrics, tracing,
error handling, and LLM-output parsing. We extracted a reusable in-process package,
`services/shared/orchestration/`, and migrated the chat RAG pipeline onto it as the proof
consumer.

The decision that this ADR records is **why we built that scaffold from first principles
rather than adopting LangGraph**, which appears prominently in the target team's stack — and
under what conditions we *would* adopt it.

### What we built (and the talking points it backs)

The scaffold is five primitives in `services/shared/orchestration/`:

| Primitive | File | Backs the requirement of… |
|-----------|------|---------------------------|
| `PipelineContext` | `context.py` | **Context injection** — typed state threaded through every stage + request-scoped `metadata` + a `check_cancelled` hook |
| `Stage` / `StreamingStage` | `stage.py` | **Role separation** at the pipeline level — each stage is one bounded responsibility |
| `Role` | `role.py` | **Role separation** at the agent level — system prompt + model binding (e.g. answerer vs. judge on different models) |
| `call_validated` | `validation.py` | **Output validation** — LLM output parsed into a Pydantic schema with retry + repair-nudge on invalid output |
| `Pipeline` | `pipeline.py` | **Coordination** — `execute`/`execute_stream` wrap each stage with per-stage metrics, an OTel span, bound structlog context, and error classification |

The chat RAG pipeline (`services/chat/app/chain.py` + `stages.py`) now runs on it as:
`Retrieve → BuildPrompt → Generate(stream)`, with the external SSE contract preserved
byte-for-byte (proven by the existing chat suite staying green).

### The decision point

During design, three approaches were weighed:

- **A — Composable `Stage` protocol + typed `Context`, imperative control flow.** The scaffold
  standardizes the *unit of work* and the *cross-cutting concerns*; the consuming service
  writes control flow in plain Python (`for`, `if`, `while`).
- **B — Declarative DAG engine** (home-grown nodes + edges + conditional transitions).
- **C — Adopt LangGraph** (or a similar graph framework).

## Decision

We chose **Approach A**: a thin, in-process scaffold where stages are first-class and control
flow stays in ordinary async Python. `Pipeline.execute` is the only place that knows about
metrics, tracing, logging context, and error handling; it does **not** own the graph topology.
This fits all four target pipeline shapes — linear (chat), conditional/early-exit (dspm
cascade), multi-agent with distinct models (eval generate→judge), and looping/agentic (debug
ReAct) — without contortion, while reusing the existing `shared/llm` provider abstraction,
OpenTelemetry tracing, structlog, and Prometheus metrics.

We explicitly **did not adopt LangGraph** for this phase. The rest of this document explains
why, and lays out a concrete path to adopt it later for learning and where it would genuinely
pay off.

## Rationale: why not LangGraph (yet)

1. **The mandate was to *architect the scaffold*, not configure a framework.** The résumé
   bullet's verb is "architect." Building the coordination layer from first principles
   demonstrates the architecture skill the role is screening for; wiring up LangGraph would
   demonstrate framework configuration. The former is the stronger signal, and it forces a
   complete understanding of exactly what a framework like LangGraph abstracts away (which is
   itself the foundation for using it well later).

2. **It would fight existing, deliberate investments.** The services already have a unified
   provider abstraction (`shared/llm`), W3C-propagated OpenTelemetry tracing
   (`shared/tracing.py`), structlog with automatic trace/span injection (`shared/logging.py`),
   and Prometheus metrics. LangGraph brings its own runtime and leans on LangSmith for
   observability; integrating it cleanly would mean writing callback adapters and partially
   duplicating instrumentation we already trust.

3. **The current consumers don't need what LangGraph is *for*.** LangGraph's differentiated
   value is durable, long-running, stateful agents: checkpointed pause/resume, crash recovery,
   human-in-the-loop interrupts, time-travel, and complex branching. The Phase-1/2 consumers
   are a streaming RAG chain and (later) a single-loop ReAct agent and a deterministic
   escalation cascade — none of which require persistence or HITL today. Adopting a graph
   runtime now would be **YAGNI**.

4. **Streaming + a tool loop fit plain async Python cleanly.** SSE token streaming
   (`execute_stream`) and a `while step < max:` ReAct loop are natural in async Python. A graph
   engine adds indirection for control flow that is already simple and readable here.

5. **Dependency weight and abstraction cost.** LangGraph (and its LangChain surface area) is a
   large dependency to take on for pipelines that are currently linear or single-branch. A thin
   scaffold keeps the blast radius small and the behavior fully inspectable.

**Honest counterpoint — what Approach A gives up:** no built-in durable execution, no
checkpoint/resume, no human-in-the-loop primitives, no graph visualization, and no ecosystem of
prebuilt agents. If a future feature needs those, re-implementing them on top of the custom
scaffold would be *more* work than adopting LangGraph. The decision is "not yet," not "never."

## Consequences

**Positive**
- The scaffold is genuinely reusable and framework-free; its behavior is fully inspectable.
- Existing `shared/llm`, tracing, logging, and metrics are reused, not bent to fit a runtime.
- The four named requirements map to small, testable, named units (25 scaffold tests; chat
  migrated with zero behavior change).
- A clean, defensible interview narrative: *I built the coordination layer myself and can
  articulate exactly when a framework like LangGraph earns its keep.*

**Trade-offs**
- No durable execution / checkpointing / HITL out of the box.
- The team will eventually want LangGraph fluency; this scaffold doesn't build that directly —
  hence the learning path below.
- As consumers grow more branch-heavy or stateful, parts of the scaffold may converge on
  re-implementing what LangGraph already provides (a signal to adopt it then).

## LangGraph concept mapping

LangGraph (1.0) and the custom scaffold solve the same problem with different ergonomics. The
mapping makes the trade-off concrete and is the basis for the learning path.

| Custom scaffold | LangGraph (1.0) | Notes |
|-----------------|-----------------|-------|
| `PipelineContext.state` (dataclass, mutated in place) | `StateGraph` State schema (`TypedDict`) with **reducers** (`Annotated[T, operator.add]`) | LangGraph merges each node's *returned partial state* via reducers instead of in-place mutation |
| `Stage.run(ctx) -> ctx` | a **node** `fn(state) -> partial_state` (`add_node`) | same idea: one bounded unit of work |
| imperative `for`/`if`/`while` control flow | **edges** (`add_edge`) + **conditional edges** (`add_conditional_edges(node, router)` returning the next node or `END`) | LangGraph encodes routing declaratively in the graph |
| `Pipeline.execute` cross-cutting wrapper (metrics/tracing/log/error) | the compiled-graph runtime + LangSmith tracing + streaming modes | our OTel/Prometheus would attach via callbacks |
| `call_validated` (schema + retry + repair loop) | a validation node + conditional edge looping back, or `model.with_structured_output(...)` | retry-on-invalid becomes a cycle in the graph |
| `StreamingStage` / `execute_stream` | `graph.stream(..., stream_mode="updates"\|"messages"\|...)` | LangGraph has richer first-class streaming modes |
| *(none — we deferred it)* | **checkpointers** (`InMemorySaver`, Postgres/SQLite) → durable pause/resume, crash recovery | the biggest capability gap |
| *(none)* | **`interrupt()`** → human-in-the-loop | approve/edit a step mid-run |
| *(none)* | **subgraphs** + `Command(goto=..., graph=Command.PARENT)` | compose graphs of graphs |
| our debug ReAct loop (hand-written) | **`create_react_agent`** (prebuilt) | a batteries-included tool-calling agent |

## Future opportunities & learning path

LangGraph adoption is a *when*, not an *if*, given the target stack. Concrete plan:

**When to reach for LangGraph (the adoption triggers):**
- A workflow needs to **survive a crash or pause for minutes/hours** (checkpointing).
- A step needs **human approval/editing mid-run** (`interrupt`).
- Control flow becomes a genuine **branching graph** rather than a short `if`/`while`.
- A true **multi-agent** topology (supervisor + workers, or agent-of-agents via subgraphs).

**Best first candidate among current consumers:** the **debug ReAct agent**. It is already a
multi-step tool loop with message memory, so it maps almost 1:1 onto `create_react_agent` +
a checkpointer, and the migration would teach the highest-value LangGraph features (state,
tool nodes, conditional edges, durable execution) with the least conceptual overhead. The eval
`generate → judge → (retry)` loop is a strong second candidate — its retry-on-invalid becomes a
clean cycle, and answerer-vs-judge maps to two nodes/models.

**Specific features to learn, in order:**
1. `StateGraph` + State schema with reducers; `add_node` / `add_edge` / `add_conditional_edges`.
2. Compiling with a **checkpointer** and running with a `thread_id` (durable execution).
3. **Streaming modes** (`updates`, `messages`, `values`) and how they differ from our SSE.
4. **`interrupt()`** for human-in-the-loop.
5. `create_react_agent` (prebuilt) and when to drop to the low-level graph API.
6. **Subgraphs** + `Command` routing for multi-agent composition.
7. **LangSmith** tracing and how it compares to our OTel/Prometheus story.

**Concrete learning experiment (recommended next ADR notebook):** reimplement one existing
consumer — start with the eval generate→judge→retry loop or the dspm escalation cascade — as a
LangGraph `StateGraph` in a Jupyter notebook under `docs/adr/orchestration/`. Put the custom
scaffold version and the LangGraph version side by side and compare: lines of code, where the
control flow lives, what observability is free vs. wired, and what new capabilities (resume,
HITL) come along. This turns the concept-mapping table above into working, runnable knowledge
and produces a second portfolio artifact. (Verify current APIs against the LangGraph docs when
writing it — the framework moves quickly; this ADR reflects 1.0.)

**Interview framing:** *"I architected the multi-agent scaffold from first principles, so I
understand precisely what a framework like LangGraph abstracts — nodes, state reducers,
conditional edges, checkpointed durable execution, and human-in-the-loop interrupts. I chose a
thin in-process design because the current pipelines are streaming/linear/single-loop and
already had strong observability; I'd adopt LangGraph the moment a workflow needs durable
pause/resume, HITL, or genuine multi-agent branching — and the debug ReAct agent is the natural
first candidate."*

## Appendix — talking points mapped to the requirement

Crisp, defensible statements for each sub-claim, each backed by real code:

- **Scaffold layer that coordinates multi-agent pipelines** — `services/shared/orchestration/`
  is a reusable package; `Pipeline.execute`/`execute_stream` coordinate stages with
  standardized metrics, tracing, logging context, and error classification. Chat runs on it
  with its SSE contract unchanged; eval/dspm/debug are designed to follow.
- **Role separation** — `Role` binds a system prompt to a specific provider/model, so eval's
  *answerer* and *judge* are distinct agents on distinct models; `Stage` boundaries separate
  retriever / reranker / generator roles in the chat chain.
- **Context injection** — `PipelineContext` threads typed state and request-scoped metadata
  through every stage; `BuildPromptStage` assembles retrieved chunks (with source markers) into
  the prompt; hybrid dense+sparse retrieval feeds the context.
- **Output validation** — `call_validated` enforces a Pydantic schema on LLM output and retries
  with a corrective repair nudge before raising `OutputValidationError`; the `errors` taxonomy
  classifies failures as retryable vs. permanent for consumers' retry/DLQ logic.
