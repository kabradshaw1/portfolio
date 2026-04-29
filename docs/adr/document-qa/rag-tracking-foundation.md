# RAG Evaluation Tracking Foundation

- **Date:** 2026-04-29
- **Status:** Accepted
- **Supersedes:** none. Builds on [rag-evaluation-service.md](rag-evaluation-service.md).
- **Related PR:** #197 (closes #84)
- **Spec:** [docs/superpowers/specs/2026-04-28-rag-tracking-dashboard-design.md](../../superpowers/specs/2026-04-28-rag-tracking-dashboard-design.md)

## Context

The RAG evaluation service (built in [rag-evaluation-service.md](rag-evaluation-service.md)) could measure quality but could not answer the natural follow-up question: *what changed between this run and that run?* A scorecard from one evaluation against a different scorecard told you nothing about whether the model, the chunk size, the retrieval k, or the prompt template moved. Worse, there was no record of what configuration produced any given run, so even after the fact you could not reconstruct the experiment.

This is the gap between "I built a RAG evaluator" and "I built a system that lets me improve RAG quality over time." Closing it is the Phase 1 backend foundation under issue #84; the corresponding UI lands in Phase 2 (#85).

## Decision

Three additive changes across `services/eval`, `services/chat`, and `services/ingestion` — no existing endpoint shape changed.

1. **Every evaluation now carries a config snapshot.** Before the RAGAS run starts, the eval service calls `chat /config` and `ingestion /collections/{name}/config` in parallel and persists the merged result on the row. The snapshot records LLM model, embedding model, `top_k`, prompt version, chunk size, and chunk overlap.
2. **Two new eval endpoints** — `GET /evaluations/compare?ids=a,b,c` (N-way side-by-side with server-computed deltas, 2-5 ids, same-dataset enforced) and `GET /evaluations/history?dataset_id=&collection=` (time-series of completed runs, both filters required).
3. **Three previously-implicit RAG knobs become first-class.** `top_k` is now env-configurable on the chat service. Prompt templates live in a `PROMPTS` registry keyed by `PROMPT_VERSION` (the active version is part of every snapshot). Per-collection chunk params are persisted at upload time so they survive long after the ingestion env vars rotate.

A new `notes` column and `baseline_eval_id` pointer round out the evaluations table — together they let the dashboard render an annotated change log and "vs baseline" deltas without a separate experiments table.

### Architecture

```
                     ┌──────────────┐
                     │ chat service │  GET /config (new)
                     └──────┬───────┘  -> {llm_model, embedding_model,
                            │              top_k, prompt_version}
                            │
                            │ asyncio.gather, 5s timeout each
                            ▼
┌──────────────┐    ┌──────────────┐    ┌────────────────┐
│ ingestion    │←───│ eval service │───→│ SQLite         │
│ /collections │    │  + 3 new     │    │  + notes       │
│  /{x}/config │    │    endpoints │    │  + config      │
│  (new)       │    │              │    │  + baseline_id │
└──────────────┘    └──────────────┘    └────────────────┘
```

## Key design decisions

### 1. Where to store per-collection chunk metadata

Qdrant collections do not carry arbitrary metadata. The choices were:

- **Qdrant payload "config point"** — insert one synthetic point per collection with the metadata as payload, skip it in normal scrolls. Hacky; couples retrieval logic to a sentinel.
- **Tag every chunk with the params it was created with** — accurate but wasteful (1000-2000 duplicated key/value pairs per collection).
- **Separate SQLite store in ingestion** — chosen. Mirrors the eval service's `aiosqlite` pattern, isolates concerns, and gives a clean `GET /collections/{name}/config` surface.

The trade-off is one more file on disk in the ingestion service (`/app/data/collection_meta.db`, backed by an `emptyDir` volume). Acceptable because metadata is rewritten on every upload via `ON CONFLICT DO UPDATE` — pod restarts lose it but the next upload restores it. For this portfolio project that's fine; a production deployment would back this with a PVC.

### 2. Snapshot at run start, never block on capture

`capture_run_config()` is the only piece of code in the run path that talks to the chat and ingestion services for config. It uses `asyncio.gather(..., return_exceptions=True)` with 5-second timeouts and **never raises** — partial or total upstream failure is recorded in a `_capture_error` field on the snapshot, and the evaluation continues.

The reason: quality data must not be blocked by metadata gaps. If the ingestion service is down at 3 AM and we want an eval run, we get the scorecards plus a `config_unavailable` badge in the UI. The wrong choice would be to fail the entire eval because we couldn't fetch the model name.

### 3. Server-side delta computation for `/compare`

The compare endpoint returns both the runs and a `deltas: dict[str, list[float]]` block, where each metric has a per-run delta-vs-first. Two reasons to do this server-side instead of in the UI:

