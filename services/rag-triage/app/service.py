from __future__ import annotations

from typing import cast

from app.models import (
    Cluster,
    Diagnosis,
    EvaluationDetail,
    MetricName,
    Scores,
    TriageResponse,
    TriageSubject,
)
from app.rules import classify_case, cluster_cases, recommendations_for_clusters


class RAGTriageService:
    def __init__(
        self,
        eval_client,
        default_metric: str,
        default_limit: int,
        max_limit: int,
    ):
        self._eval_client = eval_client
        self._default_metric = default_metric
        self._default_limit = default_limit
        self._max_limit = max_limit

    async def triage_eval_run(
        self,
        eval_id: str,
        metric: MetricName | None,
        limit: int | None,
    ) -> TriageResponse:
        selected_metric = cast(MetricName, metric or self._default_metric)
        selected_limit = self._bounded_limit(limit)
        evaluation = await self._eval_client.get_evaluation(eval_id)
        results = evaluation.results or []

        if evaluation.status != "completed" or not results:
            return self._runtime_response(evaluation, selected_metric)

        worst = sorted(
            results,
            key=lambda result: (
                self._score_for_metric(result.scores, selected_metric),
                result.query,
                result.answer,
            ),
        )[:selected_limit]
        cases = [classify_case(result) for result in worst]
        clusters = cluster_cases(cases)
        recommendations = recommendations_for_clusters(clusters)
        primary = clusters[0].failure_mode if clusters else "insufficient_evidence"
        confidence = clusters[0].confidence if clusters else "low"

        return TriageResponse(
            subject=TriageSubject(type="eval_run", eval_id=evaluation.id),
            status=evaluation.status,
            aggregate_scores=evaluation.aggregate_scores,
            config=evaluation.config or {},
            diagnosis=Diagnosis(
                primary_failure_mode=primary,
                confidence=confidence,
                summary=self._summary(primary),
            ),
            clusters=clusters,
            cases=cases,
            recommendations=recommendations,
            metric=selected_metric,
        )

    def _bounded_limit(self, limit: int | None) -> int:
        if limit is None:
            return self._default_limit
        return min(max(limit, 1), self._max_limit)

    async def close(self) -> None:
        await self._eval_client.close()

    def _runtime_response(
        self,
        evaluation: EvaluationDetail,
        metric: MetricName,
    ) -> TriageResponse:
        runtime_cluster = Cluster(
            failure_mode="runtime_or_config",
            count=1,
            confidence="high",
            summary="Runtime or configuration evidence blocks quality-only diagnosis.",
            queries=[],
        )
        diagnosis = Diagnosis(
            primary_failure_mode="runtime_or_config",
            confidence="high",
            summary="The evaluation did not produce completed results, so triage "
            "should inspect runtime or configuration evidence.",
        )
        config = evaluation.config or {}
        if evaluation.error:
            config = {**config, "eval_error": evaluation.error}

        return TriageResponse(
            subject=TriageSubject(type="eval_run", eval_id=evaluation.id),
            status=evaluation.status,
            aggregate_scores=evaluation.aggregate_scores,
            config=config,
            diagnosis=diagnosis,
            clusters=[runtime_cluster],
            cases=[],
            recommendations=recommendations_for_clusters([runtime_cluster]),
            metric=metric,
        )

    def _score_for_metric(self, scores: Scores, metric: MetricName) -> float:
        value = getattr(scores, metric)
        return 1.0 if value is None else value

    def _summary(self, mode: str) -> str:
        summaries = {
            "retrieval_recall": "Worst cases suggest retrieved contexts miss required "
            "answer coverage.",
            "retrieval_precision": "Worst cases suggest retrieved contexts are noisy "
            "relative to the questions.",
            "generation_faithfulness": "Worst cases suggest generation is not "
            "sufficiently grounded in retrieved contexts.",
            "answer_relevance": "Worst cases suggest answers are not targeting the "
            "expected response.",
            "runtime_or_config": "Runtime or configuration evidence blocks "
            "quality-only diagnosis.",
            "insufficient_evidence": "Available scores do not identify one dominant "
            "regression cause.",
        }
        return summaries[mode]
