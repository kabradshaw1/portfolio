from __future__ import annotations

import json

import boto3
import pytest
from sqlalchemy import select

from app.models.db import FindingRow, ProcessedMessageRow
from scripts.classify_one import classify_one_event


@pytest.mark.asyncio
async def test_classifies_object_end_to_end(
    session_factory,
    postgres_dsn,
    minio_container,
    monkeypatch,
):
    s3 = boto3.client(
        "s3",
        endpoint_url=minio_container["endpoint_url"],
        aws_access_key_id=minio_container["access_key"],
        aws_secret_access_key=minio_container["secret_key"],
        region_name="us-east-1",
    )
    try:
        s3.create_bucket(Bucket="acme-uploads")
    except s3.exceptions.BucketAlreadyOwnedByYou:
        pass
    s3.put_object(Bucket="acme-uploads", Key="doc.txt", Body=b"SSN: 123-45-6789")

    monkeypatch.setenv("POSTGRES_DSN", postgres_dsn)
    monkeypatch.setenv("S3_ENDPOINT_URL", minio_container["endpoint_url"])
    monkeypatch.setenv("AWS_ACCESS_KEY_ID", minio_container["access_key"])
    monkeypatch.setenv("AWS_SECRET_ACCESS_KEY", minio_container["secret_key"])
    monkeypatch.setenv("LLM_BASE_URL", "http://unused:0")
    monkeypatch.setenv("ESCALATE_TO_LLM", "false")  # avoid the network call

    output = await classify_one_event(
        event_id="evt_cli_smoke",
        tenant_id="acme",
        bucket="acme-uploads",
        key="doc.txt",
    )

    parsed = json.loads(output)
    assert parsed["sensitivity"] >= 3  # HIGH
    assert "PII" in parsed["categories"]

    async with session_factory() as s:
        findings = (
            (await s.execute(select(FindingRow).where(FindingRow.event_id == "evt_cli_smoke")))
            .scalars()
            .all()
        )
        processed = (
            (
                await s.execute(
                    select(ProcessedMessageRow).where(
                        ProcessedMessageRow.event_id == "evt_cli_smoke"
                    )
                )
            )
            .scalars()
            .all()
        )
    assert len(findings) == 1
    assert findings[0].sensitivity == 3
    assert len(processed) == 1
