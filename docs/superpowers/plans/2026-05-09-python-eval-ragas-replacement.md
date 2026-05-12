# Python Eval RAGAS Replacement Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the eval service's RAGAS dependency with a small first-party evaluator and remove the stale Python dependency audit ignores.

**Architecture:** Keep the eval service API and stored metric keys unchanged. Split evaluator logic into focused first-party units: dataset construction, deterministic retrieval metrics, an LLM judge adapter, and aggregation. Update CI/local security checks so `pip-audit` fails by default instead of carrying the broad ignore list.

**Tech Stack:** Python 3.11, FastAPI, pytest, httpx/openai-compatible LLM calls through existing shared provider patterns, GitHub Actions, pip-audit, Next.js copy updates.

---

## File Structure

- Modify `services/eval/app/evaluator.py`: replace all RAGAS imports and orchestration with first-party dataset building, retrieval metrics, judge parsing, and aggregation.
- Modify `services/eval/tests/test_evaluator.py`: rewrite tests away from `ragas.evaluate`; add unit coverage for retrieval metrics, judge parsing, malformed judge output, empty inputs, and result shape.
- Modify `services/eval/app/config.py`: change comments from RAGAS judge to first-party eval judge.
- Modify `services/eval/app/main.py`: rename internal `_RAGAS_METRICS` to `_EVAL_METRICS`, update task docstring, and update metric import/use.
- Modify `services/eval/app/metrics.py`: keep the existing Prometheus metric name `eval_ragas_score` if dashboard compatibility is desired, but rename the Python variable to `eval_quality_score` and update the help text. If no dashboard depends on the old metric name, rename the Prometheus series in a separate follow-up, not in this task.
- Modify `services/eval/requirements.txt`: remove `ragas` and `datasets`; upgrade pytest-related pins.
- Modify `services/ingestion/requirements.txt`: upgrade `python-multipart`, `langchain-text-splitters`, `pytest`, and `pytest-asyncio`.
- Modify `services/debug/requirements.txt`: upgrade `langchain-text-splitters`, `pytest`, and `pytest-asyncio`.
- Modify `services/chat/requirements.txt`: upgrade `pytest` and `pytest-asyncio`.
- Modify `Makefile`: add `pip-audit` to `preflight-security`.
- Modify `.github/workflows/ci.yml`: remove the stale broad `--ignore-vuln` list and update comments.
- Modify frontend copy in:
  - `frontend/src/components/eval/DashboardTab.tsx`
  - `frontend/src/app/ai/page.tsx`
  - `frontend/src/app/ai/eval/page.tsx`
  - `frontend/src/app/cicd/page.tsx`

## Task 1: Rewrite Evaluator Tests First

**Files:**
- Modify: `services/eval/tests/test_evaluator.py`
- Implementing file in Task 2: `services/eval/app/evaluator.py`

- [ ] **Step 1: Replace RAGAS test imports with first-party evaluator imports**

Change the top of `services/eval/tests/test_evaluator.py` to:

```python
from unittest.mock import AsyncMock, MagicMock

import pytest
from app.evaluator import (
    EvaluationError,
    JudgeScores,
    build_evaluation_dataset,
    parse_judge_scores,
    run_evaluation,
    score_context_precision,
    score_context_recall,
)
from app.rag_client import RAGClient
```

- [ ] **Step 2: Rename dataset construction tests**

Replace `test_build_ragas_dataset` with:

```python
@pytest.mark.asyncio
async def test_build_evaluation_dataset(golden_items, mock_search_results, mock_chat_answer):
    rag_client = MagicMock(spec=RAGClient)
    rag_client.search = AsyncMock(return_value=mock_search_results)
    rag_client.ask = AsyncMock(return_value=mock_chat_answer)

    dataset = await build_evaluation_dataset(
        items=golden_items,
        rag_client=rag_client,
        collection=None,
    )

    assert len(dataset) == 2
    assert dataset[0]["user_input"] == "What is chunking?"
    assert dataset[0]["response"] == (
        "Chunking splits text into smaller pieces for embedding and retrieval."
    )
    assert len(dataset[0]["retrieved_contexts"]) == 2
    assert dataset[0]["reference"] == (
        "Splitting text into smaller pieces for embedding."
    )
    assert dataset[0]["expected_sources"] == ["ingestion.pdf"]

    assert rag_client.search.call_count == 2
    assert rag_client.ask.call_count == 2
```

