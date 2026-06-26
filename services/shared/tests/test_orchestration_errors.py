from shared.orchestration.errors import (
    CancelledPipelineError,
    OutputValidationError,
    StageError,
    classify_error,
)


def test_stage_error_carries_stage_and_retryable():
    err = StageError("boom", stage="retrieve", retryable=True)
    assert err.stage == "retrieve"
    assert err.retryable is True
    assert "boom" in str(err)


def test_output_validation_error_is_permanent_by_default():
    err = OutputValidationError("bad json", stage="judge")
    assert isinstance(err, StageError)
    assert err.retryable is False


def test_classify_error_passes_through_stage_error():
    original = StageError("x", stage="a", retryable=True)
    assert classify_error(original, stage="b") is original


def test_classify_error_marks_connection_errors_retryable():
    err = classify_error(ConnectionError("conn"), stage="generate")
    assert err.stage == "generate"
    assert err.retryable is True


def test_classify_error_marks_value_errors_permanent():
    err = classify_error(ValueError("nope"), stage="generate")
    assert err.retryable is False


def test_cancelled_pipeline_error_is_not_stage_error():
    assert not issubclass(CancelledPipelineError, StageError)
