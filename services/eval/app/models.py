from typing import Any, Literal

from pydantic import BaseModel, Field


class GoldenItem(BaseModel):
    query: str = Field(max_length=2000)
    expected_answer: str = Field(max_length=5000)
    expected_sources: list[str] = Field(default_factory=list)


class CreateDatasetRequest(BaseModel):
    name: str = Field(min_length=1, max_length=100, pattern=r"^[a-zA-Z0-9_-]+$")
    items: list[GoldenItem] = Field(min_length=1, max_length=100)


class DatasetSummary(BaseModel):
    id: str
    name: str
    created_at: str
    item_count: int


class DatasetDetail(BaseModel):
    id: str
    name: str
    items: list[GoldenItem]
    created_at: str


class StartEvaluationRequest(BaseModel):
    dataset_id: str
    collection: str | None = Field(default=None, pattern=r"^[a-zA-Z0-9_-]{1,100}$")
    notes: str | None = Field(default=None, max_length=500)
    baseline_eval_id: str | None = None
    rerank: bool = False
    experiment_id: str | None = None
    experiment_label: str | None = Field(
        default=None, min_length=1, max_length=100, pattern=r"^[a-zA-Z0-9_-]+$"
    )


class QueryScore(BaseModel):
    faithfulness: float | None = None
    answer_relevancy: float | None = None
    context_precision: float | None = None
    context_recall: float | None = None


class QueryResult(BaseModel):
    query: str
    answer: str
    contexts: list[str]
    scores: QueryScore


class EvaluationSummary(BaseModel):
    id: str
    dataset_id: str
    status: str
    collection: str | None
    aggregate_scores: QueryScore | None
    created_at: str
    completed_at: str | None
    notes: str | None = None
    config: dict[str, Any] | None = None
    baseline_eval_id: str | None = None


ExperimentStatus = Literal["planned", "running", "completed", "abandoned"]
InitialExperimentStatus = Literal["planned", "running"]
ExperimentDecision = Literal["keep", "revert", "needs_more_data"]


class CreateExperimentRequest(BaseModel):
    name: str = Field(min_length=1, max_length=100)
    hypothesis: str = Field(min_length=1, max_length=1000)
    dataset_id: str
    collection: str = Field(pattern=r"^[a-zA-Z0-9_-]{1,100}$")
    baseline_eval_id: str | None = None
    status: InitialExperimentStatus = "planned"
    notes: str | None = Field(default=None, max_length=2000)


class UpdateExperimentRequest(BaseModel):
    hypothesis: str | None = Field(default=None, min_length=1, max_length=1000)
    baseline_eval_id: str | None = None
    status: ExperimentStatus | None = None
    decision: ExperimentDecision | None = None
    notes: str | None = Field(default=None, max_length=2000)


class AttachExperimentRunRequest(BaseModel):
    evaluation_id: str
    label: str = Field(min_length=1, max_length=100, pattern=r"^[a-zA-Z0-9_-]+$")
    notes: str | None = Field(default=None, max_length=1000)


class ExperimentRunEvaluation(BaseModel):
    id: str
    dataset_id: str
    status: str
    collection: str | None
    aggregate_scores: QueryScore | None
    created_at: str
    completed_at: str | None
    notes: str | None = None
    config: dict[str, Any] | None = None
    baseline_eval_id: str | None = None


class ExperimentRun(BaseModel):
    evaluation_id: str
    label: str
    notes: str | None = None
    attached_at: str
    evaluation: ExperimentRunEvaluation


class ExperimentSummary(BaseModel):
    id: str
    name: str
    hypothesis: str
    dataset_id: str
    collection: str
    baseline_eval_id: str | None = None
    status: ExperimentStatus
    decision: ExperimentDecision | None = None
    notes: str | None = None
    created_at: str
    updated_at: str


class ExperimentDetail(ExperimentSummary):
    runs: list[ExperimentRun] = Field(default_factory=list)


class EvaluationDetail(BaseModel):
    id: str
    dataset_id: str
    status: str
    collection: str | None
    aggregate_scores: QueryScore | None
    results: list[QueryResult] | None
    error: str | None
    created_at: str
    completed_at: str | None
    notes: str | None = None
    config: dict[str, Any] | None = None
    baseline_eval_id: str | None = None


class RunComparison(BaseModel):
    runs: list[EvaluationDetail]
    deltas: dict[str, list[float]]


class RunHistory(BaseModel):
    runs: list[EvaluationDetail]


class DashboardDatasetSummary(BaseModel):
    id: str
    name: str
    item_count: int


class DashboardRunSummary(BaseModel):
    id: str
    created_at: str
    completed_at: str | None
    notes: str | None = None
    config_captured: bool
    aggregate_scores: QueryScore | None
    baseline_eval_id: str | None = None


class MetricTrendPoint(BaseModel):
    evaluation_id: str
    completed_at: str | None
    score: float | None


class DashboardBaselineDeltas(BaseModel):
    baseline_eval_id: str
    latest_eval_id: str
    deltas: QueryScore


class EvaluationDashboard(BaseModel):
    dataset: DashboardDatasetSummary
    collection: str
    completed_run_count: int
    first_completed_run: DashboardRunSummary | None = None
    latest_completed_run: DashboardRunSummary | None = None
    metric_trends: dict[str, list[MetricTrendPoint]]
    recent_runs: list[DashboardRunSummary]
    baseline_to_latest_deltas: DashboardBaselineDeltas | None = None
