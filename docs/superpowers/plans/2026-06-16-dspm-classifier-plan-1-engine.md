# DSPM Classifier — Plan 1: Classification Engine

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Spec:** [`docs/superpowers/specs/2026-06-16-dspm-classifier-design.md`](../specs/2026-06-16-dspm-classifier-design.md)

**Goal:** Build the in-process classification engine (config, models, DB, idempotency, regex/NER/LLM classifiers, S3 fetcher) and a CLI driver that classifies a single S3 object end-to-end into Postgres. No Kafka yet.

**Architecture:** Async Python package `services/dspm-classifier/app/`. Pure-function classifiers behind a `Classifier` Protocol, orchestrated by `Pipeline`. SQLAlchemy 2 async + asyncpg with Alembic migrations. `aioboto3` fetches from MinIO locally / S3 in deploy. A small `scripts/classify_one.py` CLI wires everything for a manual smoke test and serves as the integration seam for Plan 2's Kafka worker.

**Tech Stack:** Python 3.12, pydantic v2, pydantic-settings, SQLAlchemy 2 async, asyncpg, Alembic, aioboto3, presidio-analyzer, spacy (en_core_web_sm), httpx, structlog, pytest, pytest-asyncio, testcontainers-python (Postgres + MinIO).

---

## File Structure (created in this plan)

```
services/dspm-classifier/
├── __init__.py
├── requirements.txt
├── pyproject.toml                  # ruff/mypy config, scoped to this service
├── README.md
├── alembic.ini
├── app/
│   ├── __init__.py
│   ├── config.py                   # pydantic-settings
│   ├── models/
│   │   ├── __init__.py
│   │   ├── events.py               # DataEvent, Finding, ClassificationResult (pydantic)
│   │   └── db.py                   # SQLAlchemy Base + Finding, ProcessedMessage
│   ├── db/
│   │   ├── __init__.py
│   │   ├── session.py              # async engine + sessionmaker
│   │   └── repository.py           # FindingRepo.upsert
│   ├── idempotency.py              # mark_processed (transactional)
│   ├── classifiers/
│   │   ├── __init__.py
│   │   ├── base.py                 # Classifier Protocol, Match, ClassificationResult
│   │   ├── regex_pass.py           # RegexClassifier
│   │   ├── ner_pass.py             # PresidioClassifier
│   │   ├── llm_pass.py             # LLMClassifier (httpx)
│   │   └── pipeline.py             # Pipeline.run (tiered escalation)
│   └── storage/
│       ├── __init__.py
│       └── s3.py                   # S3Fetcher (aioboto3)
├── scripts/
│   ├── __init__.py
│   └── classify_one.py             # CLI: classify one S3 object
├── migrations/
│   ├── env.py
│   ├── script.py.mako
│   └── versions/
│       └── 0001_initial.py
└── tests/
    ├── __init__.py
    ├── conftest.py                 # event_loop fixture, settings override
    ├── unit/
    │   ├── __init__.py
    │   ├── test_config.py
    │   ├── test_models_events.py
    │   ├── test_regex_pass.py
    │   ├── test_ner_pass.py
    │   ├── test_llm_pass.py
    │   └── test_pipeline.py
    └── integration/
        ├── __init__.py
        ├── conftest.py             # postgres + minio testcontainers
        ├── test_repository.py
        ├── test_idempotency.py
        ├── test_s3_fetcher.py
        └── test_classify_one_cli.py
```

---

## Task 1: Scaffold the service package

**Files:**
- Create: `services/dspm-classifier/__init__.py`
- Create: `services/dspm-classifier/app/__init__.py`
- Create: `services/dspm-classifier/requirements.txt`
- Create: `services/dspm-classifier/pyproject.toml`
- Create: `services/dspm-classifier/README.md`
- Create: `services/dspm-classifier/tests/__init__.py`
- Create: `services/dspm-classifier/tests/unit/__init__.py`
- Create: `services/dspm-classifier/tests/integration/__init__.py`

- [ ] **Step 1: Create the package skeleton**

```bash
mkdir -p services/dspm-classifier/app/{models,db,classifiers,storage}
mkdir -p services/dspm-classifier/{scripts,migrations/versions,tests/unit,tests/integration}
touch services/dspm-classifier/__init__.py
touch services/dspm-classifier/app/__init__.py
touch services/dspm-classifier/app/{models,db,classifiers,storage}/__init__.py
touch services/dspm-classifier/scripts/__init__.py
touch services/dspm-classifier/tests/__init__.py
touch services/dspm-classifier/tests/{unit,integration}/__init__.py
```

- [ ] **Step 2: Write `requirements.txt`**

Create `services/dspm-classifier/requirements.txt`:

```
pydantic==2.9.2
pydantic-settings==2.6.1
sqlalchemy[asyncio]==2.0.36
asyncpg==0.30.0
alembic==1.13.3
aioboto3==13.2.0
presidio-analyzer==2.2.355
spacy==3.7.6
httpx==0.27.2
structlog==24.4.0
prometheus-client==0.21.0

# Test
pytest==8.3.3
pytest-asyncio==0.24.0
pytest-cov==5.0.0
testcontainers[postgres,minio]==4.8.2
```

- [ ] **Step 3: Write `pyproject.toml`**

Create `services/dspm-classifier/pyproject.toml`:

```toml
[tool.ruff]
extend = "../../ruff.toml"
line-length = 100

[tool.ruff.lint]
select = ["E", "F", "I", "B", "UP", "ASYNC", "S", "RUF"]
ignore = ["S101"]  # asserts are fine in tests

[tool.ruff.lint.per-file-ignores]
"tests/**" = ["S105", "S106"]  # test fixtures can use literal secrets

[tool.mypy]
python_version = "3.12"
strict = true
plugins = ["pydantic.mypy"]
namespace_packages = true
explicit_package_bases = true

[tool.pytest.ini_options]
asyncio_mode = "auto"
testpaths = ["tests"]
addopts = "-ra --strict-markers"
markers = [
    "integration: requires Docker (postgres + minio testcontainers)",
]
```

- [ ] **Step 4: Write a minimal `README.md`**

Create `services/dspm-classifier/README.md`:

```markdown
# dspm-classifier

DSPM-flavored sensitive-data classification service. Consumes object-storage
events from Kafka, classifies content via a tiered regex/NER/LLM pipeline,
persists findings to Postgres, and emits to a downstream topic.

See: `docs/superpowers/specs/2026-06-16-dspm-classifier-design.md`.

## Status

Plan 1 (in progress) — classification engine + CLI driver. Kafka and FastAPI
arrive in Plans 2 and 3.

## Local development

Requires Docker (Postgres + MinIO via testcontainers for tests):

```bash
make preflight-python  # runs from repo root
```
```

- [ ] **Step 5: Commit**

```bash
git add services/dspm-classifier
git commit -m "feat(dspm-classifier): scaffold service package"
```

---

## Task 2: Settings (pydantic-settings)

**Files:**
- Create: `services/dspm-classifier/app/config.py`
- Test: `services/dspm-classifier/tests/unit/test_config.py`

- [ ] **Step 1: Write the failing test**

Create `services/dspm-classifier/tests/unit/test_config.py`:

