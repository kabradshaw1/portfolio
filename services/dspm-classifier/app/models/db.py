"""SQLAlchemy models for findings and processed-messages."""

from __future__ import annotations

from datetime import datetime

from sqlalchemy import ARRAY, Boolean, DateTime, Index, Integer, String, Text
from sqlalchemy.orm import DeclarativeBase, Mapped, mapped_column


class Base(DeclarativeBase):
    pass


class FindingRow(Base):
    __tablename__ = "findings"

    tenant_id: Mapped[str] = mapped_column(String(128), primary_key=True)
    event_id: Mapped[str] = mapped_column(String(128), primary_key=True)
    bucket: Mapped[str] = mapped_column(String(255), nullable=False)
    key: Mapped[str] = mapped_column(Text, nullable=False)
    sensitivity: Mapped[int] = mapped_column(Integer, nullable=False)
    categories: Mapped[list[str]] = mapped_column(ARRAY(String), nullable=False, default=list)
    match_count: Mapped[int] = mapped_column(Integer, nullable=False, default=0)
    classified_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), nullable=False)
    pipeline_version: Mapped[int] = mapped_column(Integer, nullable=False)
    llm_failed: Mapped[bool] = mapped_column(Boolean, nullable=False, default=False)
    reason: Mapped[str | None] = mapped_column(Text, nullable=True)

    __table_args__ = (
        Index(
            "ix_findings_tenant_sensitivity_time",
            "tenant_id",
            "sensitivity",
            "classified_at",
        ),
    )


class ProcessedMessageRow(Base):
    __tablename__ = "processed_messages"

    event_id: Mapped[str] = mapped_column(String(128), primary_key=True)
    tenant_id: Mapped[str] = mapped_column(String(128), nullable=False)
    processed_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), nullable=False)
    pipeline_version: Mapped[int] = mapped_column(Integer, nullable=False)