Replace `test_build_ragas_dataset_with_collection` with:

```python
@pytest.mark.asyncio
async def test_build_evaluation_dataset_with_collection(
    golden_items, mock_search_results, mock_chat_answer
):
    rag_client = MagicMock(spec=RAGClient)
    rag_client.search = AsyncMock(return_value=mock_search_results)
    rag_client.ask = AsyncMock(return_value=mock_chat_answer)

    await build_evaluation_dataset(
        items=golden_items,
        rag_client=rag_client,
        collection="my-docs",
    )

    call_args = rag_client.search.call_args_list[0]
    assert (
        call_args.kwargs.get("collection") == "my-docs" or call_args[0][1] == "my-docs"
    )
```

- [ ] **Step 3: Add deterministic retrieval metric tests**

Add these tests below the dataset construction tests:

```python
def test_score_context_recall_counts_reference_terms_in_contexts():
    score = score_context_recall(
        reference="Splitting text into smaller pieces for embedding.",
        contexts=[
            "Text chunking splits documents into smaller pieces.",
            "Embedding stores chunks for retrieval.",
        ],
    )

    assert score == pytest.approx(0.8, abs=0.0001)


def test_score_context_precision_averages_context_usefulness():
    score = score_context_precision(
        query="What is chunking?",
        reference="Splitting text into smaller pieces for embedding.",
        contexts=[
            "Text chunking splits documents into smaller pieces.",
            "The deployment uses Kubernetes ingress.",
        ],
    )

    assert score == pytest.approx(0.3333, abs=0.0001)


def test_context_scores_are_zero_for_empty_inputs():
    assert score_context_recall(reference="", contexts=["anything"]) == 0.0
    assert score_context_recall(reference="answer", contexts=[]) == 0.0
    assert score_context_precision(query="question", reference="answer", contexts=[]) == 0.0
```

- [ ] **Step 4: Add judge parser tests**

Add these tests:

```python
def test_parse_judge_scores_accepts_json_and_clamps_scores():
    scores = parse_judge_scores(
        '{"faithfulness": {"score": 1.2, "reason": "supported"}, '
        '"answer_relevancy": {"score": -0.1, "reason": "off topic"}}'
    )

    assert scores == JudgeScores(
        faithfulness=1.0,
        answer_relevancy=0.0,
        reasons={
            "faithfulness": "supported",
            "answer_relevancy": "off topic",
        },
    )


def test_parse_judge_scores_rejects_malformed_json():
    with pytest.raises(EvaluationError, match="judge returned invalid JSON"):
        parse_judge_scores("not json")


def test_parse_judge_scores_rejects_missing_metric():
    with pytest.raises(EvaluationError, match="missing answer_relevancy"):
        parse_judge_scores('{"faithfulness": {"score": 0.5, "reason": "partial"}}')
```

- [ ] **Step 5: Replace RAGAS run_evaluation test with first-party judge mock**

Replace the existing patched `test_run_evaluation` with:

```python
@pytest.mark.asyncio
async def test_run_evaluation_preserves_result_shape(
    golden_items,
    mock_search_results,
    mock_chat_answer,
):
    rag_client = MagicMock(spec=RAGClient)
    rag_client.search = AsyncMock(return_value=mock_search_results)
    rag_client.ask = AsyncMock(return_value=mock_chat_answer)

    judge = AsyncMock(
        side_effect=[
            JudgeScores(
                faithfulness=0.9,
                answer_relevancy=0.85,
                reasons={
                    "faithfulness": "answer is supported",
                    "answer_relevancy": "answer addresses the question",
                },
            ),
            JudgeScores(
                faithfulness=0.82,
                answer_relevancy=0.9,
                reasons={
                    "faithfulness": "mostly supported",
                    "answer_relevancy": "directly answers",
                },
            ),
        ]
    )

    aggregate, results = await run_evaluation(
        items=golden_items,
        rag_client=rag_client,
        collection=None,
        llm_provider="ollama",
        llm_base_url="http://localhost:11434",
        llm_model="qwen2.5:14b",
        llm_api_key="",
        judge=judge,
    )

    assert aggregate["faithfulness"] == 0.86
    assert aggregate["answer_relevancy"] == 0.875
    assert "context_precision" in aggregate
    assert "context_recall" in aggregate
    assert len(results) == 2
    assert results[0]["query"] == "What is chunking?"
    assert results[0]["scores"]["faithfulness"] == 0.9
    assert results[0]["score_reasons"]["faithfulness"] == "answer is supported"
```

