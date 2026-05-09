# Issue 85 RAG Improvement Tracking Dashboard — Design

**Status:** Approved for spec review
**Issue:** [#85](https://github.com/kabradshaw1/portfolio/issues/85)
**Roadmap:** [`2026-04-17-eval-portfolio-roadmap.md`](2026-04-17-eval-portfolio-roadmap.md) (Spec 4)
**Related design:** [`2026-04-28-rag-tracking-dashboard-design.md`](2026-04-28-rag-tracking-dashboard-design.md)

## Context

The eval service already supports evaluation run metadata needed for tracking RAG quality improvement: run notes, a baseline evaluation id, configuration snapshots, comparison, and history endpoints. The frontend eval client and UI have not caught up with that backend shape. `/ai/eval` currently exposes `Datasets`, `Evaluate`, and `Results` tabs, but it does not show score trends over time, side-by-side run deltas, or a useful annotated improvement timeline.

Issue 85 should close that visibility gap without expanding into a full experiment-tracking product. The dashboard should make the existing eval data legible for the portfolio narrative: Kyle measures RAG quality, changes one thing at a time, and can show whether those changes improved faithfulness, answer relevancy, context precision, and context recall.

## Goals

1. Add a fourth `Dashboard` tab to `/ai/eval` that tells the RAG improvement story from existing evaluation runs.
2. Display RAGAS score trends over completed runs for a selected dataset and collection.
3. Compare a baseline run against a candidate run with clear metric deltas.
4. Render an annotated change log from run notes, timestamps, score movement, and config snapshots.
5. Let users create annotated future runs by adding optional `notes` and `baseline_eval_id` inputs to the existing `Evaluate` tab.
6. Bring the frontend eval API types into parity with backend fields already returned by the eval service.

## Non-Goals

- No new backend experiment ledger or experiment CRUD endpoints.
- No cross-dataset comparison.
- No new charting dependency; `recharts` is already installed.
- No automatic recommendation engine, regression alerting, or rollback workflow.
- No broad redesign of the existing eval tool.

## Product Scope

Issue 85 adds a fourth tab to `/ai/eval` named `Dashboard`. The dashboard is a narrative tracking view for RAG quality improvement: select a dataset and collection, see score trends over completed runs, compare a baseline run to a later run, and read an annotated timeline of what changed and what impact it had.

The existing `Datasets`, `Evaluate`, and `Results` tabs stay in place. `Evaluate` gains two optional fields supported by the backend: `notes` and `baseline_eval_id`. This makes the workflow complete:

1. Create or choose a golden dataset.
2. Start an evaluation run with optional notes and a baseline run.
3. Review the run in `Results`.
4. Track trend, comparison, and annotation history in `Dashboard`.

## Data Flow

The frontend API client needs parity with the backend evaluation shape:

- `EvaluationSummary` and `EvaluationDetail` include `notes`, `config`, and `baseline_eval_id`.
- Dataset rendering handles missing `item_count` defensively because the current backend dataset list shape may not include it.
- `startEvaluation()` accepts optional `notes` and `baselineEvalId` values and serializes `baseline_eval_id`.
- New helpers call the existing backend endpoints:
  - `getHistory(datasetId, collection)` -> `GET /evaluations/history?dataset_id=...&collection=...`
  - `compareRuns(ids)` -> `GET /evaluations/compare?ids=baseline,candidate`

The dashboard does not create new backend data. Its annotated timeline is derived from completed evaluation runs: timestamp, notes, config snapshot, aggregate scores, and deltas from the selected baseline where available.

## UI Structure

`frontend/src/app/ai/eval/page.tsx` adds a fourth tab id, `dashboard`, and renders a new `DashboardTab` component under the existing `GoAuthProvider` and `HealthGate` wrappers.

The selected layout is a Narrative Timeline:

1. **Filters**
   - Dataset select populated from `listDatasets()`.
   - Collection text input defaulting to `documents`, matching the existing `Evaluate` tab pattern.
   - Both dataset and collection are required before history loads.

2. **Score Trend Chart**
   - A Recharts line chart plots completed runs over time.
   - Metrics: `faithfulness`, `answer_relevancy`, `context_precision`, and `context_recall`.
   - Null metric values render as gaps or are omitted without crashing.
   - Chart labels should use user-facing names: Faithfulness, Relevancy, Precision, Recall.

3. **Run Comparison Panel**
   - User selects a baseline run and a candidate run from the loaded history.
   - Calls `compareRuns([baselineId, candidateId])`.
   - Shows metric values and deltas.
   - Delta styling:
     - Positive improvement: green.
     - Negative movement: red.
     - No meaningful movement, including absolute delta below `0.005`: neutral gray.
   - If fewer than two completed runs exist, the comparison panel shows an explicit empty state.

4. **Annotated Change Log**
   - Chronological list of completed runs.
   - Each entry includes date, short run id, collection, notes when present, and compact score/delta summary.
   - Entries should provide a path to inspect the existing detailed scorecard, either by switching the page to the `Results` tab with that run selected or by another local navigation pattern consistent with the current eval UI.
   - Config snapshots should be summarized compactly when present. Missing or failed config capture should not block rendering.

The lower comparison and change-log panels should sit side by side on desktop and stack on mobile.

## Evaluate Tab Changes

`EvaluateTab` adds two optional inputs above the start button:

- **Notes:** textarea with a 500-character cap and a visible counter. Placeholder: `What changed since the last run?`
- **Baseline run:** dropdown of completed runs for the selected dataset and collection, with `(none)` as the default.

Both fields are optional. Existing evaluation behavior remains unchanged when they are blank. The tab should still poll the created evaluation until completion and hand the completed run to `Results` as it does today.

## Error And Empty States

The dashboard should render explicit states for:

- No datasets available.
- Dataset selected but collection missing.
- No completed runs for the selected dataset and collection.
- Only one completed run, so comparison is not available yet.
- History request failure.
- Compare request failure.
- Runs with missing aggregate scores or null metric values.
- Service unavailable, already handled by the existing page-level `HealthGate`.

Avoid silent failures in the new dashboard. Existing tabs do not need to be refactored except where the Evaluate changes require it.

## Testing

Testing should focus on behavior and data flow rather than Recharts internals.

Add mocked frontend coverage for `/ai/eval`:

- `Dashboard` tab appears alongside `Datasets`, `Evaluate`, and `Results`.
- Dataset and collection selections trigger history loading.
- Trend chart surface renders when completed runs exist.
- Comparison deltas render with positive, negative, and neutral styling.
- Change log renders notes and run metadata.
- Change log can navigate or switch to the detailed `Results` view for a selected run.
- Empty states render for no completed runs and fewer than two comparable runs.
- Evaluate tab sends optional `notes` and `baseline_eval_id` when provided.
- Evaluate tab preserves existing behavior when notes and baseline are blank.

Relevant preflight before committing implementation work:

```bash
make preflight-frontend
make preflight-e2e
```

## Future Spec: RAG Experiment Ledger

A first-class RAG Experiment Ledger should be a separate follow-up spec, not part of issue 85. That future work would promote annotations from run metadata into explicit experiment records.

Likely scope:

- Backend experiment models such as `Experiment`, `ExperimentSummary`, `ExperimentDetail`, and `CreateExperimentRequest`.
- A dedicated table for experiments with fields like name, hypothesis, dataset id, collection, baseline evaluation id, status, decision, notes, and timestamps.
- A relationship from evaluations to experiments, either by `experiment_id` on evaluations or a join table.
- Endpoints such as `POST /experiments`, `GET /experiments`, `GET /experiments/{id}`, `PATCH /experiments/{id}`, and `GET /experiments/{id}/runs`.
- Stronger validation that linked runs share dataset, collection, and comparable completed status.
- Frontend ledger or experiments view separate from the issue 85 dashboard.

This follow-up would be valuable because `notes` belongs to a single run, while an engineering experiment often has a hypothesis, baseline, candidate runs, config differences, and a final decision. Issue 85 should first make the existing run data visible; the ledger can then formalize the metadata that proves useful.
