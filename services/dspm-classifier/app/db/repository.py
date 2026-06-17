"""Repository for findings."""

from __future__ import annotations

from sqlalchemy.dialects.postgresql import insert
from sqlalchemy.ext.asyncio import async_sessionmaker

from app.models.db import FindingRow
from app.models.events import Finding


class FindingRepo:
    def __init__(self, session_factory: async_sessionmaker) -> None:
        self._session_factory = session_factory

    async def upsert(self, finding: Finding) -> None:
        stmt = insert(FindingRow).values(
            tenant_id=finding.tenant_id,
            event_id=finding.event_id,
            bucket=finding.bucket,
            key=finding.key,
            sensitivity=int(finding.sensitivity),
            categories=finding.categories,
            match_count=finding.match_count,
            classified_at=finding.classified_at,
            pipeline_version=finding.pipeline_version,
            llm_failed=finding.llm_failed,
            reason=finding.reason,
        )
        stmt = stmt.on_conflict_do_update(
            index_elements=["tenant_id", "event_id"],
            set_={
                "bucket": stmt.excluded.bucket,
                "key": stmt.excluded.key,
                "sensitivity": stmt.excluded.sensitivity,
                "categories": stmt.excluded.categories,
                "match_count": stmt.excluded.match_count,
                "classified_at": stmt.excluded.classified_at,
                "pipeline_version": stmt.excluded.pipeline_version,
                "llm_failed": stmt.excluded.llm_failed,
                "reason": stmt.excluded.reason,
            },
        )
        async with self._session_factory() as s, s.begin():
            await s.execute(stmt)
