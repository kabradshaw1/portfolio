"""CLI driver: classify a single S3 object end-to-end into Postgres.

Used as a smoke test for Plan 1 and as the integration seam for Plan 2's
Kafka worker (which will call `classify_one_event` directly).
"""

from __future__ import annotations

import argparse
import asyncio
import json
from datetime import UTC, datetime

from app.classifiers.llm_pass import LLMClassifier
from app.classifiers.ner_pass import PresidioClassifier
from app.classifiers.pipeline import Pipeline
from app.classifiers.regex_pass import RegexClassifier
from app.config import Settings
from app.db.repository import FindingRepo
from app.db.session import make_engine, make_session_factory
from app.idempotency import DuplicateMessage, IdempotencyStore
from app.models.events import Finding, Sensitivity
from app.storage.s3 import ObjectMissing, ObjectTooLarge, S3Fetcher


async def classify_one_event(
    *,
    event_id: str,
    tenant_id: str,
    bucket: str,
    key: str,
) -> str:
    settings = Settings()  # type: ignore[call-arg]

    engine = make_engine(settings.postgres_dsn)
    session_factory = make_session_factory(engine)

    fetcher = S3Fetcher(
        endpoint_url=settings.s3_endpoint_url,
        region=settings.s3_region,
        max_bytes=settings.max_object_bytes,
    )
    regex = RegexClassifier()
    ner = PresidioClassifier(confidence_threshold=settings.ner_confidence_threshold)
    llm = LLMClassifier(
        base_url=settings.llm_base_url,
        model=settings.llm_model,
        timeout_seconds=settings.llm_timeout_seconds,
    )
    pipeline = Pipeline(regex=regex, ner=ner, llm=llm, escalate_to_llm=settings.escalate_to_llm)

    repo = FindingRepo(session_factory)
    idempotency = IdempotencyStore(session_factory)

    now = datetime.now(UTC)
    reason: str | None = None
    try:
        content = await fetcher.fetch(bucket, key)
        result = await pipeline.run(content)
        finding = Finding(
            event_id=event_id,
            tenant_id=tenant_id,
            bucket=bucket,
            key=key,
            sensitivity=Sensitivity(result.sensitivity),
            categories=list(result.categories),
            match_count=len(result.matches),
            classified_at=now,
            pipeline_version=settings.pipeline_version,
            llm_failed=result.llm_failed,
            reason=result.reason,
        )
    except ObjectMissing:
        reason = "object_missing"
        finding = _empty_finding(
            event_id, tenant_id, bucket, key, now, settings.pipeline_version, reason
        )
    except ObjectTooLarge:
        reason = "too_large"
        finding = _empty_finding(
            event_id, tenant_id, bucket, key, now, settings.pipeline_version, reason
        )

    try:
        async with session_factory() as s, s.begin():
            await idempotency.mark_processed(
                s,
                event_id=event_id,
                tenant_id=tenant_id,
                processed_at=now,
                pipeline_version=settings.pipeline_version,
            )
        await repo.upsert(finding)
    except DuplicateMessage:
        pass
    finally:
        await llm.aclose()
        await engine.dispose()

    return json.dumps(
        {
            "event_id": finding.event_id,
            "tenant_id": finding.tenant_id,
            "sensitivity": int(finding.sensitivity),
            "categories": finding.categories,
            "match_count": finding.match_count,
            "reason": finding.reason,
        }
    )


def _empty_finding(
    event_id: str,
    tenant_id: str,
    bucket: str,
    key: str,
    now: datetime,
    pipeline_version: int,
    reason: str,
) -> Finding:
    return Finding(
        event_id=event_id,
        tenant_id=tenant_id,
        bucket=bucket,
        key=key,
        sensitivity=Sensitivity.NONE,
        categories=[],
        match_count=0,
        classified_at=now,
        pipeline_version=pipeline_version,
        llm_failed=False,
        reason=reason,
    )


def _parse_args() -> argparse.Namespace:
    p = argparse.ArgumentParser(description="Classify a single S3 object.")
    p.add_argument("--event-id", required=True)
    p.add_argument("--tenant-id", required=True)
    p.add_argument("--bucket", required=True)
    p.add_argument("--key", required=True)
    return p.parse_args()


def main() -> None:
    args = _parse_args()
    out = asyncio.run(
        classify_one_event(
            event_id=args.event_id,
            tenant_id=args.tenant_id,
            bucket=args.bucket,
            key=args.key,
        )
    )
    print(out)


if __name__ == "__main__":
    main()
