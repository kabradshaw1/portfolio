import pytest
from app.models import EvaluationDetail, TriageEvalRunRequest
from pydantic import ValidationError


def test_evaluation_detail_accepts_null_results_and_config():
    evaluation = EvaluationDetail.model_validate(
        {
            "id": "eval-1",
            "dataset_id": "dataset-1",
            "status": "completed",
            "results": None,
            "config": None,
        }
    )

    assert evaluation.results is None
    assert evaluation.config is None


@pytest.mark.parametrize("limit", [0, -1, "5"])
def test_triage_request_rejects_invalid_limit(limit):
    with pytest.raises(ValidationError):
        TriageEvalRunRequest(eval_id="eval-1", limit=limit)
