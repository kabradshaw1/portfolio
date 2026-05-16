# Eval Experiment Provenance Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Persist enough eval provenance to show both requested rerank configuration and effective per-query retrieval behavior.

**Architecture:** Keep the change centered in `services/eval`. The eval RAG client will always send an explicit rerank boolean to chat, and the evaluator will copy chat's retrieval metadata into each per-query eval result without changing aggregate metric calculation.

**Tech Stack:** Python, FastAPI service code, `httpx.MockTransport`, `pytest`, `pytest.mark.asyncio`, existing `services/eval` first-party evaluator.

---

## Execution Setup

This is production behavior and persisted API/result-shape work. Execute it in a feature worktree targeting `qa`, not directly on `qa`.

Recommended worktree:

```bash
git fetch origin qa
git worktree add .codex/worktrees/feat-eval-experiment-provenance -b feat/eval-experiment-provenance origin/qa
cd .codex/worktrees/feat-eval-experiment-provenance
pwd
git branch --show-current
git rev-parse --show-toplevel
```

Expected:

- `pwd` is inside `.codex/worktrees/feat-eval-experiment-provenance`
- branch is `feat/eval-experiment-provenance`
- repo top level is the worktree path

## File Structure

- Modify `services/eval/app/rag_client.py`: always include `rerank` in `/search` and `/chat` JSON bodies.
- Modify `services/eval/app/evaluator.py`: carry chat response retrieval metadata into dataset rows and final per-query results.
- Modify `services/eval/tests/test_rag_client.py`: prove baseline requests send `rerank: false` and rerank requests send `rerank: true`.
- Modify `services/eval/tests/test_evaluator.py`: prove retrieval metadata is persisted when present and absent when chat omits it.

No database migration is required because eval results are already stored as JSON.

---

### Task 1: Make Eval Chat Requests Explicit About Rerank

**Files:**
- Modify: `services/eval/app/rag_client.py`
- Test: `services/eval/tests/test_rag_client.py`

- [ ] **Step 1: Write failing tests for explicit baseline rerank payloads**

Add these tests to `services/eval/tests/test_rag_client.py` near the existing rerank tests:

```python
@pytest.mark.asyncio
async def test_search_sends_rerank_false_for_baseline(mock_search_response):
    async def mock_handler(request: httpx.Request) -> httpx.Response:
        body = json.loads(request.content)
        assert body["rerank"] is False
        return httpx.Response(200, json=mock_search_response)

    transport = httpx.MockTransport(mock_handler)
    client = RAGClient(base_url="http://chat:8000", transport=transport)

    await client.search("test", collection=None, limit=5, rerank=False)


@pytest.mark.asyncio
async def test_ask_sends_rerank_false_for_baseline(mock_chat_response):
    async def mock_handler(request: httpx.Request) -> httpx.Response:
        body = json.loads(request.content)
        assert body["rerank"] is False
        return httpx.Response(200, json=mock_chat_response)

    transport = httpx.MockTransport(mock_handler)
    client = RAGClient(base_url="http://chat:8000", transport=transport)

    await client.ask("test", collection=None, rerank=False)
```

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
PYTHONPATH=services/shared:services/eval pytest services/eval/tests/test_rag_client.py::test_search_sends_rerank_false_for_baseline services/eval/tests/test_rag_client.py::test_ask_sends_rerank_false_for_baseline -v
```

Expected: both tests fail with `KeyError: 'rerank'`.

- [ ] **Step 3: Implement explicit rerank fields**

Update `services/eval/app/rag_client.py` so `search` and `ask` build request bodies like this:

```python
    async def search(
        self,
        query: str,
        collection: str | None,
        limit: int,
        rerank: bool = False,
    ) -> list[dict]:
        """Call POST /search for retrieval-only results."""
        body: dict = {"query": query, "limit": limit, "rerank": rerank}
        if collection:
            body["collection"] = collection

        resp = await self._client.post("/search", json=body)
        resp.raise_for_status()
        return resp.json()["results"]

    async def ask(
        self, question: str, collection: str | None, rerank: bool = False
    ) -> dict:
        """Call POST /chat with Accept: application/json for a full RAG response."""
        body: dict = {"question": question, "rerank": rerank}
        if collection:
            body["collection"] = collection

        resp = await self._client.post(
            "/chat", json=body, headers={"Accept": "application/json"}
        )
        resp.raise_for_status()
        return resp.json()
