# Cross-Encoder Re-Ranking Design

## Context

Issue #81 adds a second retrieval stage for the RAG pipeline: fast candidate
retrieval followed by more accurate cross-encoder re-ranking.

The current `qa` baseline already includes Phase 4b hybrid retrieval. Chat can
retrieve with semantic or hybrid mode, returns retrieval metadata from `/search`
and `/chat`, and exposes retrieval config through `/config`. The eval service
has a first-party harness that can run comparable evaluations on the same
dataset and collection.

This feature should build on that shape instead of replacing it. Qdrant remains
the candidate generator. The new re-ranker only reorders candidate chunks before
they are returned by `/search` or used to build the RAG prompt.

## Goals

- Add opt-in local cross-encoder re-ranking to the Python chat service.
- Expose `rerank` on `/search` and `/chat` request bodies, defaulting to
  `false`.
- Preserve existing behavior when callers do not request re-ranking.
- Retrieve a larger candidate pool when re-ranking is requested, then trim back
  to the requested result count after scoring.
- Return and capture enough metadata to identify whether re-ranking was used or
  fell back.
- Use the eval harness to compare `rerank=false` and `rerank=true` runs on the
  same dataset and collection.

## Non-Goals

- No separate re-ranker microservice.
- No Ollama or LLM-prompt based pairwise scoring in the initial implementation.
- No paid cloud model dependency.
- No UI changes.
- No mutation of QA or production data during implementation.
- No automatic default-on rollout before evaluation demonstrates value.

## Selected Approach

Add an in-process re-ranker module to `services/chat`.

The re-ranker loads a pinned local cross-encoder model lazily, scores each
`(query, chunk["text"])` pair, sorts by score descending, and returns the top
results. Retrieval remains responsible for recall; re-ranking is responsible for
precision.

The request flow without re-ranking remains:

```text
query -> embedding/sparse encoding -> Qdrant retrieval -> chunks
```

The request flow with re-ranking becomes:

```text
query -> embedding/sparse encoding -> Qdrant candidate retrieval
      -> local cross-encoder scoring -> reordered chunks
```

This keeps the implementation focused and avoids adding another service,
network hop, deployment target, or model-serving runtime.

## Chat Service Design

Add `services/chat/app/reranker.py` with a small boundary around the concrete
model package. The module should provide:

- a lazy loader for the configured cross-encoder model
- a function or class method that accepts `query` and `chunks`
- deterministic ordering by model score, descending
- stable tie handling that preserves the original retrieval order when scores
  are equal
- no model load when the candidate list is empty or re-ranking is not requested

`services/chat/app/chain.py` remains the orchestration boundary. `retrieve_chunks`
should accept a `rerank: bool = False` argument. When `rerank` is false, it uses
the current retrieval path and requested `top_k`.

When `rerank` is true, `retrieve_chunks` should:

1. Compute an effective candidate count.
2. Ask Qdrant for that larger candidate pool through the existing semantic or
   hybrid path.
3. Pass the candidates to the re-ranker.
4. Trim re-ranked results back to the caller's requested `top_k`.
5. Return `RetrievalResult` with updated metadata.

Candidate count should be bounded by config:

```text
min(max(top_k, rerank_candidate_limit), rerank_max_candidates)
```

The default `rerank_candidate_limit` should be `20`, matching the current
hybrid prefetch scale. The default `rerank_max_candidates` should be `50` to
prevent accidental expensive request shapes.

## API Design

Extend `/search` request body:

```json
{
  "query": "string",
  "collection": "optional-string",
  "limit": 5,
  "rerank": false
}
```

Extend `/chat` request body:

```json
{
  "question": "string",
  "collection": "optional-string",
  "rerank": false
}
```

`rerank` defaults to `false` on both endpoints. Existing callers remain
compatible and receive current behavior.

`/search` should continue returning `results` and retrieval metadata. `/chat`
should include metadata in JSON responses and the final SSE `done` event, as it
does for hybrid retrieval metadata today.

## Configuration

Add chat settings:

- `rerank_enabled`: runtime kill switch, default `true`
- `rerank_model`: pinned local cross-encoder model name
- `rerank_candidate_limit`: default `20`
- `rerank_max_candidates`: default `50`
- `rerank_device`: default `cpu`

The spec intentionally does not hard-code the exact model. The implementation
plan must choose and pin one maintained local cross-encoder model that is small
enough for CPU use and Docker image growth that can pass project preflights.
Before coding, verify the package and model choice for current maintenance
status, import paths, license, and dependency audit impact.

