# First-Party RAG Evaluation Metrics

- **Date:** 2026-05-09
- **Status:** Accepted

## Context

The document QA evaluation service originally used RAGAS to score RAG quality.
That worked for the first evaluation milestone, but the service only needed a
small part of the framework:

- build evaluation rows from golden questions, retrieved contexts, generated
  answers, and references
- score `faithfulness`, `answer_relevancy`, `context_precision`, and
  `context_recall`
- return aggregate scores plus per-query score details

RAGAS pulled in a large transitive dependency tree, including LangChain,
LangSmith, diskcache, and dataset tooling that the eval service did not use.
Those dependencies created recurring security-audit noise and forced CI to
carry a broad `pip-audit --ignore-vuln` list. The ignore list had a documented
risk assessment, but it was still a weak long-term posture: new Python
dependency advisories could be hidden by stale suppressions.

The portfolio goal is production-grade engineering, so the preferred direction
is a smaller evaluator whose behavior is explicit in the codebase and whose
dependencies can pass audit without broad ignores.

## Decision

Replace the eval service's RAGAS dependency with first-party evaluation logic in
`services/eval`.

The eval service keeps the existing API and stored metric keys:

- `faithfulness`
- `answer_relevancy`
- `context_precision`
- `context_recall`

The evaluator now has four focused responsibilities:

1. Build raw evaluation rows by calling the chat service search and answer
   endpoints through `RAGClient`.
2. Compute deterministic retrieval metrics with normalized token overlap:
   `context_precision` and `context_recall`.
3. Score generated answers with a configurable LLM judge for `faithfulness` and
   `answer_relevancy`.
4. Aggregate per-query scores into the same result shape the frontend and API
   already consume.

Malformed judge output is treated as an evaluation failure. The service parses
strict JSON, requires both judged metrics, clamps numeric scores to `[0, 1]`,
and returns judge reasons in per-query results.

The Prometheus series name `eval_ragas_score` remains unchanged for dashboard
compatibility, but the application variable was renamed to
`eval_quality_score` and visible frontend copy now describes first-party RAG
quality evaluation.

RAGAS and `datasets` were removed from `services/eval/requirements.txt`.
Python dependency pins were updated across the affected services so CI can run
`pip-audit` without the stale ignore list. `make preflight-security` now also
audits each Python service locally in a Python 3.11 virtual environment.

## Consequences

**Positive:**

- The eval service has a smaller, easier-to-review dependency surface.
- CI fails on Python dependency advisories by default instead of carrying broad
  suppressions.
- Local security preflight now includes Python dependency auditing, so audit
  failures are caught before pushing.
- API consumers and stored evaluation results do not need a metric-key
  migration.
- The scoring behavior is explicit, unit-tested, and easier to explain in code
  review than a broad framework integration.

**Trade-offs:**

- The service no longer implements the full RAGAS metric algorithms. Retrieval
  metrics are intentionally simple token-overlap heuristics.
- LLM-judged `faithfulness` and `answer_relevancy` depend on prompt discipline
  and strict response parsing rather than framework-provided metric classes.
- Historical references to "RAGAS scores" in older run records or dashboard
  metric names may remain as compatibility artifacts.
- `make preflight-security` now requires Python 3.11 locally because that is
  the Python version used by the service CI jobs.

**Future work:**

- If retrieval scoring needs to become more source-aware, add a first-party
  source-hit component using `expected_sources` from golden dataset items.
- If judge quality becomes noisy, add prompt fixtures and calibration datasets
  rather than adopting another broad evaluation framework by default.
- Rename the Prometheus series away from `eval_ragas_score` only as a separate
  dashboard migration.
