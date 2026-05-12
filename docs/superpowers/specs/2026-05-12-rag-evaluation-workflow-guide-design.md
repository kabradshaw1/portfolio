# RAG Evaluation Workflow Guide Design

- **Date:** 2026-05-12
- **Status:** Draft for review
- **Scope:** Production usage workflow for the existing RAG eval service and `/ai/eval` UI

## Problem

The eval service and `/ai/eval` UI can create golden datasets, run evaluations,
show per-query scorecards, compare runs, and track score history. The missing
piece is an operator workflow. Opening `/ai/eval` does not clearly answer where
to start, what a baseline means, when to run a candidate, or how to interpret
the resulting scores.

The existing ADRs explain the architecture and metric choices, but they are not
step-by-step usage instructions. The next slice should make the current system
usable before adding more backend capability.

## Goals

- Provide a production-focused workflow for using the existing eval service.
- Make the workflow visible from `/ai/eval`, so the page teaches the user what
  to do next.
- Keep the first workflow practical: baseline run, candidate run, comparison,
  per-query inspection, and decision.
- Use the current API and UI behavior; do not require issue #240 or #241 first.
- Build a foundation for accumulating a history of measured RAG performance.

## Non-Goals

- Do not add experiment ledger records in this slice.
- Do not add the dashboard summary endpoint from issue #240 in this slice.
- Do not redesign the eval UI broadly.
- Do not make reranking the default workflow until a baseline-vs-candidate
  process is clear.
- Do not claim score changes are definitive without per-query review.

## Recommended Approach

Create a runbook as the source of truth, then expose a compact version in the
frontend:

1. Add `docs/runbooks/rag-evaluation-workflow.md`.
2. Add a discoverable `Guide` tab to `/ai/eval`, placed before `Datasets`.
3. Keep the guide action-oriented: what to click, what to enter, what to inspect,
   and what decision to make.
4. Link the guide steps to existing tabs where useful by switching the active
   tab in the current page state.

This is preferred over a docs-only runbook because the current pain happens in
the UI. It is preferred over UI-only instructions because the workflow should
also exist as durable engineering documentation.

## Workflow Content

The workflow should use production as its default context:

1. **Prepare**
   - Log in through the production frontend.
   - Open `/ai/eval`.
   - Confirm the eval service health gate passes.
   - Use the `documents` collection unless deliberately evaluating another
     collection.

2. **Create a Golden Dataset**
   - Start with 8-15 high-signal questions.
   - Each item includes a realistic user query, expected answer, and expected
     source names.
   - Include easy factual questions, multi-source questions, and edge cases that
     have failed manually.
   - Once a dataset becomes a baseline set, avoid editing its meaning. Create a
     new dataset version for materially different coverage.

3. **Run a Baseline**
   - Select the dataset and collection.
   - Leave baseline empty.
   - Add notes such as `Baseline before rerank comparison`.
   - Run the evaluation and wait for completion.

4. **Inspect Baseline Results**
   - Review aggregate scores first.
   - Expand low-scoring queries.
   - Classify failures as retrieval miss, weak answer, unsupported answer,
     dataset issue, or expected-source mismatch.
   - Treat bad dataset items as test-data fixes, not model failures.

5. **Run a Candidate**
   - Make one deliberate RAG change at a time, such as reranking, prompt changes,
     chunking changes, or retrieval tuning.
   - Run the same dataset and collection.
   - Select the completed baseline run when available.
   - Use notes that state the exact measured change.

6. **Compare**
   - Open the dashboard for the same dataset and collection.
   - Compare baseline vs candidate.
   - Read deltas for faithfulness, answer relevancy, context precision, and
     context recall.
   - Use aggregate deltas as a signal, then inspect per-query results before
     deciding.

7. **Decide**
   - Keep the change when aggregate scores improve and per-query review does not
     show unacceptable regressions.
   - Revert or adjust when gains are narrow, noisy, or created by worse answers
     on important questions.
   - Create a follow-up issue when the next improvement requires new backend or
     frontend capability.

8. **Repeat**
   - Accumulate measured runs over time.
   - Use notes and dashboard history as the lightweight change log until the
     experiment ledger from issue #241 is justified by real usage.

## Frontend Design

Add a `Guide` tab as the first tab in `/ai/eval`.

The tab should be an operator checklist, not a marketing page. It should use
compact sections with short explanations and direct actions:

- `Prepare`
- `Create dataset`
- `Run baseline`
- `Inspect results`
- `Run candidate`
- `Compare and decide`

Where useful, buttons should switch to the relevant existing tab:

- `Create dataset` -> `Datasets`
- `Run evaluation` -> `Evaluate`
- `Review results` -> `Results`
- `Compare runs` -> `Dashboard`

The page should keep the existing tab model and avoid introducing routing,
modals, or persistent onboarding state.

## Error Handling And Edge Cases

- If no datasets exist, the guide should point to `Datasets` first.
- If only one completed run exists, the guide should explain that comparison
  needs a second run against the same dataset and collection.
- If baseline selection fails, the copy should explain that baselines must be
  completed runs for the same dataset and collection.
- If a run fails, the workflow should direct the user to inspect the error in
  `Results` before starting another run.

## Testing

Documentation-only changes need no automated test.

Frontend changes should include:

- Existing frontend preflight: `make preflight-frontend`.
- Existing e2e preflight: `make preflight-e2e`.
- A mocked Playwright assertion that `/ai/eval` exposes the guide tab and can
  switch from the guide to the existing tabs.

## Implementation Notes

This should be implemented as a small frontend/documentation slice on a feature
worktree because it changes production frontend behavior. The PR should target
`qa`.

The runbook and UI guide should share wording closely, but the runbook can be
more detailed. The UI should stay concise and focused on immediate actions.
