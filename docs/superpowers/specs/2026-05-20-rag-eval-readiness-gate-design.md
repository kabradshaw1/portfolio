# RAG Eval Readiness Gate Design

Date: 2026-05-20
Status: Approved for implementation planning

## TL;DR

Add a readiness gate before RAG eval experiments start. The gate validates that
the selected retrieval collection, dataset, chat config, ingestion metadata,
and eval runtime are consistent enough to produce trustworthy eval evidence.

The gate separates findings into blocking failures and advisory warnings:

- Blocking failures stop the run because the eval would be invalid or
  misleading.
- Advisory warnings allow the run, but the warnings are captured in run
  evidence and experiment summaries so later decisions carry the right caveats.

Version 1 should be callable from the eval MCP before `start_eval_experiment`
and `start_eval_run`, and should be backed by the eval API so frontend and MCP
workflows use the same policy.

## Problem

The current eval workflow can validate that a collection name exists, but it
does not prove the collection is ready for measurement. A run can still start
when important prerequisites are missing or ambiguous:

- A collection exists but has no points.
- Ingestion returns a collection in `/collections`, but
  `/collections/{name}/config` has no metadata.
- The dataset's expected sources may not be represented in the selected
  collection.
- Chat config may expect dense or sparse vector names that do not match the
  collection metadata.
- Prior experiment evidence may be hard to interpret because runtime/config
  caveats are discovered only after a run starts or fails.

This is risky because RAG evals are expensive, slow, and easy to misread. If the
corpus is empty, stale, or mismatched, a score comparison can look like a
retrieval or reranking regression when the real issue is operational readiness.

## Goals

- Fail fast when an eval would be invalid or materially misleading.
- Preserve flexibility for exploratory runs by allowing low-confidence but
  interpretable cases as warnings.
- Record readiness findings in durable eval run evidence.
- Give MCP agents a structured preflight result they can explain before
  starting experiments.
- Keep the readiness logic centralized enough that MCP, frontend, and API users
  get consistent behavior.
- Improve the portfolio story around production-grade EvalOps: reliable
  measurement before model or retrieval tuning.

## Non-Goals

- Do not build the full experiment orchestrator in this project.
- Do not build the full EvalOps decision packet in this project.
- Do not automatically re-index documents or mutate Qdrant collections.
- Do not add a broad observability dependency to the gate. Runtime metrics and
  logs remain a follow-up through the observability MCP.
- Do not replace per-run config capture; the readiness gate complements it.

## Recommended Approach

Add a new eval API readiness endpoint and expose it through the eval MCP:

```text
POST /eval/readiness/rag
```

MCP tool:

```text
check_rag_eval_readiness
```

The check accepts the dataset ID, collection name, requested rerank flag,
optional retrieval config, optional baseline eval ID, and optional experiment
ID. It returns a structured result with:

- `status`: `ready`, `warning`, or `blocked`
- `blocking_failures`
- `warnings`
- `evidence`
- `next_steps`

The eval MCP must call this check before creating an experiment or starting an
eval run. If the result is `blocked`, the MCP should not call
`start_eval_experiment` or `start_eval_run`. Version 1 deliberately has no
override path; the first implementation should prove the default policy before
adding exception handling.

## Blocking Policy

Block when the finding invalidates the eval or makes scores materially
misleading:

- Collection does not exist.
- Collection point count is `0`.
- Collection config is missing or cannot be parsed.
- Collection metadata does not include the dense vector name required by chat.
- Chat is configured for hybrid retrieval, but collection metadata says hybrid
  is disabled or lacks the sparse vector name expected by chat.
- Dataset does not exist or has no items.
- Dataset declares expected sources and the source inventory check finds zero
  matching source names in the selected collection.
- Baseline eval ID is supplied but belongs to a different dataset or collection.
- Experiment ID is supplied but belongs to a different dataset or collection.
- Eval cannot reach ingestion or chat config endpoints during readiness
  validation.

