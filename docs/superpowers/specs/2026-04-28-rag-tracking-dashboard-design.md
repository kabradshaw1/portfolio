# RAG Improvement Tracking Dashboard — Design

**Status:** Approved (brainstorming complete)
**Issues:** [#84](https://github.com/kabradshaw1/gen_ai_engineer/issues/84) (backend), [#85](https://github.com/kabradshaw1/gen_ai_engineer/issues/85) (UI)
**Roadmap:** [`docs/superpowers/specs/2026-04-17-eval-portfolio-roadmap.md`](2026-04-17-eval-portfolio-roadmap.md) (Specs 3 + 4)

## Context

The eval service (`services/eval/`) and `/ai/eval` UI exist (issues #82, #83 closed). Today the UI lets a user upload a golden dataset, trigger an evaluation, and view a single scorecard. There is no way to compare runs over time or correlate quality changes with the parameters that produced them.

This spec covers both #84 and #85 as one design with two implementation phases:

- **Phase 1 / PR 1 (closes #84):** backend additions across eval, chat, and ingestion services that capture per-run RAG configuration and expose comparison + history endpoints.
- **Phase 2 / PR 2 (closes #85):** a "Trends" tab on `/ai/eval` rendering time-series charts, a side-by-side comparison view, and an annotated change log.

## Goals

1. Make every evaluation run **self-describing**: which model, which top_k, which chunk size/overlap, which prompt version produced these scores.
2. Let a viewer answer "did changing X improve quality?" without external bookkeeping.
3. Support the portfolio narrative: "I don't just build RAG pipelines, I measure and improve them."

## Non-goals

- A full experiment-tracking platform (no MLflow/W&B parity).
- Cross-dataset comparison (mathematically meaningless — different golden questions produce different scores).
- Prompt template versioning beyond a hand-curated dict (no registry service, no UI editor).
- Auto-rollback / regression alerting (out of scope for this spec).

## Architecture

```
                     ┌──────────────┐
                     │ chat service │  GET /config (new)
                     └──────┬───────┘  -> {llm_model, embedding_model,
                            │              top_k, prompt_version}
                            │
                            │ HTTP at run start
                            ▼
┌──────────────┐    ┌──────────────┐    ┌────────────────┐
│ ingestion    │←───│ eval service │───→│ SQLite         │
│  GET /col/{x}│    │  + 3 endpts  │    │  + notes       │
│  /config     │    │  + snapshot  │    │  + config      │
└──────────────┘    └──────┬───────┘    │  + baseline_id │
                           │            └────────────────┘
                           ▼
                  ┌──────────────────┐
                  │ /ai/eval         │  shadcn/ui charts
                  │  4th tab: Trends │  history + diff +
                  └──────────────────┘  change log
```

Two service boundaries are touched, but each change is additive — no existing endpoints change shape.

## Phase 1 — Backend (Issue #84, PR 1)

### 1.1 Eval-service schema (`services/eval/app/db.py`)

`evaluations` table gets three new columns, all additive and nullable:

| Column | Type | Purpose |
|---|---|---|
| `notes` | `TEXT` | Free-form annotation written by the user at run start. |
| `config` | `TEXT` (JSON) | Snapshot of the RAG configuration at run start, captured from chat + ingestion services. |
| `baseline_eval_id` | `TEXT` | Optional pointer to a prior eval this run is being compared against. Foreign key to `evaluations(id)`. |

Migration approach: existing `init()` already uses `CREATE TABLE IF NOT EXISTS`. Add `ALTER TABLE evaluations ADD COLUMN ...` statements in `init()`, each wrapped in `try/except aiosqlite.OperationalError` to handle the already-applied case. SQLite `ADD COLUMN` is non-locking and safe on a hot table.

### 1.2 Chat-service `/config` endpoint (`services/chat/app/main.py`)

```
GET /config
-> 200 {
    "llm_model": "qwen2.5:14b",
    "embedding_model": "nomic-embed-text",
    "top_k": 5,
    "prompt_version": "v1-baseline"
}
```

- No auth (matches `/health` pattern). Read-only, no secrets returned (no base URLs, no API keys).
- Implementation just returns values from `settings` plus the active `PROMPT_VERSION`.

### 1.3 `top_k` becomes configurable (`services/chat/app/`)

- New `Settings.top_k: int = 5` in `config.py`, sourced from `TOP_K` env var.
- `chain.py:retrieve_chunks` and `chain.py:chat_with_rag` already accept `top_k` parameters; thread `settings.top_k` from the call site instead of relying on the function default. The function-level defaults stay at 5 as a fallback.

### 1.4 Prompt versioning (`services/chat/app/prompt.py`)

```python
PROMPTS: dict[str, str] = {
    "v1-baseline": "...current template...",
    # future: "v2-cot", "v3-source-emphasis", etc.
}
```

- `Settings.prompt_version: str = "v1-baseline"` in `config.py`, sourced from `PROMPT_VERSION` env var.
- `build_rag_prompt` looks up the active template from `PROMPTS[settings.prompt_version]`. Raise `ValueError` at startup (in `settings.validate()`) if the configured version is not in the registry — fail fast.
- A new prompt is added by appending to the dict and committing. Switching prompts in QA/prod is an env-var change.

### 1.5 Ingestion-service per-collection metadata

**Read endpoint:**
```
GET /collections/{name}/config
-> 200 {"chunk_size": 1000, "chunk_overlap": 200, "embedding_model": "nomic-embed-text"}
-> 404 {"detail": "collection not found"}
```

**Write path:** when `store.py` creates a Qdrant collection, attach `chunk_size`, `chunk_overlap`, and `embedding_model` to the collection's payload metadata using Qdrant's `update_collection_aliases` / payload-schema mechanism (or a dedicated `_meta` document inside the collection — whichever Qdrant client API is cleaner; pick during implementation). The endpoint reads back from the same source.

For collections that already exist (created before this change), the endpoint returns 404 on the metadata lookup; the eval service treats that as "config unavailable" (see 1.6 error handling).

### 1.6 Eval service config snapshot at run start

`POST /evaluations` request body gains two optional fields:

```json
{
    "dataset_id": "...",
    "collection": "documents",
    "notes": "Increased chunk overlap from 200 to 300",
    "baseline_eval_id": "..."
}
```

In `_run_evaluation_task`, before the RAGAS run begins:

1. In parallel (`asyncio.gather`), call chat `/config` and ingestion `/collections/{collection}/config`.
2. Merge into a single config dict: `{"chat": {...}, "collection": {...}, "captured_at": "<iso>"}`.
3. If either upstream call fails (timeout, non-200, network), record `{"_capture_error": "<short reason>", "captured_at": "..."}` and proceed. **The eval still runs** — quality data must not be blocked by metadata gaps.
4. Persist `config` on the run.

Both `notes` and `baseline_eval_id` are persisted as-is (no validation beyond length caps; `baseline_eval_id` is *not* validated as existing — a stale pointer is harmless and just renders as "baseline missing" in the UI).

### 1.7 New eval-service endpoints

**Compare:**
```
GET /evaluations/compare?ids=a,b,c
```

- 2-5 ids (validate; 400 if outside range).
- Validates all referenced runs share the same `dataset_id` (400 with explicit message if not).
- Returns:
  ```json
  {
      "runs": [
          {"id": "a", "created_at": "...", "aggregate_scores": {...},
           "config": {...}, "notes": "...", "baseline_eval_id": null}
      ],
      "deltas": {
          "faithfulness": [0.0, 0.03, -0.01],
          "answer_relevancy": [0.0, ...],
          "context_precision": [0.0, ...],
          "context_recall": [0.0, ...]
      }
  }
  ```
- Deltas are computed server-side (`run[i] - run[0]`) so the UI doesn't recompute. First run's delta is always 0.0 by convention.
- 404 if any id not found; 400 if mixed datasets or out-of-range cardinality.

**History:**
```
GET /evaluations/history?dataset_id=x&collection=y
```

- Both query params required; 400 if either is missing.
- Returns completed runs (`status = 'completed'`) for the dataset+collection pair, ordered by `created_at` ASC:
  ```json
  {
      "runs": [
          {"id": "...", "created_at": "...", "aggregate_scores": {...},
           "config": {...}, "notes": "...", "baseline_eval_id": null}
      ]
  }
  ```
- Empty list if no completed runs (200, not 404).
- Rate-limited at `30/minute` matching other read endpoints.

### 1.8 Pydantic models

New / updated in `services/eval/app/models.py`:

- `StartEvaluationRequest` gains `notes: str | None = Field(default=None, max_length=500)` and `baseline_eval_id: str | None = None`.
- `EvaluationDetail` and `EvaluationSummary` gain `notes: str | None`, `config: dict | None`, `baseline_eval_id: str | None`.
- New `RunComparison` and `RunHistory` response models for the two new endpoints.

## Phase 2 — Trends Tab (Issue #85, PR 2)

### 2.1 Tab integration

A 4th entry in `TABS` in `frontend/src/app/ai/eval/page.tsx`:

```ts
{ id: "trends", label: "Trends" }
```

New component: `frontend/src/components/eval/TrendsTab.tsx`. Uses the existing `GoAuthProvider` + `HealthGate` wrappers from the page — no new gates needed.

### 2.2 Subsections (vertical layout inside the tab)

1. **Filters bar** — dataset dropdown (loaded from `/datasets`) + collection text input (defaults to `documents`, the platform's default collection). Both required to load the panel. Selections persisted to URL search params (`?dataset=...&collection=...`) for shareable links. A free-text collection input avoids needing a new "list collections for dataset" endpoint and matches how `collection` is treated everywhere else in the platform (a string identifier, not a managed entity).

2. **Time-series panel** — four small-multiple line charts (one per RAGAS metric: faithfulness, answer_relevancy, context_precision, context_recall). x-axis = `created_at`, y-axis = score [0,1]. Uses shadcn `<ChartContainer>` + Recharts `<LineChart>`. Each data point clickable; clicking jumps to the existing Results tab loaded with that run.

3. **Comparison panel** — a checkbox column on a "runs" table below the charts lets user select 2-5 runs. "Compare" button is disabled outside that range. On click, calls `/compare` and reveals:
   - **Score table:** rows = metrics, columns = selected runs. Cells show value + Δ-vs-first with green (improved) / red (regressed) / gray (unchanged within ±0.005) coloring.
   - **Config diff strip:** two-column "knob | values across runs" table that shows only knobs that vary across the selected runs (e.g., `chunk_overlap: 200 / 300 / 400`). Knobs that are constant across all selected runs are omitted.

4. **Annotated change log** — chronological list rendered from the same `/history` payload used by the time-series. Each entry: date, scorecard summary, notes (if present), and — when `baseline_eval_id` is set and resolves — a "vs baseline" delta block with the config-diff that explains the change. Stale baseline pointers render "baseline missing".

### 2.3 API client (`frontend/src/lib/eval-api.ts`)

New methods:

```ts
getHistory(datasetId: string, collection: string): Promise<RunHistory>
compareRuns(ids: string[]): Promise<RunComparison>
```

`startEvaluation` gets two optional params: `notes?: string`, `baselineEvalId?: string`. The Evaluate tab gets two new optional form inputs to populate them — no required-field changes.

### 2.4 EvaluateTab changes

Two new optional inputs above the "Start Evaluation" button:
- **Notes** — textarea, 500-char cap with counter, placeholder "What changed since the last run?"
- **Baseline run** — dropdown of completed runs for the same dataset, "(none)" default.

Both are optional; the existing button behavior is unchanged when both are blank.

## Data flow (one evaluation run, end to end)

```
User clicks "Start Evaluation" with notes="bumped overlap to 300" and baseline=run-X
    │
    ▼
POST /evaluations { dataset_id, collection, notes, baseline_eval_id }
    │
    ▼
eval service creates row with status=running
    │
    ├─► chat /config       ┐
    │                       ├─ asyncio.gather (5s timeout each)
    └─► ingestion /col/{x}/config ┘
    │
    ▼
Persist config snapshot on the row (or _capture_error if upstream failed)
    │
    ▼
RAGAS evaluation runs (existing logic, unchanged)
    │
    ▼
Row updated to status=completed with aggregate_scores + results
    │
    ▼
Trends tab polls /history; new point appears on the time-series; if baseline_eval_id
is set, change-log entry shows the delta and the config diff vs run-X.
```

## Error handling

| Failure | Behavior |
|---|---|
| Chat or ingestion `/config` unreachable at run start | Run proceeds; `config = {"_capture_error": "<reason>"}`; UI badges those entries "config unavailable" but still plots scores. |
| `/compare` with mixed datasets | 400 `{"detail": "all runs must belong to the same dataset"}`. |
| `/compare` with <2 or >5 ids | 400 `{"detail": "compare requires 2-5 ids"}`. |
| `/compare` with unknown id | 404 with the specific id in the message. |
| `/history` empty | 200 with `{"runs": []}`; UI shows "No completed runs yet for this dataset and collection." |
| `baseline_eval_id` references missing run | Run still completes; UI renders "baseline missing" in the change-log entry. |
| Eval service down | Existing HealthGate hides the entire `/ai/eval` route — no Trends-specific gate needed. |

## Testing

### Backend

- `services/eval/tests/test_db.py`: new `notes`/`config`/`baseline_eval_id` round-trip; ALTER TABLE idempotency.
- `services/eval/tests/test_main.py`:
  - `/compare`: happy path (2 runs, 5 runs); 400 on mixed datasets; 400 on bad cardinality; 404 on unknown id.
  - `/history`: dataset+collection filter correctness; ordering ASC; empty result returns 200.
  - `POST /evaluations` with notes + baseline persists both.
  - Config snapshot: stub chat/ingestion `/config` (httpx mock); verify merged config; verify `_capture_error` path when upstream fails.
- `services/chat/tests/test_main.py`: `/config` returns expected fields; prompt-registry lookup uses configured version.
- `services/chat/tests/test_config.py`: `validate()` raises if `prompt_version` not in registry.
- `services/ingestion/tests/`: per-collection metadata write-on-create + read-back round-trip; `/collections/{name}/config` 404 for unknown collection.

### Frontend

- Component tests for `TrendsTab` with mocked API client:
  - Renders 4 time-series charts when history has data.
  - Empty-state message when history is empty.
  - Compare button disabled outside 2-5 selections.
  - Delta colors: green for positive, red for negative, gray within ±0.005.
  - URL params reflect filter selections.
- Playwright smoke (`frontend/e2e/`): navigate to `/ai/eval`, click Trends tab, assert filters render. (Full e2e of run-with-notes-and-baseline is out of scope for this spec — the existing eval-tab e2e provides coverage of the run lifecycle.)

### Migration safety

SQLite `ALTER TABLE ADD COLUMN` is non-locking. The `init()` method runs on every service start and the try/except wrapper makes the migrations idempotent. No separate migration tool needed (matches the existing pattern).

## Phasing & PRs

**PR 1 — Phase 1 (closes #84)**
Branch: `agent/feat-eval-tracking-backend` from `main` → PR into `qa`.
Touches: `services/eval/`, `services/chat/`, `services/ingestion/`, k8s ConfigMaps for new env vars (`TOP_K`, `PROMPT_VERSION`).

**PR 2 — Phase 2 (closes #85)**
Branch: `agent/feat-eval-trends-tab` from `main` (after PR 1 merges to `qa`) → PR into `qa`.
Touches: `frontend/src/app/ai/eval/page.tsx`, `frontend/src/components/eval/`, `frontend/src/lib/eval-api.ts`, Playwright smoke.

Each PR ships independently and provides standalone value: PR 1 makes evaluations self-describing even without the dashboard; PR 2 surfaces what PR 1 captures.

## Open considerations (non-blocking)

- Whether to backfill existing evaluations with `config = null` (chosen) vs synthetic config from current settings. Backfilling synthetic data would be misleading — a `null` config is honest about the gap.
- Whether the comparison panel should support "save as named comparison" for repeat use. Out of scope for this spec; the URL params already give shareable links to a comparison setup.
