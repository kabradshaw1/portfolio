from __future__ import annotations

from datetime import UTC, datetime

import pytest
from pydantic import ValidationError

from app.models.events import DataEvent, Finding, Sensitivity


def test_data_event_round_trip():
    payload = {
        "event_id": "evt_01HZABC",
        "tenant_id": "acme-corp",
        "bucket": "acme-uploads",
        "key": "contracts/2026/q2.pdf",
        "size_bytes": 1234,
        "occurred_at": "2026-06-16T14:22:01Z",
    }
    e = DataEvent.model_validate(payload)
    assert e.event_id == "evt_01HZABC"
    assert e.occurred_at == datetime(2026, 6, 16, 14, 22, 1, tzinfo=UTC)


def test_data_event_rejects_negative_size():
    with pytest.raises(ValidationError):
        DataEvent(
            event_id="x",
            tenant_id="t",
            bucket="b",
            key="k",
            size_bytes=-1,
            occurred_at=datetime.now(UTC),
        )


def test_finding_with_categories():
    f = Finding(
        event_id="evt_1",
        tenant_id="acme",
        bucket="b",
        key="k",
        sensitivity=Sensitivity.HIGH,
        categories=["PII", "FINANCIAL"],
        match_count=4,
        classified_at=datetime.now(UTC),
        pipeline_version=1,
        llm_failed=False,
    )
    assert f.sensitivity is Sensitivity.HIGH
    assert "PII" in f.categories


def test_sensitivity_ordering():
    assert Sensitivity.NONE < Sensitivity.LOW < Sensitivity.MEDIUM < Sensitivity.HIGH