- [ ] **Step 6: Add empty item test**

Add:

```python
@pytest.mark.asyncio
async def test_run_evaluation_empty_items_returns_empty_results():
    rag_client = MagicMock(spec=RAGClient)

    aggregate, results = await run_evaluation(
        items=[],
        rag_client=rag_client,
        collection=None,
        llm_provider="ollama",
        llm_base_url="http://localhost:11434",
        llm_model="qwen2.5:14b",
        llm_api_key="",
        judge=AsyncMock(),
    )

    assert aggregate == {
        "faithfulness": None,
        "answer_relevancy": None,
        "context_precision": None,
        "context_recall": None,
    }
    assert results == []
```

- [ ] **Step 7: Run evaluator tests and verify they fail for missing symbols**

Run:

```bash
PYTHONPATH=services/eval:services pytest services/eval/tests/test_evaluator.py -v
```

Expected: FAIL with import errors for `build_evaluation_dataset`, `EvaluationError`, `JudgeScores`, or the new scoring functions.

- [ ] **Step 8: Commit failing tests**

Do not commit if unrelated files are staged.

```bash
git add services/eval/tests/test_evaluator.py
git commit -m "test: define first-party eval behavior"
```

## Task 2: Implement First-Party Evaluator

**Files:**
- Modify: `services/eval/app/evaluator.py`
- Test: `services/eval/tests/test_evaluator.py`

- [ ] **Step 1: Replace evaluator implementation**

Replace `services/eval/app/evaluator.py` with this implementation:

```python
from __future__ import annotations

import json
import logging
import re
from dataclasses import dataclass
from typing import TYPE_CHECKING, Awaitable, Callable

from llm.factory import get_llm_provider

if TYPE_CHECKING:
    from app.rag_client import RAGClient

logger = logging.getLogger(__name__)

METRIC_NAMES = (
    "faithfulness",
    "answer_relevancy",
    "context_precision",
    "context_recall",
)

STOPWORDS = {
    "a",
    "an",
    "and",
    "are",
    "as",
    "for",
    "in",
    "into",
    "is",
    "it",
    "of",
    "on",
    "or",
    "the",
    "to",
    "what",
    "with",
}


class EvaluationError(RuntimeError):
    """Raised when an evaluation run cannot produce trustworthy scores."""


@dataclass(frozen=True)
class JudgeScores:
    faithfulness: float
    answer_relevancy: float
    reasons: dict[str, str]


JudgeFn = Callable[[dict], Awaitable[JudgeScores]]


def _tokens(text: str) -> set[str]:
    return {
        token
        for token in re.findall(r"[a-z0-9]+", text.lower())
        if len(token) > 2 and token not in STOPWORDS
    }


def _round_score(value: float) -> float:
    return round(max(0.0, min(1.0, value)), 4)


def score_context_recall(reference: str, contexts: list[str]) -> float:
    reference_terms = _tokens(reference)
    if not reference_terms or not contexts:
        return 0.0
    context_terms = _tokens(" ".join(contexts))
    return _round_score(len(reference_terms & context_terms) / len(reference_terms))


def score_context_precision(query: str, reference: str, contexts: list[str]) -> float:
    if not contexts:
        return 0.0
    useful_terms = _tokens(f"{query} {reference}")
    if not useful_terms:
        return 0.0

    context_scores = []
    for context in contexts:
        context_terms = _tokens(context)
        if not context_terms:
            context_scores.append(0.0)
            continue
        context_scores.append(len(context_terms & useful_terms) / len(context_terms))
    return _round_score(sum(context_scores) / len(context_scores))


async def build_evaluation_dataset(
    items: list[dict],
    rag_client: RAGClient,
    collection: str | None,
) -> list[dict]:
    """Run each golden item through the RAG pipeline and build evaluation rows."""
    dataset = []
    for item in items:
        query = item["query"]
        search_results = await rag_client.search(query, collection=collection, limit=5)
        chat_response = await rag_client.ask(query, collection=collection)

        dataset.append(
            {
                "user_input": query,
                "retrieved_contexts": [r["text"] for r in search_results],
                "response": chat_response["answer"],
                "reference": item["expected_answer"],
                "expected_sources": item.get("expected_sources", []),
            }
        )
    return dataset


def parse_judge_scores(raw: str) -> JudgeScores:
    try:
        payload = json.loads(raw)
    except json.JSONDecodeError as exc:
        raise EvaluationError("judge returned invalid JSON") from exc

    scores: dict[str, float] = {}
    reasons: dict[str, str] = {}
    for metric in ("faithfulness", "answer_relevancy"):
        if metric not in payload:
            raise EvaluationError(f"judge response missing {metric}")
        metric_payload = payload[metric]
        if not isinstance(metric_payload, dict):
            raise EvaluationError(f"judge response {metric} must be an object")
        if "score" not in metric_payload:
            raise EvaluationError(f"judge response missing {metric}.score")
        try:
            score = float(metric_payload["score"])
        except (TypeError, ValueError) as exc:
            raise EvaluationError(f"judge response {metric}.score must be numeric") from exc
        reason = metric_payload.get("reason", "")
        scores[metric] = _round_score(score)
        reasons[metric] = reason if isinstance(reason, str) else ""

    return JudgeScores(
        faithfulness=scores["faithfulness"],
        answer_relevancy=scores["answer_relevancy"],
        reasons=reasons,
    )


def _judge_prompt(row: dict) -> str:
    contexts = "\n\n".join(
        f"[Context {index + 1}]\n{text}"
        for index, text in enumerate(row["retrieved_contexts"])
    )
    return f"""Score this RAG answer. Return only valid JSON.

JSON schema:
{{
  "faithfulness": {{"score": 0.0, "reason": "short reason"}},
  "answer_relevancy": {{"score": 0.0, "reason": "short reason"}}
}}

Scoring rules:
- faithfulness: 1.0 means the answer is fully supported by the contexts; 0.0 means unsupported or contradicted.
- answer_relevancy: 1.0 means the answer directly addresses the question and reference; 0.0 means irrelevant.

Question:
{row["user_input"]}

Reference answer:
{row["reference"]}

Retrieved contexts:
{contexts or "(no contexts)"}

Generated answer:
{row["response"]}
"""


async def judge_generation_scores(
    row: dict,
    provider: str,
    base_url: str,
    model: str,
    api_key: str,
) -> JudgeScores:
    llm = get_llm_provider(
        provider=provider,
        base_url=base_url,
        api_key=api_key,
        model=model,
    )
    response = await llm.chat(
        [
            {
                "role": "system",
                "content": "You are a strict RAG evaluation judge. Return only valid JSON.",
            },
            {"role": "user", "content": _judge_prompt(row)},
        ]
    )
    raw = response.get("message", {}).get("content", "")
    return parse_judge_scores(raw)


def _aggregate(scores: list[dict]) -> dict:
    aggregate = {}
    for name in METRIC_NAMES:
        values = [score.get(name) for score in scores if score.get(name) is not None]
        aggregate[name] = round(sum(values) / len(values), 4) if values else None
    return aggregate


async def run_evaluation(
    items: list[dict],
    rag_client: RAGClient,
    collection: str | None,
    llm_provider: str,
    llm_base_url: str,
    llm_model: str,
    llm_api_key: str,
    judge: JudgeFn | None = None,
) -> tuple[dict, list[dict]]:
    """Run a full first-party RAG evaluation."""
    raw_dataset = await build_evaluation_dataset(items, rag_client, collection)
    if not raw_dataset:
        return {name: None for name in METRIC_NAMES}, []

    if judge is None:
        async def judge(row: dict) -> JudgeScores:
            return await judge_generation_scores(
                row=row,
                provider=llm_provider,
                base_url=llm_base_url,
                model=llm_model,
                api_key=llm_api_key,
            )

    per_query = []
    all_scores = []
    for index, row in enumerate(raw_dataset):
        try:
            judge_scores = await judge(row)
        except Exception as exc:
            raise EvaluationError(f"judge failed for row {index}: {exc}") from exc

        scores = {
            "faithfulness": judge_scores.faithfulness,
            "answer_relevancy": judge_scores.answer_relevancy,
            "context_precision": score_context_precision(
                query=row["user_input"],
                reference=row["reference"],
                contexts=row["retrieved_contexts"],
            ),
            "context_recall": score_context_recall(
                reference=row["reference"],
                contexts=row["retrieved_contexts"],
            ),
        }
        all_scores.append(scores)
        per_query.append(
            {
                "query": row["user_input"],
                "answer": row["response"],
                "contexts": row["retrieved_contexts"],
                "scores": scores,
                "score_reasons": judge_scores.reasons,
            }
        )

    return _aggregate(all_scores), per_query
```