- **Deterministic, version-stable formatting.** Rounding, NaN handling, and missing-metric defaults all live in one place. The frontend just renders.
- **Smaller surface for "value" definition.** "Improvement" can mean different things (percent change, absolute change, normalized). Picking one server-side prevents drift between the dashboard and any future export tools.

### 4. N-way comparison (2-5 ids), same-dataset only

Cardinality is bounded at 5 because that's the largest small-multiple chart that's still readable, and unbounded N invites accidental DoS via huge ID lists. Same-dataset enforcement is hard-required (returns 400) because cross-dataset comparison is mathematically meaningless — different golden questions produce different scores, the deltas would be nonsense.

### 5. Baseline pointer model, not a separate experiments entity

The dashboard needs to render "this run was a deliberate experiment vs that baseline." Two ways to model that:

- **Separate experiments table** with `{title, hypothesis, baseline_id, treatment_id, observed_delta}`. Reads like a lab notebook but requires CRUD for a second entity.
- **`baseline_eval_id` pointer on the evaluation row.** Chosen. Same single source of truth as everything else; the change log is just `GET /history` rendered chronologically, with delta-vs-baseline shown for runs that have a baseline pointer.

Stale baseline pointers (the referenced run was deleted, or never existed) render as "baseline missing" — no foreign-key cascade, no validation at write time. Wrong pointers are harmless data, not corruption.

### 6. Prompt registry, not a hash of the active prompt

Two ways to track which prompt produced a run:

- **Hash the active prompt template** at request time and store the hash. Captures every change including typos. But the hash means nothing to a human reader and you can't easily roll back to "the v2 prompt."
- **Named version registry.** Chosen. `PROMPTS: dict[str, str]` keyed by `"v1-baseline"`, `"v2-cot"`, etc. `PROMPT_VERSION` env var picks the active one. Validated at startup against the registry to fail fast on typos.

The trade-off is that you must remember to bump the version when you change a template — a copy-paste edit to `v1-baseline`'s string would silently change scores without showing up in the snapshot. For a portfolio system that's an acceptable discipline; a production version would compute a hash *over* the named template as a tamper guard.

### 7. Idempotent SQLite migrations via try/except

SQLite has no `ADD COLUMN IF NOT EXISTS`. Each `ALTER TABLE evaluations ADD COLUMN ...` is wrapped in `try/except aiosqlite.OperationalError` and only swallows the "duplicate column name" message. The `init()` method runs on every service start, so adding a new column is one PR — no separate migration job, no manual step.

This works because the eval service is single-writer (one pod). For a multi-writer service we would need a real migration framework (alembic). The trade-off is acceptable for SQLite's intended single-process use case.

### 8. Same-namespace ingestion routing (no ExternalName)

`INGESTION_SERVICE_URL=http://ingestion:8000` resolves *within* the eval service's namespace — `ai-services` in prod, `ai-services-qa` in QA. Both namespaces have an `ingestion` Service. No cross-namespace ExternalName routing needed, and the QA environment cannot accidentally call prod's ingestion (the shared-infra rule from CLAUDE.md).

### 9. Route order for `/evaluations/compare` and `/evaluations/history`

FastAPI matches routes in declaration order. `/evaluations/{eval_id}` would otherwise match `/evaluations/compare` with `eval_id="compare"` first, returning 404 from the database lookup. The literal-segment routes are declared *before* the parameterized one, with a comment explaining why future maintainers shouldn't reorder them.

## Consequences

**Positive:**
- Every evaluation is now self-describing — the snapshot records exactly which RAG configuration produced its scores.
- The dashboard Phase 2 can be built by reading the API directly; no schema guessing.
- Three additive endpoints, three additive columns — no existing client breaks.
- Adding a new prompt variant is now a one-PR affair (append to `PROMPTS`, set `PROMPT_VERSION`).
- The "did this change help?" question has a concrete API behind it.

**Trade-offs:**
- Three services touched in one PR rather than landing changes incrementally per service. Reviewable because each service's diff is small and the integration point (`capture_run_config`) is unit-tested in isolation.
- Per-collection metadata in an `emptyDir` volume is ephemeral — pod restarts lose history that hasn't been re-uploaded. Acceptable for portfolio scale.
- Prompt-registry model lets a copy-paste edit to a registered template silently invalidate historical scores. Discipline-based, not enforced.
- The `_capture_error` path means a UI must distinguish "no config recorded" from "config recorded as `null`" — a small extra branch in rendering, but the alternative (block evals on missing metadata) is worse.

**Future work:**
- Phase 2 (#85): the Trends tab UI that consumes `/history`, `/compare`, and the change log narrative.
- Add a `prompt_hash` field alongside `prompt_version` to detect in-place edits to registered templates.
- Promote the per-collection metadata SQLite to a PVC if we ever want long-term retention.
