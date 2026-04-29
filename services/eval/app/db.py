from __future__ import annotations

import json
import uuid
from datetime import datetime, timezone

import aiosqlite

# timezone.utc spelled out for Python 3.9 compat; noqa suppresses UP017 on each use
_UTC = timezone.utc  # noqa: UP017


class EvalDB:
    def __init__(self, db_path: str):
        self.db_path = db_path
        self._db: aiosqlite.Connection | None = None

    async def init(self):
        """Initialize the database and create tables."""
        self._db = await aiosqlite.connect(self.db_path)
        self._db.row_factory = aiosqlite.Row
        await self._db.executescript(
            """
            CREATE TABLE IF NOT EXISTS datasets (
                id TEXT PRIMARY KEY,
                name TEXT UNIQUE NOT NULL,
                items TEXT NOT NULL,
                created_at TEXT NOT NULL
            );
            CREATE TABLE IF NOT EXISTS evaluations (
                id TEXT PRIMARY KEY,
                dataset_id TEXT NOT NULL REFERENCES datasets(id),
                status TEXT NOT NULL DEFAULT 'running',
                collection TEXT,
                aggregate_scores TEXT,
                results TEXT,
                error TEXT,
                created_at TEXT NOT NULL,
                completed_at TEXT
            );
            """
        )

        # Idempotent migrations for new tracking columns. SQLite has no
        # `ADD COLUMN IF NOT EXISTS`, so each ALTER is wrapped and we ignore
        # only the "duplicate column name" error from re-running init().
        for column_ddl in (
            "ALTER TABLE evaluations ADD COLUMN notes TEXT",
            "ALTER TABLE evaluations ADD COLUMN config TEXT",
            "ALTER TABLE evaluations "
            "ADD COLUMN baseline_eval_id TEXT REFERENCES evaluations(id)",
        ):
            try:
                await self._db.execute(column_ddl)
            except aiosqlite.OperationalError as exc:
                if "duplicate column name" not in str(exc).lower():
                    raise

        await self._db.commit()

    async def close(self):
        if self._db:
            await self._db.close()

    async def create_dataset(self, name: str, items: list[dict]) -> str:
        """Create a golden dataset. Raises ValueError if name already exists."""
        existing = await self._db.execute(
            "SELECT id FROM datasets WHERE name = ?", (name,)
        )
        if await existing.fetchone():
            raise ValueError(f"Dataset '{name}' already exists")

        ds_id = str(uuid.uuid4())
        now = datetime.now(_UTC).isoformat()
        await self._db.execute(
            "INSERT INTO datasets (id, name, items, created_at) VALUES (?, ?, ?, ?)",
            (ds_id, name, json.dumps(items), now),
        )
        await self._db.commit()
        return ds_id

    async def get_dataset(self, ds_id: str) -> dict | None:
        cursor = await self._db.execute("SELECT * FROM datasets WHERE id = ?", (ds_id,))
        row = await cursor.fetchone()
        if not row:
            return None
        return {
            "id": row["id"],
            "name": row["name"],
            "items": json.loads(row["items"]),
            "created_at": row["created_at"],
        }

    async def list_datasets(self) -> list[dict]:
        cursor = await self._db.execute(
            "SELECT id, name, created_at FROM datasets ORDER BY created_at DESC"
        )
        rows = await cursor.fetchall()
        return [
            {"id": r["id"], "name": r["name"], "created_at": r["created_at"]}
            for r in rows
        ]

    async def create_evaluation(
        self,
        dataset_id: str,
        collection: str,
        notes: str | None = None,
        baseline_eval_id: str | None = None,
    ) -> str:
        eval_id = str(uuid.uuid4())
        now = datetime.now(_UTC).isoformat()
        await self._db.execute(
            "INSERT INTO evaluations "
            "(id, dataset_id, status, collection, created_at, notes, baseline_eval_id) "
            "VALUES (?, ?, 'running', ?, ?, ?, ?)",
            (eval_id, dataset_id, collection, now, notes, baseline_eval_id),
        )
        await self._db.commit()
        return eval_id

    def _row_to_dict(self, row, *, include_results: bool = True) -> dict:
        """Shared row → dict conversion for evaluation rows."""
        out = {
            "id": row["id"],
            "dataset_id": row["dataset_id"],
            "status": row["status"],
            "collection": row["collection"],
            "aggregate_scores": (
                json.loads(row["aggregate_scores"]) if row["aggregate_scores"] else None
            ),
            "created_at": row["created_at"],
            "completed_at": row["completed_at"],
            "notes": row["notes"],
            "config": json.loads(row["config"]) if row["config"] else None,
            "baseline_eval_id": row["baseline_eval_id"],
        }
        if include_results:
            out["results"] = json.loads(row["results"]) if row["results"] else None
            out["error"] = row["error"]
        return out

    async def set_evaluation_config(self, eval_id: str, config: dict) -> None:
        await self._db.execute(
            "UPDATE evaluations SET config = ? WHERE id = ?",
            (json.dumps(config), eval_id),
        )
        await self._db.commit()

    async def get_evaluation(self, eval_id: str) -> dict | None:
        cursor = await self._db.execute(
            "SELECT * FROM evaluations WHERE id = ?", (eval_id,)
        )
        row = await cursor.fetchone()
        if not row:
            return None
        return self._row_to_dict(row)

    async def list_evaluations(self, limit: int = 20, offset: int = 0) -> list[dict]:
        cursor = await self._db.execute(
            "SELECT * FROM evaluations ORDER BY created_at DESC LIMIT ? OFFSET ?",
            (limit, offset),
        )
        rows = await cursor.fetchall()
        return [self._row_to_dict(r, include_results=False) for r in rows]

    async def get_evaluations_by_ids(self, ids: list[str]) -> list[dict]:
        """Return rows in the same order as `ids`. Missing ids are skipped."""
        if not ids:
            return []
        # placeholders is a sequence of '?' characters built from ids length.
        # All user values flow through SQLite parameter binding via the
        # tuple(ids) below — no untrusted strings are interpolated.
        placeholders = ",".join("?" for _ in ids)
        cursor = await self._db.execute(
            f"SELECT * FROM evaluations WHERE id IN ({placeholders})",  # nosec B608
            tuple(ids),
        )
        rows = await cursor.fetchall()
        by_id = {r["id"]: r for r in rows}
        return [self._row_to_dict(by_id[eid]) for eid in ids if eid in by_id]

    async def get_history(self, dataset_id: str, collection: str) -> list[dict]:
        """Completed runs for the given dataset+collection, ordered ASC."""
        cursor = await self._db.execute(
            "SELECT * FROM evaluations "
            "WHERE dataset_id = ? AND collection = ? AND status = 'completed' "
            "ORDER BY created_at ASC",
            (dataset_id, collection),
        )
        rows = await cursor.fetchall()
        return [self._row_to_dict(r) for r in rows]

    async def complete_evaluation(
        self, eval_id: str, aggregate_scores: dict, results: list[dict]
    ):
        now = datetime.now(_UTC).isoformat()
        await self._db.execute(
            "UPDATE evaluations "
            "SET status = 'completed', aggregate_scores = ?, results = ?, "
            "completed_at = ? WHERE id = ?",
            (json.dumps(aggregate_scores), json.dumps(results), now, eval_id),
        )
        await self._db.commit()

    async def fail_evaluation(self, eval_id: str, error: str):
        now = datetime.now(_UTC).isoformat()
        await self._db.execute(
            "UPDATE evaluations "
            "SET status = 'failed', error = ?, completed_at = ? WHERE id = ?",
            (error, now, eval_id),
        )
        await self._db.commit()
