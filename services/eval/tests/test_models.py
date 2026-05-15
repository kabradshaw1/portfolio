import pytest
from app.models import (
    AttachExperimentRunRequest,
    CreateExperimentRequest,
    EvaluationDetail,
    ExperimentDetail,
    ExperimentRun,
    ExperimentRunEvaluation,
    QueryScore,
    RunComparison,
    RunHistory,
    StartEvaluationRequest,
    UpdateExperimentRequest,
)
from pydantic import ValidationError


def test_start_request_accepts_notes_and_baseline():
    req = StartEvaluationRequest(
        dataset_id="ds-1",
        notes="bumped chunk overlap to 300",
        baseline_eval_id="eval-prev",
    )
    assert req.notes == "bumped chunk overlap to 300"
    assert req.baseline_eval_id == "eval-prev"


def test_start_request_notes_max_length():
    with pytest.raises(ValidationError):
        StartEvaluationRequest(dataset_id="ds-1", notes="x" * 501)


def test_start_request_defaults_keep_optional_fields_none():
    req = StartEvaluationRequest(dataset_id="ds-1")
    assert req.notes is None
    assert req.baseline_eval_id is None


def test_start_request_accepts_rerank_flag():
    req = StartEvaluationRequest(dataset_id="ds-1", rerank=True)

    assert req.rerank is True


def test_evaluation_detail_includes_new_fields():
    detail = EvaluationDetail(
        id="e1",
        dataset_id="ds-1",
        status="completed",
        collection="documents",
        aggregate_scores=None,
        results=None,
        error=None,
        created_at="2026-04-28T00:00:00+00:00",
        completed_at=None,
        notes="bumped overlap",
        config={"chat": {"llm_model": "qwen"}},
        baseline_eval_id="eval-prev",
    )
    assert detail.notes == "bumped overlap"
    assert detail.config == {"chat": {"llm_model": "qwen"}}
    assert detail.baseline_eval_id == "eval-prev"


def test_run_comparison_shape():
    comp = RunComparison(
        runs=[],
        deltas={
            "faithfulness": [0.0],
            "answer_relevancy": [0.0],
            "context_precision": [0.0],
            "context_recall": [0.0],
        },
    )
    assert comp.deltas["faithfulness"] == [0.0]
    assert comp.runs == []


def test_run_history_shape():
    hist = RunHistory(runs=[])
    assert hist.runs == []


def test_create_experiment_request_defaults_to_planned():
    req = CreateExperimentRequest(
        name="precision tuning",
        hypothesis="Reranking improves context precision",
        dataset_id="ds-1",
        collection="documents",
    )

    assert req.status == "planned"
    assert req.baseline_eval_id is None
    assert req.notes is None


def test_create_experiment_request_accepts_running_status():
    req = CreateExperimentRequest(
        name="precision tuning",
        hypothesis="Reranking improves context precision",
        dataset_id="ds-1",
        collection="documents",
        status="running",
    )

    assert req.status == "running"


def test_create_experiment_request_rejects_final_initial_status():
    with pytest.raises(ValidationError):
        CreateExperimentRequest(
            name="precision tuning",
            hypothesis="Reranking improves context precision",
            dataset_id="ds-1",
            collection="documents",
            status="completed",
        )


def test_update_experiment_request_accepts_decision_values():
    req = UpdateExperimentRequest(status="completed", decision="keep")

    assert req.status == "completed"
    assert req.decision == "keep"


def test_update_experiment_request_rejects_unknown_decision():
    with pytest.raises(ValidationError):
        UpdateExperimentRequest(decision="ship_it")


def test_attach_experiment_run_request_requires_label():
    with pytest.raises(ValidationError):
        AttachExperimentRunRequest(evaluation_id="eval-1", label="")


def test_start_evaluation_request_accepts_experiment_attachment():
    req = StartEvaluationRequest(
        dataset_id="ds-1",
        experiment_id="exp-1",
        experiment_label="rerank_on",
    )

    assert req.experiment_id == "exp-1"
    assert req.experiment_label == "rerank_on"


def test_experiment_detail_includes_attached_runs():
    detail = ExperimentDetail(
        id="exp-1",
        name="precision tuning",
        hypothesis="Reranking improves context precision",
        dataset_id="ds-1",
        collection="documents",
        baseline_eval_id="eval-base",
        status="running",
        decision=None,
        notes="first pass",
        created_at="2026-05-13T10:00:00+00:00",
        updated_at="2026-05-13T10:00:00+00:00",
        runs=[
            ExperimentRun(
                evaluation_id="eval-base",
                label="baseline",
                notes="rerank off",
                attached_at="2026-05-13T10:01:00+00:00",
                evaluation=ExperimentRunEvaluation(
                    id="eval-base",
                    dataset_id="ds-1",
                    status="completed",
                    collection="documents",
                    aggregate_scores=QueryScore(context_precision=0.31),
                    created_at="2026-05-13T09:50:00+00:00",
                    completed_at="2026-05-13T09:55:00+00:00",
                    notes="baseline",
                    config=None,
                    baseline_eval_id=None,
                ),
            )
        ],
    )

    assert detail.runs[0].label == "baseline"
    assert detail.runs[0].evaluation.aggregate_scores.context_precision == 0.31
