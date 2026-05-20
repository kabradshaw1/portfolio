# RAG Eval RabbitMQ Item Worker Design

## Goal

Make the RAG eval platform more production-grade by moving evaluation
execution out of FastAPI in-process background tasks and into durable,
observable RabbitMQ-backed item work.

The first implementation should teach and demonstrate workflow reliability in
AI systems: idempotent job ownership, bounded retry, DLQ handling, partial
progress, stale work recovery, and operational evidence. It should not add
Kafka, LangGraph, or a full dashboard in the first slice.

## Current State

`services/eval` is the source of truth for datasets, evaluations, experiments,
run labels, decisions, conclusions, config capture, scores, and result
persistence. `POST /evaluations` creates an evaluation row and uses FastAPI
`BackgroundTasks` to run the evaluation asynchronously inside the API process.

The current run loop is sequential at the dataset item level:

1. Capture chat and ingestion configuration for the run.
2. For each golden item, call chat `/search`.
3. For each golden item, call chat `/chat`.
4. Judge generation quality with the configured eval judge model.
5. Compute local context precision and recall.
6. Store aggregate scores and per-query results on the evaluation row.

This works for small local experiments, but the work is tied to the API process
lifecycle. If the API process exits while a background task is running, durable
state only sees a stale `running` evaluation until recovery marks it failed.
There is no durable item state, no item-level retry boundary, no partial result
visibility, and no DLQ for poison work.

`go/eval-mcp-service` already treats eval as an asynchronous API. It starts
runs, polls status, fetches evidence, compares completed runs, and summarizes
experiments. The MCP does not need to become a queue consumer; it should remain
a workflow adapter over the eval API.

## Recommended Approach

Use RabbitMQ to execute one durable message per eval item.

The eval API remains the source of truth. RabbitMQ is a work transport, not the
database of record. Workers consume item messages, claim item rows
idempotently, execute the RAG and judge steps for one item, persist the item
result, and trigger run aggregation when all items reach terminal state.

This is intentionally more complex than a whole-run queue, because the goal is
workflow reliability rather than simply replacing `BackgroundTasks`.
Item-level work exposes the practical AI-platform problems worth learning:
expensive upstream calls, provider timeouts, partial failures, duplicate
delivery, retry policy, stuck item recovery, and progress evidence.

## Run And Item State

Add durable item state to the eval database.

Evaluation run statuses:

- `queued`: run accepted and item rows created.
- `running`: at least one item has been claimed or completed.
- `completed`: every item completed and aggregate scores were stored.
- `completed_with_failures`: at least one item failed permanently, but enough
  successful items exist to report partial aggregate scores.
- `failed`: the run cannot produce trustworthy aggregate results.
- `cancelled`: reserved for a later cancellation feature.

Evaluation item statuses:

- `queued`: item row exists and a message should be available or republished.
- `running`: a worker has claimed the item lease.
- `completed`: item result and scores are stored.
- `failed`: item exhausted retry policy or hit a non-retryable error.

Each item row should include:

- `id`
- `evaluation_id`
- `item_index`
- `query`
- `expected_answer`
- `expected_sources`
- `status`
- `attempt_count`
- `max_attempts`
- `lease_owner`
- `lease_expires_at`
- `last_error`
- `result`
- `scores`
- `score_reasons`
- `started_at`
- `completed_at`
- `created_at`
- `updated_at`

The first implementation can keep raw query and expected answer in SQLite
because eval results already expose raw queries for explicit run review. Logs
and metrics should continue to avoid raw query text.

## Message Contract

Use one RabbitMQ queue for eval item work in the first slice:
`eval.item.requested`.

Message body:

```json
{
  "message_version": 1,
  "evaluation_id": "uuid",
  "item_id": "uuid",
  "item_index": 0,
  "attempt": 1
}
```

The message must not contain raw query text, expected answer text, API keys, or
model secrets. Workers load item and run configuration from the eval database.
This keeps RabbitMQ payloads small and makes redelivery safe.

The queue should be durable, messages should be persistent, and workers should
ack only after durable item status is written. Worker crashes before ack should
produce redelivery. Duplicate redelivery must be safe because item claiming is
idempotent.

## API Flow

`POST /evaluations` should:

1. Validate dataset, collection, baseline, experiment, and answer model override
   using the current rules.
2. Capture the intended run request fields.
3. Create the evaluation row with status `queued`.
4. Create one eval item row per dataset item.
5. Publish one persistent RabbitMQ message per item.
6. Return `202` with the run id and status `queued`.

If item row creation succeeds but publishing fails, the run should remain
recoverable. A startup or periodic repair path should republish queued items
that do not have terminal state. Do not mark a run failed only because publish
failed transiently after the database transaction succeeded.

## Worker Flow

The worker process should:

1. Consume `eval.item.requested` with bounded prefetch.
2. Load the item and evaluation row from the eval API database layer.
3. Attempt to claim the item with a lease using a conditional update.
4. If the item is already completed or failed, ack the message.
5. If another worker owns an unexpired lease, nack/requeue or delay retry.
6. Mark the parent evaluation `running` if it is still `queued`.
7. Execute search, chat, judge, and local scoring for exactly one item.
8. Store item result, scores, reasons, usage, retrieval metadata, and status.
9. Ack the message after the item update commits.
10. Ask the aggregation path to finalize the run if all items are terminal.

The worker should create its own `RAGClient` and judge provider clients using
the existing configuration patterns from `services/eval`. It should reuse the
current scoring functions instead of duplicating eval metric logic.

## Aggregation

