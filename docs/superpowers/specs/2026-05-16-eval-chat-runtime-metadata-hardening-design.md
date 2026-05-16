# Eval Chat Runtime And Metadata Hardening Design

## Purpose

Make Python eval runs predictable during baseline-versus-rerank experiments.
The eval API should reject impossible runs before background execution, capture
per-run metadata that explains the result, and remain ready to accept API
traffic during long-running eval and rerank work.

## Background

The corrected rerank experiment used the real Qdrant collection `documents`,
but external polling later received nginx `503` responses while the eval pod was
NotReady. Runtime evidence showed readiness probe failures for eval and one
slow rerank search request through chat. The eval service currently checks chat
from `/health`, which makes readiness sensitive to downstream latency during
active experiments.

Run config capture also records chat's global `rerank_enabled` setting, but
that does not say whether a specific eval run requested reranking. Baseline and
rerank runs need explicit per-run metadata for later comparison.

## Scope

Implement this primarily in `services/eval`.

Include:

- preflight validation that the requested retrieval collection exists before an
  evaluation is accepted
- run config metadata that records per-run rerank intent and effective
  collection
- health/readiness behavior that reflects eval process readiness without being
  overly dependent on chat health
- direct eval API safeguards for comparing non-completed runs
- focused tests around startup validation, metadata capture, health behavior,
  and comparison semantics

Touch `services/chat` or `services/ingestion` only if a small compatibility
change is required. The expected path is to reuse ingestion `/collections` and
`/collections/{name}/config`.

Exclude:

- Go MCP service changes
- a full worker queue or distributed job system
- Kubernetes resource tuning, replica changes, or manifest updates
- generated dataset creation

## Architecture

Add small eval-side helpers rather than expanding route handlers:

- A collection validation helper calls ingestion `/collections` and confirms
  the requested collection exists.
- Existing config capture is extended to include eval-request metadata such as
  `requested_rerank` and `effective_collection`.
- Eval health separates local process readiness from downstream dependency
  status.
- Compare logic rejects incomplete runs before calculating deltas.

This keeps the Python service changes focused and lets MCP workflow reliability
proceed independently in a separate branch.

## Run Startup Validation

`POST /evaluations` should validate the retrieval collection after dataset,
baseline, and experiment consistency checks, but before creating the evaluation
row or scheduling the background task.

Behavior:

- calculate `effective_collection` as `body.collection or "documents"`.
- call ingestion `/collections`.
- confirm a collection with that name exists.
- if ingestion is unreachable, return a clear dependency error and do not
  create the evaluation row.
- if the collection is missing, return a 400 or 422 with a message like
  `retrieval collection "product-docs-rag-v1" does not exist`.
- pass `effective_collection` to the background task so defaulting is
  consistent between validation, persistence, config capture, and RAG calls.

Missing ingestion metadata from `/collections/{name}/config` should not block
the run. Qdrant collection existence and local ingestion metadata are different
facts.

## Run Config Metadata

Extend the config stored on each evaluation with:

- `requested_rerank`: boolean from the eval request
- `effective_collection`: collection used after defaulting
- `chat`: existing chat `/config` response when available
- `collection`: existing ingestion collection metadata when available
- `_capture_error` or structured warning data for failed config capture

The baseline run should store `requested_rerank: false`. The rerank candidate
should store `requested_rerank: true`. This removes ambiguity from chat's
global `rerank_enabled` setting.

The config capture function should continue returning a dict and should not
raise for metadata capture failures.

## Health And Readiness

Eval readiness should prove the eval API can accept and manage requests, not
that chat is always immediately healthy.

Recommended behavior:

- `/health` returns HTTP 200 when the eval process is up and its local database
  can be reached.
- the response includes chat dependency status when checked successfully or a
  degraded dependency field when chat is unavailable.
- downstream chat health failure does not by itself make eval NotReady.
- any dependency check should use a short timeout.

If the service later needs a stricter dependency endpoint, add a separate route
instead of using the readiness endpoint for deep dependency checks.

## Comparison Semantics

`GET /evaluations/compare` should reject runs that are not completed.

Behavior:

- preserve current cardinality and same-dataset validation.
- after fetching runs, inspect each status.
- if any status is not `completed`, return HTTP 400.
- the response detail should name the run IDs and statuses.
- calculate deltas only for completed runs.

This backs up MCP completed-only comparison with an API-level invariant.

## Error Handling

Important cases:

- missing dataset remains `404 Dataset not found`
- baseline and experiment dataset or collection mismatches keep existing
  validation behavior
- missing retrieval collection is a client-correctable request error
- ingestion connection failures are reported as dependency failures and do not
  create a run
- missing collection metadata is recorded in config capture warnings and does
  not fail the run
- non-completed compare requests report invalid run IDs and statuses

## Testing

Add Python tests for:

- `POST /evaluations` rejects a missing retrieval collection before
  `db.create_evaluation`
- `POST /evaluations` accepts an existing collection and passes the effective
  collection to the background task
- default collection validation uses `documents`
- ingestion dependency failure does not create an evaluation
- config capture includes `requested_rerank: false` for baseline runs
- config capture includes `requested_rerank: true` for rerank runs
- missing collection metadata produces capture warning metadata but does not
  raise
- `/health` remains HTTP 200 when chat is degraded but local eval state is
  reachable
- `/evaluations/compare` rejects running runs
- `/evaluations/compare` rejects failed runs
- completed-only compare still returns deltas as before

Verification target:

```bash
make preflight-python
make preflight-security
```

During implementation, focused tests are acceptable before full preflight:

```bash
pytest services/eval/tests -q
```

## Implementation Notes

The implementation branch should be a feature worktree because this changes
runtime service behavior. Target the PR to `qa`.

Keep the implementation narrow. Do not introduce a job queue, new persistence
tables, or Kubernetes resource changes in this spec. Those can follow if the
readiness and validation fixes are not enough for larger eval workloads.
