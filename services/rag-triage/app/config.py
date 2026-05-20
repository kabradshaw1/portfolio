from pydantic import field_validator, model_validator
from pydantic_settings import BaseSettings

METRIC_NAMES = {
    "faithfulness",
    "answer_relevancy",
    "context_precision",
    "context_recall",
}


class Settings(BaseSettings):
    eval_api_url: str = "http://eval:8000"
    eval_api_token: str = ""
    request_timeout_seconds: float = 30.0
    default_metric: str = "context_precision"
    default_limit: int = 5
    max_limit: int = 20
    allowed_origins: str = "https://kylebradshaw.dev"

    @field_validator("default_metric")
    @classmethod
    def validate_default_metric(cls, value: str) -> str:
        if value not in METRIC_NAMES:
            allowed = ", ".join(sorted(METRIC_NAMES))
            raise ValueError(f"default_metric must be one of: {allowed}")
        return value

    @field_validator("request_timeout_seconds")
    @classmethod
    def validate_timeout(cls, value: float) -> float:
        if value <= 0:
            raise ValueError("request_timeout_seconds must be positive")
        return value

    @field_validator("default_limit", "max_limit")
    @classmethod
    def validate_limit(cls, value: int) -> int:
        if value <= 0:
            raise ValueError("limits must be positive")
        return value

    @model_validator(mode="after")
    def validate_default_limit_within_max(self) -> "Settings":
        if self.default_limit > self.max_limit:
            raise ValueError("default_limit cannot exceed max_limit")
        return self


settings = Settings()
