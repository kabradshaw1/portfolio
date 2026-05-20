from __future__ import annotations

from typing import Any, Literal

from pydantic import BaseModel, Field

MetricName = Literal[
    "faithfulness",
    "answer_relevancy",
    "context_precision",
    "context_recall",
]

FailureMode = Literal[
    "retrieval_recall",
    "retrieval_precision",
    "generation_faithfulness",
    "answer_relevance",
    "runtime_or_config",
    "insufficient_evidence",
]

Confidence = Literal["low", "medium", "high"]


class Scores(BaseModel):
    faithfulness: float | None = None
    answer_relevancy: float | None = None
    context_precision: float | None = None
    context_recall: float | None = None


class QueryResult(BaseModel):
    query: str
    answer: str
    contexts: list[str] = Field(default_factory=list)
    scores: Scores
    score_reasons: dict[str, str] = Field(default_factory=dict)
    retrieval: dict[str, Any] = Field(default_factory=dict)


class EvaluationDetail(BaseModel):
    id: str
    dataset_id: str
    status: str
    collection: str | None = None
    aggregate_scores: Scores | None = None
    results: list[QueryResult] = Field(default_factory=list)
    error: str | None = None
    created_at: str | None = None
    completed_at: str | None = None
    notes: str | None = None
    config: dict[str, Any] = Field(default_factory=dict)
    baseline_eval_id: str | None = None


class TriageEvalRunRequest(BaseModel):
    eval_id: str
    metric: MetricName | None = None
    limit: int | None = None
    include_observability: bool = False


class TriageComparisonRequest(BaseModel):
    baseline_eval_id: str
    candidate_eval_id: str
    metric: MetricName | None = None
    limit: int | None = None
    include_observability: bool = False


class Diagnosis(BaseModel):
    primary_failure_mode: FailureMode
    confidence: Confidence
    summary: str


class CaseDiagnosis(BaseModel):
    query: str
    answer: str
    scores: Scores
    score_reasons: dict[str, str] = Field(default_factory=dict)
    failure_mode: FailureMode
    confidence: Confidence
    rationale: str
    evidence: dict[str, Any] = Field(default_factory=dict)


class Cluster(BaseModel):
    failure_mode: FailureMode
    count: int
    confidence: Confidence
    summary: str
    queries: list[str]


class Recommendation(BaseModel):
    action: str
    reason: str
    expected_impact: str
    evidence: dict[str, Any] = Field(default_factory=dict)


class TriageSubject(BaseModel):
    type: Literal["eval_run", "comparison"]
    eval_id: str | None = None
    baseline_eval_id: str | None = None
    candidate_eval_id: str | None = None


class TriageResponse(BaseModel):
    subject: TriageSubject
    status: str
    aggregate_scores: Scores | None = None
    config: dict[str, Any] = Field(default_factory=dict)
    diagnosis: Diagnosis
    clusters: list[Cluster]
    cases: list[CaseDiagnosis]
    recommendations: list[Recommendation]
    metric: MetricName
