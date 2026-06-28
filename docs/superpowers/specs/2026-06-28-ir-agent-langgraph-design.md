# IR Agent — LangGraph Multi-Agent Incident Response Service

**Date:** 2026-06-28
**Status:** Approved — ready for implementation planning
**Service:** `services/ir-agent`

## Motivation

The `post.md` job posting (Surefire Cyber, an incident-response / cybersecurity
firm) names **LangChain twice** and repeatedly ties agentic work to **IR
workflows**. It explicitly asks for engineers who can "architect the scaffold
layer that coordinates multi-agent pipelines, including role separation, context
injection, and output validation."

The current Python portfolio (`services/`) covers ingestion, RAG chat, a
single-agent `debug` loop, eval, rag-triage, and a DSPM classifier. The largest
gaps relative to the posting are:

1. **Multi-agent orchestration** with role separation — only a hand-rolled
   single-agent loop exists today (`services/debug/app/agent.py`).
2. **LangChain / LangGraph depth** — current usage is limited to
   `langchain-text-splitters` for chunking. No LangGraph anywhere.
3. **Output validation** as a first-class concern.
4. **Domain fit** — nothing in the portfolio speaks to the cybersecurity / IR
   domain the role centers on.

This service closes all four at once: a LangGraph multi-agent pipeline that
investigates a synthetic security incident, built in the idiomatic LangChain
stack, with per-role Claude models and a two-layer output-validation design.

## Goals

- Demonstrate a production-grade **LangGraph** multi-agent pipeline with
  role-separated nodes, shared typed state, and a bounded validation loop.
- Showcase **output validation** at two layers: Pydantic-enforced structured
  output per node, plus an adversarial grounding check against retrieved
  evidence.
- Use **per-role Claude models** (Haiku / Sonnet / Opus) to show deliberate
  model tiering, not "use the biggest model everywhere."
- Stay **deterministic and CI-safe**: the evidence corpus is bundled fixtures,
  and the test suite mocks the LLM so CI incurs no API cost.
- Fit the existing service conventions (FastAPI, SSE, Prometheus, structlog,
  pydantic-settings) so it slots into the monorepo cleanly.

## Non-Goals (v1 — YAGNI)

- Supervisor / dynamic routing topology (a linear pipeline with a validation
  loop is enough and is more testable).
- Parallel fan-out investigators.
- Real SIEM / EDR / MCP integration — evidence is synthetic fixtures.
- The "GitHub-based org-wide knowledge store" runbook-retrieval tool. Noted as a
  clean future extension (it would reuse the existing Qdrant / RAG retrieval),
  but it is out of scope for this build.

## Architecture

A LangGraph `StateGraph` orchestrates four role-separated agents over a single
synthetic incident. Each node has its own system prompt (role), its own Claude
model tier, and its own allowed tool set.

```
            ┌──────────┐
  incident →│  triage  │  classify: severity, category, confidence   (Haiku)
            └────┬─────┘
                 ▼
            ┌──────────────┐   tools: search_alerts, get_logs,
            │ investigate  │◄─┐ lookup_ioc, get_asset_context        (Opus)
            └────┬─────────┘  │ (ReAct-style tool loop inside the node)
                 ▼            │
            ┌──────────┐      │ loop back if evidence is thin
            │ validate │──────┘ (bounded: max 2 retries)             (Opus)
            └────┬─────┘
                 ▼ verdict = grounded
            ┌──────────┐
            │  report  │  structured IR report + MITRE mapping       (Sonnet)
            └──────────┘
```

### Design principles mapped to the posting

- **Role separation** — `roles.py` maps each role to a configured `ChatAnthropic`
  instance with its own system prompt and tool bindings.
- **Context injection** — the investigator's evidence is written into the shared
  state as structured `EvidenceItem`s; downstream nodes read structured state,
  not a raw transcript.