- [ ] **Step 2: Run evaluator tests**

Run:

```bash
PYTHONPATH=services/eval:services pytest services/eval/tests/test_evaluator.py -v
```

Expected: PASS.

- [ ] **Step 3: Search for stale RAGAS imports in eval app/tests**

Run:

```bash
rg -n "ragas|RAGAS|build_ragas_dataset|_create_llm" services/eval/app services/eval/tests
```

Expected: only naming/copy locations assigned to Task 3 or Task 6, no import from `ragas`.

- [ ] **Step 4: Commit implementation**

```bash
git add services/eval/app/evaluator.py services/eval/tests/test_evaluator.py
git commit -m "feat: replace ragas evaluator with first-party metrics"
```

## Task 3: Clean Eval Naming Without Breaking Metric Keys

**Files:**
- Modify: `services/eval/app/config.py`
- Modify: `services/eval/app/main.py`
- Modify: `services/eval/app/metrics.py`
- Test: `services/eval/tests/test_main.py`

- [ ] **Step 1: Update config comment**

In `services/eval/app/config.py`, replace:

```python
    # LLM config for RAGAS judge calls
```

with:

```python
    # LLM config for first-party evaluation judge calls
```

- [ ] **Step 2: Rename metric variable in metrics.py**

In `services/eval/app/metrics.py`, replace:

```python
eval_ragas_score = Gauge(
    "eval_ragas_score",
    "Latest RAGAS metric score",
    ["metric"],
)
```

with:

```python
eval_quality_score = Gauge(
    "eval_ragas_score",
    "Latest RAG evaluation metric score",
    ["metric"],
)
```

- [ ] **Step 3: Update main.py imports and constants**

In `services/eval/app/main.py`, replace:

```python
from app.metrics import eval_queries_total, eval_ragas_score, eval_run_duration_seconds
```

with:

```python
from app.metrics import eval_quality_score, eval_queries_total, eval_run_duration_seconds
```

Replace:

```python
    """Background task that runs the RAGAS evaluation."""
```

with:

```python
    """Background task that runs the RAG quality evaluation."""
```

Replace:

```python
                eval_ragas_score.labels(metric=metric_name).set(score)
```

with:

```python
                eval_quality_score.labels(metric=metric_name).set(score)
```

Replace:

```python
_RAGAS_METRICS = (
```

with:

```python
_EVAL_METRICS = (
```

Replace:

```python
    for metric in _RAGAS_METRICS:
```

with:

```python
    for metric in _EVAL_METRICS:
```

- [ ] **Step 4: Run main tests**

Run:

```bash
PYTHONPATH=services/eval:services pytest services/eval/tests/test_main.py -v
```

Expected: PASS.

- [ ] **Step 5: Search eval service for stale RAGAS names**

Run:

```bash
rg -n "RAGAS|ragas|eval_ragas|_RAGAS|build_ragas" services/eval
```

Expected: only the Prometheus series name string `eval_ragas_score` may remain for compatibility.

- [ ] **Step 6: Commit naming cleanup**

```bash
git add services/eval/app/config.py services/eval/app/main.py services/eval/app/metrics.py
git commit -m "refactor: remove ragas naming from eval service"
```

## Task 4: Update Python Dependencies

