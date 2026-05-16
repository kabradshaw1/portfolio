# Eval Dashboard Summary API Design

## Summary

Add a purpose-built backend summary endpoint for RAG evaluation dashboards:

`GET /evaluations/dashboard?dataset_id=...&collection=documents&recent_limit=10`

The endpoint returns compact, trend-ready metadata for one dataset and
collection without forcing clients to orchestrate `/datasets`,
`/evaluations/history`, and `/evaluations/compare`. It is additive: existing
history, compare, and detail endpoints remain unchanged, and full per-query
results stay behind `GET /evaluations/{id}`.

## Context

Issue 240 asks for a clean backend measurement surface for dashboards, scripts,
reports, and future agent workflows. Issue 85 already built a frontend
dashboard by combining lower-level eval endpoints client-side. That works, but
it makes the client infer summary state that the eval service can compute more
consistently.

This design complements
`2026-05-15-eval-experiment-source-of-truth-design.md`. The dashboard endpoint
reads canonical `services/eval` dataset and evaluation data, but it must not
require experiment records to exist. Later experiment work can add richer
annotations without changing the core dashboard contract.

## Goals

- Add one compact backend endpoint for dataset and collection RAG performance
  tracking.
- Return dataset metadata, completed run count, first/latest completed run
  summaries, metric trend points, capped recent annotated runs, and
  baseline-to-latest deltas when possible.
- Keep the response useful for ordinary evaluation runs even when no experiment
  rows exist.
- Avoid returning full per-query result payloads by default.
- Preserve existing `/evaluations/history`, `/evaluations/compare`, and
  `/evaluations/{id}` behavior.

## Non-Goals

- Do not create experiment records.
- Do not replace the history, compare, or detail endpoints.
- Do not modify frontend consumption in this slice.
- Do not return full per-query evaluation results from the dashboard endpoint.
- Do not introduce new RAG metrics.

## API Contract

Add:

`GET /evaluations/dashboard`

Query parameters:

- `dataset_id`: required string.
- `collection`: required string.
- `recent_limit`: optional integer. Defaults to `10`, minimum `1`, maximum
  `100`.

Response behavior:

- Return `400` when `dataset_id` or `collection` is missing.
- Return `400` when `recent_limit` is outside `1..100`.
- Return `404` when `dataset_id` does not exist.
- Return `200` with an empty dashboard summary when the dataset exists but has
  no completed runs for the requested collection.
- Include only completed runs in dashboard metrics.
- Exclude `results` and `error` fields from dashboard run summaries.

Example response:

```json
{
  "dataset": {
    "id": "ds-1",
    "name": "rag-golden",
    "item_count": 12
  },
  "collection": "documents",
  "completed_run_count": 4,
  "first_completed_run": {
    "id": "eval-1",
    "created_at": "2026-05-01T12:00:00+00:00",
    "completed_at": "2026-05-01T12:03:00+00:00",
    "notes": "baseline",
    "config_captured": true,
    "aggregate_scores": {
      "faithfulness": 0.82,
      "answer_relevancy": 0.8,
      "context_precision": 0.71,
      "context_recall": 0.76
    },
    "baseline_eval_id": null
  },
  "latest_completed_run": {
    "id": "eval-4",
    "created_at": "2026-05-04T12:00:00+00:00",
    "completed_at": "2026-05-04T12:03:00+00:00",
    "notes": "rerank enabled",
    "config_captured": true,
    "aggregate_scores": {
      "faithfulness": 0.86,
      "answer_relevancy": 0.81,
      "context_precision": 0.79,
      "context_recall": 0.74
    },
    "baseline_eval_id": "eval-1"
  },
  "metric_trends": {
    "faithfulness": [
      {
        "evaluation_id": "eval-1",
        "completed_at": "2026-05-01T12:03:00+00:00",
        "score": 0.82
      }
    ],
    "answer_relevancy": [],
    "context_precision": [],
    "context_recall": []
  },
  "recent_runs": [
    {
      "id": "eval-4",
      "created_at": "2026-05-04T12:00:00+00:00",
      "completed_at": "2026-05-04T12:03:00+00:00",
      "notes": "rerank enabled",
      "config_captured": true,
      "aggregate_scores": {
        "faithfulness": 0.86,
        "answer_relevancy": 0.81,
        "context_precision": 0.79,
        "context_recall": 0.74
      },
      "baseline_eval_id": "eval-1"
    }
  ],
  "baseline_to_latest_deltas": {
    "baseline_eval_id": "eval-1",
    "latest_eval_id": "eval-4",
    "deltas": {
      "faithfulness": 0.04,
      "answer_relevancy": 0.01,
      "context_precision": 0.08,
      "context_recall": -0.02
    }
  }
}
```

