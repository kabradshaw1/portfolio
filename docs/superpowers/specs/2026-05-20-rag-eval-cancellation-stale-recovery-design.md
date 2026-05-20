# RAG Eval Cancellation And Stale Recovery Design

## Goal

Add explicit cancellation and stronger stale work recovery to the RabbitMQ-backed
RAG eval item worker.

The system should let operators cancel queued or running evals, stop cancelled
item work safely, preserve retryable stale item work, repair terminal run
aggregation when item rows already contain the truth, and expose evidence that
distinguishes stale, cancelled, failed, and retryable states.

## Context

`services/eval` already uses durable evaluation item rows and RabbitMQ messages
for item-level RAG eval work. The eval API creates a queued run and item rows,
publishes one message per item, and the worker claims item leases before running
search, chat, judge scoring, and local metrics for one dataset item.

The current implementation has the right base ownership model, but cancellation
and recovery are still incomplete:

- There is no explicit cancel endpoint or `cancelled` item state.
- The worker claims items before checking whether the parent run is already
  terminal or cancelled.
- Startup recovery resets expired item leases but does not republish reset work.
- Startup recovery can still fail stale running runs wholesale, even when item
  work remains retryable.
- MCP run evidence reports stale running runs, but does not clearly distinguish
  queued stale, cancelled, failed, and retryable item states.

## Recommended Approach

Use cooperative cancellation at item boundaries and before expensive stages.

Cancellation should be durable in the eval database. RabbitMQ remains a work
transport only; duplicate or stale messages are safe because item rows and parent
run status remain the source of truth. The worker should not try to interrupt an
already in-flight HTTP or LLM request in this slice. Existing upstream timeouts
bound that work. The worker should check cancellation before starting the next
expensive call and stop there.

This keeps the first cancellation slice deterministic and testable while still
preventing new expensive work after an operator cancels a run.

## Run And Item State

Add `cancelled` as a terminal status for both evaluations and evaluation items.

Terminal evaluation statuses become:

- `completed`
- `completed_with_failures`
- `failed`
- `cancelled`

Terminal item statuses become:

- `completed`
- `failed`
- `cancelled`

Operator cancellation wins over partial completion. If a run is marked
`cancelled`, later aggregation or repair should not convert it to
`completed_with_failures` because some items completed before the cancellation.

## Cancellation API

Add `POST /evaluations/{eval_id}/cancel`.

The endpoint should:

1. Return `404` when the run does not exist.
2. Return `409` when the run is already terminal: `completed`,
   `completed_with_failures`, `failed`, or `cancelled`.
3. For `queued` or `running`, mark the evaluation `cancelled`, set
   `completed_at`, and store a bounded reason such as
   `evaluation cancelled by operator`.
4. Mark all non-terminal items for the run as `cancelled`.
5. Clear cancelled item lease fields.
6. Return the updated evaluation detail, including item counts.

Do not require a cancellation reason in the first slice. It can be added later
without changing the core state model.

## Worker Behavior

The worker should check cancellation before it starts or continues expensive
work.

Before claiming an item:

1. Load the parent evaluation by `evaluation_id`.
2. If the evaluation is missing, keep the existing permanent failure behavior.
3. If the evaluation status is `cancelled`, mark the item `cancelled` if it is
   still non-terminal and ack the message.

After claiming an item:

1. Re-check the parent evaluation status.
2. If it is `cancelled`, mark the claimed item `cancelled`, clear its lease, and
   ack the message.
3. If it is still `queued`, mark the parent evaluation `running`.

Before expensive stages:

1. Check cancellation before search.
2. Check cancellation before chat.
3. Check cancellation before judge scoring.

If cancellation is observed at any of those boundaries, the item should be
marked `cancelled` and the message should be acked. A cancelled item should not
be retried, marked failed, or rejected to the DLQ.

The worker should not cancel an already in-flight upstream request in this
design. That can be a later enhancement if single upstream calls prove long
enough to make immediate interruption operationally necessary.

## Recovery

Recovery should repair item work before deciding the parent run is failed.

Expired running item leases:

- If `attempt_count < max_attempts`, reset the item to `queued`, clear lease
  fields, preserve bounded error evidence, and republish an item message.
- If attempts are exhausted, mark the item `failed` with a bounded stale lease
  error and do not republish.

Queued items without live messages:

- Republish queued items for non-terminal, non-cancelled runs during startup or
  periodic repair.
- Publishing is idempotent because `claim_evaluation_item` remains the
  ownership gate.

Terminal item repair:

- For any non-terminal run where all items are terminal, run finalization.
- If all items completed, mark the run `completed`.
- If some items failed and at least one completed, mark the run
  `completed_with_failures`.
- If every item failed, mark the run `failed`.
- If the parent run is already `cancelled`, leave it `cancelled`.

Stale parent runs:

- Do not automatically mark a parent run `failed` just because its row is old.
- Report stale queued or running conditions through evidence.
- Let unrecoverable item exhaustion or terminal item repair drive final failure.

## Evidence

`GET /evaluations/{eval_id}` should expose item counts for:

- `queued`
- `running`
- `completed`
- `failed`
- `cancelled`
- `retryable`
- `stale`
- `total`

`retryable` should be derived from item state and attempts remaining. `stale`
should identify queued or running work older than the configured recovery
threshold or with expired leases. Evidence must not expose raw query text beyond
the existing explicit result payloads.

`go/eval-mcp-service` should translate the richer eval API response into clearer
operator next steps:

- `cancelled`: explain that the run was cancelled and a new run is required if
  evaluation is still needed.
- stale with retryable work: explain that recovery should reset or republish
  item work and suggest checking eval worker logs if counts do not change.
- stale without retryable work: explain that item attempts may be exhausted or
  terminal repair may be needed.
- `failed`: keep current failure triage guidance.
- `completed_with_failures`: keep current partial-result guidance.

## Testing

Add focused tests for:

- Cancelling queued and running runs through the API.
- Returning conflict when cancelling terminal runs.
- Marking non-terminal items `cancelled` and clearing leases.
- Worker acking cancelled work without retry or DLQ.
- Worker checking cancellation before expensive stages.
- Expired leases resetting and republishing when attempts remain.
- Expired leases becoming failed when attempts are exhausted.
- Queued item republish for non-terminal, non-cancelled runs.
- All-terminal item repair finalizing a non-terminal run.
- Evidence distinguishing stale, retryable, cancelled, and failed states.

## Out Of Scope

This design does not add:

- In-flight HTTP or LLM request interruption.
- User-supplied cancellation reasons.
- A new dashboard UI for cancellation.
- RabbitMQ payloads containing raw queries, expected answers, secrets, or model
  credentials.
