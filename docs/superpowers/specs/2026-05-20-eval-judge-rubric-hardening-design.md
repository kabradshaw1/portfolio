# Eval Judge Rubric Hardening Design

## Summary

Issue 266 strengthens the eval service's LLM judge prompt and output validation
without changing the shared LLM provider interface. The change keeps local
Ollama compatibility, improves scoring consistency for prompt experiments, and
adds focused tests around the rubric and parser behavior.

## Goals

- Expand judge scoring rules with clear anchors for `0.0`, partial credit, and
  `1.0`.
- Define how the judge scores unsupported, contradicted, citation-free, and
  correct-but-ungrounded answers.
- Keep parser validation strict and add tests for malformed or incomplete judge
  output where coverage is missing.
- Avoid provider-interface changes for structured output in this issue.

## Non-Goals

- Add shared structured-output support to `services/shared/llm/`.
- Change the persisted eval result shape.
- Add new eval metrics beyond `faithfulness` and `answer_relevancy`.
- Replace the existing lexical `context_precision` and `context_recall`
  heuristics.

## Current State

`services/eval/app/evaluator.py` sends the judge a JSON-only prompt and parses
the response with `parse_judge_scores()`. The parser already rejects invalid
JSON, missing metrics, non-object metric payloads, missing scores, and
non-numeric scores. It clamps numeric scores to `[0.0, 1.0]` and stores
non-string reasons as empty strings.

The current prompt only gives thin definitions for `faithfulness` and
`answer_relevancy`, which leaves too much room for model-specific scoring drift
when comparing RAG prompt experiments.

## Design

### Rubric

Keep the current JSON output contract:

```json
{
  "faithfulness": {"score": 0.0, "reason": "short reason"},
  "answer_relevancy": {"score": 0.0, "reason": "short reason"}
}
```

Update `_judge_prompt()` with explicit scoring anchors.

`faithfulness` measures whether the generated answer is supported by retrieved
contexts:

- `1.0`: all material claims are supported by the contexts, with no material
  contradictions.
- Partial credit: the answer is mostly grounded but includes minor unsupported
  details, weak overstatements, or incomplete grounding.
- `0.0`: the answer is unsupported, contradicted by the contexts, or primarily
  relies on facts absent from the contexts.
- Correct-but-ungrounded answers receive low faithfulness even when they match
  the reference answer.
- Citation-free answers are not automatically wrong, but missing source
  attribution should reduce faithfulness when the answer makes source-specific
  claims that are not clearly grounded in the provided contexts.

`answer_relevancy` measures whether the generated answer addresses the question
and reference need:

- `1.0`: directly answers the question at the right specificity.
- Partial credit: partly answers the question, is too broad or too narrow,
  omits key requested detail, or mixes relevant and irrelevant content.
- `0.0`: off-topic, refuses without cause, or answers a different question.
- Unsupported or contradicted answers can still be relevant, but should not
  receive high faithfulness.

The prompt should continue to require raw JSON only: no markdown, prose,
comments, or top-level wrapper.

### Parser Validation

Keep the parser as the enforcement boundary. It should continue to:

- Reject malformed JSON.
- Reject missing `faithfulness` or `answer_relevancy`.
- Reject metric payloads that are not JSON objects.
- Reject missing `score` fields.
- Reject non-numeric scores.
- Clamp numeric scores to `[0.0, 1.0]`.
- Preserve string reasons and normalize non-string reasons to empty strings.

No provider-specific structured-output mode is added in this issue. The current
`LLMProvider.chat(messages, tools=None)` contract does not expose a normalized
schema or JSON-mode option, and adding one would expand the blast radius across
OpenAI-compatible, Ollama, and Anthropic providers. That should be handled as a
separate follow-up if eval or other services need provider-level schema
enforcement.

## Tests

Update `services/eval/tests/test_evaluator.py` with focused coverage:

- Existing parser tests still pass for valid JSON, malformed JSON, and missing
  metrics.
- Add parser tests for non-object metric payloads, missing scores, non-numeric
  scores, and non-string reasons.
- Add a prompt test that asserts `_judge_prompt()` includes the required rubric
  concepts from issue 266 without snapshotting the full prompt.
- Keep `judge_generation_scores()` tests mocked at the provider boundary.

## Verification

Before committing implementation changes, run:

```bash
make preflight-python
make preflight-security
```

If a narrower test run is useful during development, run the eval service tests
first, then finish with the required Python and security preflights.