Blocking failures must include a remediation hint. Example:

```json
{
  "code": "collection_empty",
  "message": "Collection documents has 0 points.",
  "remediation": "Re-run ingestion or choose a populated collection before starting the eval."
}
```

## Warning Policy

Warn when the run may still be useful, but confidence is lower:

- Collection point count is nonzero but below the number expected for the
  dataset size.
- Collection config exists but is missing noncritical fields, such as
  `sparse_model`.
- Some, but not all, expected sources are represented in the collection.
- Requested rerank is true while chat runtime rerank support is disabled.
- Requested top-k differs from chat runtime default.
- Existing baseline or candidate run has partial failures, but still has
  triageable results.
- The collection appears indexed with older chunk settings, but the config is
  still internally consistent.

Warnings must be persisted with the run config when a run starts. Experiment
summaries should surface readiness warnings alongside score deltas so the
decision step does not bury caveats.

## API Shape

Request:

```json
{
  "dataset_id": "19d3ee69-6544-44b8-94bd-0fac110f06f5",
  "collection": "documents",
  "rerank": false,
  "retrieval_config": {
    "top_k": 5
  },
  "baseline_eval_id": null,
  "experiment_id": null
}
```

Response:

```json
{
  "status": "warning",
  "blocking_failures": [],
  "warnings": [
    {
      "code": "partial_expected_source_coverage",
      "message": "12 of 15 expected source references were observed in collection metadata or sample payloads.",
      "remediation": "Review missing sources before treating source-sensitive regressions as retrieval failures."
    }
  ],
  "evidence": {
    "dataset": {
      "id": "19d3ee69-6544-44b8-94bd-0fac110f06f5",
      "item_count": 15,
      "expected_sources": [
        "laptop-pro-15-specs.pdf"
      ]
    },
    "collection": {
      "name": "documents",
      "points_count": 128,
      "config": {
        "chunk_size": 1000,
        "chunk_overlap": 200,
        "embedding_model": "nomic-embed-text",
        "hybrid_enabled": true,
        "dense_vector_name": "dense",
        "sparse_vector_name": "sparse",
        "sparse_model": "Qdrant/bm25"
      }
    },
    "chat": {
      "retrieval_mode": "hybrid",
      "dense_vector_name": "dense",
      "sparse_vector_name": "sparse",
      "rerank_enabled": true,
      "top_k": 5
    },
    "requested": {
      "rerank": false,
      "retrieval_config": {
        "top_k": 5
      }
    }
  },
  "next_steps": [
    "Proceed with the eval, but review warning caveats before recording a conclusion."
  ]
}
```

Use typed Pydantic models in `services/eval/app/models.py` rather than passing
opaque dictionaries between handlers and tests.

## Data Sources

The readiness API should gather evidence from existing services:

- Eval DB: dataset item count, expected sources, baseline run, experiment
  metadata.
- Ingestion `/collections`: collection existence and point count.
- Ingestion `/collections/{name}/config`: chunking, embedding, dense/sparse
  metadata.
- Chat `/config`: runtime retrieval mode, top-k, vector names, rerank settings,
  prompt version, model settings.

Expected-source coverage needs a conservative version 1 implementation. Add a
narrow ingestion endpoint:

```text
GET /collections/{name}/sources
```

It should return distinct filenames and counts, not document text. The gate can
then compare dataset expected sources against indexed source names without
performing vector search or reading all documents.

## Data Flow

1. User or agent selects dataset, collection, rerank, and retrieval config.
2. MCP calls `check_rag_eval_readiness`.
3. Eval API gathers dataset, collection, collection config, source coverage,
   chat config, and optional baseline/experiment compatibility.
4. Eval API classifies findings as blocking failures or warnings.
5. MCP displays the readiness result.
6. If blocked, MCP stops and suggests remediation.
7. If ready or warning, MCP starts the experiment/run.
8. Eval API stores the readiness result in the run config alongside existing
   config capture.
