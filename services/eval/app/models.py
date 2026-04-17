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


class DatasetDetail(BaseModel):
    id: str
    name: str
    items: list[GoldenItem]
    created_at: str


class StartEvaluationRequest(BaseModel):
    dataset_id: str
    collection: str | None = Field(default=None, pattern=r"^[a-zA-Z0-9_-]{1,100}$")


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
