from __future__ import annotations

from datetime import UTC, datetime

import pytest
from sqlalchemy import select

from app.idempotency import DuplicateMessage, IdempotencyStore
from app.models.db import ProcessedMessageRow


@pytest.mark.asyncio
async def test_mark_processed_inserts_row(session_factory):
    store = IdempotencyStore(session_factory)
    async with session_factory() as s, s.begin():
        await store.mark_processed(
            s,
            event_id="evt_1",
            tenant_id="acme",
            processed_at=datetime(2026, 6, 16, tzinfo=UTC),
            pipeline_version=1,
        )

    async with session_factory() as s:
        rows = (await s.execute(select(ProcessedMessageRow))).scalars().all()
        assert len(rows) == 1
        assert rows[0].event_id == "evt_1"


@pytest.mark.asyncio
async def test_duplicate_event_raises(session_factory):
    store = IdempotencyStore(session_factory)
    ts = datetime(2026, 6, 16, tzinfo=UTC)
    async with session_factory() as s, s.begin():
        await store.mark_processed(
            s, event_id="evt_2", tenant_id="acme", processed_at=ts, pipeline_version=1
        )

    with pytest.raises(DuplicateMessage):
        async with session_factory() as s, s.begin():
            await store.mark_processed(
                s, event_id="evt_2", tenant_id="acme", processed_at=ts, pipeline_version=1
            )


@pytest.mark.asyncio
async def test_pipeline_version_bump_allows_reprocess(session_factory):
    store = IdempotencyStore(session_factory)
    ts = datetime(2026, 6, 16, tzinfo=UTC)
    async with session_factory() as s, s.begin():
        await store.mark_processed(
            s, event_id="evt_3", tenant_id="acme", processed_at=ts, pipeline_version=1
        )

    async with session_factory() as s, s.begin():
        await store.mark_processed(
            s, event_id="evt_3", tenant_id="acme", processed_at=ts, pipeline_version=2
        )

    async with session_factory() as s:
        rows = (
            (
                await s.execute(
                    select(ProcessedMessageRow).where(ProcessedMessageRow.event_id == "evt_3")
                )
            )
            .scalars()
            .all()
        )
        assert len(rows) == 1
        assert rows[0].pipeline_version == 2
