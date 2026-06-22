"""Wire-format pydantic models for events and findings."""

from __future__ import annotations

from enum import IntEnum

from pydantic import AwareDatetime, BaseModel, ConfigDict, Field


class Sensitivity(IntEnum):
    NONE = 0
    LOW = 1
    MEDIUM = 2
    HIGH = 3


class DataEvent(BaseModel):
    model_config = ConfigDict(frozen=True)

    event_id: str
    tenant_id: str
    bucket: str
    key: str
    size_bytes: int = Field(ge=0)
    occurred_at: AwareDatetime


class Finding(BaseModel):
    model_config = ConfigDict(frozen=False)

    event_id: str
    tenant_id: str
    bucket: str
    key: str
    sensitivity: Sensitivity
    categories: list[str] = Field(default_factory=list)
    match_count: int = Field(ge=0)
    classified_at: AwareDatetime
    pipeline_version: int = Field(ge=1)
    llm_failed: bool = False
    reason: str | None = None  # e.g. "too_large", "object_missing"