```

- [ ] **Step 4: Run focused RAG client tests**

Run:

```bash
PYTHONPATH=services/shared:services/eval pytest services/eval/tests/test_rag_client.py -v
```

Expected: all tests in `test_rag_client.py` pass.

- [ ] **Step 5: Commit Task 1**

```bash
git add services/eval/app/rag_client.py services/eval/tests/test_rag_client.py
git commit -m "feat: send explicit eval rerank requests"
```

---

### Task 2: Persist Per-Query Retrieval Provenance

**Files:**
- Modify: `services/eval/app/evaluator.py`
- Test: `services/eval/tests/test_evaluator.py`

- [ ] **Step 1: Write failing dataset test for retrieval metadata**

Add this fixture near `mock_chat_answer` in `services/eval/tests/test_evaluator.py`:

```python
@pytest.fixture
def mock_chat_answer_with_retrieval(mock_chat_answer):
    return {
        **mock_chat_answer,
        "retrieval": {
            "retrieval_mode": "hybrid",
            "retrieval_fallback": False,
            "rerank_requested": True,
            "rerank_enabled": True,
            "rerank_applied": True,
            "rerank_fallback": False,
            "rerank_model": "cross-encoder/ms-marco-MiniLM-L6-v2",
            "rerank_candidate_count": 20,
            "rerank_returned_count": 5,
        },
    }
```

Add this test near `test_build_evaluation_dataset_passes_rerank`:

```python
@pytest.mark.asyncio
async def test_build_evaluation_dataset_preserves_retrieval_metadata(
    golden_items, mock_search_results, mock_chat_answer_with_retrieval
):
    rag_client = AsyncMock()
    rag_client.search.return_value = mock_search_results
    rag_client.ask.return_value = mock_chat_answer_with_retrieval

    dataset = await build_evaluation_dataset(
        items=golden_items,
        rag_client=rag_client,
        collection="documents",
        rerank=True,
    )

    assert dataset[0]["retrieval"] == mock_chat_answer_with_retrieval["retrieval"]
```

- [ ] **Step 2: Run test to verify it fails**

Run:

```bash
PYTHONPATH=services/shared:services/eval pytest services/eval/tests/test_evaluator.py::test_build_evaluation_dataset_preserves_retrieval_metadata -v
```

Expected: FAIL with `KeyError: 'retrieval'`.

- [ ] **Step 3: Preserve retrieval metadata in dataset rows**

Update `build_evaluation_dataset` in `services/eval/app/evaluator.py` so each row is built through a local variable and copies `chat_response["retrieval"]` only when present:

```python
        row = {
            "user_input": query,
            "retrieved_contexts": [r["text"] for r in search_results],
            "response": chat_response["answer"],
            "reference": item["expected_answer"],
            "expected_sources": item.get("expected_sources", []),
        }
        if "retrieval" in chat_response:
            row["retrieval"] = chat_response["retrieval"]
        dataset.append(row)
