# Eval Experiment Source Of Truth Design

## Summary

Consolidate RAG experiment recording so `services/eval` is the only durable
source of truth for experiments. The local Go MCP service remains useful as an
agent workflow adapter, but it should create, read, update, and summarize
experiments through the Python eval API instead of keeping a second experiment
ledger in local SQLite.

This supersedes the local experiment-metadata ownership from
`2026-05-13-local-eval-mcp-adapter-design.md`. It builds on the backend ledger
from `2026-05-13-eval-experiment-ledger-design.md` and keeps frontend UI work
out of scope because issue 240 is already covering `/ai/eval`.

## Goals

- Make `services/eval` the durable owner of experiment records, labeled runs,
  conclusions, and evidence packets.
- Preserve support for local experiment execution through the MCP workflow.
- Support one baseline and multiple labeled candidate runs per experiment.
- Store enough structured evidence to explain why an experiment decision was
  made without duplicating full per-query evaluation payloads.
- Keep MCP tools agent-friendly while backing them with eval API experiment
  endpoints.
- Leave frontend implementation to issue 240 while providing a stable API
  contract for that work to consume.

## Non-Goals

- Do not modify frontend files in this slice.
- Do not add new RAG scoring metrics.
- Do not support cross-dataset or cross-collection experiments.
- Do not create a deployable MCP service.
- Do not store full evaluation results in experiment evidence.
- Do not migrate or preserve existing local MCP SQLite experiment rows unless a
  specific manual migration need is identified before implementation.

## Ownership

`services/eval` owns:

- datasets
- evaluations and their raw measured results
- experiment IDs
- experiment status and decision
- experiment hypothesis, notes, focus metric, conclusion, and evidence
- labeled experiment runs

`go/eval-mcp-service` owns:

- local stdio MCP protocol handling
- agent-oriented workflow orchestration
- request validation for required MCP tool arguments
- polling and summarization logic that reads from `services/eval`
- auth/token cache state as described by the auth integration spec

The MCP service should not own durable experiment metadata. If any local storage
remains, it should be limited to runtime concerns such as authentication cache,
not experiment records.

## Data Model

Keep the existing `datasets`, `evaluations`, `experiments`, and
`experiment_runs` tables in `services/eval`.

Formalize or add these experiment fields:

- `focus_metric`: one of `faithfulness`, `answer_relevancy`,
  `context_precision`, or `context_recall`.
- `conclusion`: final written recommendation.
- `evidence`: JSON evidence packet.
- `decision`: `keep`, `revert`, `needs_more_data`, or null.
- `status`: `planned`, `running`, `completed`, or `abandoned`.

`experiment_runs` supports multiple labeled runs for each experiment. Labels
are unique within an experiment. `baseline` is the reserved label for the
baseline run; candidate labels are caller-defined, such as `rerank_on`,
`chunk_512_overlap_80`, or `prompt_v2`.

Raw per-query output remains in `evaluations.results`. Experiment evidence
references selected query cases and evaluation IDs rather than copying every
result into the experiment row.

Suggested `evidence` shape:

```json
{
  "baseline_eval_id": "uuid",
  "candidate_eval_ids": ["uuid"],
  "focus_metric": "context_precision",
  "metric_deltas": {
    "candidate_label": {
      "faithfulness": 0.02,
      "answer_relevancy": -0.01,
      "context_precision": 0.08,
      "context_recall": 0.03
    }
  },
  "worst_cases": [
    {
      "label": "rerank_on",
      "eval_id": "uuid",
      "query": "question text",
      "metric": "context_precision",
      "score": 0.25,
      "reason": "retrieved context missed expected source"
    }
  ],
  "config_diffs": [
    {
      "label": "rerank_on",
      "summary": "rerank enabled; same collection and embedding model"
    }
  ],
  "caveats": ["small dataset size"]
}
```

The API may accept additional evidence keys for future compatibility, but the
core fields above should be documented and covered by tests.

## Workflow

The canonical experiment workflow is:

