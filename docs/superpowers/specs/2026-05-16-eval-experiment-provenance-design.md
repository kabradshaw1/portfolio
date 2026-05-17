# Eval Experiment Provenance Design

## Purpose

Make RAG eval comparisons traceable enough to distinguish requested experiment
configuration from the effective retrieval behavior that happened for each
query.

This follows the eval runtime metadata hardening work, which added run-level
fields such as `requested_rerank` and `effective_collection`. This design adds
per-query provenance so rerank experiments can answer: was reranking requested,
was it enabled, did it actually run, and did the pipeline fall back?

## Scope

Implement structured experiment provenance for `services/eval` and the chat
client path it uses.

Include:

- send explicit `rerank: true` or `rerank: false` from eval to chat
- persist effective retrieval metadata on each eval query result
- keep existing aggregate metric behavior unchanged
- add focused tests for baseline and rerank result metadata

Exclude:

- full request and response archival
- raw prompt snapshots
- token-level telemetry in the eval database
- vector score dumps
- retention policy or audit-log storage design
- frontend reporting changes

## Terminology

- **Requested configuration**: what the eval run asked the system to do, such
  as `requested_rerank: true`.
- **Service capability configuration**: what the chat service is configured to
  support, such as `chat.rerank_enabled: true`.
- **Effective retrieval behavior**: what happened for one query, such as
  `rerank_applied: true` or `rerank_fallback: true`.
- **Experiment provenance**: structured metadata needed to reproduce and
  explain eval results.

## Data Flow

1. MCP or a direct API caller starts an eval run with `rerank: true` or
   `rerank: false`.
2. `services/eval` stores run-level config through `capture_run_config`,
   including `requested_rerank` and `effective_collection`.
3. `services/eval` calls `services/chat` with an explicit JSON `rerank` field.
4. `services/chat` returns the answer, sources, and retrieval metadata.
5. `services/eval` stores each query result with scores, contexts, and a
   `retrieval` object containing effective retrieval metadata.

## Result Shape

Each eval result should retain the existing fields and add a `retrieval` object
when chat returns retrieval metadata.

Expected fields include:

- `retrieval_mode`
- `retrieval_fallback`
- `rerank_requested`
- `rerank_enabled`
- `rerank_applied`
- `rerank_fallback`
- `rerank_error`, when present
- `rerank_model`
- `rerank_candidate_count`
- `rerank_returned_count`

The implementation should preserve unknown retrieval metadata keys returned by
chat unless there is a clear reason to filter them. This keeps the eval service
from needing frequent updates when chat adds low-risk retrieval diagnostics.

## API Behavior

`services/eval/app/rag_client.py` should always send the rerank flag:

- baseline run: `"rerank": false`
- rerank candidate run: `"rerank": true`

This does not change chat behavior because chat already defaults missing
`rerank` to false. It makes request traces and tests less ambiguous.

## Error Handling

If chat omits retrieval metadata, the eval should still complete and store no
`retrieval` object for that query. Missing retrieval metadata is a provenance
gap, not a quality-evaluation failure.

If reranking is requested but chat falls back, chat's retrieval metadata should
carry `rerank_fallback: true` and, when available, `rerank_error`. Eval should
store those fields without special-casing them.

## Testing

Add or update tests to prove:

- baseline eval requests send `rerank: false`
- rerank eval requests send `rerank: true`
- eval results persist chat retrieval metadata
- existing aggregate metric calculation is unchanged
- missing retrieval metadata does not fail an eval run

Tests should stay focused on `services/eval` unless a chat contract test is
needed to document returned retrieval metadata.

## Branching

This changes persisted eval result shape and production behavior. Implement it
in a feature worktree targeting `qa`, not directly on `qa`.