9. `get_eval_run_evidence` and `summarize_eval_experiment` include readiness
   status and findings.

## MCP Workflow Changes

Update the eval MCP prompt and workflow resource:

- Datasets and collections remain separate concepts.
- Always call readiness before starting experiments or runs.
- Treat `blocked` as a stop condition.
- Treat `warning` as a caveated run condition.
- Mention readiness status in final experiment summaries.

New MCP tool:

```text
check_rag_eval_readiness(dataset_id, collection, rerank?, retrieval_config?, baseline_eval_id?, experiment_id?)
```

Existing `start_eval_experiment` and `start_eval_run` should also call the
workflow service readiness check internally. This prevents agents from skipping
the standalone tool by mistake.

## Persistence

Persist readiness status and findings in evaluation config:

```json
{
  "readiness": {
    "status": "warning",
    "blocking_failures": [],
    "warnings": [
      {
        "code": "partial_expected_source_coverage",
        "message": "12 of 15 expected source references were observed.",
        "remediation": "Review missing sources before deciding."
      }
    ],
    "checked_at": "2026-05-20T00:00:00+00:00"
  }
}
```

The existing config capture remains the source for runtime chat and collection
settings. Readiness records whether those settings passed policy checks before
the run started.

## Error Handling

Readiness checks should return structured findings for expected operational
problems rather than generic 500s:

- Missing collection: blocked finding.
- Empty collection: blocked finding.
- Missing collection config: blocked finding.
- Chat config unavailable: blocked finding.
- Source inventory unavailable: blocked finding, because version 1 depends on
  this endpoint to distinguish valid source-sensitive evals from misleading
  runs.

Unexpected exceptions should still produce a 500 with logs, because they
represent defects in the readiness service rather than normal readiness
outcomes.

## Testing

Python eval tests:

- Blocks missing collection.
- Blocks empty collection.
- Blocks missing collection config.
- Blocks dataset with no items.
- Blocks baseline dataset or collection mismatch.
- Blocks chat/collection dense vector mismatch.
- Blocks hybrid chat config against non-hybrid collection metadata.
- Warns on partial expected-source coverage.
- Warns when requested rerank is true but chat rerank is disabled.
- Persists readiness results into evaluation config.
- `get_eval_run_evidence` includes readiness status.

Python ingestion tests:

- If adding `/collections/{name}/sources`, returns distinct filenames and
  counts.
- Rejects invalid collection names.
- Returns 404 for missing collection.

Go eval MCP tests:

- Adds `check_rag_eval_readiness` schema and handler.
- `start_eval_experiment` refuses to proceed on blocked readiness.
- `start_eval_run` refuses to proceed on blocked readiness.
- Ready and warning readiness statuses allow the run request.
- Workflow prompt/resource instruct agents to call readiness first.
- Run evidence and experiment summary expose readiness findings.

Frontend tests can wait for a follow-up unless the first implementation touches
the eval UI.

## Rollout Plan

1. Add Python eval readiness models and service-level policy.
2. Add any narrow ingestion source inventory endpoint needed for source
   coverage.
3. Add `POST /eval/readiness/rag`.
4. Store readiness output in run config when starting evaluations.
5. Surface readiness in `get_eval_run_evidence`.
6. Add eval MCP tool and internal workflow guard.
7. Update MCP prompt/workflow resource.
8. Run targeted Python and Go tests, then the relevant preflights.

## Follow-Up Projects

This spec intentionally leaves two larger projects for separate GitHub issues:

- [#340](https://github.com/kabradshaw1/portfolio/issues/340) Experiment
  Reliability Orchestrator: sequential baseline/candidate workflow, timeout
  evidence packets, stale-run handling, and baseline validity gates.
- [#341](https://github.com/kabradshaw1/portfolio/issues/341) EvalOps Decision
  Packet: combined readiness, score deltas, worst cases, triage, observability
  evidence, and final keep/revert/needs-more-data summary.