Aggregation should be deterministic and idempotent. Multiple workers may try
to aggregate the same run after completing different items.

The aggregation path should:

1. Load all item statuses for the evaluation.
2. Return without action if any item is still `queued` or `running`.
3. Compute aggregate scores from completed items only.
4. If all items completed, mark the run `completed`.
5. If some items failed and at least one completed, mark the run
   `completed_with_failures` and store partial aggregate scores plus failure
   counts.
6. If every item failed, mark the run `failed`.
7. Persist per-query results in the same shape the current API returns, using
   item rows as the source.

`compare_evaluations` should initially require `completed` runs only. A later
explicit design can decide whether `completed_with_failures` is comparable.
This prevents partial evals from silently entering experiment conclusions.

## Retry And DLQ

Retries should be bounded and explicit.

Retryable failures:

- chat `/search` timeout or 5xx
- chat `/chat` timeout or 5xx
- judge timeout or transient provider error
- RabbitMQ connection interruption before ack

Non-retryable failures:

- invalid run or item state
- missing dataset item
- invalid collection after request validation
- answer model override resolution errors
- judge response that repeatedly returns invalid JSON after bounded attempts

Each failed attempt increments `attempt_count` and records a bounded
`last_error` with error type, upstream service, operation, and retryability.
When attempts are exhausted, mark the item `failed` and reject the message to a
DLQ such as `eval.item.requested.dlq`.

The DLQ payload should still only contain identifiers. Operators and MCP tools
should use eval API state for detailed evidence.

## Recovery

Add recovery paths for common failure modes:

- API starts and finds `queued` items older than a threshold: republish item
  messages.
- Worker starts and finds `running` items with expired leases: reset them to
  `queued` and republish if attempts remain.
- Run has all items terminal but no terminal run status: rerun aggregation.
- Run remains `queued` or `running` past the configured max age: expose it as
  stale in evidence before changing terminal state.

The existing stale-running recovery can be adapted, but it should avoid
marking the whole run failed while retryable item work still has attempts
remaining.

## Observability

Add bounded Prometheus metrics:

- `eval_queue_publish_total{status}`
- `eval_item_messages_total{outcome}`
- `eval_item_attempts_total{status,operation}`
- `eval_item_duration_seconds{stage,status}`
- `eval_item_retries_total{reason}`
- `eval_item_dlq_total{reason}`
- `eval_running_items`
- `eval_queued_items`
- `eval_stale_items`

Do not use `evaluation_id`, `item_id`, raw query, collection, or raw error text
as metric labels.

Logs should include correlation fields:

- `evaluation_id`
- `item_id`
- `item_index`
- `attempt`
- `worker_id`
- `operation`
- `status`
- `error_type`
- `retryable`

Log raw query text only in explicit API result payloads, not worker lifecycle
logs.

## MCP Impact

`go/eval-mcp-service` should continue to call eval API endpoints. It should
not publish RabbitMQ messages or inspect queues directly in V1.

Eval API evidence should become richer so MCP tools can report:

- run status
- item counts by status
- completed item count
- failed item count
- retry-exhausted count
- stale item count
- aggregate scores when available
- whether a run is comparable
- next safe actions

Queue-level operational evidence belongs in the observability MCP, not the eval
MCP, unless a future design adds a narrow read-only queue-health endpoint to
the eval API.

## Deployment And Configuration

The first implementation should add RabbitMQ configuration to the Python eval
service in a way that stays aligned across local Docker Compose, QA, and
Kubernetes manifests.

Expected settings:

- RabbitMQ URL or host/port/user/password
- eval item queue name
- eval item DLQ name
- worker prefetch
- max item attempts
- item lease seconds
- stale item threshold
- worker concurrency

The worker can live in `services/eval` as a separate process entry point and
Docker command. This keeps shared eval code local while allowing independent
API and worker deployment.

## Testing

Unit tests should cover:

- item row creation from dataset items
- idempotent claim behavior
- duplicate message on completed item
- expired lease recovery
- retryable failure increments attempt count
- exhausted retry marks item failed
- aggregation with all completed items
- aggregation with partial failures
- aggregation with all failed items
- message payload excludes raw query and secrets

Integration-style tests should use fake RabbitMQ adapters or in-memory
interfaces first. Do not require a live broker for normal unit tests. A later
compose smoke test can verify broker wiring once the behavior is stable.

Before committing implementation, run `make preflight-python` and
`make preflight-security`.

## Deferred Roadmap

Create roadmap issues after this spec is accepted:

1. RabbitMQ item-level eval worker V1.
2. Eval item progress and evidence API.
3. DLQ triage and replay tooling.
4. Stale item recovery and cancellation.
5. Eval dashboard with item progress and failure causes.
6. Kafka eval lifecycle event stream for analytics and replayable history.
7. LangGraph eval orchestrator spike for multi-step judge, critique, and
   diagnosis workflows inside the worker.
8. Multi-worker concurrency, provider rate-limit tuning, and autoscaling.

Kafka is intentionally deferred until there is a concrete consumer need for
replayable lifecycle events. LangGraph is intentionally deferred until the
execution substrate is reliable enough to host more complex agentic workflows.

## Acceptance Criteria

The design is successful when a future implementation can show:

- Eval API no longer executes eval work in process.
- A submitted run creates durable item rows and persistent RabbitMQ messages.
- Workers can crash before ack without corrupting run state.
- Duplicate messages are safe.
- Item failures retry within policy and then move to DLQ.
- Runs expose progress and terminal status from durable item state.
- MCP-driven workflows still work through the eval API.
- Observability can explain queue health, stale work, retries, and DLQ causes.
