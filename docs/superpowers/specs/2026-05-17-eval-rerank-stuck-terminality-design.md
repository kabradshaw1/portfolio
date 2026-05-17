# Eval Rerank Stuck Terminality Design

## Context

`services/eval` starts eval runs by inserting an `evaluations` row with
`status='running'`, then schedules `_run_evaluation_task` as a FastAPI
background task. The task snapshots config, calls the RAG service through
`RAGClient`, judges each row, and finally calls `complete_evaluation`. Ordinary
exceptions are caught and persisted through `fail_evaluation`.

The fresh rerank experiment exposed an operator-facing gap. Baseline completed
quickly. The rerank-on candidate initially stayed `running` with
`aggregate_scores`, `results`, and `error` all null, then later completed after
the MCP wait had already timed out. That means two cases must be distinguished:

- A long but legitimate rerank run that is still making progress.
- A stuck or orphaned run that will never call `complete_evaluation` or
  `fail_evaluation`.

## Problem

The eval API currently has no server-side max runtime, stale-run recovery, or
per-run heartbeat/progress. A run can remain `running` indefinitely if the
background task is cancelled, the process exits after row creation, an awaited
dependency never returns, or failure persistence is bypassed. `RAGClient` has a
per-request HTTP timeout, but there is no whole-run timeout across all dataset
items and judge calls.

`go/eval-mcp-service` already has a wait timeout, but that timeout only stops
the MCP tool call. It does not change the eval row and currently returns a
minimal message with the latest status only. For rerank runs, the MCP default
timeout can be shorter than a valid full eval run.

## Goals

- Ensure rerank-on eval failures become terminal `failed` rows with useful
  errors.
- Persist failures for upstream HTTP errors, per-request timeouts, whole-run
  timeouts, and task cancellation where the process is still alive.
- Recover stale `running` rows on eval API startup.
- Keep MCP wait behavior non-mutating while improving timeout diagnostics.
- Add focused tests proving failure terminality and useful error text.

## Non-Goals

- Do not replace FastAPI background tasks with a durable queue in this change.
- Do not broaden into rate limiting, observability allowlisting, or production
  alerting.
- Do not add detailed per-item progress in the minimal fix; leave it as a
  follow-up.

## Design

Add two eval API settings:

- `eval_run_max_seconds`: whole-run timeout for one evaluation task. Default
  `900.0` seconds.
- `eval_stale_grace_seconds`: startup recovery grace beyond max runtime.
  Default `300.0` seconds.

Wrap the awaited `run_evaluation(...)` call in `asyncio.wait_for` using
`eval_run_max_seconds`. On `asyncio.TimeoutError`, call `fail_evaluation` with
an error that includes eval id, collection, rerank flag, max runtime, and
elapsed seconds. On ordinary exceptions, keep the existing failure persistence
but enrich the error with the same run context. On `asyncio.CancelledError`,
attempt to persist a cancellation failure, then re-raise so server shutdown
semantics are preserved.

Add `EvalDB.fail_stale_running_evaluations(max_age_seconds: float) -> int`.
It computes a UTC cutoff, marks only `status='running'` rows older than the
cutoff as `failed`, sets `completed_at`, stores a stale-recovery error message,
commits, and returns the affected row count.

Register a startup hook `recover_stale_evaluations()` that calls the DB method
with `eval_run_max_seconds + eval_stale_grace_seconds`. This makes stale runs
terminal after process restarts without changing fresh in-progress runs.

Update `go/eval-mcp-service` wait timeout formatting only. `WaitForRun` should
still return a timeout and latest run snapshot without mutating eval state, but
the error should include latest status, created_at, completed_at when present,
collection, row error when present, and a sentence that the eval API run may
still finish after the MCP wait timeout.

## Error Handling

Expected terminal failures:

- Upstream HTTP status failures from `/search` or `/chat`.
- `httpx.TimeoutException` from RAG calls.
- Whole-run timeout from `asyncio.wait_for`.
- Judge failures already wrapped by `EvaluationError`.
- Task cancellation while the process is still able to write to SQLite.
- Stale `running` rows discovered on startup.

Process death between row creation and background task completion cannot be
caught in-process. Startup stale recovery is the intended safety net.

## Tests

Python eval API:

- `_run_evaluation_task` persists `failed` for an upstream HTTP timeout and
  includes eval id, collection, rerank flag, and original error text.
- `_run_evaluation_task` persists `failed` for whole-run timeout with useful
  max-runtime context.
- `_run_evaluation_task` attempts `failed` persistence on
  `asyncio.CancelledError`.
- `EvalDB.fail_stale_running_evaluations` marks only old running rows failed.
- `recover_stale_evaluations` calls stale recovery with max runtime plus grace.

Go MCP service:

- `WaitForRun` timeout error includes latest run metadata and clearly states
  that MCP timeout does not necessarily mean the eval API run has failed.

## Follow-Ups

- Add per-item progress columns or a separate progress table if eval runs stay
  hard to reason about during long rerank runs.
- Consider increasing `EVAL_MCP_WAIT_TIMEOUT` after measuring normal rerank
  runtimes.
- Add observability allowlisting for eval services as a separate operational
  change.
