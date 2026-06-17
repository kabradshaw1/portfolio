"""Transactional idempotency store.

`mark_processed` is intended to be called inside the same transaction as the
findings upsert. A PK collision with the same `pipeline_version` raises
`DuplicateMessage` so the caller can roll back and commit the offset. A
different `pipeline_version` updates the row instead (used for intentional
reprocessing passes).
"""

from __future__ import annotations

from datetime import datetime

from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker

from app.models.db import ProcessedMessageRow


class DuplicateMessage(Exception):
    """Raised when an event_id has already been processed at the same pipeline_version."""


class IdempotencyStore:
    def __init__(self, session_factory: async_sessionmaker) -> None:
        self._session_factory = session_factory

    async def mark_processed(
        self,
        session: AsyncSession,
        *,
        event_id: str,
        tenant_id: str,
        processed_at: datetime,
        pipeline_version: int,
    ) -> None:
        existing = (
            await session.execute(
                select(ProcessedMessageRow).where(ProcessedMessageRow.event_id == event_id)
            )
        ).scalar_one_or_none()

        if existing is None:
            session.add(
                ProcessedMessageRow(
                    event_id=event_id,
                    tenant_id=tenant_id,
                    processed_at=processed_at,
                    pipeline_version=pipeline_version,
                )
            )
            return

        if existing.pipeline_version == pipeline_version:
            raise DuplicateMessage(event_id)

        existing.processed_at = processed_at
        existing.pipeline_version = pipeline_version
