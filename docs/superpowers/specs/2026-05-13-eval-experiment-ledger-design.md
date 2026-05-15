# Eval Experiment Ledger Design

## Summary

Add first-class RAG experiment records to the Python eval service so evaluation
runs can be grouped into durable measurement narratives: hypothesis, baseline,
candidate runs, results, and decision. The ledger is backend-owned and
additive; existing dataset, evaluation, history, and comparison endpoints remain
valid.

This closes the ownership gap between ad hoc run notes and the planned local
eval MCP workflow. The MCP can initially keep local metadata, then migrate to
these backend experiment endpoints without duplicating durable experiment
state.

## Goals

- Store RAG experiments as first-class eval API records.
- Support experiments that are still in progress.
- Allow running, completed, and failed evaluations to be attached to an
  experiment.
- Preserve fixed dataset and collection boundaries for comparable runs.
- Give agents and scripts stable labels for attached runs such as `baseline`,
  `rerank_on`, or `top_k_3`.
- Keep detailed per-query payloads behind `GET /evaluations/{id}`.

## Non-Goals

- Do not replace the existing `/evaluations/history` or `/evaluations/compare`
  endpoints.
- Do not build frontend experiment management in the first slice.
- Do not add new RAG scoring metrics in this issue.
- Do not require the eval MCP migration to happen in the same change.
- Do not support cross-dataset or cross-collection experiments.

## Data Model

Add an `experiments` table:

- `id`
- `name`
- `hypothesis`
- `dataset_id`
- `collection`
- `baseline_eval_id`
- `status`: `planned`, `running`, `completed`, or `abandoned`
- `decision`: `keep`, `revert`, `needs_more_data`, or null
- `notes`
- `created_at`
- `updated_at`

Add an `experiment_runs` table:

- `experiment_id`
- `evaluation_id`
- `label`
- `notes`
- `created_at`

Rules:

- Experiment dataset and collection are fixed after creation.
- Attached evaluations must exist and match the experiment dataset and
  collection.
- Attached evaluations may be `running`, `completed`, or `failed`.
- Run labels are unique within an experiment.
- If `baseline_eval_id` is provided, it must match the experiment dataset and
  collection. The baseline can be attached automatically with label `baseline`.
- A final decision is only accepted when the experiment is completed, or when
  the same update sets status to `completed`.
- Completed experiments are read-only for new run attachments unless reopened
  to `running`.

## API Surface

Add these endpoints:

- `POST /experiments`
  - Creates an experiment.
  - Accepts `name`, `hypothesis`, `dataset_id`, `collection`, optional
    `baseline_eval_id`, optional `notes`, and optional initial `status`.
  - Defaults status to `planned`.
  - Allows initial status `planned` or `running`; callers cannot create an
    experiment directly as `completed` or `abandoned`.

- `GET /experiments`
  - Lists experiment summaries newest first.
  - Supports optional filters: `dataset_id`, `collection`, and `status`.

- `GET /experiments/{experiment_id}`
  - Returns experiment detail plus attached run summaries.
  - Does not include full per-query evaluation results.

- `PATCH /experiments/{experiment_id}`
  - Updates `status`, `decision`, `notes`, `hypothesis`, and
    `baseline_eval_id`.
  - Does not allow changing dataset or collection.

- `GET /experiments/{experiment_id}/runs`
  - Returns attached evaluation summaries ordered by attachment time.

- `POST /experiments/{experiment_id}/runs`
  - Attaches an existing evaluation.
  - Accepts `evaluation_id`, `label`, and optional `notes`.

Extend `POST /evaluations` with optional `experiment_id` and
`experiment_label`. When supplied, the eval service creates the evaluation and
immediately attaches it to the experiment while it is still running.

Validation behavior:

- Unknown experiment: `404`.
- Unknown evaluation: `404`.
- Dataset or collection mismatch: `400`.
- Duplicate run label within an experiment: `409`.
- Decision without completed status: `400`, unless the same patch sets status
  to `completed`.
- Attach to completed experiment: `400`, unless the experiment is reopened.

## Response Shapes

Add Pydantic models:

- `ExperimentSummary`
- `ExperimentDetail`
- `CreateExperimentRequest`
- `UpdateExperimentRequest`
- `AttachExperimentRunRequest`
- `ExperimentRun`

`ExperimentDetail` includes the experiment fields and a compact `runs` list.
Each attached run includes the label, attachment notes, attachment timestamp,
and a compact evaluation summary.

Example detail response:

```json
{
  "id": "exp-123",
  "name": "precision tuning",
  "hypothesis": "Reranking will improve context precision",
  "dataset_id": "ds-123",
  "collection": "documents",
  "baseline_eval_id": "eval-base",
  "status": "running",
  "decision": null,
  "notes": "first rerank experiment",
  "created_at": "2026-05-13T10:00:00+00:00",
  "updated_at": "2026-05-13T10:00:00+00:00",
  "runs": [
    {
      "evaluation_id": "eval-base",
      "label": "baseline",
      "notes": "rerank off",
      "attached_at": "2026-05-13T10:01:00+00:00",
      "evaluation": {
        "id": "eval-base",
        "dataset_id": "ds-123",
        "status": "completed",
        "collection": "documents",
        "aggregate_scores": {
          "context_precision": 0.31
        },
        "created_at": "2026-05-13T09:50:00+00:00",
        "completed_at": "2026-05-13T09:55:00+00:00",
        "notes": "baseline run",
        "config": null,
        "baseline_eval_id": null
      }
    }
  ]
}
```

## MCP Relationship

The current eval MCP can be finished with its local SQLite ledger if that work
is already nearly complete. After this backend ledger exists, the MCP should
prefer the eval API for durable experiment records and use local storage only
for cache or session convenience.

The MCP should eventually map its tools onto these backend operations:

- `start_eval_experiment` -> `POST /experiments`
- `start_eval_run` with label -> `POST /evaluations` with `experiment_id` and
  `experiment_label`
- `attach_eval_run` -> `POST /experiments/{id}/runs`
- `get_eval_experiment` -> `GET /experiments/{id}`
- `record_eval_experiment_conclusion` -> `PATCH /experiments/{id}`

## Testing

Add DB tests for:

- Create, get, list, and update experiments.
- Baseline validation.
- Attach running, completed, and failed evaluations.
- Reject unknown evaluations.
- Reject dataset or collection mismatches.
- Reject duplicate labels.
- Reject final decisions unless status is or becomes `completed`.
- Reject attaching new runs to completed experiments unless reopened.
- Idempotent schema initialization after experiment tables already exist.

Add API tests for:

- Each new endpoint.
- Filtered experiment listing.
- Starting an evaluation with `experiment_id` and `experiment_label`.
- Validation errors and status codes.
- Existing evaluation create, history, and compare behavior staying unchanged.

Verification before implementation commit:

```bash
make preflight-python
make preflight-security
```

## Decisions

- `POST /experiments` accepts initial status `planned` or `running` only.
  This supports in-progress agent workflows without allowing callers to create
  already-final experiment records.
- `baseline_eval_id` is nullable. Agents can create an experiment before the
  baseline run exists or before a running baseline has completed.
