"""Async SQLAlchemy engine + sessionmaker factory."""

from __future__ import annotations

from sqlalchemy.ext.asyncio import AsyncEngine, async_sessionmaker, create_async_engine


def make_engine(dsn: str) -> AsyncEngine:
    return create_async_engine(dsn, future=True, pool_pre_ping=True)


def make_session_factory(engine: AsyncEngine) -> async_sessionmaker:
    return async_sessionmaker(engine, expire_on_commit=False)
