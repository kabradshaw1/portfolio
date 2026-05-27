from typing import Any, Literal

from pydantic import BaseModel, ConfigDict, Field, field_validator, model_validator


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


class RetrievalConfig(BaseModel):
    model_config = ConfigDict(extra="forbid")

    top_k: int | None = Field(default=None, ge=1, le=20, strict=True)


ReadinessStatus = Literal["ready", "warning", "blocked"]


class ReadinessFinding(BaseModel):
    code: str
    message: str
    remediation: str


class RAGReadinessRequest(BaseModel):
    dataset_id: str
    collection: str = Field(pattern=r"^[a-zA-Z0-9_-]{1,100}$")
    rerank: bool = False
    retrieval_config: RetrievalConfig | None = None
    baseline_eval_id: str | None = None
    experiment_id: str | None = None


class RAGReadinessResponse(BaseModel):
    status: ReadinessStatus
    blocking_failures: list[ReadinessFinding] = Field(default_factory=list)
    warnings: list[ReadinessFinding] = Field(default_factory=list)
    evidence: dict[str, Any] = Field(default_factory=dict)
    next_steps: list[str] = Field(default_factory=list)


class StartEvaluationRequest(BaseModel):
    dataset_id: str
    collection: str | None = Field(default=None, pattern=r"^[a-zA-Z0-9_-]{1,100}$")
    notes: str | None = Field(default=None, max_length=500)
    baseline_eval_id: str | None = None
    rerank: bool = False
    retrieval_config: RetrievalConfig | None = None
    experiment_id: str | None = None
    experiment_label: str | None = Field(
        default=None, min_length=1, max_length=100, pattern=r"^[a-zA-Z0-9_-]+$"
    )
    answer_tier: str | None = Field(default=None, pattern=r"^[a-zA-Z0-9_-]{1,50}$")
    answer_provider: str | None = None
    answer_base_url: str | None = Field(default=None, max_length=300)
    answer_model: str | None = Field(default=None, max_length=100)
    answer_api_key_secret: str | None = Field(
        default=None, pattern=r"^[A-Z][A-Z0-9_]{1,100}$"
    )

    @field_validator("answer_provider")
    @classmethod
    def validate_answer_provider(cls, value: str | None) -> str | None:
        if value is None:
            return None
        if value not in {"ollama", "openai", "anthropic"}:
            raise ValueError(
                "answer_provider must be 'ollama', 'openai', or 'anthropic'"
            )
        return value

    @field_validator("answer_api_key_secret")
    @classmethod
    def reject_secret_values(cls, value: str | None) -> str | None:
        if value is None:
            return None
        lowered = value.lower()
        if lowered.startswith(("sk-", "sk_", "bearer ", "api-")):
            raise ValueError("answer_api_key_secret must name an environment variable")
        return value

    @model_validator(mode="after")
    def validate_answer_override(self) -> "StartEvaluationRequest":
        fields = [
            self.answer_tier,
            self.answer_provider,
            self.answer_base_url,
            self.answer_model,
            self.answer_api_key_secret,
        ]
        if any(value is not None for value in fields):
            if not self.answer_provider:
                raise ValueError("answer_provider is required with answer override")
            if not self.answer_model:
                raise ValueError("answer_model is required with answer override")
        return self


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


class DLQPayload(BaseModel):
    message_version: int
    evaluation_id: str
    item_id: str
    item_index: int
    attempt: int


class DLQRouting(BaseModel):
    exchange: str
    routing_key: str
    queue: str
    death_count: int
    death_reason: str


class DLQItemEvidence(BaseModel):
    evaluation_id: str
    item_id: str
    item_index: int
    status: str
    attempt_count: int
    max_attempts: int
    last_error: dict[str, Any] | None = None
    replay_count: int = 0
    last_replayed_at: str | None = None


class DLQEvaluationEvidence(BaseModel):
    status: str
    collection: str | None = None
    created_at: str
    completed_at: str | None = None


class DLQEntryResponse(BaseModel):
    index: int
    delivery_tag: str
    redelivered: bool
    payload: DLQPayload | None = None
    routing: DLQRouting
    item: DLQItemEvidence | None = None
    evaluation: DLQEvaluationEvidence | None = None
    invalid_payload: str | None = None


class DLQListResponse(BaseModel):
    entries: list[DLQEntryResponse]
    indexes_are_transient: bool = True


class ReplayDLQItemRequest(BaseModel):
    item_id: str | None = None
    index: int | None = Field(default=None, ge=0)

    @model_validator(mode="after")
    def exactly_one_selector(self) -> "ReplayDLQItemRequest":
        selectors = [self.item_id is not None, self.index is not None]
        if selectors.count(True) != 1:
            raise ValueError("provide exactly one of item_id or index")
        return self


class ReplayDLQItemResponse(BaseModel):
    evaluation_id: str
    item_id: str
    item_index: int
    status: str
    replay_count: int
    message_published: bool


ExperimentStatus = Literal["planned", "running", "completed", "abandoned"]
InitialExperimentStatus = Literal["planned", "running"]
ExperimentDecision = Literal["keep", "revert", "needs_more_data"]
FocusMetric = Literal[
    "faithfulness",
    "answer_relevancy",
    "context_precision",
    "context_recall",
]
ExperimentEvidence = dict[str, Any]


class CreateExperimentRequest(BaseModel):
    name: str = Field(min_length=1, max_length=100)
    hypothesis: str = Field(min_length=1, max_length=1000)
    dataset_id: str
    collection: str = Field(pattern=r"^[a-zA-Z0-9_-]{1,100}$")
    baseline_eval_id: str | None = None
    focus_metric: FocusMetric = "context_precision"
    status: InitialExperimentStatus = "planned"
    notes: str | None = Field(default=None, max_length=2000)


class UpdateExperimentRequest(BaseModel):
    hypothesis: str | None = Field(default=None, min_length=1, max_length=1000)
    baseline_eval_id: str | None = None
    focus_metric: FocusMetric | None = None
    status: ExperimentStatus | None = None
    decision: ExperimentDecision | None = None
    conclusion: str | None = Field(default=None, max_length=5000)
    evidence: ExperimentEvidence | None = None
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
    focus_metric: FocusMetric
    status: ExperimentStatus
    decision: ExperimentDecision | None = None
    conclusion: str | None = None
    evidence: ExperimentEvidence | None = None
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
