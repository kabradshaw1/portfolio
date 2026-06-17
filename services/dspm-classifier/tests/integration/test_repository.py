from __future__ import annotations

from datetime import UTC, datetime

import pytest
from sqlalchemy import select

from app.db.repository import FindingRepo
from app.models.db import FindingRow
from app.models.events import Finding, Sensitivity


@pytest.mark.asyncio
async def test_upsert_inserts_then_updates(session_factory):
    repo = FindingRepo(session_factory)

    f1 = Finding(
        event_id="evt_1",
        tenant_id="acme",
        bucket="b",
        key="k1",
        sensitivity=Sensitivity.LOW,
        categories=["PII"],
        match_count=1,
        classified_at=datetime(2026, 6, 16, tzinfo=UTC),
        pipeline_version=1,
    )
    await repo.upsert(f1)

    f2 = f1.model_copy(update={"sensitivity": Sensitivity.HIGH, "match_count": 3})
    await repo.upsert(f2)

    async with session_factory() as s:
        result = await s.execute(select(FindingRow).where(FindingRow.event_id == "evt_1"))
        rows = result.scalars().all()
        assert len(rows) == 1
        assert rows[0].sensitivity == int(Sensitivity.HIGH)
        assert rows[0].match_count == 3