```

Replace the current inline `dataset.append({...})` block with this code.

- [ ] **Step 4: Run focused dataset tests**

Run:

```bash
PYTHONPATH=services/shared:services/eval pytest services/eval/tests/test_evaluator.py::test_build_evaluation_dataset services/eval/tests/test_evaluator.py::test_build_evaluation_dataset_preserves_retrieval_metadata -v
```

Expected: both tests pass. The existing dataset test proves missing retrieval metadata remains accepted.

- [ ] **Step 5: Write failing final-result test for retrieval metadata**

Add this test near `test_run_evaluation_preserves_result_shape`:

```python
@pytest.mark.asyncio
async def test_run_evaluation_persists_retrieval_metadata_in_results(
    golden_items,
    mock_search_results,
    mock_chat_answer_with_retrieval,
):
    rag_client = MagicMock(spec=RAGClient)
    rag_client.search = AsyncMock(return_value=mock_search_results)
    rag_client.ask = AsyncMock(return_value=mock_chat_answer_with_retrieval)
    judge = AsyncMock(
        return_value=JudgeScores(
            faithfulness=1.0,
            answer_relevancy=1.0,
            reasons={
                "faithfulness": "supported",
                "answer_relevancy": "direct",
            },
        )
    )

    aggregate, results = await run_evaluation(
        items=golden_items,
        rag_client=rag_client,
        collection="documents",
        llm_provider="ollama",
        llm_base_url="http://localhost:11434",
        llm_model="qwen2.5:14b",
        llm_api_key="",
        rerank=True,
        judge=judge,
    )

    assert aggregate["faithfulness"] == 1.0
    assert results[0]["retrieval"]["rerank_requested"] is True
    assert results[0]["retrieval"]["rerank_applied"] is True
    assert results[0]["retrieval"]["rerank_candidate_count"] == 20
```

- [ ] **Step 6: Run test to verify it fails**

Run:

```bash
PYTHONPATH=services/shared:services/eval pytest services/eval/tests/test_evaluator.py::test_run_evaluation_persists_retrieval_metadata_in_results -v
```

Expected: FAIL with `KeyError: 'retrieval'`.

- [ ] **Step 7: Persist retrieval metadata in per-query results**

Update the per-query append block in `run_evaluation` in `services/eval/app/evaluator.py` so it creates a result dict first:

```python
        result = {
            "query": row["user_input"],
            "answer": row["response"],
            "contexts": row["retrieved_contexts"],
            "scores": scores,
            "score_reasons": judge_scores.reasons,
        }
        if "retrieval" in row:
            result["retrieval"] = row["retrieval"]
        per_query.append(result)
```

Replace the current inline `per_query.append({...})` block with this code.

- [ ] **Step 8: Run evaluator tests**

Run:

```bash
PYTHONPATH=services/shared:services/eval pytest services/eval/tests/test_evaluator.py -v
```

Expected: all tests in `test_evaluator.py` pass.

- [ ] **Step 9: Commit Task 2**

```bash
git add services/eval/app/evaluator.py services/eval/tests/test_evaluator.py
git commit -m "feat: persist eval retrieval provenance"
```

---

### Task 3: Verify Python Service Preflight

**Files:**
- No source files changed in this task.

- [ ] **Step 1: Run focused eval test suite**

Run:

```bash
PYTHONPATH=services/shared:services/eval pytest services/eval/tests -v
```

Expected: all `services/eval` tests pass.

- [ ] **Step 2: Run required Python preflight**

Run:

```bash
make preflight-python
```

Expected: Python lint, formatting, and tests pass.

- [ ] **Step 3: Run required security preflight**

Run:

```bash
make preflight-security
```

Expected: security checks pass. If dependency audit tooling is unavailable locally, capture the exact failure and leave that specific check to CI.

- [ ] **Step 4: Inspect final diff**

Run:

```bash
git diff --stat origin/qa...HEAD
git diff origin/qa...HEAD -- services/eval/app/rag_client.py services/eval/app/evaluator.py services/eval/tests/test_rag_client.py services/eval/tests/test_evaluator.py
```

Expected:

- `rag_client.py` only changes request body construction to include explicit `rerank`.
- `evaluator.py` only adds optional retrieval metadata propagation.
- tests cover explicit request flags and result provenance.

- [ ] **Step 5: Push and open PR**

Run:

```bash
git status --short
git push -u origin feat/eval-experiment-provenance
```

Expected: working tree is clean before push, and the branch pushes successfully.

Open a PR to `qa` with this summary:

```markdown
## Summary
- send explicit rerank booleans from eval to chat
- persist per-query retrieval provenance in eval results
- cover baseline, rerank, and missing-metadata behavior with focused tests

## Verification
- PYTHONPATH=services/shared:services/eval pytest services/eval/tests -v
- make preflight-python
- make preflight-security
```

