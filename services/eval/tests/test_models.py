import pytest
from app.models import (
    EvaluationDetail,
    RunComparison,
    RunHistory,
    StartEvaluationRequest,
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
