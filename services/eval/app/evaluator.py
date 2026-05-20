from __future__ import annotations

import json
import logging
import re
import time
from collections.abc import Awaitable, Callable
from dataclasses import dataclass
from typing import TYPE_CHECKING

from llm.factory import get_llm_provider
from shared.llm.admission import generate_limiter

from app.metrics import eval_item_duration_seconds, eval_items_total

if TYPE_CHECKING:
    from app.rag_client import RAGClient

logger = logging.getLogger(__name__)

METRIC_NAMES = (
    "faithfulness",
    "answer_relevancy",
    "context_precision",
    "context_recall",
)

USAGE_KEYS = {
    "answer_model",
    "prompt_tokens",
    "completion_tokens",
    "generation_seconds",
    "answer_model_override",
}

ANSWER_MODEL_OVERRIDE_USAGE_KEYS = {
    "tier",
    "provider",
    "base_url",
    "model",
}

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
class EvalRunContext:
    eval_id: str
    collection: str
    requested_rerank: bool


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
    rerank: bool = False,
    top_k: int = 5,
    run_context: EvalRunContext | None = None,
    answer_model: dict | None = None,
) -> list[dict]:
    """Run each golden item through the RAG pipeline and build evaluation rows."""
    dataset = []
    for index, item in enumerate(items):
        started_at = time.perf_counter()
        query = item["query"]
        requested_rerank = str(rerank).lower()
        if run_context:
            logger.info(
                "eval_item_start eval_id=%s item_index=%s collection=%s rerank=%s",
                run_context.eval_id,
                index,
                run_context.collection,
                str(run_context.requested_rerank).lower(),
            )
        try:
            search_results = await rag_client.search(
                query, collection=collection, limit=top_k, rerank=rerank
            )
            chat_response = await rag_client.ask(
                query,
                collection=collection,
                rerank=rerank,
                retrieval_config={"top_k": top_k},
                answer_model=answer_model,
            )
        except Exception:
            eval_items_total.labels(
                status="failed", requested_rerank=requested_rerank
            ).inc()
            eval_item_duration_seconds.labels(
                stage="rag", requested_rerank=requested_rerank
            ).observe(time.perf_counter() - started_at)
            if run_context:
                logger.exception(
                    "eval_item_failed eval_id=%s item_index=%s collection=%s rerank=%s",
                    run_context.eval_id,
                    index,
                    run_context.collection,
                    str(run_context.requested_rerank).lower(),
                )
            raise

        row = {
            "user_input": query,
            "retrieved_contexts": [r["text"] for r in search_results],
            "response": chat_response["answer"],
            "reference": item["expected_answer"],
            "expected_sources": item.get("expected_sources", []),
        }
        if "retrieval" in chat_response:
            row["retrieval"] = chat_response["retrieval"]
        if "usage" in chat_response:
            row["usage"] = _safe_usage(chat_response["usage"])
        dataset.append(row)
        eval_items_total.labels(
            status="completed", requested_rerank=requested_rerank
        ).inc()
        eval_item_duration_seconds.labels(
            stage="rag", requested_rerank=requested_rerank
        ).observe(time.perf_counter() - started_at)
        if run_context:
            logger.info(
                "eval_item_completed eval_id=%s item_index=%s collection=%s rerank=%s",
                run_context.eval_id,
                index,
                run_context.collection,
                str(run_context.requested_rerank).lower(),
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
            raise EvaluationError(
                f"judge response {metric}.score must be numeric"
            ) from exc
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
- faithfulness: 1.0 means the answer is fully supported by the contexts;
  0.0 means unsupported or contradicted.
- answer_relevancy: 1.0 means the answer directly addresses the question
  and reference; 0.0 means irrelevant.

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
    permit = await generate_limiter.acquire()
    try:
        response = await llm.chat(
            [
                {
                    "role": "system",
                    "content": (
                        "You are a strict RAG evaluation judge. Return only valid JSON."
                    ),
                },
                {"role": "user", "content": _judge_prompt(row)},
            ]
        )
    finally:
        permit.release()
    raw = response.get("message", {}).get("content", "")
    return parse_judge_scores(raw)


def _aggregate(scores: list[dict]) -> dict:
    aggregate = {}
    for name in METRIC_NAMES:
        values = [score.get(name) for score in scores if score.get(name) is not None]
        aggregate[name] = round(sum(values) / len(values), 4) if values else None
    return aggregate


def _without_sensitive_keys(value: object) -> object:
    if isinstance(value, dict):
        return {
            key: _without_sensitive_keys(item)
            for key, item in value.items()
            if key.lower() != "api_key"
        }
    if isinstance(value, list):
        return [_without_sensitive_keys(item) for item in value]
    return value


def _safe_usage(raw_usage: object) -> dict:
    if not isinstance(raw_usage, dict):
        return {}

    usage = {
        key: _without_sensitive_keys(raw_usage[key])
        for key in USAGE_KEYS & raw_usage.keys()
    }
    override = usage.get("answer_model_override")
    if isinstance(override, dict):
        usage["answer_model_override"] = {
            key: override[key]
            for key in ANSWER_MODEL_OVERRIDE_USAGE_KEYS & override.keys()
        }
    else:
        usage.pop("answer_model_override", None)
    return usage


async def run_evaluation(
    items: list[dict],
    rag_client: RAGClient,
    collection: str | None,
    llm_provider: str,
    llm_base_url: str,
    llm_model: str,
    llm_api_key: str,
    rerank: bool = False,
    top_k: int = 5,
    judge: JudgeFn | None = None,
    run_context: EvalRunContext | None = None,
    answer_model: dict | None = None,
) -> tuple[dict, list[dict]]:
    """Run a full first-party RAG evaluation."""
    raw_dataset = await build_evaluation_dataset(
        items,
        rag_client,
        collection,
        rerank=rerank,
        top_k=top_k,
        run_context=run_context,
        answer_model=answer_model,
    )
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
        result = {
            "query": row["user_input"],
            "answer": row["response"],
            "contexts": row["retrieved_contexts"],
            "scores": scores,
            "score_reasons": judge_scores.reasons,
        }
        if "retrieval" in row:
            result["retrieval"] = row["retrieval"]
        if "usage" in row:
            result["usage"] = row["usage"]
        per_query.append(result)

    return _aggregate(all_scores), per_query
