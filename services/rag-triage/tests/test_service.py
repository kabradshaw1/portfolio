import pytest
from app.models import EvaluationDetail, QueryResult, Scores
from app.service import RAGTriageService


class FakeEvalClient:
    def __init__(self, evaluation: EvaluationDetail):
        self.evaluation = evaluation
        self.closed = False

    async def get_evaluation(self, eval_id: str) -> EvaluationDetail:
        assert eval_id == self.evaluation.id
        return self.evaluation

    async def close(self) -> None:
        self.closed = True


class FakeComparisonEvalClient:
    def __init__(self, evaluations):
        self.evaluations = evaluations

    async def get_evaluation(self, eval_id: str) -> EvaluationDetail:
        return self.evaluations[eval_id]


@pytest.mark.asyncio
async def test_triage_eval_run_returns_worst_cases_first():
    evaluation = EvaluationDetail(
        id="eval-1",
        dataset_id="dataset-1",
        status="completed",
        aggregate_scores=Scores(context_precision=0.4),
        results=[
            QueryResult(query="good", answer="a", scores=Scores(context_precision=0.9)),
            QueryResult(
                query="bad",
                answer="a",
                scores=Scores(context_precision=0.1, context_recall=0.9),
            ),
        ],
        config={"effective_retrieval_config": {"top_k": 5}},
    )
    service = RAGTriageService(
        eval_client=FakeEvalClient(evaluation),
        default_metric="context_precision",
        default_limit=5,
        max_limit=20,
    )

    response = await service.triage_eval_run("eval-1", metric=None, limit=None)

    assert response.subject.eval_id == "eval-1"
    assert response.metric == "context_precision"
    assert response.cases[0].query == "bad"
    assert response.diagnosis.primary_failure_mode == "retrieval_precision"


@pytest.mark.asyncio
async def test_triage_eval_run_uses_partial_results_for_completed_with_failures():
    evaluation = EvaluationDetail(
        id="eval-1",
        dataset_id="dataset-1",
        status="completed_with_failures",
        error="failed_items=1",
        aggregate_scores=Scores(context_precision=0.4),
        results=[
            QueryResult(
                query="bad",
                answer="a",
                scores=Scores(context_precision=0.1, context_recall=0.9),
            ),
        ],
        item_counts={"completed": 1, "failed": 1, "total": 2},
    )
    service = RAGTriageService(
        eval_client=FakeEvalClient(evaluation),
        default_metric="context_precision",
        default_limit=5,
        max_limit=20,
    )

    response = await service.triage_eval_run("eval-1", metric=None, limit=None)

    assert response.status == "completed_with_failures"
    assert response.cases[0].query == "bad"
    assert response.diagnosis.primary_failure_mode == "retrieval_precision"
    assert response.config["partial_results"] is True
    assert response.config["eval_error"] == "failed_items=1"
    assert response.config["item_counts"]["failed"] == 1


@pytest.mark.asyncio
async def test_failed_eval_run_is_runtime_or_config():
    evaluation = EvaluationDetail(
        id="eval-1",
        dataset_id="dataset-1",
        status="failed",
        error="evaluation timed out",
        results=[],
    )
    service = RAGTriageService(
        eval_client=FakeEvalClient(evaluation),
        default_metric="context_precision",
        default_limit=5,
        max_limit=20,
    )

    response = await service.triage_eval_run(
        "eval-1", metric="context_precision", limit=5
    )

    assert response.diagnosis.primary_failure_mode == "runtime_or_config"
    assert response.recommendations[0].action == "inspect_runtime_evidence"
    assert response.config["eval_error"] == "evaluation timed out"


