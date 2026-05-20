# RAG Eval DLQ Triage And Replay Design

## Goal

Build safe operator tooling for RAG eval item dead-letter queues. Operators and
MCP workflows should be able to inspect failed item work, load detailed evidence
from eval API state, and explicitly replay one failed item without exposing raw
query text, expected answers, answer text, contexts, API keys, or model secrets
in RabbitMQ payloads or DLQ listing responses.

This follows the RabbitMQ item worker design in
`docs/superpowers/specs/2026-05-20-rag-eval-rabbitmq-item-worker-design.md`.
`services/eval` remains the source of truth. RabbitMQ is transport, including
for DLQ messages.

## Current Context

`services/eval` already creates one durable RabbitMQ message per eval item. The
message contract contains only:

```json
{
  "message_version": 1,
  "evaluation_id": "uuid",
  "item_id": "uuid",
  "item_index": 0,
  "attempt": 1
}
```

The eval database stores item state, attempts, bounded last error data, result
payloads, scores, and parent evaluation state. Workers reject permanently failed
messages to `eval.item.requested.dlq`.

The Go eval MCP service already wraps eval API workflows. It should remain a
workflow adapter and should not connect directly to RabbitMQ.

## Recommended Approach

Add operator-only DLQ inspection and replay endpoints to `services/eval`, then
add thin MCP tools that call those endpoints.

This keeps queue credentials, replay safety checks, redaction, metrics, logs,
and item state transitions in the service that owns eval execution state. MCP
gets an operator-friendly workflow surface without duplicating broker semantics
or exposing RabbitMQ payloads as evidence.

## API Surface

Add these eval API endpoints:

- `GET /evaluations/items/dlq?limit=...`
- `POST /evaluations/items/dlq/replay`

Both endpoints are operator-only. They must require authenticated auth context
tier `operator`, not only the broader `eval_read` or `eval_write` rate-limit
groups. Anonymous and normal user requests return `403`.

`GET /evaluations/items/dlq` peeks at up to a bounded limit of DLQ messages
without removing them.

`POST /evaluations/items/dlq/replay` accepts exactly one selector:

```json
{"item_id": "item-id"}
```

or:

```json
{"index": 0}
```

`item_id` is preferred for MCP automation because queue indexes are transient.
Index replay remains useful for quick manual workflows after listing the DLQ.

## DLQ Listing Semantics

Listing is a true peek. The implementation may use RabbitMQ `basic.get` with
`auto_ack=false`, collect up to `limit`, then `nack(requeue=true)` each inspected
message so the DLQ remains intact.

Each listed entry includes only safe identifiers and bounded metadata:

```json
{
  "index": 0,
  "delivery_tag": "...",
  "redelivered": false,
  "payload": {
    "message_version": 1,
    "evaluation_id": "eval-id",
    "item_id": "item-id",
    "item_index": 3,
    "attempt": 3
  },
  "routing": {
    "exchange": "",
    "routing_key": "eval.item.requested",
    "queue": "eval.item.requested.dlq",
    "death_count": 1,
    "death_reason": "rejected"
  },
  "item": {
    "evaluation_id": "eval-id",
    "item_id": "item-id",
    "item_index": 3,
    "status": "failed",
    "attempt_count": 3,
    "max_attempts": 3,
    "last_error": {"error_type": "TimeoutError", "retryable": false}
  },
  "evaluation": {
    "status": "completed_with_failures",
    "collection": "documents",
    "created_at": "...",
    "completed_at": "..."
  }
}
```

Do not return raw query text, expected answers, generated answers, contexts,
full result payloads, API keys, secret names, or model secret values from DLQ
listing. Operators who need deeper run evidence should use existing eval run
evidence APIs and MCP tools by `evaluation_id`.

The response should state that indexes are queue-order dependent and transient.

## Replay Semantics

Replay is explicit and conservative.

The API should:

1. Find exactly one matching DLQ message by `item_id` or transient `index`.
2. Decode and validate the identifier-only payload.
3. Load the item row by `item_id`.
4. Require the item status to be `failed`.
5. Reset the item to `queued`.
6. Clear lease fields.
7. Preserve `attempt_count` and `last_error`.
8. Increment `replay_count` and set `last_replayed_at`.
9. Republish a persistent identifier-only message to `eval.item.requested`.
10. Ack and remove the DLQ message only after the database update and publish
    succeed.

The replay message body keeps the safe contract:

```json
{
  "message_version": 1,
  "evaluation_id": "eval-id",
  "item_id": "item-id",
  "item_index": 3,
  "attempt": 4
}
```

Add `replay_count` and `last_replayed_at` columns to `evaluation_items` rather
than storing replay history in RabbitMQ headers. The database remains the
queryable audit source.

Replay does not reset the retry budget. Preserving `attempt_count` means an
operator replay gives the selected item one explicit new processing attempt; if
that attempt fails again, the existing worker retry policy can dead-letter it
again without hiding the earlier failure history.

If publish fails after the item was reset, the operation should return a clear
failure and leave enough state for existing queued-item recovery to republish
the work. The DLQ message should not be acked before the publish succeeds.

## MCP Surface

Add two tools to `go/eval-mcp-service`:

- `list_eval_item_dlq(limit?: int)`
- `replay_eval_item_dlq(item_id?: string, index?: int)`

The MCP service calls eval API endpoints. It does not connect to RabbitMQ, does
not parse queue internals directly, and does not enrich responses with raw query
or answer fields.

Tool validation should enforce exactly one replay selector. The replay tool
description must state that it is explicit and mutating.

## Observability

Add bounded metrics:

- `eval_item_dlq_inspections_total{outcome}`
- `eval_item_dlq_replays_total{outcome}`

Allowed outcomes should be bounded values such as `success`, `empty`,
`not_found`, `invalid_payload`, `not_failed`, `publish_failed`, and `error`.

Replay logs should include:

- `evaluation_id`
- `item_id`
- `item_index`
- selector type
- original routing key
- outcome

Logs must not include raw query text, expected answers, generated answers,
contexts, full result payloads, API keys, secret names, or model secret values.

## Error Handling

Invalid DLQ payloads should appear in list output as invalid entries with safe
metadata and an `invalid_payload` outcome. Replay should reject invalid payloads
without removing unrelated messages.

If an `item_id` selector matches no DLQ message, return `404`.

If the DLQ message exists but the database item is missing, return a safe error
that includes only identifiers.

If the item is not `failed`, reject replay with `409`. Completed or currently
running items must not be replayed from DLQ tooling.

If an index selector is out of range, return `404` and leave all messages in the
DLQ.

## Testing

Python tests should cover:

- DLQ payload decoding and invalid payload handling.
- Peek without removal.
- Listing redaction.
- Operator-only authorization.
- Replay selector validation.
- Replay requiring failed item state.
- DB reset with `replay_count` and `last_replayed_at`.
- Publish-before-ack ordering.
- Bounded metrics and log labels.

Go MCP tests should cover:

- Tool schema registration.
- Exactly-one selector validation.
- Eval API method, path, and request body.
- Safe response propagation without raw query fields.

## Out Of Scope

- A web dashboard for DLQ management.
- Bulk replay.
- Automatic replay during listing.
- Direct RabbitMQ access from MCP.
- Storing raw query, expected answer, generated answer, contexts, API keys, or
  model secrets in RabbitMQ payloads.
