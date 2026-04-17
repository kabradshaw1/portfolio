from __future__ import annotations

import logging
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from app.rag_client import RAGClient

logger = logging.getLogger(__name__)


async def build_ragas_dataset(
    items: list[dict],
    rag_client: RAGClient,
    collection: str | None,
) -> list[dict]:
    """Run each golden item through the RAG pipeline and build RAGAS evaluation rows."""
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
            }
        )
    return dataset


def _create_llm(provider: str, base_url: str, model: str, api_key: str):
    """Create a RAGAS-compatible LLM from the service config."""
    from ragas.llms import llm_factory

    if provider == "ollama":
        return llm_factory(model=model, base_url=f"{base_url}/v1")
    else:
        return llm_factory(model=model, base_url=base_url)


async def run_evaluation(
    items: list[dict],
    rag_client: RAGClient,
    collection: str | None,
    llm_provider: str,
    llm_base_url: str,
    llm_model: str,
    llm_api_key: str,
) -> tuple[dict, list[dict]]:
    """Run a full RAGAS evaluation and return (aggregate_scores, per_query_results).

    RAGAS imports are deferred to call time because ragas uses nest_asyncio
    at import time, which is incompatible with uvloop (used by uvicorn).
    """
    # Lazy imports — ragas + nest_asyncio conflict with uvloop at import time
    from ragas import EvaluationDataset
    from ragas import evaluate as ragas_evaluate
    from ragas.dataset_schema import SingleTurnSample
    from ragas.metrics import (
        AnswerRelevancy,
        ContextPrecision,
        ContextRecall,
        Faithfulness,
    )

    # Step 1: Build dataset by running queries through RAG pipeline
    raw_dataset = await build_ragas_dataset(items, rag_client, collection)

    # Step 2: Convert to RAGAS EvaluationDataset
    samples = [
        SingleTurnSample(
            user_input=row["user_input"],
            retrieved_contexts=row["retrieved_contexts"],
            response=row["response"],
            reference=row["reference"],
        )
        for row in raw_dataset
    ]
    eval_dataset = EvaluationDataset(samples=samples)

    # Step 3: Create LLM for judge calls
    judge_llm = _create_llm(llm_provider, llm_base_url, llm_model, llm_api_key)

    # Step 4: Run RAGAS evaluate
    metrics = [
        Faithfulness(llm=judge_llm),
        AnswerRelevancy(llm=judge_llm),
        ContextPrecision(llm=judge_llm),
        ContextRecall(llm=judge_llm),
    ]

    result = ragas_evaluate(dataset=eval_dataset, metrics=metrics)

    # Step 5: Extract scores
    scores = result.scores
    metric_names = [
        "faithfulness",
        "answer_relevancy",
        "context_precision",
        "context_recall",
    ]

    # Compute aggregates
    aggregate = {}
    for name in metric_names:
        values = [s.get(name) for s in scores if s.get(name) is not None]
        aggregate[name] = round(sum(values) / len(values), 4) if values else None

    # Build per-query results
    per_query = []
    for i, row in enumerate(raw_dataset):
        per_query.append(
            {
                "query": row["user_input"],
                "answer": row["response"],
                "contexts": row["retrieved_contexts"],
                "scores": scores[i] if i < len(scores) else {},
            }
        )

    return aggregate, per_query
