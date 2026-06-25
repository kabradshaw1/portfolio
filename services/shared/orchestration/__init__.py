"""In-process multi-agent pipeline scaffold.

Coordinates stages with standardized metrics, tracing, structured-logging
context, output validation, and error classification.
"""

from shared.orchestration.context import PipelineContext
from shared.orchestration.errors import (
    CancelledPipelineError,
    OutputValidationError,
    StageError,
    classify_error,
)
from shared.orchestration.pipeline import Pipeline
from shared.orchestration.role import Role
from shared.orchestration.stage import Stage, StreamingStage
from shared.orchestration.validation import call_validated

__all__ = [
    "CancelledPipelineError",
    "OutputValidationError",
    "Pipeline",
    "PipelineContext",
    "Role",
    "Stage",
    "StreamingStage",
    "StageError",
    "call_validated",
    "classify_error",
]