**Files:**
- Modify: `services/eval/requirements.txt`
- Modify: `services/ingestion/requirements.txt`
- Modify: `services/debug/requirements.txt`
- Modify: `services/chat/requirements.txt`

- [ ] **Step 1: Update eval requirements**

In `services/eval/requirements.txt`:

Remove:

```text
ragas==0.2.15
datasets>=3.0.0
```

Replace:

```text
pytest==8.4.2
pytest-asyncio==0.26.0
pytest-cov==5.0.0
```

with:

```text
pytest==9.0.3
pytest-asyncio==1.3.0
pytest-cov==7.1.0
```

- [ ] **Step 2: Update ingestion requirements**

In `services/ingestion/requirements.txt`, replace:

```text
python-multipart==0.0.26
langchain-text-splitters==0.3.11
pytest==8.4.2
pytest-asyncio==0.26.0
```

with:

```text
python-multipart==0.0.27
langchain-text-splitters==1.1.2
pytest==9.0.3
pytest-asyncio==1.3.0
```

- [ ] **Step 3: Update debug requirements**

In `services/debug/requirements.txt`, replace:

```text
langchain-text-splitters==0.3.11
pytest==8.4.2
pytest-asyncio==0.26.0
```

with:

```text
langchain-text-splitters==1.1.2
pytest==9.0.3
pytest-asyncio==1.3.0
```

- [ ] **Step 4: Update chat requirements**

In `services/chat/requirements.txt`, replace:

```text
pytest==8.4.2
pytest-asyncio==0.26.0
```

with:

```text
pytest==9.0.3
pytest-asyncio==1.3.0
```

- [ ] **Step 5: Verify upgraded package resolution in a Python 3.11 environment**

Run locally if `python3.11` is available:

```bash
tmpdir=$(mktemp -d /tmp/eval-deps.XXXXXX)
python3.11 -m venv "$tmpdir/venv"
"$tmpdir/venv/bin/pip" install --upgrade pip setuptools
"$tmpdir/venv/bin/pip" install services/shared/
"$tmpdir/venv/bin/pip" install -r services/eval/requirements.txt
"$tmpdir/venv/bin/python" -c "import app.evaluator; import pytest; print(pytest.__version__)"
```

Expected: command exits 0 and prints `9.0.3`.

If `python3.11` is unavailable locally, run the same check with the repo's available Python and record the version limitation in the final handoff.

- [ ] **Step 6: Commit dependency changes**

```bash
git add services/eval/requirements.txt services/ingestion/requirements.txt services/debug/requirements.txt services/chat/requirements.txt
git commit -m "chore: update python dependencies for audit"
```

## Task 5: Remove Audit Ignores And Add Local pip-audit

**Files:**
- Modify: `Makefile`
- Modify: `.github/workflows/ci.yml`

- [ ] **Step 1: Add local pip-audit loop to Makefile**

In the `preflight-security` target in `Makefile`, after the CORS guardrail block, add:

```make
	@echo "\n=== Security: pip-audit ==="
	@for service in ingestion chat debug eval; do \
		echo "  Auditing Python service: $$service"; \
		tmpdir=$$(mktemp -d /tmp/preflight-pip-audit-$$service.XXXXXX); \
		python3 -m venv "$$tmpdir/venv"; \
		"$$tmpdir/venv/bin/pip" install --upgrade pip setuptools >/dev/null; \
		"$$tmpdir/venv/bin/pip" install services/shared/ >/dev/null; \
		"$$tmpdir/venv/bin/pip" install -r "services/$$service/requirements.txt" >/dev/null; \
		"$$tmpdir/venv/bin/pip" install pip-audit >/dev/null; \
		PIPAPI_PYTHON_LOCATION="$$tmpdir/venv/bin/python" "$$tmpdir/venv/bin/pip-audit"; \
		rm -rf "$$tmpdir"; \
	done
```

If local default `python3` is not 3.11-compatible with service requirements, change `python3 -m venv` to use `python3.11 -m venv` and add a preflight check:

```make
	@command -v python3.11 >/dev/null 2>&1 || { echo "python3.11 is required for pip-audit preflight"; exit 1; }
```

- [ ] **Step 2: Remove CI ignore comments**

