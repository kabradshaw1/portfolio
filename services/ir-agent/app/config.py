from pydantic_settings import BaseSettings


class Settings(BaseSettings):
    anthropic_api_key: str = ""

    triage_model: str = "claude-haiku-4-5"
    investigate_model: str = "claude-opus-4-8"
    validate_model: str = "claude-opus-4-8"
    report_model: str = "claude-sonnet-4-6"

    max_investigate_attempts: int = 2
    max_tool_steps: int = 6
    request_timeout_seconds: int = 120

    allowed_origins: str = "https://kylebradshaw.dev"
    jwt_secret: str = ""

    model_config = {"env_prefix": "IR_"}

    def validate(self) -> None:
        """Fail fast if the Anthropic key is missing."""
        if not self.anthropic_api_key:
            raise ValueError("IR_ANTHROPIC_API_KEY is required")


settings = Settings()