```python
from __future__ import annotations

from app.config import Settings


def test_defaults_load_with_required_env(monkeypatch):
    monkeypatch.setenv("POSTGRES_DSN", "postgresql+asyncpg://u:p@h/db")
    monkeypatch.setenv("LLM_BASE_URL", "http://mock-ollama:11434")

    s = Settings()

    assert s.postgres_dsn == "postgresql+asyncpg://u:p@h/db"
    assert s.llm_base_url == "http://mock-ollama:11434"
    assert s.max_object_bytes == 10 * 1024 * 1024
    assert s.llm_concurrency == 8
    assert s.escalate_to_llm is True
    assert s.pipeline_version == 1
    assert s.s3_endpoint_url is None  # real AWS by default
    assert s.ner_confidence_threshold == 0.6


def test_env_overrides(monkeypatch):
    monkeypatch.setenv("POSTGRES_DSN", "postgresql+asyncpg://u:p@h/db")
    monkeypatch.setenv("LLM_BASE_URL", "http://x:1")
    monkeypatch.setenv("MAX_OBJECT_BYTES", "1024")
    monkeypatch.setenv("LLM_CONCURRENCY", "2")
    monkeypatch.setenv("ESCALATE_TO_LLM", "false")
    monkeypatch.setenv("S3_ENDPOINT_URL", "http://minio:9000")
    monkeypatch.setenv("PIPELINE_VERSION", "3")

    s = Settings()

    assert s.max_object_bytes == 1024
    assert s.llm_concurrency == 2
    assert s.escalate_to_llm is False
    assert s.s3_endpoint_url == "http://minio:9000"
    assert s.pipeline_version == 3
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd services/dspm-classifier && pytest tests/unit/test_config.py -v
```

Expected: `ModuleNotFoundError: No module named 'app.config'`.

- [ ] **Step 3: Implement `Settings`**

Create `services/dspm-classifier/app/config.py`:

```python
"""Environment-driven settings for the dspm-classifier service."""

from __future__ import annotations

from pydantic_settings import BaseSettings, SettingsConfigDict


class Settings(BaseSettings):
    model_config = SettingsConfigDict(env_file=None, case_sensitive=False)

    postgres_dsn: str

    s3_endpoint_url: str | None = None
    s3_region: str = "us-east-1"

    llm_base_url: str
    llm_model: str = "llama3.1:8b"
    llm_timeout_s: float = 15.0
    llm_concurrency: int = 8

    max_object_bytes: int = 10 * 1024 * 1024
    pipeline_version: int = 1
    ner_confidence_threshold: float = 0.6
    escalate_to_llm: bool = True

    log_level: str = "INFO"
    metrics_port: int = 9100
```

- [ ] **Step 4: Run tests, verify pass**

```bash
pytest tests/unit/test_config.py -v
```

Expected: 2 passed.

- [ ] **Step 5: Commit**

```bash
git add services/dspm-classifier/app/config.py services/dspm-classifier/tests/unit/test_config.py
git commit -m "feat(dspm-classifier): env-driven Settings"
```

---

## Task 3: Pydantic event/finding models

**Files:**
- Create: `services/dspm-classifier/app/models/events.py`
- Test: `services/dspm-classifier/tests/unit/test_models_events.py`

- [ ] **Step 1: Write the failing test**

Create `services/dspm-classifier/tests/unit/test_models_events.py`:

```python
from __future__ import annotations

from datetime import datetime, timezone

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
    assert e.occurred_at == datetime(2026, 6, 16, 14, 22, 1, tzinfo=timezone.utc)


def test_data_event_rejects_negative_size():
    with pytest.raises(ValidationError):
        DataEvent(
            event_id="x",
            tenant_id="t",
            bucket="b",
            key="k",
            size_bytes=-1,
            occurred_at=datetime.now(timezone.utc),
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
        classified_at=datetime.now(timezone.utc),
        pipeline_version=1,
        llm_failed=False,
    )
    assert f.sensitivity is Sensitivity.HIGH
    assert "PII" in f.categories


def test_sensitivity_ordering():
    assert Sensitivity.NONE < Sensitivity.LOW < Sensitivity.MEDIUM < Sensitivity.HIGH
```

- [ ] **Step 2: Run test to verify it fails**

```bash
pytest tests/unit/test_models_events.py -v
```

Expected: import error for `app.models.events`.

- [ ] **Step 3: Implement models**

Create `services/dspm-classifier/app/models/events.py`:

```python
"""Wire-format pydantic models for events and findings."""

from __future__ import annotations

from datetime import datetime
from enum import IntEnum

from pydantic import BaseModel, ConfigDict, Field


class Sensitivity(IntEnum):
    NONE = 0
    LOW = 1
    MEDIUM = 2
    HIGH = 3


class DataEvent(BaseModel):
    model_config = ConfigDict(frozen=True)

    event_id: str
    tenant_id: str
    bucket: str
    key: str
    size_bytes: int = Field(ge=0)
    occurred_at: datetime


class Finding(BaseModel):
    model_config = ConfigDict(frozen=False)

    event_id: str
    tenant_id: str
    bucket: str
    key: str
    sensitivity: Sensitivity
    categories: list[str] = Field(default_factory=list)
    match_count: int = Field(ge=0)
    classified_at: datetime
    pipeline_version: int
    llm_failed: bool = False
    reason: str | None = None  # e.g. "too_large", "object_missing"
```

- [ ] **Step 4: Run tests, verify pass**

```bash
pytest tests/unit/test_models_events.py -v
```

Expected: 4 passed.

- [ ] **Step 5: Commit**

```bash
git add services/dspm-classifier/app/models services/dspm-classifier/tests/unit/test_models_events.py
git commit -m "feat(dspm-classifier): DataEvent, Finding, Sensitivity models"
```

---

## Task 4: SQLAlchemy DB models + Alembic initial migration

**Files:**
- Create: `services/dspm-classifier/app/models/db.py`
- Create: `services/dspm-classifier/alembic.ini`
- Create: `services/dspm-classifier/migrations/env.py`
- Create: `services/dspm-classifier/migrations/script.py.mako`
- Create: `services/dspm-classifier/migrations/versions/0001_initial.py`

- [ ] **Step 1: Implement SQLAlchemy models**

Create `services/dspm-classifier/app/models/db.py`:

```python
"""SQLAlchemy models for findings and processed-messages."""

from __future__ import annotations

from datetime import datetime

from sqlalchemy import ARRAY, Boolean, DateTime, Index, Integer, String, Text
from sqlalchemy.orm import DeclarativeBase, Mapped, mapped_column


class Base(DeclarativeBase):
    pass


class FindingRow(Base):
    __tablename__ = "findings"

    tenant_id: Mapped[str] = mapped_column(String(128), primary_key=True)
    event_id: Mapped[str] = mapped_column(String(128), primary_key=True)
    bucket: Mapped[str] = mapped_column(String(255), nullable=False)
    key: Mapped[str] = mapped_column(Text, nullable=False)
    sensitivity: Mapped[int] = mapped_column(Integer, nullable=False)
    categories: Mapped[list[str]] = mapped_column(ARRAY(String), nullable=False, default=list)
    match_count: Mapped[int] = mapped_column(Integer, nullable=False, default=0)
    classified_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), nullable=False)
    pipeline_version: Mapped[int] = mapped_column(Integer, nullable=False)
    llm_failed: Mapped[bool] = mapped_column(Boolean, nullable=False, default=False)
    reason: Mapped[str | None] = mapped_column(Text, nullable=True)

    __table_args__ = (
        Index(
            "ix_findings_tenant_sensitivity_time",
            "tenant_id",
            "sensitivity",
            "classified_at",
        ),
    )


class ProcessedMessageRow(Base):
    __tablename__ = "processed_messages"

    event_id: Mapped[str] = mapped_column(String(128), primary_key=True)
    tenant_id: Mapped[str] = mapped_column(String(128), nullable=False)
    processed_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), nullable=False)
    pipeline_version: Mapped[int] = mapped_column(Integer, nullable=False)
```

- [ ] **Step 2: Create Alembic config**

Create `services/dspm-classifier/alembic.ini`:

```ini
[alembic]
script_location = migrations
prepend_sys_path = .
sqlalchemy.url =

[loggers]
keys = root,sqlalchemy,alembic

[handlers]
keys = console

[formatters]
keys = generic

[logger_root]
level = WARNING
handlers = console
qualname =

[logger_sqlalchemy]
level = WARNING
handlers =
qualname = sqlalchemy.engine

[logger_alembic]
level = INFO
handlers =
qualname = alembic

[handler_console]
class = StreamHandler
args = (sys.stderr,)
level = NOTSET
formatter = generic

[formatter_generic]
format = %(levelname)-5.5s [%(name)s] %(message)s
datefmt = %H:%M:%S
```

- [ ] **Step 3: Create migration env**

Create `services/dspm-classifier/migrations/env.py`:

```python
"""Alembic env. Uses sync URL derived from POSTGRES_DSN."""

from __future__ import annotations

import os
from logging.config import fileConfig

from alembic import context
from sqlalchemy import engine_from_config, pool

from app.models.db import Base

config = context.config
if config.config_file_name:
    fileConfig(config.config_file_name)

target_metadata = Base.metadata

DSN = os.environ.get("POSTGRES_DSN", "")
SYNC_DSN = DSN.replace("+asyncpg", "+psycopg2") if DSN else ""
config.set_main_option("sqlalchemy.url", SYNC_DSN)


def run_migrations_offline() -> None:
    context.configure(url=SYNC_DSN, target_metadata=target_metadata, literal_binds=True)
    with context.begin_transaction():
        context.run_migrations()


def run_migrations_online() -> None:
    connectable = engine_from_config(
        config.get_section(config.config_ini_section) or {},
        prefix="sqlalchemy.",
        poolclass=pool.NullPool,
    )
    with connectable.connect() as connection:
        context.configure(connection=connection, target_metadata=target_metadata)
        with context.begin_transaction():
            context.run_migrations()


if context.is_offline_mode():
    run_migrations_offline()
else:
    run_migrations_online()
```

- [ ] **Step 4: Create migration script template**

Create `services/dspm-classifier/migrations/script.py.mako`:

```python
"""${message}

Revision ID: ${up_revision}
Revises: ${down_revision | comma,n}
Create Date: ${create_date}

"""
from __future__ import annotations

from alembic import op
import sqlalchemy as sa
${imports if imports else ""}

revision = ${repr(up_revision)}
down_revision = ${repr(down_revision)}
branch_labels = ${repr(branch_labels)}
depends_on = ${repr(depends_on)}


def upgrade() -> None:
    ${upgrades if upgrades else "pass"}


def downgrade() -> None:
    ${downgrades if downgrades else "pass"}
```

- [ ] **Step 5: Write the initial migration**

Create `services/dspm-classifier/migrations/versions/0001_initial.py`:

```python
"""initial findings + processed_messages tables.

Revision ID: 0001
Revises:
Create Date: 2026-06-16
"""
from __future__ import annotations

import sqlalchemy as sa
from alembic import op

revision = "0001"
down_revision = None
branch_labels = None
depends_on = None


def upgrade() -> None:
    op.create_table(
        "findings",
        sa.Column("tenant_id", sa.String(128), primary_key=True),
        sa.Column("event_id", sa.String(128), primary_key=True),
        sa.Column("bucket", sa.String(255), nullable=False),
        sa.Column("key", sa.Text, nullable=False),
        sa.Column("sensitivity", sa.Integer, nullable=False),
        sa.Column("categories", sa.ARRAY(sa.String), nullable=False, server_default="{}"),
        sa.Column("match_count", sa.Integer, nullable=False, server_default="0"),
        sa.Column("classified_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("pipeline_version", sa.Integer, nullable=False),
        sa.Column("llm_failed", sa.Boolean, nullable=False, server_default=sa.false()),
        sa.Column("reason", sa.Text, nullable=True),
    )
    op.create_index(
        "ix_findings_tenant_sensitivity_time",
        "findings",
        ["tenant_id", "sensitivity", "classified_at"],
    )

    op.create_table(
        "processed_messages",
        sa.Column("event_id", sa.String(128), primary_key=True),
        sa.Column("tenant_id", sa.String(128), nullable=False),
        sa.Column("processed_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("pipeline_version", sa.Integer, nullable=False),
    )


def downgrade() -> None:
    op.drop_table("processed_messages")
    op.drop_index("ix_findings_tenant_sensitivity_time", table_name="findings")
    op.drop_table("findings")
```

- [ ] **Step 6: Commit**

```bash
git add services/dspm-classifier/app/models/db.py services/dspm-classifier/alembic.ini services/dspm-classifier/migrations
git commit -m "feat(dspm-classifier): db models + alembic initial migration"
```

---

## Task 5: Integration-test conftest (Postgres + MinIO testcontainers)

**Files:**
- Create: `services/dspm-classifier/tests/conftest.py`
- Create: `services/dspm-classifier/tests/integration/conftest.py`

- [ ] **Step 1: Root conftest**

Create `services/dspm-classifier/tests/conftest.py`:

```python
"""Top-level pytest config. Adds the service root to sys.path for `app` imports."""

from __future__ import annotations

import sys
from pathlib import Path

SERVICE_ROOT = Path(__file__).resolve().parents[1]
if str(SERVICE_ROOT) not in sys.path:
    sys.path.insert(0, str(SERVICE_ROOT))
```

- [ ] **Step 2: Integration conftest (Docker-gated)**

Create `services/dspm-classifier/tests/integration/conftest.py`:

```python
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


pytest_plugins: list[str] = []


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
```

- [ ] **Step 3: Commit**

```bash
git add services/dspm-classifier/tests
git commit -m "test(dspm-classifier): testcontainers conftest for postgres + minio"
```

---

## Task 6: DB session factory

**Files:**
- Create: `services/dspm-classifier/app/db/session.py`

- [ ] **Step 1: Implement the session factory**

Create `services/dspm-classifier/app/db/session.py`:

```python
"""Async SQLAlchemy engine + sessionmaker factory."""

from __future__ import annotations

from sqlalchemy.ext.asyncio import AsyncEngine, async_sessionmaker, create_async_engine


def make_engine(dsn: str) -> AsyncEngine:
    return create_async_engine(dsn, future=True, pool_pre_ping=True)


def make_session_factory(engine: AsyncEngine) -> async_sessionmaker:
    return async_sessionmaker(engine, expire_on_commit=False)
```