`/config` should expose non-secret re-ranker settings:

- `rerank_enabled`
- `rerank_model`
- `rerank_candidate_limit`
- `rerank_max_candidates`
- `rerank_device`

## Metadata

Extend `RetrievalResult.metadata` with re-ranking fields:

- `rerank_requested`: whether the caller requested re-ranking
- `rerank_applied`: whether cross-encoder scores were actually applied
- `rerank_enabled`: active runtime setting value
- `rerank_model`: configured model name when relevant
- `rerank_candidate_count`: number of candidates sent to the re-ranker
- `rerank_returned_count`: number of chunks returned after trimming
- `rerank_fallback`: whether the service fell back to original retrieval order
- `rerank_error`: short non-secret error class or reason when fallback happens

These fields make eval runs and search/chat responses explainable without
exposing model paths, credentials, or stack traces.

## Error Handling

Re-ranking should be availability-preserving.

- If `rerank=false`, no re-ranker model is loaded.
- If `rerank=true` and `rerank_enabled=false`, return normal retrieval order and
  mark metadata as requested but not applied.
- If candidate retrieval fails, preserve the existing endpoint failure behavior.
- If model loading or scoring fails, log the error, increment a metric, return
  the original retrieval order trimmed to `top_k`, and set fallback metadata.
- Re-ranker errors must not fail `/search` or `/chat` unless base retrieval
  itself failed.

Logs should include collection, error type, and candidate count, but not chunk
text.

## Metrics

Add Prometheus metrics for the chat service:

- re-ranker duration histogram labeled by model and outcome
- re-ranker candidate-count histogram
- re-ranker fallback/error counter labeled by reason

Continue using existing RAG stage timing for retrieval and prompt construction.
If a new `rerank` RAG stage label is added, tests should assert it is recorded
only when re-ranking is requested.

## Evaluation

Extend `services/eval/app/rag_client.py` so `search()` and `ask()` can pass
`rerank`.

Extend evaluation requests with `rerank: bool = false`. The eval service should
pass the flag to both `/search` and `/chat` so retrieved contexts and generated
answers use the same retrieval behavior.

The comparison workflow is:

1. Run baseline evaluation with `rerank=false`.
2. Run experiment evaluation with `rerank=true` and `baseline_eval_id` set to
   the baseline run.
3. Compare the two completed runs with the existing comparison endpoint.

Context precision and context recall are the primary retrieval-quality signals.
Faithfulness and answer relevancy should still be recorded because re-ranked
contexts can change generation quality.

Config capture should include the chat `/config` re-ranker fields. Per-run
results should continue storing retrieved contexts and scores; no schema change
is required for per-query results unless the implementation chooses to preserve
per-result re-rank scores.

## Testing

Unit tests should cover:

- re-ranker sorts candidates by cross-encoder score
- equal re-ranker scores preserve original retrieval order
- empty candidate lists return empty results without model scoring
- `retrieve_chunks(..., rerank=False)` preserves current behavior and candidate
  count
- `retrieve_chunks(..., rerank=True)` retrieves the larger candidate pool,
  applies re-ranking, trims to `top_k`, and records metadata
- re-ranker model/scoring exceptions fall back to original retrieval order and
  set fallback metadata
- `/search` threads the request-level `rerank` flag into `retrieve_chunks`
- `/chat` threads the request-level `rerank` flag into `rag_query`
- `/config` includes non-secret re-ranker settings
- eval `RAGClient.search` and `RAGClient.ask` include `rerank` only when needed
- evaluation requests can run with `rerank=true` and pass the flag through to
  chat

Verification before commit should include:

- `make preflight-python`
- `make preflight-security`

If the chosen model dependency makes local security or Python preflight fail,
the implementation should stop and revise the dependency choice rather than
ignoring advisories.

## Acceptance Criteria

- `/search` and `/chat` accept `rerank`, defaulting to `false`.
- Existing callers behave the same when `rerank` is omitted.
- `rerank=true` uses a local cross-encoder to reorder a larger candidate pool
  and returns only the requested top results.
- Re-ranker failures fall back to original retrieval order without failing the
  endpoint.
- `/config`, response metadata, metrics, and eval config capture show whether
  re-ranking was requested, applied, or bypassed.
- The eval service can produce comparable baseline and experiment runs for
  `rerank=false` and `rerank=true`.
- Relevant Python and security preflights pass, or any blocker is documented.
