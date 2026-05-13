# Eval Baseline Validation

- **Date:** 2026-05-12
- **Status:** Accepted
- **Related PR:** #263
- **Builds on:** [rag-tracking-foundation.md](rag-tracking-foundation.md)

## Context

The eval service stores `baseline_eval_id` on an evaluation run so the
dashboard can show score deltas against a deliberate baseline. The original
tracking foundation intentionally treated stale or wrong baseline pointers as
display concerns: the API accepted any baseline id and persisted it with the new
run.

That was too loose once baseline deltas became part of the experiment workflow.
A missing, running, failed, cross-dataset, or cross-collection baseline can make
the dashboard imply that two runs are comparable when they are not. The baseline
relationship is request-time comparison policy, not a persistence constraint:
SQLite can express existence with a foreign key, but it cannot cleanly enforce
"completed, same dataset, same effective collection" for this workflow.

## Decision

`POST /evaluations` validates `baseline_eval_id` before inserting the new
evaluation row. The endpoint still loads the requested dataset first, preserving
the existing `404 Dataset not found` behavior and ensuring missing datasets are
not masked by baseline errors.

When a baseline id is supplied, the eval API:

1. Looks up the referenced run with `EvalDB.get_evaluation()`.
2. Returns `404 Baseline evaluation not found` if it does not exist.
3. Returns `400 Baseline evaluation must be completed` unless the baseline
   status is `completed`.
4. Returns `400 Baseline evaluation must use the same dataset` when the
   baseline dataset differs from the requested dataset.
5. Returns `400 Baseline evaluation must use the same collection` when the
   baseline collection differs from the new run's effective collection.

The effective collection is computed once as `body.collection or "documents"`
and is used for both baseline comparison and evaluation creation. The background
runner still receives the original request collection so its existing defaulting
behavior remains unchanged.

No schema, migration, comparison endpoint, history endpoint, or frontend change
is needed. The existing `baseline_eval_id` column remains the storage model; the
new behavior is an API-layer guard before persistence.

## Consequences

Positive outcomes:

- Dashboard deltas are only created from completed runs that are comparable by
  dataset and collection.
- Bad baseline requests fail before a new evaluation row or background task is
  created.
- Error responses distinguish unknown baselines from incomparable baselines.
- No-baseline evaluation runs keep the existing `202` behavior.
- The implementation is small and testable at the API layer.

Trade-offs:

- The API now rejects requests that previously stored arbitrary baseline ids.
  This is intentional, but callers must create or select a valid completed
  baseline first.
- Legacy rows with a null or otherwise nonstandard collection value are not
  normalized during validation; the stored baseline collection must match the new
  run's effective collection.
- The database still allows invalid historical pointers because this decision
  does not add schema constraints or backfill old data.

This ADR supersedes the earlier "stale baseline pointers are harmless data"
portion of the tracking foundation decision. Baseline pointers are still stored
on the evaluation row, but new writes are now validated before persistence.
