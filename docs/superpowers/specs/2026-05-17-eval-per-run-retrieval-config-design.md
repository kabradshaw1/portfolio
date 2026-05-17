# Eval Per-Run Retrieval Config Design

Date: 2026-05-17
Status: Approved for implementation planning

## TL;DR

Add a typed per-run `retrieval_config` object to the MCP-backed evaluation path.
Version 1 supports only `top_k`, which controls the final context budget for a
single evaluation run. The value flows from MCP to the eval API, into both
`/search` and `/chat`, and is captured in run metadata as requested and
effective retrieval configuration.

This avoids changing the chat service's global `TOP_K` runtime setting for
controlled RAG experiments.

## Problem

The current eval workflow supports `rerank` as a per-run parameter, but not
`top_k`. The eval service calls chat `/chat` without a top-k override, and chat
uses `settings.top_k` from runtime configuration. The eval service also calls
`/search` with `limit=5`.

That creates two issues:

- Top-k experiments require changing global chat runtime config, which is not
  repeatable or run-scoped.
- Eval can mix contexts scored from one configuration with generated answers
  produced from another configuration.

The observed failure mode on `product-docs-rag-v1` is that relevant context is
usually present, but extra irrelevant chunks depress `context_precision`.

## Experiment Hypothesis

Reducing the final context budget from `top_k=5` to `top_k=3` will improve
`context_precision` on `product-docs-rag-v1` while preserving
`answer_relevancy`, `faithfulness`, and acceptable `context_recall`.

Use "final context budget" for this experiment. If reranking is enabled later,
the rerank candidate pool may still be larger than the final number of contexts
returned to the answer generator.

## Recommended Approach

Add a typed `retrieval_config` object:

```json
{
  "retrieval_config": {
    "top_k": 3
  }
}
```

`retrieval_config` is optional. In version 1, `top_k` is the only accepted
field. Unknown fields are rejected.

This shape is preferable to a top-level `top_k` because it names the domain of
the override and leaves room for future controlled retrieval variables without
turning `StartEvaluationRequest` into a flat list of knobs.

## API Shape

Python eval API:

```python
class RetrievalConfig(BaseModel):
    top_k: int | None = Field(default=None, ge=1, le=20)


class StartEvaluationRequest(BaseModel):
    dataset_id: str
    collection: str | None = None
    notes: str | None = None
    baseline_eval_id: str | None = None
    rerank: bool = False
    experiment_id: str | None = None
    experiment_label: str | None = None
    retrieval_config: RetrievalConfig | None = None
```

Chat API:

```python
class ChatRequest(BaseModel):
    question: str
    collection: str | None = None
    rerank: bool = False
    retrieval_config: RetrievalConfig | None = None
```

The chat service resolves:

```python
effective_top_k = (
    body.retrieval_config.top_k
    if body.retrieval_config and body.retrieval_config.top_k is not None
    else settings.top_k
)
```

The eval service resolves:

```python
effective_top_k = (
    retrieval_config.top_k
    if retrieval_config and retrieval_config.top_k is not None
    else captured_chat_top_k
    if captured_chat_top_k is not None
    else 5
)
```

When no override is supplied, eval should prefer the chat runtime default from
`/config` so `/search` scoring contexts and `/chat` answer generation stay
aligned. The final fallback to `5` preserves the current eval-side default if
config capture is unavailable.

## Data Flow

1. MCP `start_eval_run` accepts `retrieval_config.top_k`.
2. Go MCP handler decodes it into a typed workflow input.
3. Go eval API client serializes it to the eval API.
4. Python eval API validates it on `StartEvaluationRequest`.
5. `_run_evaluation_task` captures config, resolves the effective top-k, and
   passes the effective retrieval config to `run_evaluation`.
6. `build_evaluation_dataset` uses one effective top-k value for both eval
   retrieval and answer generation:
   - `/search` receives `limit=effective_top_k`.
   - `/chat` receives `retrieval_config: {"top_k": effective_top_k}`.
7. Chat `/chat` threads `effective_top_k` into `rag_query`.
8. Per-query retrieval metadata remains attached to eval results.

Do not add `retrieval_config` to `/search` in version 1. `/search.limit`
already represents that endpoint's final result count.

## Metadata Capture

Persist both runtime defaults and run-scoped intent:

