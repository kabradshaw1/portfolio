# RAG Triage Service Design

## TL;DR

Add a new Python FastAPI service, `services/rag-triage`, that analyzes RAG
evaluation runs and comparisons from the eval API. The service returns
deterministic regression diagnoses, worst-case groupings, confidence, evidence,
and recommended follow-up experiments. Add a thin `triage_rag_regression` tool
to the eval MCP so agents can use the service from the existing eval workflow.

LangGraph is intentionally deferred. It becomes appropriate after triage grows
into a resumable, conditional investigation workflow with branching evidence
collection and human approval checkpoints.

## Goals

- Keep eval as the source of truth for datasets, runs, comparisons, configs,
  experiment labels, conclusions, and persisted results.
- Add a companion analysis service that interprets eval results without
  mutating eval state in v1.
- Preserve the eval MCP as the user-facing workflow adapter.
- Keep v1 deterministic and testable; do not make an LLM the source of truth.
- Leave room for optional LLM summaries and LangGraph orchestration later.

## Non-Goals

- Do not move eval run storage or scoring out of `services/eval`.
- Do not read the eval database directly.
- Do not add gRPC between eval and triage in v1.
- Do not launch follow-up eval runs from triage in v1.
- Do not add LangGraph until the workflow has real branching, persistence, or
  human-in-the-loop needs.

## Architecture

`services/rag-triage` is a new Python/FastAPI service. It calls the eval API
over HTTP using the same deployment boundary as other Python services. The
service is read-only against eval data in v1.

The eval MCP gets a small HTTP client for the triage service and one additive
tool:

- `triage_rag_regression`

The MCP tool validates arguments, calls the triage API, and returns the
structured JSON response. Classification logic stays in the Python service, not
the Go MCP.

Service responsibilities:

- Fetch eval run details, comparison data, and captured config from eval.
- Rank or select worst cases by requested metric.
- Classify likely failure modes using deterministic rules.
- Group similar failures into clusters.
- Recommend next experiments.
- Return machine-readable evidence and concise human-readable summaries.

## API Shape

### `GET /health`

Returns service health and whether the eval API is reachable.

### `POST /triage/eval-run`

Input:

```json
{
  "eval_id": "eval-id",
  "metric": "context_precision",
  "limit": 5,
  "include_observability": false
}
```

Output:

```json
{
  "subject": {"type": "eval_run", "eval_id": "eval-id"},
  "status": "completed",
  "aggregate_scores": {},
  "config": {},
  "diagnosis": {
    "primary_failure_mode": "retrieval_precision",
    "confidence": "medium",
    "summary": "Retrieved contexts are noisy relative to the reference answers."
  },
  "clusters": [],
  "cases": [],
  "recommendations": []
}
```

### `POST /triage/comparison`

Input:

```json
{
  "baseline_eval_id": "baseline-id",
  "candidate_eval_id": "candidate-id",
  "metric": "context_precision",
  "limit": 5,
  "include_observability": false
}
```

Output includes baseline and candidate summaries, metric deltas, changed
failure modes, clusters, per-query diagnoses, and recommendations.

## Failure Modes

V1 classifications are deterministic:

- `retrieval_recall`: low `context_recall`, especially when generated answer is
  also weak.
- `retrieval_precision`: low `context_precision` with acceptable recall,
  suggesting noisy retrieval or reranker behavior.
- `generation_faithfulness`: low `faithfulness` with usable retrieved context.
- `answer_relevance`: low `answer_relevancy`, suggesting the answer does not
  target the question or reference.
- `runtime_or_config`: failed runs, missing results, captured config mismatch,
  timeout, or upstream error evidence.
- `insufficient_evidence`: contradictory or sparse signals.

Each case includes:

- query
- answer
- scores
- score reasons when available
- relevant config/evidence
- assigned failure mode
- confidence
- rationale

## Recommendations

Recommendations are structured objects, not prose-only advice. Examples:

- `increase_top_k`
- `enable_or_tune_rerank`
- `inspect_collection_ingestion`
- `adjust_chunking`
- `prompt_grounding_change`
- `review_expected_answer`
- `inspect_runtime_evidence`

Each recommendation includes a reason, expected impact, and the evidence that
triggered it.

## Eval MCP Integration

Add eval MCP configuration for the triage API base URL, defaulting to the local
compose service URL when appropriate.

Add a small package such as `internal/triageapi` with:

- request/response DTOs
- HTTP client
- auth/token handling consistent with existing eval API calls where needed
- typed errors for non-2xx responses

Add one tool to the eval MCP:

- Name: `triage_rag_regression`
- Inputs: `eval_id`, optional `baseline_eval_id`, optional `metric`, optional
  `limit`, optional `include_observability`
- Behavior: call `services/rag-triage` and return JSON

The handler should be thin. It should not duplicate triage rules.

## LangGraph Path

LangGraph is deferred until there is a natural graph-shaped workflow:

1. Fetch eval evidence.
2. Classify likely failure mode.
3. Branch to retrieval, generation, config, or runtime evidence gathering.
4. Ask for approval before launching follow-up evals.
5. Resume from checkpoint after the eval completes.
6. Compare follow-up results and update the investigation summary.

At that point, LangGraph belongs inside `services/rag-triage` as an optional
investigation runtime above the deterministic triage core.

## Testing

Python service tests:

- health endpoint
- eval API client success and error handling
- single-run triage classifications
- comparison triage classifications
- recommendation generation
- insufficient-evidence handling
- auth/header behavior if required

Eval MCP tests:

- tool registration
- request validation
- triage API client success path
- non-2xx triage API errors
- JSON result passthrough

Preflight before committing implementation:

- `make preflight-python`
- `make preflight-go`
- `make preflight-security`

## Rollout

1. Scaffold `services/rag-triage` with FastAPI, config, metrics, health, tests,
   Dockerfile, and compose wiring.
2. Implement deterministic triage against mocked eval API responses.
3. Wire the service to the real eval API over HTTP.
4. Add the eval MCP `triage_rag_regression` wrapper.
5. Add deployment manifests only after local and compose behavior are stable.
