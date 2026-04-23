# RAG Evaluation Service

- **Date:** 2026-04-17
- **Status:** Accepted

## Context

The RAG pipeline (ingestion → chat) was functional but had no way to measure quality. A Gen AI Engineer should be able to answer: "How good are the answers?" and "What metrics prove it?" Without evaluation, we couldn't distinguish a working pipeline from a good one.

We also wanted to deepen our understanding of RAG evaluation concepts — faithfulness, answer relevancy, context precision, context recall — and learn the RAGAS framework, which is the industry standard for RAG quality measurement.

## Decision

Built a new Python FastAPI microservice at `services/eval/` that evaluates RAG pipeline quality using RAGAS metrics. The service:

- **Manages golden datasets** — curated query + expected answer + expected source triples uploaded via API, stored in SQLite
- **Runs evaluations** — for each golden query, calls the chat service's `/search` (retrieval) and `/chat` (generation) endpoints, then scores results with RAGAS
- **Stores scorecards** — per-query scores and aggregate metrics persisted in SQLite, retrievable via API
- **Runs asynchronously** — evaluation is a background task (RAGAS judge calls are slow), status polled via `GET /evaluations/{id}`

### RAGAS Metrics

Four metrics, each scored 0–1:

- **Faithfulness** — Is every claim in the answer supported by the retrieved context? Uses LLM-as-judge to decompose the answer into statements and verify each against chunks. Catches hallucination.
- **Answer Relevancy** — Does the answer address the question? Generates hypothetical questions from the answer and measures cosine similarity to the original. Catches tangential responses.
- **Context Precision** — Are the top-ranked retrieved chunks the relevant ones? Evaluates retrieval ranking quality.
- **Context Recall** — Did retrieval find all relevant information? Compares expected answer against retrieved contexts. Requires ground truth from the golden dataset.

RAGAS uses LLM-as-judge for faithfulness and answer relevancy scoring. Judge calls go through the same Ollama/Qwen 2.5 14B used by the RAG pipeline.

### Architecture

```
POST /evaluations
  → validate golden dataset
  → create eval record (status: running)
  → background task:
      for each query:
        POST /search → retrieved chunks
        POST /chat (Accept: application/json) → generated answer
      → RAGAS evaluate(dataset, metrics)
      → store per-query scores + aggregates
      → update status to completed
```

The eval service depends on the MCP-RAG bridge endpoints (`POST /search` and `POST /chat` JSON mode) added to the chat service in the same release cycle.

### Technical Details

- **RAGAS + uvloop incompatibility:** RAGAS calls `nest_asyncio.apply()` at import time, which crashes with uvloop (used by uvicorn). Fixed by lazy-importing all RAGAS modules inside `run_evaluation()` rather than at module level.
- **Storage:** SQLite via `aiosqlite` — lightweight, no infrastructure needed. Stored on an `emptyDir` volume in K8s (evaluation results are ephemeral/reproducible).
- **Auth:** JWT via shared auth module, same as all other Python services.
- **Metrics:** Custom Prometheus gauges/histograms for evaluation run duration, RAGAS scores, and query count.

### API Endpoints

| Endpoint | Method | Purpose |
|----------|--------|---------|
| `/datasets` | POST | Upload a golden dataset |
| `/datasets` | GET | List available datasets |
| `/evaluations` | POST | Start an evaluation run (async) |
| `/evaluations/{id}` | GET | Get evaluation results/status |
| `/evaluations` | GET | List past evaluation runs |
| `/health` | GET | Health check (chat service reachability) |

## Consequences

**Positive:**
- Can now measure and track RAG quality over time with industry-standard metrics
- Golden datasets make evaluation reproducible and comparable across changes
- Demonstrates understanding of RAG evaluation concepts for portfolio/interviews
- LLM-as-judge pattern is a core Gen AI skill

**Trade-offs:**
- RAGAS 0.2.x pulls a large transitive dependency tree (langchain 0.2.x, 200+ packages) — adds ~20 min to uncached CI installs (mitigated by venv caching)
- 11 known CVEs in transitive deps (all assessed as unexploitable — see `ragas-cve-risk-assessment.md`)
- Evaluation runs are slow (~2-5 min per query due to RAG pipeline + LLM judge calls)
- SQLite on emptyDir means evaluation history is lost on pod restart (acceptable for a portfolio project)

**Future work:**
- RAG pipeline logger service (structured logging of every RAG query for debugging)
- MCP agent tracer service (recording agent tool-call sequences and reasoning)
- Upgrade to RAGAS 0.3.0 when stable (resolves transitive CVEs, modernizes langchain deps)
