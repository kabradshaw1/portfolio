"""Async S3 fetcher backed by aioboto3.

Works against MinIO locally (set `endpoint_url`) and AWS S3 in deploy
(leave `endpoint_url=None`). Caps download size to `max_bytes` and raises
`ObjectTooLarge` if HEAD reports a larger object.
"""

from __future__ import annotations

import aioboto3
from botocore.exceptions import ClientError


class ObjectMissing(Exception):
    pass


class ObjectTooLarge(Exception):
    pass


class S3Fetcher:
    def __init__(
        self,
        *,
        endpoint_url: str | None,
        access_key: str | None = None,
        secret_key: str | None = None,
        region: str = "us-east-1",
        max_bytes: int,
    ) -> None:
        self._endpoint_url = endpoint_url
        self._access_key = access_key
        self._secret_key = secret_key
        self._region = region
        self._max_bytes = max_bytes
        self._session = aioboto3.Session()

    async def fetch(self, bucket: str, key: str) -> str:
        async with self._session.client(
            "s3",
            endpoint_url=self._endpoint_url,
            aws_access_key_id=self._access_key,
            aws_secret_access_key=self._secret_key,
            region_name=self._region,
        ) as client:
            try:
                head = await client.head_object(Bucket=bucket, Key=key)
            except ClientError as e:
                code = e.response.get("Error", {}).get("Code", "")
                if code in {"404", "NoSuchKey", "NotFound"}:
                    raise ObjectMissing(f"{bucket}/{key}") from e
                raise

            content_length = int(head.get("ContentLength", 0))
            if content_length > self._max_bytes:
                raise ObjectTooLarge(f"{bucket}/{key} = {content_length} bytes")

            obj = await client.get_object(Bucket=bucket, Key=key)
            body = await obj["Body"].read()
            return body.decode("utf-8", errors="replace")
