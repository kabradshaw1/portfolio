# RAG Evaluation Workflow

Use this workflow in production when measuring whether a RAG change improved
answer quality. The goal is not to produce one perfect score. The goal is to
build a repeatable habit: stable dataset, baseline run, candidate run, score
comparison, per-query review, and a written decision.

## Prerequisites

- You can log in to the production frontend.
- The eval service health gate on `/ai/eval` passes.
- The chat and ingestion services are serving the collection you want to test.
- Use the `documents` collection unless you are deliberately evaluating another
  collection.

## 1. Create Or Select A Golden Dataset

Open `/ai/eval`, then open `Datasets`.

Create a dataset with 8-15 high-signal questions for the first pass. Each item
must include:

- `query`: a realistic user question.
- `expected_answer`: the answer the system should be able to produce from the
  indexed documents.
- `expected_sources`: source names that should support the answer.

Good first datasets include a mix of:

- Easy factual questions that should always pass.
- Multi-source questions that test retrieval coverage.
- Questions that previously failed during manual testing.
- Edge cases where a plausible answer could hallucinate unsupported details.

Once a dataset becomes a baseline set, avoid changing its meaning. If the test
coverage needs to change materially, create a new dataset version with a clear
name such as `product-docs-rag-v2`.

## 2. Run A Baseline

Open `Evaluate`.

Use these settings:

- Dataset: the golden dataset you want to track.
- Collection: `documents`, unless evaluating another collection intentionally.
- Baseline run: leave empty.
- Notes: describe the baseline context, for example
  `Baseline before rerank comparison`.

Start the evaluation and wait for it to complete. The run may take several
minutes because every dataset item calls retrieval, generation, and judge logic.

## 3. Inspect Baseline Results

Open `Results`.

Review aggregate scores first:

- `faithfulness`: whether the answer is supported by retrieved context.
- `answer_relevancy`: whether the answer addresses the question.
- `context_precision`: whether the top retrieved chunks are useful.
- `context_recall`: whether retrieval found enough supporting information.

Then expand low-scoring questions and classify each failure:

- Retrieval miss: the needed context was not retrieved.
- Weak answer: context was present but the generated answer was incomplete.
- Unsupported answer: the model made claims not supported by context.
- Dataset issue: the expected answer or source is wrong or ambiguous.
- Expected-source mismatch: the answer is right, but the source expectation is
  too narrow or stale.

Fix dataset issues before treating the score as a system failure.

## 4. Run A Candidate

Make one deliberate RAG change at a time. Examples:

- Enable or tune reranking.
- Change chunk size or overlap.
- Change prompt version.
- Change retrieval `top_k`.
- Re-index a collection after improving parsing or chunking.

Open `Evaluate` and run the same dataset against the same collection.

Use these settings:

- Baseline run: select the completed baseline run for the same dataset and
  collection.
- Notes: state the exact change, for example
  `Candidate: enabled cross-encoder reranking`.

If the baseline selector rejects a run, use a completed run from the same
dataset and collection.

## 5. Compare Runs

Open `Dashboard`.

Select the same dataset and collection. Compare the baseline run to the
candidate run.

Read metric deltas as directional evidence:

- Positive faithfulness delta suggests fewer unsupported claims.
- Positive answer relevancy delta suggests answers better address the query.
- Positive context precision delta suggests better ranking.
- Positive context recall delta suggests retrieval found more required support.

Do not accept a change from aggregate deltas alone. Inspect per-query results,
especially when one metric improves and another regresses.

## 6. Decide

Keep the change when:

- Aggregate scores improve or remain stable in the metrics the change targeted.
- Important per-query results do not regress.
- The result makes sense after reading retrieved contexts and answers.

Revert or adjust the change when:

- Improvements are tiny and likely noisy.
- A high-value query regresses.
- Retrieval metrics improve but answers become less faithful.
- The dataset was too weak to support a clear decision.

When the next improvement requires code or UI work, create a focused follow-up
issue with the baseline run, candidate run, metric deltas, and failure examples.

## 7. Repeat

Build history gradually. For now, evaluation notes and dashboard history are the
lightweight experiment log. After several real measurement cycles, use that
experience to decide what the experiment ledger should store.
