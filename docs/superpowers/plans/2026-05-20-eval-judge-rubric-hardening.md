# Eval Judge Rubric Hardening Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Strengthen the eval service LLM judge rubric and parser test coverage for issue 266 without changing the shared LLM provider interface.

**Architecture:** Keep the existing eval judge flow in `services/eval/app/evaluator.py`: build a JSON-only prompt, call the configured LLM through `LLMProvider.chat()`, then enforce the contract in `parse_judge_scores()`. Add focused tests in `services/eval/tests/test_evaluator.py` that prove the prompt encodes the new rubric and the parser rejects malformed or incomplete judge output.

**Tech Stack:** Python, FastAPI service code, pytest, unittest.mock, existing shared LLM provider abstraction.

---

### Task 1: Add Parser Validation Coverage

**Files:**
- Modify: `services/eval/tests/test_evaluator.py`

- [ ] **Step 1: Confirm working directory and branch**

Run:

```bash
pwd
git branch --show-current
git status --short
```

Expected: work is running from the task branch/worktree selected for implementation, not from an unrelated directory. If this is feature work started from `qa`, create or switch to a feature worktree before editing.

- [ ] **Step 2: Add parser coverage tests**

Add these tests after `test_parse_judge_scores_rejects_missing_metric()` in `services/eval/tests/test_evaluator.py`:

```python
def test_parse_judge_scores_rejects_metric_that_is_not_object():
    with pytest.raises(EvaluationError, match="faithfulness must be an object"):
        parse_judge_scores(
            json.dumps(
                {
                    "faithfulness": 0.5,
                    "answer_relevancy": {"score": 0.8, "reason": "direct"},
                }
            )
        )


def test_parse_judge_scores_rejects_missing_score():
    with pytest.raises(EvaluationError, match="missing faithfulness.score"):
        parse_judge_scores(
            json.dumps(
                {
                    "faithfulness": {"reason": "grounded"},
                    "answer_relevancy": {"score": 0.8, "reason": "direct"},
                }
            )
        )


def test_parse_judge_scores_rejects_non_numeric_score():
    with pytest.raises(EvaluationError, match="answer_relevancy.score must be numeric"):
        parse_judge_scores(
            json.dumps(
                {
                    "faithfulness": {"score": 0.7, "reason": "grounded"},
                    "answer_relevancy": {"score": "high", "reason": "direct"},
                }
            )
        )


def test_parse_judge_scores_normalizes_non_string_reason():
    scores = parse_judge_scores(
        json.dumps(
            {
                "faithfulness": {"score": 0.7, "reason": ["grounded"]},
                "answer_relevancy": {"score": 0.8, "reason": {"why": "direct"}},
            }
        )
    )

    assert scores == JudgeScores(
        faithfulness=0.7,
        answer_relevancy=0.8,
        reasons={"faithfulness": "", "answer_relevancy": ""},
    )
```

- [ ] **Step 3: Run parser tests and verify current behavior**

Run:

```bash
pytest services/eval/tests/test_evaluator.py \
  -k "parse_judge_scores" \
  -q
```

Expected: PASS if existing parser behavior already covers these cases. If a message assertion fails because the existing message is more specific than expected, update the test regex to match the existing `EvaluationError` message without weakening the parser.

- [ ] **Step 4: Commit parser test coverage**

Run:

```bash
git add services/eval/tests/test_evaluator.py
git commit -m "test(eval): cover judge score parser validation"
```

Expected: commit succeeds after repository hooks run.

### Task 2: Expand Judge Rubric Prompt

**Files:**
- Modify: `services/eval/app/evaluator.py`
- Modify: `services/eval/tests/test_evaluator.py`

- [ ] **Step 1: Write failing prompt coverage**

Update the import list in `services/eval/tests/test_evaluator.py` to include `_judge_prompt`:

```python
from app.evaluator import (
    EvalRunContext,
    EvaluationError,
    JudgeScores,
    _judge_prompt,
    build_evaluation_dataset,
    judge_generation_scores,
    parse_judge_scores,
    run_evaluation,
    score_context_precision,
    score_context_recall,
)
```

Add this test after `_judge_row()`:

```python
def test_judge_prompt_includes_strict_rubric_anchors():
    prompt = _judge_prompt(_judge_row())

    required_phrases = [
        "Return raw JSON only",
        "no markdown",
        "all material claims are supported",
        "minor unsupported details",
        "contradicted by the contexts",
        "Correct-but-ungrounded",
        "Citation-free answers are not automatically wrong",
        "directly answers the question",
        "too broad or too narrow",
        "answers a different question",
        "Unsupported or contradicted answers can still be relevant",
    ]

    for phrase in required_phrases:
        assert phrase in prompt
```

- [ ] **Step 2: Run the prompt test and verify it fails**

Run:

```bash
pytest services/eval/tests/test_evaluator.py::test_judge_prompt_includes_strict_rubric_anchors -q
```

Expected: FAIL because `_judge_prompt()` does not yet contain the expanded rubric phrases.

- [ ] **Step 3: Update `_judge_prompt()`**

In `services/eval/app/evaluator.py`, replace the current `return f"""Score this RAG answer...` prompt body in `_judge_prompt()` with:

```python
    return f"""Score this RAG answer. Return raw JSON only: no markdown, prose, comments, or top-level wrapper.

JSON schema:
{{
  "faithfulness": {{"score": 0.0, "reason": "short reason"}},
  "answer_relevancy": {{"score": 0.0, "reason": "short reason"}}
}}

Scoring rules:
- Score each metric from 0.0 to 1.0. Use partial credit when the answer is
  mixed rather than forcing only 0.0 or 1.0.
- Keep reasons short and specific. Mention the strongest scoring factor.

Faithfulness measures whether the generated answer is supported by the
retrieved contexts:
- 1.0: all material claims are supported by the contexts, with no material
  contradictions.
- Partial credit: the answer is mostly grounded but includes minor unsupported
  details, weak overstatements, or incomplete grounding.
- 0.0: the answer is unsupported, contradicted by the contexts, or primarily
  relies on facts absent from the contexts.
- Correct-but-ungrounded answers receive low faithfulness even when they match
  the reference answer.
- Citation-free answers are not automatically wrong, but missing source
  attribution should reduce faithfulness when the answer makes source-specific
  claims that are not clearly grounded in the provided contexts.

Answer relevancy measures whether the generated answer addresses the question
and reference need:
- 1.0: directly answers the question at the right specificity.
- Partial credit: partly answers the question, is too broad or too narrow,
  omits key requested detail, or mixes relevant and irrelevant content.
- 0.0: off-topic, refuses without cause, or answers a different question.
- Unsupported or contradicted answers can still be relevant, but should not
  receive high faithfulness.

Question:
{row["user_input"]}

Reference answer:
{row["reference"]}

Retrieved contexts:
{contexts or "(no contexts)"}

Generated answer:
{row["response"]}
"""
```

- [ ] **Step 4: Run the prompt test and parser tests**

Run:

```bash
pytest services/eval/tests/test_evaluator.py \
  -k "judge_prompt or parse_judge_scores" \
  -q
```

Expected: PASS.

- [ ] **Step 5: Commit prompt rubric change**

Run:

```bash
git add services/eval/app/evaluator.py services/eval/tests/test_evaluator.py
git commit -m "fix(eval): strengthen judge rubric prompt"
```

Expected: commit succeeds after repository hooks run.

### Task 3: Run Required Verification

**Files:**
- No file edits expected.

- [ ] **Step 1: Run eval evaluator tests**

Run:

```bash
pytest services/eval/tests/test_evaluator.py -q
```

Expected: PASS.

- [ ] **Step 2: Run Python preflight**

Run:

```bash
make preflight-python
```

Expected: PASS. If formatting hooks change files, inspect the diff, commit the formatter-only changes with the relevant task commit, and rerun this command.

- [ ] **Step 3: Run security preflight**

Run:

```bash
make preflight-security
```

Expected: PASS.

- [ ] **Step 4: Inspect final diff and status**

Run:

```bash
git status --short
git log --oneline -3
```

Expected: working tree is clean except for unrelated user changes, and the latest commits include the parser test and prompt rubric changes.

- [ ] **Step 5: Push and create PR to `qa`**

Run:

```bash
git push -u origin "$(git branch --show-current)"
gh pr create --base qa --fill
```

Expected: a PR targeting `qa` is created. Do not watch CI unless Kyle explicitly asks.
