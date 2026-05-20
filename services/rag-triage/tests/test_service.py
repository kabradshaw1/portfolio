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
