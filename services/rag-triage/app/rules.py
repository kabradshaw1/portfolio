from __future__ import annotations

from collections import Counter, defaultdict

from app.models import CaseDiagnosis, Cluster, QueryResult, Recommendation

LOW = 0.5
ACCEPTABLE = 0.7


def classify_case(result: QueryResult) -> CaseDiagnosis:
    scores = result.scores
    evidence = {
        "context_recall": scores.context_recall,
        "context_precision": scores.context_precision,
        "faithfulness": scores.faithfulness,
        "answer_relevancy": scores.answer_relevancy,
    }

    if scores.context_recall is not None and scores.context_recall < LOW:
        return _case(
            result,
            "retrieval_recall",
            "high",
            "Low context recall indicates the expected answer is not covered by "
            "retrieved context.",
            evidence,
        )

    if (
        scores.context_precision is not None
        and scores.context_precision < LOW
        and scores.context_recall is not None
        and scores.context_recall >= ACCEPTABLE
    ):
        return _case(
            result,
            "retrieval_precision",
            "high",
            "Low context precision with acceptable recall indicates noisy retrieval "
            "or reranking.",
            evidence,
        )

    if (
        scores.faithfulness is not None
        and scores.faithfulness < LOW
        and _has_usable_context(scores.context_recall, scores.context_precision)
    ):
        return _case(
            result,
            "generation_faithfulness",
            "medium",
            "Retrieved context is usable, but the generated answer is weakly "
            "supported.",
            evidence,
        )

    if scores.answer_relevancy is not None and scores.answer_relevancy < LOW:
        return _case(
            result,
            "answer_relevance",
            "medium",
            "Answer relevancy is low, suggesting the response does not directly "
            "target the question.",
            evidence,
        )

    return _case(
        result,
        "insufficient_evidence",
        "low",
        "Scores do not point to one clear failure mode.",
        evidence,
    )


def cluster_cases(cases: list[CaseDiagnosis]) -> list[Cluster]:
    counts = Counter(case.failure_mode for case in cases)
    queries_by_mode: dict[str, list[str]] = defaultdict(list)
    confidence_by_mode: dict[str, str] = {}

    for case in cases:
        queries_by_mode[case.failure_mode].append(case.query)
        confidence_by_mode.setdefault(case.failure_mode, case.confidence)

    return [
        Cluster(
            failure_mode=mode,
            count=count,
            confidence=confidence_by_mode.get(mode, "low"),
            summary=_cluster_summary(mode, count),
            queries=queries_by_mode[mode],
        )
        for mode, count in counts.most_common()
    ]


def recommendations_for_clusters(clusters: list[Cluster]) -> list[Recommendation]:
    recommendations: list[Recommendation] = []

    for cluster in clusters:
        evidence = {"failure_mode": cluster.failure_mode, "case_count": cluster.count}
        if cluster.failure_mode == "retrieval_recall":
            recommendations.append(
                Recommendation(
                    action="increase_top_k",
                    reason="Worst cases lack expected answer coverage in retrieved "
                    "contexts.",
                    expected_impact="Improves context recall for answerable questions.",
                    evidence=evidence,
                )
            )
        elif cluster.failure_mode == "retrieval_precision":
            recommendations.append(
                Recommendation(
                    action="enable_or_tune_rerank",
                    reason="Worst cases retrieve relevant material plus too much "
                    "noise.",
                    expected_impact="Improves context precision without requiring "
                    "corpus changes.",
                    evidence=evidence,
                )
            )
        elif cluster.failure_mode == "generation_faithfulness":
            recommendations.append(
                Recommendation(
                    action="prompt_grounding_change",
                    reason="Retrieved contexts look usable but answers are not "
                    "sufficiently supported.",
                    expected_impact="Reduces unsupported claims in generated answers.",
                    evidence=evidence,
                )
            )
        elif cluster.failure_mode == "answer_relevance":
            recommendations.append(
                Recommendation(
                    action="review_expected_answer",
                    reason="Answer relevancy is weak; expected answer or prompt "
                    "targeting may be misaligned.",
                    expected_impact="Clarifies whether the failure is model behavior "
                    "or dataset expectation.",
                    evidence=evidence,
                )
            )
        elif cluster.failure_mode == "runtime_or_config":
            recommendations.append(
                Recommendation(
                    action="inspect_runtime_evidence",
                    reason="Run status or configuration prevents quality-only "
                    "diagnosis.",
                    expected_impact="Separates infrastructure failures from RAG "
                    "quality regressions.",
                    evidence=evidence,
                )
            )

    return recommendations


def _has_usable_context(recall: float | None, precision: float | None) -> bool:
    return (recall is not None and recall >= ACCEPTABLE) or (
        precision is not None and precision >= ACCEPTABLE
    )


def _case(
    result: QueryResult,
    mode: str,
    confidence: str,
    rationale: str,
    evidence: dict,
) -> CaseDiagnosis:
    return CaseDiagnosis(
        query=result.query,
        answer=result.answer,
        scores=result.scores,
        score_reasons=result.score_reasons,
        failure_mode=mode,
        confidence=confidence,
        rationale=rationale,
        evidence=evidence,
    )


def _cluster_summary(mode: str, count: int) -> str:
    labels = {
        "retrieval_recall": "Retrieved contexts miss expected answer coverage.",
        "retrieval_precision": "Retrieved contexts are noisy for the question.",
        "generation_faithfulness": "Generated answers are not sufficiently grounded.",
        "answer_relevance": "Generated answers do not directly address the expected "
        "answer.",
        "runtime_or_config": "Runtime or configuration evidence blocks quality-only "
        "diagnosis.",
        "insufficient_evidence": "Scores do not identify a clear failure mode.",
    }
    return f"{labels[mode]} Cases: {count}."
