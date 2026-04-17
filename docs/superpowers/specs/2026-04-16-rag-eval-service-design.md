# RAG Evaluation Service — Design Spec

## Context

Kyle is building a portfolio demonstrating Gen AI engineering skills. The existing RAG pipeline (ingestion → chat) works but has no way to measure quality. A RAG evaluation service fills this gap — showing interviewers that Kyle understands not just how to build RAG, but how to measure and improve it. This is the first of three planned observability services (eval, pipeline logger, MCP agent tracer).

The MCP-RAG bridge (in progress on `agent/fix-pip-audit-backlog`) adds `/search` and `/chat` JSON endpoints to the chat service. The eval service depends on those endpoints existing.

## Prerequisites

- MCP-RAG bridge merged (provides `POST /search` and `POST /chat` with `Accept: application/json` on the chat service). Currently in progress on `agent/fix-pip-audit-backlog`.

## Architecture

New Python FastAPI microservice at `services/eval/`, deployed in the `ai-services` K8s namespace alongside chat/ingestion/debug.

**Core flow:**
1. User uploads a golden dataset (query + expected answer + expected source triples)
2. User triggers an evaluation run against a dataset
3. Eval service calls `/search` (retrieval) and `/chat` with `Accept: application/json` (generation) on the existing chat service for each query
4. Results are scored using RAGAS (faithfulness, answer relevancy, context precision, context recall)
5. Scores are stored in SQLite and returned as a scorecard

**Dependencies:**
- `ragas` — industry-standard RAG evaluation framework
- `datasets` — HuggingFace datasets (RAGAS dependency)
- `httpx` — async HTTP client for calling chat service
- `aiosqlite` — async SQLite for storing results
- Shared modules: `shared.auth` (JWT), `shared.llm` (LLM factory for RAGAS judge calls)
- Prometheus instrumentation via `prometheus-fastapi-instrumentator`

## API Endpoints

### `POST /datasets`
Upload a golden dataset for reuse across evaluation runs.

**Request:**
```json
{
  "name": "document-qa-v1",
  "items": [
    {
      "query": "What is the chunking strategy?",
      "expected_answer": "RecursiveCharacterTextSplitter with chunk_size=1000 and overlap=200",
      "expected_sources": ["ingestion-adr.pdf"]
    }
  ]
}
```

**Response:** `201` with dataset ID.

### `GET /datasets`
List available datasets. Paginated.

### `POST /evaluations`
Start an evaluation run against a dataset.

**Request:**
```json
{
  "dataset_id": "ds-abc123",
  "collection": "documents"
}
```

**Response:** `202` with evaluation ID (run starts in background).

### `GET /evaluations/{id}`
Get evaluation results.

**Response:**
```json
{
  "id": "eval-abc123",
  "status": "completed",
  "dataset": "document-qa-v1",
  "created_at": "2026-04-16T12:00:00Z",
  "duration_seconds": 145.3,
  "aggregate": {
    "faithfulness": 0.87,
    "answer_relevancy": 0.92,
    "context_precision": 0.78,
    "context_recall": 0.85
  },
  "results": [
    {
      "query": "What is the chunking strategy?",
      "answer": "The ingestion service uses...",
      "contexts": ["chunk1...", "chunk2..."],
      "scores": {
        "faithfulness": 0.9,
        "answer_relevancy": 0.95,
        "context_precision": 0.8,
        "context_recall": 0.88
      }
    }
  ]
}
```

### `GET /evaluations`
List past evaluation runs with summary scores. Paginated, sorted by date.

### `GET /health`
Returns healthy/degraded based on chat service reachability and LLM availability.

## RAGAS Metrics

Four metrics, each scored 0-1:

- **Faithfulness** — Is every claim in the answer supported by the retrieved context? Uses LLM-as-judge to decompose answer into statements and verify each against chunks. Catches hallucination.
- **Answer Relevancy** — Does the answer address the question? Generates hypothetical questions from the answer and measures cosine similarity to the original. Catches tangential responses.
- **Context Precision** — Are the top-ranked retrieved chunks the relevant ones? Evaluates retrieval ranking quality.
- **Context Recall** — Did retrieval find all relevant information? Compares expected answer against retrieved contexts. Requires ground truth from golden dataset.