In `.github/workflows/ci.yml`, delete the comment block beginning with:

```yaml
      # 2026-04-16: CVE-2025-71176 (pytest)
```

and ending with:

```yaml
      # CVE-2026-3219 (pip) — pip itself, no fix version available yet
```

- [ ] **Step 3: Replace CI pip-audit command**

In `.github/workflows/ci.yml`, replace the multi-line ignored audit command:

```yaml
          pip-audit \
            --ignore-vuln CVE-2025-71176 \
            --ignore-vuln GHSA-fv5p-p927-qmxr \
            --ignore-vuln CVE-2025-45691 \
            --ignore-vuln CVE-2025-69872 \
            --ignore-vuln CVE-2025-6984 \
            --ignore-vuln CVE-2025-65106 \
            --ignore-vuln CVE-2025-68664 \
            --ignore-vuln CVE-2026-26013 \
            --ignore-vuln CVE-2026-40087 \
            --ignore-vuln CVE-2026-34070 \
            --ignore-vuln GHSA-r7w7-9xr2-qq2r \
            --ignore-vuln CVE-2025-6985 \
            --ignore-vuln GHSA-rr7j-v2q5-chgv \
            --ignore-vuln CVE-2025-8869 \
            --ignore-vuln CVE-2026-1703 \
            --ignore-vuln ECHO-ffe1-1d3c-d9bc \
            --ignore-vuln ECHO-7db2-03aa-5591 \
            --ignore-vuln CVE-2026-6587 \
            --ignore-vuln CVE-2026-3219 \
            --ignore-vuln CVE-2026-44843
```

with:

```yaml
          pip-audit
```

- [ ] **Step 4: Search for stale ignored advisory IDs**

Run:

```bash
rg -n "ignore-vuln|CVE-2025-71176|GHSA-fv5p-p927-qmxr|CVE-2025-45691|CVE-2025-69872|CVE-2026-6587|CVE-2026-44843" .github Makefile services
```

Expected: no output.

- [ ] **Step 5: Run security preflight**

Run:

```bash
make preflight-security
```

Expected: Bandit passes, CORS guardrail passes, and pip-audit reports no known vulnerabilities for ingestion, chat, debug, and eval. If local Python version blocks the audit, do not weaken CI; report the local toolchain blocker.

- [ ] **Step 6: Commit security gate changes**

```bash
git add Makefile .github/workflows/ci.yml
git commit -m "ci: fail python audit without stale ignores"
```

## Task 6: Update Frontend Copy That Mentions RAGAS

**Files:**
- Modify: `frontend/src/components/eval/DashboardTab.tsx`
- Modify: `frontend/src/app/ai/page.tsx`
- Modify: `frontend/src/app/ai/eval/page.tsx`
- Modify: `frontend/src/app/cicd/page.tsx`

- [ ] **Step 1: Update dashboard copy**

In `frontend/src/components/eval/DashboardTab.tsx`, replace:

```tsx
          Track RAGAS scores over time, compare runs, and connect changes to
          measured quality impact.
```

with:

```tsx
          Track RAG quality scores over time, compare runs, and connect changes
          to measured quality impact.
```

- [ ] **Step 2: Update AI page eval description**

In `frontend/src/app/ai/page.tsx`, replace:

```tsx
            Create golden datasets with expected answers, run RAGAS evaluations
            against the live pipeline, and view scorecards with per-query
            breakdowns — faithfulness, answer relevancy, context precision, and
            context recall.
```

with:

```tsx
            Create golden datasets with expected answers, run first-party RAG
            quality evaluations against the live pipeline, and view scorecards
            with per-query breakdowns — faithfulness, answer relevancy, context
            precision, and context recall.
```

Replace:

```tsx
            <li>RAGAS evaluation framework for RAG quality measurement</li>
```

with:

```tsx
            <li>First-party RAG quality evaluator with LLM-judged metrics</li>
```

- [ ] **Step 3: Update eval app page subtitle**

In `frontend/src/app/ai/eval/page.tsx`, replace:

```tsx
          Measure RAG pipeline quality with golden datasets and RAGAS metrics.
```

with:

```tsx
          Measure RAG pipeline quality with golden datasets and first-party metrics.
```

