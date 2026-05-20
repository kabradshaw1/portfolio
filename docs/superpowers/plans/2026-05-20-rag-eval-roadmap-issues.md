# RAG Eval Platform Roadmap Issue Drafts

## Issue 1: DLQ Triage And Replay Tooling

Build read-only DLQ inspection and explicit replay support for eval item jobs.
The tool must show message identifiers only, load detailed evidence from the
eval API, and require an explicit replay command.

Acceptance criteria:
- List eval item DLQ messages without removing them.
- Show evaluation id, item id, attempt count, and original routing metadata.
- Replay one selected DLQ item by id or index.
- Record replay attempts in metrics and logs.

## Issue 2: Cancellation And Stale Run Recovery

Add cancellation support and stronger stale run recovery for queued/running evals.

Acceptance criteria:
- API can mark a queued or running run as cancelled.
- Worker stops processing cancelled items before upstream calls.
- Expired item leases are reset and republished.
- Stale terminal aggregation is repaired automatically.

## Issue 3: Eval Dashboard Item Progress

Expose eval run item progress and failure causes in the frontend dashboard.

Acceptance criteria:
- Dashboard shows item counts by status.
- Dashboard shows failed item reason counts.
- Dashboard distinguishes completed from completed_with_failures.
- Dashboard links worst cases only for comparable completed runs.

## Issue 4: Kafka Eval Lifecycle Events

Publish eval lifecycle events for analytics and replayable experiment history.

Acceptance criteria:
- Publish bounded events for run queued/running/completed/failed.
- Publish bounded events for item completed/failed.
- Do not include raw query text or model secrets in event payloads.
- Add one consumer that produces useful analytics from the stream.

## Issue 5: LangGraph Eval Orchestrator Spike

Explore LangGraph inside the eval worker for multi-step judge, critique, and
failure diagnosis workflows.

Acceptance criteria:
- Keep RabbitMQ as the job substrate.
- Run graph orchestration inside one item worker claim.
- Compare direct judge vs graph judge on the same dataset.
- Record latency, cost, score stability, and failure-mode differences.