@pytest.mark.asyncio
async def test_triage_eval_run_tie_breaks_by_query_and_answer():
    evaluation = EvaluationDetail(
        id="eval-1",
        dataset_id="dataset-1",
        status="completed",
        results=[
            QueryResult(query="z", answer="b", scores=Scores(context_precision=0.2)),
            QueryResult(query="a", answer="c", scores=Scores(context_precision=0.2)),
            QueryResult(query="a", answer="a", scores=Scores(context_precision=0.2)),
        ],
    )
    service = RAGTriageService(
        eval_client=FakeEvalClient(evaluation),
        default_metric="context_precision",
        default_limit=5,
        max_limit=20,
    )

    response = await service.triage_eval_run("eval-1", metric=None, limit=None)

    assert [(case.query, case.answer) for case in response.cases] == [
        ("a", "a"),
        ("a", "c"),
        ("z", "b"),
    ]


@pytest.mark.asyncio
async def test_triage_eval_run_handles_null_results_and_config():
    evaluation = EvaluationDetail(
        id="eval-1",
        dataset_id="dataset-1",
        status="completed",
        results=None,
        config=None,
    )
    service = RAGTriageService(
        eval_client=FakeEvalClient(evaluation),
        default_metric="context_precision",
        default_limit=5,
        max_limit=20,
    )

    response = await service.triage_eval_run("eval-1", metric=None, limit=None)

    assert response.diagnosis.primary_failure_mode == "runtime_or_config"
    assert response.config == {}


@pytest.mark.asyncio
async def test_close_delegates_to_eval_client():
    evaluation = EvaluationDetail(id="eval-1", dataset_id="dataset-1", status="failed")
    client = FakeEvalClient(evaluation)
    service = RAGTriageService(
        eval_client=client,
        default_metric="context_precision",
        default_limit=5,
        max_limit=20,
    )

    await service.close()

    assert client.closed


@pytest.mark.asyncio
async def test_triage_comparison_uses_candidate_worst_cases_and_delta():
    baseline = EvaluationDetail(
        id="base",
        dataset_id="dataset-1",
        status="completed",
        aggregate_scores=Scores(context_precision=0.8),
        results=[
            QueryResult(
                query="q1",
                answer="a",
                scores=Scores(context_precision=0.8, context_recall=0.8),
            )
        ],
    )
    candidate = EvaluationDetail(
        id="cand",
        dataset_id="dataset-1",
        status="completed",
        aggregate_scores=Scores(context_precision=0.3),
        results=[
            QueryResult(
                query="q1",
                answer="a",
                scores=Scores(context_precision=0.2, context_recall=0.8),
            )
        ],
    )
    service = RAGTriageService(
        eval_client=FakeComparisonEvalClient({"base": baseline, "cand": candidate}),
        default_metric="context_precision",
        default_limit=5,
        max_limit=20,
    )

    response = await service.triage_comparison("base", "cand", metric=None, limit=None)

    assert response.subject.type == "comparison"
    assert response.subject.baseline_eval_id == "base"
    assert response.subject.candidate_eval_id == "cand"
    assert response.diagnosis.primary_failure_mode == "retrieval_precision"
    assert response.config["metric_delta"] == -0.5


@pytest.mark.asyncio
async def test_triage_comparison_runtime_preserves_statuses_and_candidate_error():
    baseline = EvaluationDetail(
        id="base",
        dataset_id="dataset-1",
        status="completed",
        aggregate_scores=Scores(context_precision=0.8),
    )
    candidate = EvaluationDetail(
        id="cand",
        dataset_id="dataset-1",
        status="failed",
        aggregate_scores=Scores(context_precision=0.3),
        results=[],
        error="candidate timed out",
    )
    service = RAGTriageService(
        eval_client=FakeComparisonEvalClient({"base": baseline, "cand": candidate}),
        default_metric="context_precision",
        default_limit=5,
        max_limit=20,
    )

    response = await service.triage_comparison("base", "cand", metric=None, limit=None)

    assert response.subject.type == "comparison"
    assert response.config["baseline_status"] == "completed"
    assert response.config["candidate_status"] == "failed"
    assert response.config["eval_error"] == "candidate timed out"
    assert response.config["metric_delta"] == -0.5