- **Output validation (two layers)** —
  1. Every node emits a **Pydantic-validated** structured output via
     `ChatAnthropic.with_structured_output(...)`. Malformed output triggers a
     retry.
  2. The **validator agent** adversarially checks that the investigator's
     findings are *grounded in retrieved evidence* (anti-hallucination) and
     either loops back for more investigation or passes.
- **Bounded loop** — an `investigate_attempts` counter in state caps
  validator→investigator cycles so the graph cannot spin.

### Why LangGraph natively (not the legacy `shared/llm` abstraction)

The existing `services/shared/llm` `LLMProvider` protocol is Ollama-shaped
(`chat()` returns `{"message": {...}}`). Forcing LangGraph through it would
bypass the LangChain features that are the point of this exercise. This service
uses `langgraph` + `langchain-anthropic` directly, gaining native tool-binding,
`with_structured_output()`, message types, and streaming. Per-role models are
distinct `ChatAnthropic` instances configured centrally.

## Data Contracts

### Shared graph state (`state.py`)

A typed `IRState` threaded through every node:

```python
class IRState(TypedDict):
    incident: Incident               # input alert(s) + metadata
    triage: TriageResult | None      # severity, category, confidence
    evidence: list[EvidenceItem]     # everything tools returned (append-only)
    findings: Findings | None        # hypothesis + supporting evidence refs
    verdict: ValidationVerdict | None
    report: IRReport | None
    investigate_attempts: int        # bounds the validator loop
```

### Pydantic models (`models.py`)

Every node output is a validated schema, not free text:

- **`Incident`** — alert id, source (EDR / SIEM / email-gw), raw fields,
  observables (IPs, hashes, users, hosts).
- **`TriageResult`** — `severity: Literal["low","medium","high","critical"]`,
  `category` (phishing / malware / lateral-movement / data-exfil / …),
  `confidence: float`, `rationale`.
- **`EvidenceItem`** — stable `id`, `source_tool`, `query`, `content`. Created
  from each tool result; `Findings.evidence_refs` must cite these ids.
- **`Findings`** — `summary`, `hypothesis`, `evidence_refs: list[str]`, `iocs`,
  `affected_assets`.
- **`ValidationVerdict`** — `grounded: bool`, `unsupported_claims: list[str]`,
  `gaps: list[str]`, `needs_more_investigation: bool`.
- **`IRReport`** — `executive_summary`, `timeline`, `severity`, `iocs`,
  `mitre_attack: list[str]`, `recommended_containment: list[str]`, `confidence`.

### Tools (`tools.py`)

Read-only over bundled fixtures, so runs are deterministic and CI-safe. Each is a
`@tool`-decorated LangChain tool bound to the investigator model; results become
`EvidenceItem`s with stable ids.

| Tool | Returns |
|---|---|
| `search_alerts(query)` | matching alerts from the corpus |
| `get_logs(host \| user \| time_range)` | related log lines |
| `lookup_ioc(indicator)` | synthetic threat-intel reputation for an IP / hash / domain |
| `get_asset_context(host \| user)` | asset criticality, owner, identity context |

### The grounding check

The validator receives *only* the structured `Findings` plus the actual
`evidence` list. It must confirm each `evidence_ref` exists and each claim traces
to retrieved evidence. `grounded=false` or `needs_more_investigation=true` (under
the retry cap) routes back to the investigator with the `gaps` injected;
otherwise the graph proceeds to the report node.

### Fixtures (`fixtures/`)

3–4 hand-authored incident scenarios, each with its alerts, logs, IOCs, and asset
data. Proposed scenarios:

- Phishing → credential theft
- Malware beacon → C2
- Insider data exfiltration

These double as the deterministic test corpus.

## Service Surface

FastAPI service (`main.py`), mirroring `services/debug`:

- **`POST /investigate`** — body references a fixture incident id (or inlines an
  `Incident`); returns an **SSE stream** of graph events: `triage`,
  `tool_call`, `tool_result`, `findings`, `verdict`, `report`, `done`. Reuses the
  debug service's SSE event shape for frontend consistency.
