# Eval Baseline Validation Design

## Issue

GitHub issue: https://github.com/kabradshaw1/portfolio/issues/239

The eval service accepts `baseline_eval_id` when starting an evaluation run, but
the current `POST /evaluations` flow stores the id without validating that the
referenced run is a meaningful baseline. This can create misleading dashboard
deltas if a run is compared to a missing, incomplete, failed, cross-dataset, or
cross-collection baseline.

## Goals

- Validate `baseline_eval_id` before creating a new evaluation run.
- Reject unknown baseline ids with a clear `404` response.
- Reject incomparable baselines with clear `400` responses.
- Require baselines to be completed runs.
- Require baselines to use the same dataset as the new run.
- Require baselines to use the same effective collection as the new run.
- Preserve existing no-baseline behavior.
- Add focused API tests for the validation behavior.

## Non-Goals

- Do not add or change database schema.
- Do not change the stored `baseline_eval_id` field.
- Do not change comparison or history endpoint behavior.
- Do not add frontend error presentation work.
- Do not validate metric compatibility beyond the existing dataset, collection,
  and completed-status checks.

## Recommended Approach

Add a small API-layer validation helper in `services/eval/app/main.py`, called
from `POST /evaluations` after the requested dataset is confirmed to exist and
before `EvalDB.create_evaluation()` is called.

This keeps HTTP status-code mapping close to the endpoint, uses the existing
`EvalDB.get_evaluation()` method, and avoids a database abstraction that would
only wrap already-loaded row data. The rule is request-time comparison policy,
not a persistence-model change.

## Alternatives Considered

### Database Validation Helper

`EvalDB` could expose a method such as `validate_baseline(...)` or
`get_valid_baseline(...)`. That would centralize the rule outside the endpoint,
but the database layer would still need policy-specific return values or
exceptions so the endpoint could produce `400` versus `404` responses. For the
current scope, that adds ceremony without improving data integrity.

### SQLite Constraints

A foreign key can express that a referenced evaluation exists, but it cannot
cleanly express completed status, same dataset, and same effective collection.
Those checks are clearer and more testable as explicit application logic.

## API Behavior

`POST /evaluations` computes the effective collection once:

```python
collection = body.collection or "documents"
```

If `baseline_eval_id` is omitted, the endpoint keeps the current behavior:
create the run, store `baseline_eval_id=None`, and schedule the background task.

If `baseline_eval_id` is provided, the endpoint validates the baseline before
creating the new run:

1. Fetch the baseline with `db.get_evaluation(body.baseline_eval_id)`.
2. If no row is returned, raise `HTTPException(status_code=404, detail=...)`.
3. If `baseline["status"] != "completed"`, raise `400`.
4. If `baseline["dataset_id"] != body.dataset_id`, raise `400`.
5. If `baseline["collection"] != collection`, raise `400`.
6. If all checks pass, call `db.create_evaluation(...)` with the original
   `baseline_eval_id`.

The background task must not be scheduled when validation fails.

Recommended response details:

- Unknown baseline: `Baseline evaluation not found`
- Non-completed baseline: `Baseline evaluation must be completed`
- Dataset mismatch: `Baseline evaluation must use the same dataset`
- Collection mismatch: `Baseline evaluation must use the same collection`

## Components

### `start_evaluation`

The endpoint should:

- Load the requested dataset first and keep the existing `Dataset not found`
  behavior.
- Compute `collection = body.collection or "documents"` once.
- Call the baseline helper only when `body.baseline_eval_id` is not `None`.
- Pass the computed `collection` into `db.create_evaluation(...)`.
- Continue passing the original `body.collection` to `_run_evaluation_task` so
  the background runner keeps its current defaulting behavior.

### `_validate_baseline`

Add a private async helper in `main.py`:

```python
async def _validate_baseline(
    db: EvalDB,
    baseline_eval_id: str,
    dataset_id: str,
    collection: str,
) -> None:
    ...
```

The helper should raise `HTTPException` for invalid baselines and return `None`
for valid baselines.

## Error Handling

Validation errors are client errors because the request references an invalid
baseline relationship. They should be raised before any new evaluation row is
inserted.

Use `404` only when the baseline id does not exist. Use `400` when the baseline
exists but is not comparable.

## Test Plan

Update `services/eval/tests/test_main.py` with focused API tests:

- A valid completed baseline with the same dataset and default `documents`
  collection returns `202` and persists `baseline_eval_id`.
- An unknown baseline returns `404` and does not call `create_evaluation`.
- A running or failed baseline returns `400` and does not call
  `create_evaluation`.
- A completed baseline from a different dataset returns `400`.
- A completed baseline from a different collection returns `400`.
- A request with no baseline keeps the existing `202` behavior.
- A request for a missing dataset still returns `404` before baseline
  validation.

Existing DB tests are sufficient for persistence because this design keeps the
comparison policy in the API layer. No migration tests are needed.

## Verification

Before committing the implementation, run:

```bash
make preflight-python
make preflight-security
```