RAGAS judge calls use the existing shared LLM factory (Ollama / Qwen 2.5 14B). Each evaluation query requires the RAG pipeline call + 2-3 LLM judge calls, so runs are async.

## Module Layout

```
services/eval/
├── Dockerfile
├── requirements.txt
├── app/
│   ├── __init__.py
│   ├── main.py          # FastAPI app, endpoints, background task dispatch
│   ├── config.py         # Pydantic Settings (chat_url, llm settings, db path)
│   ├── metrics.py        # Prometheus: eval_run_duration, ragas_scores, queries_total
│   ├── db.py             # SQLite setup via aiosqlite, CRUD for datasets/evaluations
│   ├── evaluator.py      # RAGAS integration, dataset format conversion, score extraction
│   ├── rag_client.py     # httpx client calling /search and /chat on chat service
│   └── models.py         # Pydantic models (Dataset, EvalRun, Scorecard, GoldenItem)
└── tests/
    ├── conftest.py       # Fixtures: in-memory SQLite, mock RAG responses, rate limiter
    ├── test_main.py      # Endpoint tests (validation, auth, pagination, status codes)
    ├── test_evaluator.py # RAGAS wrapper tests (mocked evaluate(), format conversion)
    ├── test_rag_client.py # HTTP client tests (mock server, error handling)
    └── test_db.py        # SQLite CRUD, status transitions, pagination queries
```

## Data Storage

SQLite via `aiosqlite`, stored at a configurable path (PVC in K8s, local file in dev).

**Tables:**

`datasets`:
- `id` TEXT PRIMARY KEY
- `name` TEXT UNIQUE
- `items` TEXT (JSON)
- `created_at` TIMESTAMP

`evaluations`:
- `id` TEXT PRIMARY KEY
- `dataset_id` TEXT REFERENCES datasets(id)
- `status` TEXT (running, completed, failed)
- `collection` TEXT
- `aggregate_scores` TEXT (JSON)
- `results` TEXT (JSON)
- `error` TEXT
- `created_at` TIMESTAMP
- `completed_at` TIMESTAMP

## Custom Prometheus Metrics

- `eval_run_duration_seconds` — histogram of full evaluation run time
- `eval_ragas_score` — gauge by metric name (faithfulness, answer_relevancy, context_precision, context_recall)
- `eval_queries_total` — counter of queries evaluated

Plus standard RED metrics from `prometheus-fastapi-instrumentator`.

## Testing Strategy

**Unit tests** (run in CI):
- `test_main.py` — endpoint validation, auth, health check, pagination
- `test_rag_client.py` — mock HTTP server for /search and /chat, error handling (timeouts, 503s)
- `test_evaluator.py` — mock RAGAS `evaluate()`, dataset format conversion, partial failure handling
- `test_db.py` — SQLite CRUD, status transitions

**Integration tests** (manual, post-deploy):
- Upload a 3-5 query golden dataset
- Run evaluation against real RAG pipeline
- Verify scores are reasonable (not all 0.0 or 1.0)

**ADR notebook** — `docs/adr/rag-evaluation/` following existing notebook format:
- RAGAS concepts and metric interpretation
- Diagnosing low scores (what to fix for each metric)
- LLM-as-judge patterns

## Infrastructure

**Docker Compose:**
- New `eval` service, depends on `chat`
- Port 8000 internal, NGINX routes `/eval/*`
- Volume for SQLite persistence

**K8s (`k8s/ai-services/`):**
- `deployments/eval.yml` — 1 replica, 512Mi/500m resources
- `services/eval.yml` — ClusterIP:8000
- `configmaps/eval-config.yml` — CHAT_URL pointing to chat service
- PVC (1Gi) for SQLite
- Update `kustomization.yaml`, `ingress.yml`, network policy

**CI (`ci.yml`):**
- Add `eval` to matrices: backend-tests, docker-build, pip-audit, hadolint

**NGINX:**
- Add `/eval/` routing rule

## Verification

1. `make preflight-python` — ruff + pytest pass for all services including eval
2. `make preflight-security` — pip-audit, bandit pass
3. Docker Compose smoke test — eval service starts, health check returns 200
4. Manual: upload golden dataset, trigger eval run, verify scorecard returned with scores
5. K8s deploy: eval pod healthy, `/eval/health` returns 200 through ingress