```json
{
  "captured_at": "2026-05-17T00:00:00+00:00",
  "effective_collection": "documents",
  "requested_rerank": false,
  "requested_retrieval_config": {
    "top_k": 3
  },
  "effective_retrieval_config": {
    "top_k": 3
  },
  "chat": {
    "top_k": 5,
    "retrieval_mode": "hybrid",
    "rerank_enabled": true,
    "rerank_candidate_limit": 20
  },
  "collection": {}
}
```

Field meanings:

- `chat.top_k`: chat service runtime default from `/config`.
- `requested_retrieval_config.top_k`: run-scoped override requested through
  MCP/eval.
- `effective_retrieval_config.top_k`: value actually used by the eval run.
- If no override is supplied, `effective_retrieval_config.top_k` should equal
  `chat.top_k` when config capture succeeds. If chat config capture fails, it
  falls back to the legacy eval default of `5`.

Config capture must remain non-blocking. If chat or ingestion config capture
fails, the eval still runs and records `_capture_error`.

Chat retrieval metadata should include the final context budget as row-level
evidence, using the existing retrieval metadata object returned by `/chat`.

## Validation

Validation rules:

- `retrieval_config` is optional.
- `retrieval_config.top_k` must be an integer from `1` through `20`.
- Unknown `retrieval_config` fields are rejected.
- MCP schema rejects unknown fields and validates the same range.
- If `rerank=true`, `top_k` means final contexts returned after reranking. It
  does not override the rerank candidate pool.

## Test Plan

Chat service tests:

- `ChatRequest` accepts `retrieval_config.top_k`.
- `/chat` threads `retrieval_config.top_k=3` into `rag_query`.
- `/chat` falls back to `settings.top_k` when no override is supplied.
- Invalid `top_k` values are rejected.

Eval service tests:

- `StartEvaluationRequest` accepts `retrieval_config`.
- `RAGClient.ask()` forwards `retrieval_config`.
- `build_evaluation_dataset()` uses one effective top-k value for
  `/search.limit` and `/chat.retrieval_config`.
- `_run_evaluation_task()` passes `retrieval_config` into config capture and
  evaluation.
- Config capture records requested and effective retrieval config.
- Invalid or unknown retrieval config fields are rejected.

Go MCP service tests:

- `start_eval_run` schema accepts `retrieval_config.top_k`.
- Handler decodes and forwards it.
- Workflow service preserves it.
- Eval API client serializes it.
- Existing unauthorized retry tests preserve the original request body,
  including `retrieval_config`.

## First QA Experiment

After deployment to QA, run an explicit baseline and candidate so both runs
record the controlled variable in metadata.

Baseline:

```json
{
  "dataset_id": "19d3ee69-6544-44b8-94bd-0fac110f06f5",
  "collection": "documents",
  "experiment_id": "EXP_ID",
  "label": "top_k_5_baseline",
  "rerank": false,
  "retrieval_config": {
    "top_k": 5
  },
  "notes": "Baseline final context budget 5, rerank off."
}
```

Candidate:

```json
{
  "dataset_id": "19d3ee69-6544-44b8-94bd-0fac110f06f5",
  "collection": "documents",
  "experiment_id": "EXP_ID",
  "label": "top_k_3_candidate",
  "rerank": false,
  "retrieval_config": {
    "top_k": 3
  },
  "notes": "Candidate final context budget 3, rerank off."
}
```

Compare completed runs and inspect worst cases for `context_precision` and
`context_recall`.

Decision rule: keep the change only if `context_precision` improves
meaningfully while `answer_relevancy` and `faithfulness` remain effectively
stable, with no unacceptable loss in `context_recall`.

## Implementation Scope

Implementation should happen in a feature worktree targeting `qa`, because it
changes runtime behavior across deployable services.

Likely areas:

- `services/chat/app/main.py`
- `services/chat` tests around `ChatRequest` and top-k threading
- `services/eval/app/models.py`
- `services/eval/app/rag_client.py`
- `services/eval/app/evaluator.py`
- `services/eval/app/main.py`
- `services/eval/app/config_capture.py`
- `services/eval` tests for forwarding and config capture
- `go/eval-mcp-service/internal/evalapi`
- `go/eval-mcp-service/internal/evalworkflow`
- `go/eval-mcp-service/internal/mcpserver`
- Go MCP tests

`k8s/ai-services/configmaps/chat-config.yml` is background context only, not
the preferred solution.