1. Create or select a golden dataset.
2. Create an experiment with name, hypothesis, dataset, collection, and focus
   metric.
3. Run or attach a completed `baseline` evaluation.
4. Run one or more labeled candidate evaluations against the same dataset and
   collection.
5. Compare candidate runs against the baseline.
6. Inspect worst cases for the focus metric and regressions in secondary
   metrics.
7. Generate an evidence packet from comparison output, selected weak cases,
   config snapshots, and caveats.
8. Mark the experiment `completed` with `decision`, `conclusion`, and
   `evidence`.

The MCP tools should follow the same flow. Existing tool names can largely
remain, but their backing behavior changes:

- `start_eval_experiment` calls `POST /experiments`.
- `list_eval_experiments` calls `GET /experiments`.
- `get_eval_experiment` calls `GET /experiments/{id}`.
- `start_eval_run` calls `POST /evaluations` with `experiment_id` and
  `experiment_label` when the run belongs to an experiment.
- `attach_eval_run` calls `POST /experiments/{id}/runs`.
- `record_eval_experiment_conclusion` calls `PATCH /experiments/{id}` with
  completed status, decision, conclusion, and evidence.

MCP comparison and worst-case tools can still compute agent-friendly summaries,
but they should fetch run data from `services/eval` by API ID or experiment
label.

## API Contract

Extend `services/eval` models and endpoints as needed:

- `CreateExperimentRequest` accepts `focus_metric`.
- `ExperimentSummary` and `ExperimentDetail` return `focus_metric`,
  `conclusion`, and `evidence`.
- `UpdateExperimentRequest` accepts `focus_metric`, `conclusion`, and
  `evidence`.
- `PATCH /experiments/{id}` can complete an experiment by setting `status`,
  `decision`, `conclusion`, and `evidence` in one request.

Existing experiment endpoints remain the API surface:

- `POST /experiments`
- `GET /experiments`
- `GET /experiments/{id}`
- `PATCH /experiments/{id}`
- `GET /experiments/{id}/runs`
- `POST /experiments/{id}/runs`
- `POST /evaluations` with optional `experiment_id` and `experiment_label`

The frontend work in issue 240 should consume these API responses directly.
This spec does not require frontend changes.

## Validation And Errors

`services/eval` enforces source-of-truth invariants:

- An experiment must reference an existing dataset.
- Every attached run must use the experiment dataset and collection.
- Labels are unique within an experiment.
- `baseline` is reserved for the baseline run.
- A completed experiment must have a `decision`.
- A completed experiment should have `conclusion` and `evidence`.
- Evidence generation requires at least one baseline and one completed
  candidate run.
- Running or failed evaluations may be attached, but generated evidence should
  use completed runs only.
- Completed experiments reject new run attachments unless reopened explicitly.

MCP code should keep only lightweight validation for missing tool arguments and
invalid metric names. It should surface eval API errors with enough context for
the agent or user to fix the request.

## Testing

Python tests should cover:

- database initialization and idempotent migrations for `focus_metric`,
  `conclusion`, and `evidence`
- model serialization and validation for experiment create/update/detail
- endpoint validation for create, attach, update, and complete experiment paths
- completed experiment requirements for decision and evidence fields
- evidence JSON round-trip behavior

Go MCP tests should cover:

- eval API client methods for experiment endpoints
- workflow methods using mocked eval API responses
- `start_eval_run` sending `experiment_id` and `experiment_label`
- conclusion recording through eval API rather than local SQLite
- removal or retirement of local experiment store behavior without changing MCP
  tool semantics

Before commit, run:

- `make preflight-python`
- `make preflight-go`

If implementation later touches only one language, the corresponding focused
preflight is sufficient for that commit.

## Open Implementation Notes

- Use a feature branch or worktree for implementation because this changes
  runtime behavior on `main`.
- Review existing local MCP SQLite tests before deleting store code. Some may
  be converted into workflow tests against mocked API clients.
- Consider a compatibility shim only if local MCP experiment rows already need
  manual migration. Otherwise, prefer removing the duplicate store cleanly.
