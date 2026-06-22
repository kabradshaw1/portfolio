"""Integration test fixtures. Skips gracefully when Docker is unavailable."""

from __future__ import annotations

import os
import shutil
from collections.abc import AsyncIterator, Iterator

import pytest
import pytest_asyncio
from alembic import command
from alembic.config import Config
from sqlalchemy.ext.asyncio import AsyncEngine, async_sessionmaker, create_async_engine

pytestmark = pytest.mark.integration


def _docker_available() -> bool:
    return shutil.which("docker") is not None and os.environ.get("DOCKER_HOST", "") != "skip"


@pytest.fixture(scope="session")
def postgres_dsn() -> Iterator[str]:
    if not _docker_available():
        pytest.skip("Docker not available; skipping integration tests.")
    from testcontainers.postgres import PostgresContainer

    with PostgresContainer("postgres:16-alpine") as pg:
        sync_url = pg.get_connection_url()  # postgresql+psycopg2://...
        async_url = sync_url.replace("+psycopg2", "+asyncpg")
        os.environ["POSTGRES_DSN"] = async_url

        cfg = Config("alembic.ini")
        cfg.set_main_option("script_location", "migrations")
        command.upgrade(cfg, "head")

        yield async_url


@pytest_asyncio.fixture
async def engine(postgres_dsn: str) -> AsyncIterator[AsyncEngine]:
    e = create_async_engine(postgres_dsn, future=True)
    try:
        yield e
    finally:
        await e.dispose()


@pytest_asyncio.fixture
async def session_factory(engine: AsyncEngine) -> async_sessionmaker:
    return async_sessionmaker(engine, expire_on_commit=False)


@pytest.fixture(scope="session")
def minio_container() -> Iterator[dict[str, str]]:
    if not _docker_available():
        pytest.skip("Docker not available; skipping integration tests.")
    from testcontainers.minio import MinioContainer

    with MinioContainer() as minio:
        host_ip = minio.get_container_host_ip()
        port = minio.get_exposed_port(9000)
        yield {
            "endpoint_url": f"http://{host_ip}:{port}",
            "access_key": minio.access_key,
            "secret_key": minio.secret_key,
        }