- [ ] **Step 4: Update CI/CD optimization story**

In `frontend/src/app/cicd/page.tsx`, replace:

```tsx
            bottlenecks. The eval service depends on RAGAS, which pulls in 200+
            transitive packages including LangChain. That single addition pushed
            the pipeline from a manageable ~10 minutes to 30+ minutes per run,
            with most of the time spent on redundant work. Here&apos;s how each
            bottleneck was diagnosed and fixed.
```

with:

```tsx
            bottlenecks. The original eval service depended on RAGAS, which
            pulled in 200+ transitive packages including LangChain. That single
            addition pushed the pipeline from a manageable ~10 minutes to 30+
            minutes per run, with most of the time spent on redundant work.
            Here&apos;s how each bottleneck was diagnosed and fixed.
```

Replace:

```tsx
              cache to reuse. The eval service&apos;s dependency tree (RAGAS →
              LangChain → dozens of ML packages) made cold installs
              exceptionally slow.
```

with:

```tsx
              cache to reuse. The eval service&apos;s former dependency tree
              (RAGAS → LangChain → dozens of ML packages) made cold installs
              exceptionally slow.
```

- [ ] **Step 5: Verify no stale visible RAGAS references remain**

Run:

```bash
rg -n "RAGAS|ragas" frontend services/eval .github Makefile
```

Expected: no output except the compatibility Prometheus metric string if it remains in `services/eval/app/metrics.py`.

- [ ] **Step 6: Run frontend lint if frontend text changed**

Run:

```bash
make preflight-frontend
```

Expected: PASS. If the local frontend environment lacks dependencies, report the blocker and leave CI to verify.

- [ ] **Step 7: Commit frontend copy changes**

```bash
git add frontend/src/components/eval/DashboardTab.tsx frontend/src/app/ai/page.tsx frontend/src/app/ai/eval/page.tsx frontend/src/app/cicd/page.tsx
git commit -m "docs: update eval copy for first-party metrics"
```

## Task 7: Full Verification And Final Commit Hygiene

**Files:**
- Verify all changed files.

- [ ] **Step 1: Run Python preflight**

Run:

```bash
make preflight-python
```

Expected: Ruff check passes, Ruff format check passes, ingestion/chat/debug tests pass. Add eval tests to the Makefile in a follow-up if desired; for this task run eval tests directly in Step 2.

- [ ] **Step 2: Run eval tests directly**

Run:

```bash
PYTHONPATH=services/eval:services pytest services/eval/tests/ -v
```

Expected: PASS.

- [ ] **Step 3: Run security preflight**

Run:

```bash
make preflight-security
```

Expected: PASS and no Python audit ignores are used.

- [ ] **Step 4: Run frontend preflight**

Run:

```bash
make preflight-frontend
```

Expected: PASS.

- [ ] **Step 5: Confirm RAGAS dependency and stale ignores are gone**

Run:

```bash
rg -n "ragas==|from ragas|import ragas|ignore-vuln|CVE-2025-71176|CVE-2025-69872|CVE-2026-6587" services .github Makefile
```

Expected: no output.

- [ ] **Step 6: Review final diff**

Run:

```bash
git status --short
git log --oneline -6
git diff --stat HEAD~5..HEAD
```

Expected: only intended files are changed across the task commits.

- [ ] **Step 7: Commit any missed verification-only edits**

If verification required a small fix, commit it:

```bash
git add <fixed-files>
git commit -m "fix: complete first-party eval replacement"
```

Do not create an empty commit if there are no remaining changes.

## Self-Review

- Spec coverage: RAGAS removal is covered by Tasks 1-4; API/result-shape preservation by Tasks 1-3; CI ignore removal and local preflight audit by Task 5; visible RAGAS wording by Task 6; verification by Task 7.
- Placeholder scan: no `TBD`, `TODO`, or omitted implementation steps are intentionally left in this plan.
- Type consistency: `JudgeScores`, `EvaluationError`, `build_evaluation_dataset`, `score_context_precision`, `score_context_recall`, `parse_judge_scores`, and `run_evaluation(..., judge=...)` are introduced in Task 1 tests and implemented in Task 2 with matching names.
