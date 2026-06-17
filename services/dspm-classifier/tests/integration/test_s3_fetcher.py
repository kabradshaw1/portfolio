from __future__ import annotations

import boto3
import pytest

from app.storage.s3 import ObjectMissing, ObjectTooLarge, S3Fetcher


@pytest.fixture
def bucket_with_object(minio_container) -> dict[str, str]:
    from botocore.exceptions import ClientError

    s3 = boto3.client(
        "s3",
        endpoint_url=minio_container["endpoint_url"],
        aws_access_key_id=minio_container["access_key"],
        aws_secret_access_key=minio_container["secret_key"],
        region_name="us-east-1",
    )
    try:
        s3.create_bucket(Bucket="acme-uploads")
    except ClientError as e:
        code = e.response.get("Error", {}).get("Code", "")
        if code not in {"BucketAlreadyOwnedByYou", "BucketAlreadyExists"}:
            raise
    s3.put_object(Bucket="acme-uploads", Key="hello.txt", Body=b"My SSN is 123-45-6789")
    s3.put_object(Bucket="acme-uploads", Key="big.bin", Body=b"x" * 2048)
    return minio_container


@pytest.mark.asyncio
async def test_fetch_returns_text(bucket_with_object):
    fetcher = S3Fetcher(
        endpoint_url=bucket_with_object["endpoint_url"],
        access_key=bucket_with_object["access_key"],
        secret_key=bucket_with_object["secret_key"],
        region="us-east-1",
        max_bytes=10_000,
    )
    text = await fetcher.fetch("acme-uploads", "hello.txt")
    assert "123-45-6789" in text


@pytest.mark.asyncio
async def test_missing_raises(bucket_with_object):
    fetcher = S3Fetcher(
        endpoint_url=bucket_with_object["endpoint_url"],
        access_key=bucket_with_object["access_key"],
        secret_key=bucket_with_object["secret_key"],
        region="us-east-1",
        max_bytes=10_000,
    )
    with pytest.raises(ObjectMissing):
        await fetcher.fetch("acme-uploads", "does-not-exist")


@pytest.mark.asyncio
async def test_too_large_raises(bucket_with_object):
    fetcher = S3Fetcher(
        endpoint_url=bucket_with_object["endpoint_url"],
        access_key=bucket_with_object["access_key"],
        secret_key=bucket_with_object["secret_key"],
        region="us-east-1",
        max_bytes=512,
    )
    with pytest.raises(ObjectTooLarge):
        await fetcher.fetch("acme-uploads", "big.bin")