When no completed runs exist, `completed_run_count` is `0`,
`first_completed_run`, `latest_completed_run`, and
`baseline_to_latest_deltas` are `null`, trend arrays are empty, and
`recent_runs` is empty.

## Models

Add Pydantic response models in `services/eval/app/models.py`:

- `DashboardDatasetSummary`
- `DashboardRunSummary`
- `MetricTrendPoint`
- `DashboardBaselineDeltas`
- `EvaluationDashboard`

Reuse `QueryScore` for aggregate score fields. `DashboardRunSummary` should be
compact and include:

- `id`
- `created_at`
- `completed_at`
- `notes`
- `config_captured`
- `aggregate_scores`
- `baseline_eval_id`

The model should not include `results`, `error`, or raw `config`. The boolean
`config_captured` is enough for dashboard/change-log use and keeps the response
compact.

## Data Flow

1. The route validates `dataset_id`, `collection`, and `recent_limit`.
2. The service loads the dataset by ID.
3. If the dataset is missing, the route returns `404`.
4. The service loads completed evaluations for the dataset and collection,
   ordered by `created_at ASC` to match the existing history behavior.
5. The response builder computes:
   - `completed_run_count` from all completed runs.
   - `first_completed_run` from the oldest completed run.
   - `latest_completed_run` from the newest completed run.
   - `metric_trends` from all completed runs.
   - `recent_runs` from newest completed runs capped by `recent_limit`.
   - `baseline_to_latest_deltas` when at least two comparable completed runs
     exist.
6. The route returns the typed dashboard response.

## Database Design

Reuse `EvalDB.get_dataset` for dataset existence and metadata.

Add a dashboard-specific helper, for example:

`get_completed_evaluations_for_dashboard(dataset_id, collection)`

This should return completed evaluation rows for one dataset and collection in
`created_at ASC` order and should exclude per-query `results` from row
conversion. A dashboard-specific helper is preferable to changing
`get_history`, because `/evaluations/history` currently returns full evaluation
detail shape and should stay compatible.

The endpoint can compute `item_count` from the dataset `items` field after
calling `get_dataset`, or the DB helper can expose a compact dataset summary.
Either option is acceptable as long as the route returns the documented shape.

## Delta Rules

`baseline_to_latest_deltas` compares the first completed run to the latest
completed run for the requested dataset and collection.

For each metric:

- If both baseline and latest scores are present, return
  `round(latest - baseline, 6)`.
- If either score is missing, return `null` for that metric.

Return `baseline_to_latest_deltas: null` when fewer than two completed runs
exist.

This does not replace `/evaluations/compare`, which remains the endpoint for
arbitrary user-selected run comparisons.

## Metric Trends

Trend points are compact and may include all completed runs because each point
contains only:

- `evaluation_id`
- `completed_at`
- `score`

Each metric gets its own trend array. If a run is missing a score for a metric,
include the point with `score: null` so clients can preserve timeline spacing
without inventing missing values.

`recent_limit` controls only `recent_runs`, not trend points. The recent run
list is the noisier annotated section, while trend points are small enough to
return for all completed runs at current project scale.

## Testing

Add or update Python tests for:

- dashboard response model shape
- happy path with multiple completed runs
- missing `dataset_id` returns `400`
- missing `collection` returns `400`
- missing dataset returns `404`
- existing dataset with no completed runs returns an empty `200` summary
- `recent_limit` defaults to `10`
- `recent_limit` rejects values below `1` and above `100`
- `recent_runs` is capped while `metric_trends` still includes all completed
  runs
- failed or running evaluations are excluded from dashboard metrics
- response does not include per-query `results`
- missing metric scores produce null trend scores or null deltas without
  crashing
- dashboard DB helper returns completed runs in stable order without results

Existing history, compare, and evaluation detail tests should continue to pass
without contract changes.

## Verification

Before committing implementation changes, run:

```bash
make preflight-python
make preflight-security
```

This spec is documentation-only. Implementation should happen in a feature
worktree because it changes runtime API behavior.