- [ ] **Step 2: Commit (covered by Task 7's integration tests)**

```bash
git add services/dspm-classifier/app/db/session.py
git commit -m "feat(dspm-classifier): async DB session factory"
```

---

## Task 7: `FindingRepo.upsert` (integration test)

**Files:**
- Create: `services/dspm-classifier/app/db/repository.py`
- Test: `services/dspm-classifier/tests/integration/test_repository.py`

- [ ] **Step 1: Write the failing test**

Create `services/dspm-classifier/tests/integration/test_repository.py`:

```python
from __future__ import annotations

from datetime import datetime, timezone

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
        classified_at=datetime(2026, 6, 16, tzinfo=timezone.utc),
        pipeline_version=1,
    )
    await repo.upsert(f1)

    f2 = f1.model_copy(update={"sensitivity": Sensitivity.HIGH, "match_count": 3})
    await repo.upsert(f2)

    async with session_factory() as s:
        rows = (await s.execute(select(FindingRow).where(FindingRow.event_id == "evt_1"))).scalars().all()
        assert len(rows) == 1
        assert rows[0].sensitivity == int(Sensitivity.HIGH)
        assert rows[0].match_count == 3
```

- [ ] **Step 2: Run test, verify it fails**

```bash
pytest tests/integration/test_repository.py -v
```

Expected: `ModuleNotFoundError: No module named 'app.db.repository'` (or skip if Docker unavailable).

- [ ] **Step 3: Implement `FindingRepo`**

Create `services/dspm-classifier/app/db/repository.py`:

```python
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
```

- [ ] **Step 4: Run tests, verify pass**

```bash
pytest tests/integration/test_repository.py -v
```

Expected: 1 passed (or skipped if no Docker).

- [ ] **Step 5: Commit**

```bash
git add services/dspm-classifier/app/db/repository.py services/dspm-classifier/tests/integration/test_repository.py
git commit -m "feat(dspm-classifier): FindingRepo.upsert with on_conflict_do_update"
```

---

## Task 8: Idempotency helper

**Files:**
- Create: `services/dspm-classifier/app/idempotency.py`
- Test: `services/dspm-classifier/tests/integration/test_idempotency.py`

- [ ] **Step 1: Write the failing test**

Create `services/dspm-classifier/tests/integration/test_idempotency.py`:

```python
from __future__ import annotations

from datetime import datetime, timezone

import pytest
from sqlalchemy import select

from app.idempotency import IdempotencyStore, DuplicateMessage
from app.models.db import ProcessedMessageRow


@pytest.mark.asyncio
async def test_mark_processed_inserts_row(session_factory):
    store = IdempotencyStore(session_factory)
    async with session_factory() as s, s.begin():
        await store.mark_processed(s, event_id="evt_1", tenant_id="acme",
                                   processed_at=datetime(2026, 6, 16, tzinfo=timezone.utc),
                                   pipeline_version=1)

    async with session_factory() as s:
        rows = (await s.execute(select(ProcessedMessageRow))).scalars().all()
        assert len(rows) == 1
        assert rows[0].event_id == "evt_1"


@pytest.mark.asyncio
async def test_duplicate_event_raises(session_factory):
    store = IdempotencyStore(session_factory)
    ts = datetime(2026, 6, 16, tzinfo=timezone.utc)
    async with session_factory() as s, s.begin():
        await store.mark_processed(s, event_id="evt_2", tenant_id="acme",
                                   processed_at=ts, pipeline_version=1)

    with pytest.raises(DuplicateMessage):
        async with session_factory() as s, s.begin():
            await store.mark_processed(s, event_id="evt_2", tenant_id="acme",
                                       processed_at=ts, pipeline_version=1)


@pytest.mark.asyncio
async def test_pipeline_version_bump_allows_reprocess(session_factory):
    store = IdempotencyStore(session_factory)
    ts = datetime(2026, 6, 16, tzinfo=timezone.utc)
    async with session_factory() as s, s.begin():
        await store.mark_processed(s, event_id="evt_3", tenant_id="acme",
                                   processed_at=ts, pipeline_version=1)

    # Different pipeline version → allowed (treated as a fresh classification).
    async with session_factory() as s, s.begin():
        await store.mark_processed(s, event_id="evt_3", tenant_id="acme",
                                   processed_at=ts, pipeline_version=2)

    async with session_factory() as s:
        rows = (await s.execute(select(ProcessedMessageRow).where(
            ProcessedMessageRow.event_id == "evt_3"))).scalars().all()
        assert len(rows) == 1
        assert rows[0].pipeline_version == 2
```

- [ ] **Step 2: Run test, verify it fails**

```bash
pytest tests/integration/test_idempotency.py -v
```

Expected: import error.

- [ ] **Step 3: Implement `IdempotencyStore`**

Create `services/dspm-classifier/app/idempotency.py`:

```python
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
```

- [ ] **Step 4: Run tests, verify pass**

```bash
pytest tests/integration/test_idempotency.py -v
```

Expected: 3 passed.

- [ ] **Step 5: Commit**

```bash
git add services/dspm-classifier/app/idempotency.py services/dspm-classifier/tests/integration/test_idempotency.py
git commit -m "feat(dspm-classifier): transactional idempotency store"
```

---

## Task 9: Classifier protocol + `Match` / `ClassificationResult`

**Files:**
- Create: `services/dspm-classifier/app/classifiers/base.py`

- [ ] **Step 1: Implement the Protocol + dataclasses**

Create `services/dspm-classifier/app/classifiers/base.py`:

```python
"""Classifier Protocol and shared dataclasses."""

from __future__ import annotations

from dataclasses import dataclass, field
from typing import Protocol


@dataclass(frozen=True)
class Match:
    category: str  # e.g. "PII.SSN", "SECRETS.AWS_KEY", "PII.EMAIL"
    span: tuple[int, int]  # (start, end) char offsets
    text: str
    confidence: float  # 0..1


@dataclass(frozen=True)
class ClassificationResult:
    matches: tuple[Match, ...] = ()
    confidence: float = 1.0  # classifier's confidence in its own output
    needs_escalation: bool = False

    @property
    def has_matches(self) -> bool:
        return len(self.matches) > 0


@dataclass(frozen=True)
class PipelineOutput:
    matches: tuple[Match, ...]
    categories: tuple[str, ...]
    sensitivity: int  # Sensitivity int value
    llm_failed: bool = False
    reason: str | None = None
    stages_run: tuple[str, ...] = field(default_factory=tuple)


class Classifier(Protocol):
    async def classify(self, content: str) -> ClassificationResult: ...
```

- [ ] **Step 2: Commit (no tests yet — covered by classifier tests)**

```bash
git add services/dspm-classifier/app/classifiers/base.py
git commit -m "feat(dspm-classifier): Classifier Protocol + result dataclasses"
```

---

## Task 10: Regex classifier

**Files:**
- Create: `services/dspm-classifier/app/classifiers/regex_pass.py`
- Test: `services/dspm-classifier/tests/unit/test_regex_pass.py`

- [ ] **Step 1: Write the failing test**

Create `services/dspm-classifier/tests/unit/test_regex_pass.py`:

```python
from __future__ import annotations

import pytest

from app.classifiers.regex_pass import RegexClassifier


@pytest.mark.asyncio
async def test_detects_ssn():
    c = RegexClassifier()
    r = await c.classify("My SSN is 123-45-6789.")
    cats = {m.category for m in r.matches}
    assert "PII.SSN" in cats


@pytest.mark.asyncio
async def test_rejects_ssn_shaped_but_invalid():
    # SSN cannot start with 000, 666, or 9.
    c = RegexClassifier()
    r = await c.classify("ID number 000-12-3456 is not an SSN.")
    assert all(m.category != "PII.SSN" for m in r.matches)


@pytest.mark.asyncio
async def test_detects_credit_card_with_luhn():
    # Test Visa, passes Luhn.
    c = RegexClassifier()
    r = await c.classify("Card 4111 1111 1111 1111 declined.")
    cats = {m.category for m in r.matches}
    assert "FINANCIAL.CREDIT_CARD" in cats


@pytest.mark.asyncio
async def test_rejects_credit_card_failing_luhn():
    c = RegexClassifier()
    r = await c.classify("Card 4111 1111 1111 1112 invalid.")  # last digit changed
    assert all(m.category != "FINANCIAL.CREDIT_CARD" for m in r.matches)


@pytest.mark.asyncio
async def test_detects_jwt():
    jwt = "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxIn0.dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
    c = RegexClassifier()
    r = await c.classify(f"Authorization: Bearer {jwt}")
    assert any(m.category == "SECRETS.JWT" for m in r.matches)


@pytest.mark.asyncio
async def test_detects_aws_access_key():
    c = RegexClassifier()
    r = await c.classify("AKIAIOSFODNN7EXAMPLE is the prod key.")
    assert any(m.category == "SECRETS.AWS_KEY" for m in r.matches)


@pytest.mark.asyncio
async def test_no_matches_on_clean_text():
    c = RegexClassifier()
    r = await c.classify("This is an ordinary paragraph about nothing in particular.")
    assert r.matches == ()
    assert r.needs_escalation is False
```

- [ ] **Step 2: Run test, verify fail**

```bash
pytest tests/unit/test_regex_pass.py -v
```

Expected: import error.

- [ ] **Step 3: Implement `RegexClassifier`**

Create `services/dspm-classifier/app/classifiers/regex_pass.py`:

```python
"""Deterministic regex-based detectors for high-precision categories."""

from __future__ import annotations

import re

from app.classifiers.base import ClassificationResult, Match

# --- Patterns ----------------------------------------------------------------

_SSN_RE = re.compile(r"\b(?!000|666|9\d{2})\d{3}-(?!00)\d{2}-(?!0000)\d{4}\b")
_CC_RE = re.compile(r"\b(?:\d[ -]?){13,19}\b")
_JWT_RE = re.compile(r"\beyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\b")
_AWS_KEY_RE = re.compile(r"\b(AKIA|ASIA)[0-9A-Z]{16}\b")
_GENERIC_API_KEY_RE = re.compile(
    r"(?i)(?:api[_-]?key|secret|token)[\"'\s:=]+([A-Za-z0-9_\-]{24,})"
)


def _luhn_ok(digits: str) -> bool:
    total = 0
    for i, ch in enumerate(reversed(digits)):
        n = int(ch)
        if i % 2 == 1:
            n *= 2
            if n > 9:
                n -= 9
        total += n
    return total % 10 == 0


class RegexClassifier:
    """Implements Classifier."""

    async def classify(self, content: str) -> ClassificationResult:
        matches: list[Match] = []

        for m in _SSN_RE.finditer(content):
            matches.append(Match("PII.SSN", m.span(), m.group(), 1.0))

        for m in _CC_RE.finditer(content):
            digits = re.sub(r"[ -]", "", m.group())
            if 13 <= len(digits) <= 19 and _luhn_ok(digits):
                matches.append(Match("FINANCIAL.CREDIT_CARD", m.span(), m.group(), 1.0))

        for m in _JWT_RE.finditer(content):
            matches.append(Match("SECRETS.JWT", m.span(), m.group(), 0.95))

        for m in _AWS_KEY_RE.finditer(content):
            matches.append(Match("SECRETS.AWS_KEY", m.span(), m.group(), 1.0))

        for m in _GENERIC_API_KEY_RE.finditer(content):
            matches.append(Match("SECRETS.GENERIC", m.span(1), m.group(1), 0.7))

        # High-confidence regex hits never escalate further.
        return ClassificationResult(
            matches=tuple(matches),
            confidence=1.0,
            needs_escalation=False,
        )
```

`RegexClassifier` satisfies the `Classifier` Protocol structurally — mypy will
verify that at usage sites (e.g., when passed to `Pipeline(regex=...)`).

- [ ] **Step 4: Run tests, verify pass**

```bash
pytest tests/unit/test_regex_pass.py -v
```

Expected: 7 passed.

- [ ] **Step 5: Commit**

```bash
git add services/dspm-classifier/app/classifiers/regex_pass.py services/dspm-classifier/tests/unit/test_regex_pass.py
git commit -m "feat(dspm-classifier): regex classifier (SSN, CC+Luhn, JWT, AWS, generic API key)"
```

---

## Task 11: NER classifier (Presidio)

**Files:**
- Create: `services/dspm-classifier/app/classifiers/ner_pass.py`
- Test: `services/dspm-classifier/tests/unit/test_ner_pass.py`

- [ ] **Step 1: Write the failing test**

Create `services/dspm-classifier/tests/unit/test_ner_pass.py`:

```python
from __future__ import annotations

import pytest

from app.classifiers.ner_pass import PresidioClassifier


@pytest.fixture(scope="module")
def classifier() -> PresidioClassifier:
    return PresidioClassifier(confidence_threshold=0.4)


@pytest.mark.asyncio
async def test_detects_email_and_phone(classifier):
    r = await classifier.classify("Contact Alice at alice@example.com or 415-555-1212.")
    cats = {m.category for m in r.matches}
    assert "PII.EMAIL" in cats
    assert "PII.PHONE" in cats


@pytest.mark.asyncio
async def test_low_confidence_flags_escalation(classifier):
    # Ambiguous: 'John' alone may produce a low-confidence PERSON match.
    r = await classifier.classify("John was here.")
    assert r.needs_escalation is True or r.matches == ()


@pytest.mark.asyncio
async def test_clean_text_no_matches(classifier):
    r = await classifier.classify("The weather report is unremarkable today.")
    assert r.matches == ()
```

- [ ] **Step 2: Run test, verify fail**

```bash
pytest tests/unit/test_ner_pass.py -v
```

Expected: import error.

- [ ] **Step 3: Implement `PresidioClassifier`**

Create `services/dspm-classifier/app/classifiers/ner_pass.py`:

```python
"""Presidio + spaCy NER pass.

Maps Presidio entity types to our taxonomy. Sets `needs_escalation=True` when
any returned entity confidence falls below `confidence_threshold`, signalling
the pipeline to consult the LLM tier.
"""

from __future__ import annotations

import asyncio
from typing import ClassVar

from presidio_analyzer import AnalyzerEngine

from app.classifiers.base import ClassificationResult, Match


_PRESIDIO_TO_CATEGORY: dict[str, str] = {
    "EMAIL_ADDRESS": "PII.EMAIL",
    "PHONE_NUMBER": "PII.PHONE",
    "PERSON": "PII.PERSON",
    "LOCATION": "PII.LOCATION",
    "IP_ADDRESS": "PII.IP",
    "CREDIT_CARD": "FINANCIAL.CREDIT_CARD",
    "US_SSN": "PII.SSN",
    "IBAN_CODE": "FINANCIAL.IBAN",
    "MEDICAL_LICENSE": "HEALTH.MEDICAL_LICENSE",
}


class PresidioClassifier:
    """Implements Classifier."""

    _SHARED_ENGINE: ClassVar[AnalyzerEngine | None] = None

    def __init__(self, *, confidence_threshold: float = 0.6, language: str = "en") -> None:
        if PresidioClassifier._SHARED_ENGINE is None:
            PresidioClassifier._SHARED_ENGINE = AnalyzerEngine()
        self._engine = PresidioClassifier._SHARED_ENGINE
        self._threshold = confidence_threshold
        self._language = language

    async def classify(self, content: str) -> ClassificationResult:
        results = await asyncio.to_thread(
            self._engine.analyze, text=content, language=self._language
        )

        matches: list[Match] = []
        low_conf = False
        for r in results:
            category = _PRESIDIO_TO_CATEGORY.get(r.entity_type, f"OTHER.{r.entity_type}")
            matches.append(
                Match(
                    category=category,
                    span=(r.start, r.end),
                    text=content[r.start : r.end],
                    confidence=float(r.score),
                )
            )
            if r.score < self._threshold:
                low_conf = True

        return ClassificationResult(
            matches=tuple(matches),
            confidence=min((m.confidence for m in matches), default=1.0),
            needs_escalation=low_conf,
        )
```

- [ ] **Step 4: Run tests, verify pass**

```bash
python -m spacy download en_core_web_sm   # first-time only
pytest tests/unit/test_ner_pass.py -v
```

Expected: 3 passed.

- [ ] **Step 5: Add spaCy model download to README**

Append to `services/dspm-classifier/README.md`:

```markdown
## First-time setup

```bash
pip install -r requirements.txt
python -m spacy download en_core_web_sm
```
```

- [ ] **Step 6: Commit**

```bash
git add services/dspm-classifier/app/classifiers/ner_pass.py services/dspm-classifier/tests/unit/test_ner_pass.py services/dspm-classifier/README.md
git commit -m "feat(dspm-classifier): Presidio NER classifier"
```

---

## Task 12: LLM classifier (httpx + structured output)

**Files:**
- Create: `services/dspm-classifier/app/classifiers/llm_pass.py`
- Test: `services/dspm-classifier/tests/unit/test_llm_pass.py`

- [ ] **Step 1: Write the failing test**

Create `services/dspm-classifier/tests/unit/test_llm_pass.py`:

```python
from __future__ import annotations

import json

import httpx
import pytest

from app.classifiers.llm_pass import LLMClassifier, LLMUnavailable


def _ok_response(payload: dict) -> httpx.Response:
    body = {"response": json.dumps(payload)}
    return httpx.Response(200, json=body)


@pytest.mark.asyncio
async def test_happy_path_returns_matches():
    payload = {
        "category": "PII",
        "sensitivity": "high",
        "reasoning": "Names + address",
        "entities": [{"type": "PII.PERSON", "text": "Jane Doe", "confidence": 0.9}],
    }

    def handler(req: httpx.Request) -> httpx.Response:
        return _ok_response(payload)

    transport = httpx.MockTransport(handler)
    c = LLMClassifier(base_url="http://x", model="m", transport=transport)

    r = await c.classify("Jane Doe lives at 1 Main St.")
    assert any(m.category == "PII.PERSON" for m in r.matches)
    assert r.confidence == pytest.approx(0.9)


@pytest.mark.asyncio
async def test_malformed_json_retries_then_gives_up():
    calls = {"n": 0}

    def handler(req: httpx.Request) -> httpx.Response:
        calls["n"] += 1
        return httpx.Response(200, json={"response": "not json at all"})

    transport = httpx.MockTransport(handler)
    c = LLMClassifier(base_url="http://x", model="m", transport=transport, max_parse_retries=2)

    with pytest.raises(LLMUnavailable):
        await c.classify("anything")
    assert calls["n"] == 3  # initial + 2 retries


@pytest.mark.asyncio
async def test_5xx_raises_unavailable():
    def handler(req: httpx.Request) -> httpx.Response:
        return httpx.Response(503, text="Service Unavailable")

    transport = httpx.MockTransport(handler)
    c = LLMClassifier(base_url="http://x", model="m", transport=transport)

    with pytest.raises(LLMUnavailable):
        await c.classify("anything")
```

- [ ] **Step 2: Run test, verify fail**

```bash
pytest tests/unit/test_llm_pass.py -v
```

Expected: import error.

- [ ] **Step 3: Implement `LLMClassifier`**

Create `services/dspm-classifier/app/classifiers/llm_pass.py`:

```python
"""LLM classifier targeting an Ollama-compatible JSON endpoint.

Asks the model for a structured JSON object with category, sensitivity, and a
list of entities. Re-prompts up to `max_parse_retries` times on malformed JSON,
then raises `LLMUnavailable` so the pipeline can persist a partial finding.
"""

from __future__ import annotations

import json
from typing import Any

import httpx

from app.classifiers.base import ClassificationResult, Match


_PROMPT_TEMPLATE = (
    "You are a sensitive-data classifier. Read the CONTENT and return a SINGLE "
    "JSON object with this exact shape and no prose:\n"
    '{"category": "PII|FINANCIAL|HEALTH|SECRETS|NONE", '
    '"sensitivity": "none|low|medium|high", '
    '"reasoning": "...", '
    '"entities": [{"type": "PII.PERSON|PII.EMAIL|...", "text": "...", "confidence": 0-1}]}\n'
    "CONTENT:\n{content}\n"
)


_SENSITIVITY_TO_CONF = {"none": 0.0, "low": 0.4, "medium": 0.7, "high": 0.95}


class LLMUnavailable(Exception):
    """Raised when the LLM returns 5xx, times out, or yields unparseable output."""


class LLMClassifier:
    def __init__(
        self,
        *,
        base_url: str,
        model: str,
        timeout_s: float = 15.0,
        max_parse_retries: int = 2,
        transport: httpx.BaseTransport | None = None,
    ) -> None:
        self._url = f"{base_url.rstrip('/')}/api/generate"
        self._model = model
        self._client = httpx.AsyncClient(timeout=timeout_s, transport=transport)
        self._max_parse_retries = max_parse_retries

    async def classify(self, content: str) -> ClassificationResult:
        last_error: Exception | None = None
        for _ in range(self._max_parse_retries + 1):
            try:
                resp = await self._client.post(
                    self._url,
                    json={
                        "model": self._model,
                        "prompt": _PROMPT_TEMPLATE.format(content=content[:8000]),
                        "stream": False,
                        "format": "json",
                    },
                )
            except httpx.HTTPError as e:
                raise LLMUnavailable(str(e)) from e

            if resp.status_code >= 500 or resp.status_code == 429:
                raise LLMUnavailable(f"upstream {resp.status_code}")
            resp.raise_for_status()

            try:
                payload = self._parse(resp.json())
            except (ValueError, KeyError, TypeError) as e:
                last_error = e
                continue
            return _to_result(payload)

        raise LLMUnavailable(f"unparseable output after retries: {last_error}")

    @staticmethod
    def _parse(envelope: dict[str, Any]) -> dict[str, Any]:
        raw = envelope.get("response", "")
        parsed = json.loads(raw)
        if not isinstance(parsed, dict):
            raise ValueError("expected object")
        return parsed

    async def aclose(self) -> None:
        await self._client.aclose()


def _to_result(payload: dict[str, Any]) -> ClassificationResult:
    sensitivity = str(payload.get("sensitivity", "none")).lower()
    entities = payload.get("entities") or []
    matches: list[Match] = []
    for e in entities:
        matches.append(
            Match(
                category=str(e.get("type", "OTHER")),
                span=(0, 0),  # LLM doesn't give us reliable offsets
                text=str(e.get("text", "")),
                confidence=float(e.get("confidence", _SENSITIVITY_TO_CONF.get(sensitivity, 0.5))),
            )
        )
    top_conf = max((m.confidence for m in matches), default=_SENSITIVITY_TO_CONF.get(sensitivity, 0.0))
    return ClassificationResult(
        matches=tuple(matches),
        confidence=top_conf,
        needs_escalation=False,
    )


_: Classifier = LLMClassifier(base_url="http://x", model="m")
```

- [ ] **Step 4: Run tests, verify pass**

```bash
pytest tests/unit/test_llm_pass.py -v
```

Expected: 3 passed.

- [ ] **Step 5: Commit**

```bash
git add services/dspm-classifier/app/classifiers/llm_pass.py services/dspm-classifier/tests/unit/test_llm_pass.py
git commit -m "feat(dspm-classifier): LLM classifier with structured JSON + retries"
```

---

## Task 13: Pipeline (tiered orchestration)

**Files:**
- Create: `services/dspm-classifier/app/classifiers/pipeline.py`
- Test: `services/dspm-classifier/tests/unit/test_pipeline.py`

- [ ] **Step 1: Write the failing test**

Create `services/dspm-classifier/tests/unit/test_pipeline.py`:

```python
from __future__ import annotations

import pytest

from app.classifiers.base import ClassificationResult, Match
from app.classifiers.pipeline import Pipeline
from app.models.events import Sensitivity


class _Stub:
    def __init__(self, result: ClassificationResult) -> None:
        self._result = result
        self.called = 0

    async def classify(self, content: str) -> ClassificationResult:
        self.called += 1
        return self._result


@pytest.mark.asyncio
async def test_regex_hit_skips_ner_and_llm():
    regex = _Stub(ClassificationResult(
        matches=(Match("PII.SSN", (0, 11), "123-45-6789", 1.0),),
        confidence=1.0, needs_escalation=False,
    ))
    ner = _Stub(ClassificationResult())
    llm = _Stub(ClassificationResult())

    p = Pipeline(regex=regex, ner=ner, llm=llm, escalate_to_llm=True)
    out = await p.run("My SSN is 123-45-6789")

    assert regex.called == 1
    assert ner.called == 0
    assert llm.called == 0
    assert out.sensitivity == int(Sensitivity.HIGH)
    assert "PII" in out.categories


@pytest.mark.asyncio
async def test_ner_low_conf_escalates_to_llm():
    regex = _Stub(ClassificationResult())  # no regex hits
    ner = _Stub(ClassificationResult(
        matches=(Match("PII.PERSON", (0, 4), "John", 0.3),),
        confidence=0.3, needs_escalation=True,
    ))
    llm = _Stub(ClassificationResult(
        matches=(Match("PII.PERSON", (0, 0), "John Doe", 0.92),),
        confidence=0.92, needs_escalation=False,
    ))

    p = Pipeline(regex=regex, ner=ner, llm=llm, escalate_to_llm=True)
    out = await p.run("John was here.")

    assert llm.called == 1
    assert out.sensitivity >= int(Sensitivity.MEDIUM)
    assert "llm" in out.stages_run


@pytest.mark.asyncio
async def test_escalate_disabled_does_not_call_llm():
    regex = _Stub(ClassificationResult())
    ner = _Stub(ClassificationResult(
        matches=(Match("PII.PERSON", (0, 4), "John", 0.3),),
        needs_escalation=True,
    ))
    llm = _Stub(ClassificationResult())

    p = Pipeline(regex=regex, ner=ner, llm=llm, escalate_to_llm=False)
    out = await p.run("John was here.")

    assert llm.called == 0
    assert "llm" not in out.stages_run


@pytest.mark.asyncio
async def test_dedupes_overlapping_spans():
    regex = _Stub(ClassificationResult(
        matches=(Match("PII.SSN", (10, 21), "123-45-6789", 1.0),),
    ))
    ner = _Stub(ClassificationResult(
        matches=(Match("PII.SSN", (10, 21), "123-45-6789", 0.8),),
    ))
    llm = _Stub(ClassificationResult())

    p = Pipeline(regex=regex, ner=ner, llm=llm, escalate_to_llm=False)
    out = await p.run("My SSN is 123-45-6789")

    assert sum(1 for m in out.matches if m.category == "PII.SSN") == 1


@pytest.mark.asyncio
async def test_no_matches_is_sensitivity_none():
    regex = _Stub(ClassificationResult())
    ner = _Stub(ClassificationResult())
    llm = _Stub(ClassificationResult())

    p = Pipeline(regex=regex, ner=ner, llm=llm, escalate_to_llm=True)
    out = await p.run("Just an ordinary paragraph.")

    assert out.sensitivity == int(Sensitivity.NONE)
    assert out.matches == ()
```

- [ ] **Step 2: Run test, verify fail**

```bash
pytest tests/unit/test_pipeline.py -v
```

Expected: import error.

- [ ] **Step 3: Implement `Pipeline`**

Create `services/dspm-classifier/app/classifiers/pipeline.py`:

```python
"""Tiered classification pipeline: regex → NER → LLM."""

from __future__ import annotations

import logging
from collections.abc import Iterable

from app.classifiers.base import (
    Classifier,
    ClassificationResult,
    Match,
    PipelineOutput,
)
from app.classifiers.llm_pass import LLMUnavailable
from app.models.events import Sensitivity


_LOGGER = logging.getLogger(__name__)


# Categories whose presence implies a sensitivity floor.
_SENSITIVITY_FLOOR: dict[str, Sensitivity] = {
    "PII.SSN": Sensitivity.HIGH,
    "FINANCIAL.CREDIT_CARD": Sensitivity.HIGH,
    "SECRETS.JWT": Sensitivity.HIGH,
    "SECRETS.AWS_KEY": Sensitivity.HIGH,
    "SECRETS.GENERIC": Sensitivity.MEDIUM,
    "HEALTH.MEDICAL_LICENSE": Sensitivity.HIGH,
    "FINANCIAL.IBAN": Sensitivity.HIGH,
    "PII.EMAIL": Sensitivity.MEDIUM,
    "PII.PHONE": Sensitivity.MEDIUM,
    "PII.PERSON": Sensitivity.LOW,
    "PII.LOCATION": Sensitivity.LOW,
    "PII.IP": Sensitivity.LOW,
}


def _dedupe(matches: Iterable[Match]) -> tuple[Match, ...]:
    seen: dict[tuple[str, tuple[int, int]], Match] = {}
    for m in matches:
        key = (m.category, m.span)
        existing = seen.get(key)
        if existing is None or m.confidence > existing.confidence:
            seen[key] = m
    return tuple(seen.values())


def _top_category(category: str) -> str:
    return category.split(".", 1)[0]


def _sensitivity_from(matches: Iterable[Match]) -> Sensitivity:
    floor = Sensitivity.NONE
    for m in matches:
        bumped = _SENSITIVITY_FLOOR.get(m.category, Sensitivity.LOW)
        if bumped > floor:
            floor = bumped
    return floor


class Pipeline:
    def __init__(
        self,
        *,
        regex: Classifier,
        ner: Classifier,
        llm: Classifier,
        escalate_to_llm: bool = True,
    ) -> None:
        self._regex = regex
        self._ner = ner
        self._llm = llm
        self._escalate_to_llm = escalate_to_llm

    async def run(self, content: str) -> PipelineOutput:
        stages_run: list[str] = []
        all_matches: list[Match] = []
        llm_failed = False
        reason: str | None = None

        # Stage 1: regex.
        regex_result = await self._regex.classify(content)
        stages_run.append("regex")
        all_matches.extend(regex_result.matches)

        if _has_high_conf_match(regex_result.matches):
            return _output(all_matches, stages_run, llm_failed=False, reason=None)

        # Stage 2: NER.
        ner_result = await self._ner.classify(content)
        stages_run.append("ner")
        all_matches.extend(ner_result.matches)

        # Stage 3: LLM, only if NER asked for it.
        if self._escalate_to_llm and ner_result.needs_escalation:
            try:
                llm_result = await self._llm.classify(content)
                stages_run.append("llm")
                all_matches.extend(llm_result.matches)
            except LLMUnavailable as e:
                _LOGGER.warning("llm unavailable; persisting partial finding: %s", e)
                llm_failed = True
                reason = "llm_unavailable"

        return _output(all_matches, stages_run, llm_failed=llm_failed, reason=reason)


def _has_high_conf_match(matches: Iterable[Match]) -> bool:
    return any(m.confidence >= 0.9 and m.category != "OTHER" for m in matches)


def _output(
    matches: list[Match],
    stages_run: list[str],
    *,
    llm_failed: bool,
    reason: str | None,
) -> PipelineOutput:
    deduped = _dedupe(matches)
    sensitivity = _sensitivity_from(deduped)
    categories = tuple(sorted({_top_category(m.category) for m in deduped}))
    return PipelineOutput(
        matches=deduped,
        categories=categories,
        sensitivity=int(sensitivity),
        llm_failed=llm_failed,
        reason=reason,
        stages_run=tuple(stages_run),
    )
```

- [ ] **Step 4: Run tests, verify pass**

```bash
pytest tests/unit/test_pipeline.py -v
```

Expected: 5 passed.

- [ ] **Step 5: Commit**

```bash
git add services/dspm-classifier/app/classifiers/pipeline.py services/dspm-classifier/tests/unit/test_pipeline.py
git commit -m "feat(dspm-classifier): tiered classification pipeline"
```

---

## Task 14: S3 fetcher (aioboto3)

**Files:**
- Create: `services/dspm-classifier/app/storage/s3.py`
- Test: `services/dspm-classifier/tests/integration/test_s3_fetcher.py`

- [ ] **Step 1: Write the failing test**

Create `services/dspm-classifier/tests/integration/test_s3_fetcher.py`:

```python
from __future__ import annotations

import boto3
import pytest

from app.storage.s3 import ObjectMissing, ObjectTooLarge, S3Fetcher


@pytest.fixture
def bucket_with_object(minio_container) -> dict[str, str]:
    s3 = boto3.client(
        "s3",
        endpoint_url=minio_container["endpoint_url"],
        aws_access_key_id=minio_container["access_key"],
        aws_secret_access_key=minio_container["secret_key"],
        region_name="us-east-1",
    )
    s3.create_bucket(Bucket="acme-uploads")
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
```

- [ ] **Step 2: Run test, verify fail**

```bash
pytest tests/integration/test_s3_fetcher.py -v
```

Expected: import error.

- [ ] **Step 3: Implement `S3Fetcher`**

Create `services/dspm-classifier/app/storage/s3.py`:

```python
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
```

- [ ] **Step 4: Run tests, verify pass**

```bash
pytest tests/integration/test_s3_fetcher.py -v
```

Expected: 3 passed.

- [ ] **Step 5: Commit**

```bash
git add services/dspm-classifier/app/storage/s3.py services/dspm-classifier/tests/integration/test_s3_fetcher.py
git commit -m "feat(dspm-classifier): aioboto3 S3 fetcher with size cap"
```

---

## Task 15: `classify_one` CLI driver (end-to-end integration)

**Files:**
- Create: `services/dspm-classifier/scripts/classify_one.py`
- Test: `services/dspm-classifier/tests/integration/test_classify_one_cli.py`

This task wires every component together end-to-end. The CLI takes
`--bucket --key --tenant --event-id`, fetches the object from S3, runs the
pipeline, upserts the finding, marks the message processed in the same
transaction, and prints the result as JSON.

- [ ] **Step 1: Write the failing test**

Create `services/dspm-classifier/tests/integration/test_classify_one_cli.py`:

```python
from __future__ import annotations

import json

import boto3
import pytest
from sqlalchemy import select

from app.models.db import FindingRow, ProcessedMessageRow
from scripts.classify_one import classify_one_event


@pytest.mark.asyncio
async def test_classifies_object_end_to_end(
    session_factory, postgres_dsn, minio_container, monkeypatch,
):
    s3 = boto3.client(
        "s3",
        endpoint_url=minio_container["endpoint_url"],
        aws_access_key_id=minio_container["access_key"],
        aws_secret_access_key=minio_container["secret_key"],
        region_name="us-east-1",
    )
    s3.create_bucket(Bucket="acme-uploads")
    s3.put_object(Bucket="acme-uploads", Key="doc.txt", Body=b"SSN: 123-45-6789")

    monkeypatch.setenv("POSTGRES_DSN", postgres_dsn)
    monkeypatch.setenv("S3_ENDPOINT_URL", minio_container["endpoint_url"])
    monkeypatch.setenv("AWS_ACCESS_KEY_ID", minio_container["access_key"])
    monkeypatch.setenv("AWS_SECRET_ACCESS_KEY", minio_container["secret_key"])
    monkeypatch.setenv("LLM_BASE_URL", "http://unused:0")
    monkeypatch.setenv("ESCALATE_TO_LLM", "false")  # avoid the network call

    output = await classify_one_event(
        event_id="evt_1",
        tenant_id="acme",
        bucket="acme-uploads",
        key="doc.txt",
    )

    parsed = json.loads(output)
    assert parsed["sensitivity"] >= 3  # HIGH
    assert "PII" in parsed["categories"]

    async with session_factory() as s:
        findings = (await s.execute(select(FindingRow))).scalars().all()
        processed = (await s.execute(select(ProcessedMessageRow))).scalars().all()
    assert len(findings) == 1
    assert findings[0].sensitivity == 3
    assert len(processed) == 1
```

- [ ] **Step 2: Run test, verify fail**

```bash
pytest tests/integration/test_classify_one_cli.py -v
```

Expected: import error on `scripts.classify_one`.

- [ ] **Step 3: Implement `classify_one`**

Create `services/dspm-classifier/scripts/classify_one.py`:

```python
"""CLI driver: classify a single S3 object end-to-end into Postgres.

Used as a smoke test for Plan 1 and as the integration seam for Plan 2's
Kafka worker (which will call `classify_one_event` directly).
"""

from __future__ import annotations

import argparse
import asyncio
import json
from datetime import datetime, timezone

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
        timeout_s=settings.llm_timeout_s,
    )
    pipeline = Pipeline(regex=regex, ner=ner, llm=llm, escalate_to_llm=settings.escalate_to_llm)

    repo = FindingRepo(session_factory)
    idempotency = IdempotencyStore(session_factory)

    now = datetime.now(timezone.utc)
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
        finding = _empty_finding(event_id, tenant_id, bucket, key, now,
                                 settings.pipeline_version, reason)
    except ObjectTooLarge:
        reason = "too_large"
        finding = _empty_finding(event_id, tenant_id, bucket, key, now,
                                 settings.pipeline_version, reason)

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
```

- [ ] **Step 4: Run integration test, verify pass**

```bash
pytest tests/integration/test_classify_one_cli.py -v
```

Expected: 1 passed.

- [ ] **Step 5: Run the full unit + integration suite**

```bash
pytest -v
```

Expected: all tests pass (or integration tests skipped if Docker unavailable).

- [ ] **Step 6: Commit**

```bash
git add services/dspm-classifier/scripts services/dspm-classifier/tests/integration/test_classify_one_cli.py
git commit -m "feat(dspm-classifier): classify_one CLI wires the engine end-to-end"
```

---

## Task 16: Preflight + branch hygiene

- [ ] **Step 1: Run repo-wide Python preflight**

From the repo root:

```bash
make preflight-python
```

Expected: pass. Fix any ruff/mypy findings before continuing.

- [ ] **Step 2: Run security preflight**

```bash
make preflight-security
```

Expected: pass (no committed secrets, no hardcoded credentials in source — test fixtures are scoped via the per-file ignore in `pyproject.toml`).

- [ ] **Step 3: Push the feature branch**

```bash
git push -u origin <feature-branch>
```

(Kyle's rule: feature branches push, doc-only changes wait. Plan 1 contains code, so push is appropriate. Open the PR to `qa` per the repo workflow.)

- [ ] **Step 4: Create the PR**

```bash
gh pr create --base qa --title "feat(dspm-classifier): classification engine (Plan 1)" --body "$(cat <<'EOF'
## Summary
- Scaffolds the `dspm-classifier` service.
- Implements config, db models, `FindingRepo`, idempotency store.
- Implements regex / Presidio NER / LLM classifiers and the tiered pipeline.
- Adds `aioboto3` S3 fetcher with size cap.
- Adds a `classify_one` CLI that ties it all together end-to-end.

Spec: `docs/superpowers/specs/2026-06-16-dspm-classifier-design.md`
Plan: `docs/superpowers/plans/2026-06-16-dspm-classifier-plan-1-engine.md`

## Test plan
- [ ] `make preflight-python` passes
- [ ] `pytest` (unit) passes locally
- [ ] `pytest -m integration` passes locally with Docker (postgres + minio testcontainers)
- [ ] Manual: `python -m scripts.classify_one --event-id e1 --tenant-id acme --bucket b --key k` returns a JSON finding

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

---

## What's next (Plan 2 preview)

Plan 2 builds on Plan 1 by adding:
- `app/kafka/consumer.py` — aiokafka consumer with manual offset commits.
- `app/kafka/producer.py` — findings / retry / DLQ producers.
- `app/kafka/backpressure.py` — bounded work pool + pause/resume.
- `worker/main.py` — entrypoint that consumes events and calls
  `classify_one_event` (or an extracted `EventProcessor` derived from it).
- Integration test against Redpanda testcontainer.
