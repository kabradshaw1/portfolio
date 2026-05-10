# Python Eval RAGAS Replacement Design

## TL;DR

Replace the eval service's RAGAS dependency with a first-party focused evaluator
that preserves the existing FastAPI API and result shape while removing the
unfixed RAGAS and transitive dependency vulnerabilities from the Python security
gate.

## Problem

The CI `security-pip-audit` job currently ignores a broad set of Python
dependency advisories. Most can be removed by upgrading direct dependencies, but
the eval service's `ragas` dependency is the hard case. Upgrading RAGAS removes
the old LangChain advisory cluster, but current RAGAS still leaves unfixed
advisories in RAGAS itself and its `diskcache` dependency.

The current eval service uses only a narrow RAGAS surface:

- Build evaluation rows from golden questions, retrieved contexts, generated
  answers, and references.
- Score four metrics: `faithfulness`, `answer_relevancy`,
  `context_precision`, and `context_recall`.
- Return aggregate scores and per-query score details.

The service does not need RAGAS dataset generation, multimodal metrics,
LangChain loaders, serialization utilities, or the broader RAGAS integration
stack.

## Goals

- Remove `ragas` from `services/eval/requirements.txt`.
- Preserve the eval service API and persisted run/result shape.
- Keep the four existing metric names so frontend/API consumers do not need a
  migration.
- Make dependency auditing meaningful by removing the stale broad
  `pip-audit --ignore-vuln` list from CI.
- Keep the implementation small, testable, and understandable in code review.

## Non-Goals

- Recreate the full RAGAS metric suite.
- Add a new third-party eval framework.
- Change the chat or ingestion service contracts.
- Add paid cloud dependencies beyond the service's existing configurable LLM
  providers.

## Design

`services/eval/app/evaluator.py` remains the orchestration boundary. It will no
longer import RAGAS. It will:

1. Build raw evaluation rows by calling the existing `RAGClient.search` and
   `RAGClient.ask` methods.
2. Score each row with first-party metric functions.
3. Aggregate metric values across rows.
4. Return the same `(aggregate, per_query)` tuple shape used today.

The evaluator will use two metric categories.

### Deterministic Retrieval Metrics

`context_recall` measures how much of the reference answer is covered by the
retrieved contexts. The initial implementation should use normalized token
overlap between the reference answer and all retrieved context text. Empty
references or empty contexts produce `0.0`.

`context_precision` measures how much retrieved context appears useful for the
question/reference pair. The initial implementation should score each retrieved
context using normalized token overlap against the query plus reference answer,
then average the context scores. Empty contexts produce `0.0`.

If a golden item includes `expected_sources`, the evaluator may add a source-hit
component later, but the first implementation should not require source metadata
because the current raw evaluator rows only retain context text.

### LLM-Judged Generation Metrics

`faithfulness` checks whether the generated answer is supported by retrieved
contexts.

`answer_relevancy` checks whether the generated answer addresses the query and
matches the reference answer.

Both metrics use a small judge adapter backed by the eval service's configured
LLM provider, base URL, model, and API key. The judge prompt must request strict
JSON with a numeric `score` in `[0, 1]` and a short `reason`. The evaluator
validates the JSON, clamps accepted scores to `[0, 1]`, and treats malformed
judge output as a clear evaluation failure rather than silently passing a bad
run.

The first implementation can use one combined judge call per row that returns
both generation metrics. That keeps runtime bounded and simplifies tests.

## Error Handling

- Missing or malformed judge JSON raises an evaluation error with enough context
  to diagnose the failed row and metric.
- Empty retrieved contexts produce deterministic retrieval scores of `0.0`; the
  LLM judge still receives the answer and query so it can score relevancy.
- Empty item lists return empty per-query results and `None` aggregates for all
  four metrics.
- LLM provider/network failures fail the evaluation run rather than fabricating
  scores.

## Testing

Unit tests should cover:

- Raw dataset construction still calls search and ask for each golden item.
- Deterministic retrieval metrics handle normal rows, empty contexts, and empty
  references.
- Judge output parsing accepts valid JSON, clamps boundary values, and rejects
  malformed or incomplete JSON.
- `run_evaluation` preserves aggregate and per-query response shapes.
- The service no longer imports RAGAS in tests or application code.

Security verification should cover:

- `pip-audit` for each Python service without the old broad ignore list.
- `.github/workflows/ci.yml` no longer carries the stale broad
  `--ignore-vuln` list.
- `make preflight-security` includes the Python dependency audit path.
- `make preflight-python`.
- `make preflight-security`.

## CI And Dependency Policy

After replacing RAGAS, the Python security work should update CI so
`security-pip-audit` fails on new Python advisories by default. The current
broad `--ignore-vuln` list should be removed from `.github/workflows/ci.yml`.
Any remaining ignore must be narrow, documented next to the command, and tied
to an advisory with no available fixed version. Stale ignored IDs should be
removed.

Local preflight should also grow an equivalent `pip-audit` check so developers
do not discover Python dependency advisories only after pushing to CI.

The same dependency upgrades should be made locally and in CI:

- Upgrade `python-multipart` to a fixed version.
- Upgrade `langchain-text-splitters` to a fixed version for ingestion/debug.
- Upgrade `pytest` and `pytest-asyncio` together because current
  `pytest-asyncio` rejects `pytest>=9`.

## Open Questions

No product behavior changes are expected. The only implementation choice left is
the exact LLM client helper used by the judge adapter; it should follow the
existing eval/shared provider pattern already used in the Python services.