- **`GET /healthz`** — liveness + Anthropic reachability.
- **`GET /metrics`** — Prometheus, reusing the existing `metrics.py` pattern:
  node latency, tool-call counts, validator loop count, tokens per role.
- Structured logging via `structlog` + OpenTelemetry, matching the other
  services.

### Config (`config.py`, `pydantic-settings`)

- Per-role model ids:
  - `IR_TRIAGE_MODEL=claude-haiku-4-5`
  - `IR_INVESTIGATE_MODEL=claude-opus-4-8`
  - `IR_VALIDATE_MODEL=claude-opus-4-8`
  - `IR_REPORT_MODEL=claude-sonnet-4-6`
- `ANTHROPIC_API_KEY`
- Max retry cap for the validator loop.
- Request timeouts.
- Prompt caching enabled on the shared system / role prefixes to reduce repeated
  input cost.

## Cost

Estimated tokens per full investigation: ~40K input + ~5K output across all
agents and tool loops.

| Setup | ~Cost per run | Runs for $5 |
|---|---|---|
| Mixed per-role (Haiku triage, Opus investigate/validate, Sonnet report) | ~$0.15 | ~33 |
| Opus everywhere | ~$0.33 | ~15 |

Real API calls happen only in a small set of manual end-to-end runs during
development (~$3–5 total, once). All unit and integration tests mock the LLM, so
CI incurs no cost. A spend limit set in the Anthropic console is the hard
backstop.

## Testing

Production-grade and free of API cost in CI:

- **Unit per node** — inject a fake / stub chat model so each node is
  deterministic and free. Assert structured-output validation, role prompt
  wiring, and state transitions.
- **Tool tests** — over the fixtures, exact-match assertions.
- **Graph integration test** — full `triage → investigate → validate → report`
  with stubbed models, asserting the validator loop fires and bounds correctly
  (force an ungrounded finding → expect one retry → then proceed).
- **Live smoke test** — one real end-to-end run gated behind `RUN_LIVE_LLM=1`,
  **skipped in CI**. This is where the one-time real-API spend lives.

## Dependencies

Add to `services/ir-agent/requirements.txt`: `langgraph`, `langchain-core`,
`langchain-anthropic`. Reuse the existing stack: `fastapi`, `uvicorn`,
`sse-starlette`, `pydantic-settings`, `prometheus-fastapi-instrumentator`,
`structlog`, OpenTelemetry, `pytest` / `pytest-asyncio` / `pytest-cov`,
`anthropic`.

## Repo Integration

Per `services/AGENTS.md`, adding a new service requires updating:

1. `.github/workflows/ci.yml` backend test matrix
2. `.github/workflows/ci.yml` docker build matrix
3. `.github/workflows/ci.yml` pip-audit matrix
4. `.github/workflows/ci.yml` Hadolint Dockerfile matrix
5. `docker-compose.yml`
6. CI deploy pull commands
7. A companion ADR notebook under `docs/adr/ir-agent/` explaining the LangGraph
   topology, per-role model tiering, and the two-layer validation design.

Run `make preflight-python` and `make preflight-security` before committing
Python changes.

## File Layout

```
services/ir-agent/
  Dockerfile
  requirements.txt
  app/
    __init__.py
    main.py          # FastAPI app, /investigate SSE, /healthz, /metrics
    config.py        # pydantic-settings, per-role models
    roles.py         # role → ChatAnthropic instance + system prompt + tools
    state.py         # IRState TypedDict
    models.py        # Pydantic contracts
    tools.py         # @tool fixtures-backed evidence tools
    graph.py         # StateGraph wiring: nodes, edges, bounded loop
    nodes/
      triage.py
      investigate.py
      validate.py
      report.py
    metrics.py       # Prometheus collectors
  fixtures/          # synthetic incident scenarios
  tests/             # unit per node, tool tests, graph integration, live smoke
```

## Open Questions

None blocking. The runbook-retrieval / org-knowledge-store tool is deferred to a
follow-up that would reuse the existing RAG retrieval.
