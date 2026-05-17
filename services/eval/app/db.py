from __future__ import annotations

import json
import uuid
from datetime import datetime, timedelta, timezone

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
            CREATE TABLE IF NOT EXISTS experiments (
                id TEXT PRIMARY KEY,
                name TEXT NOT NULL,
                hypothesis TEXT NOT NULL,
                dataset_id TEXT NOT NULL REFERENCES datasets(id),
                collection TEXT NOT NULL,
                baseline_eval_id TEXT REFERENCES evaluations(id),
                focus_metric TEXT NOT NULL DEFAULT 'context_precision',
                status TEXT NOT NULL,
                decision TEXT,
                conclusion TEXT,
                evidence TEXT,
                notes TEXT,
                created_at TEXT NOT NULL,
                updated_at TEXT NOT NULL
            );
            CREATE TABLE IF NOT EXISTS experiment_runs (
                experiment_id TEXT NOT NULL REFERENCES experiments(id),
                evaluation_id TEXT NOT NULL REFERENCES evaluations(id),
                label TEXT NOT NULL,
                notes TEXT,
                created_at TEXT NOT NULL,
                PRIMARY KEY (experiment_id, evaluation_id),
                UNIQUE (experiment_id, label)
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
            "ALTER TABLE experiments "
            "ADD COLUMN focus_metric TEXT NOT NULL DEFAULT 'context_precision'",
            "ALTER TABLE experiments ADD COLUMN conclusion TEXT",
            "ALTER TABLE experiments ADD COLUMN evidence TEXT",
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
            "SELECT id, name, items, created_at FROM datasets ORDER BY created_at DESC"
        )
        rows = await cursor.fetchall()
        return [
            {
                "id": r["id"],
                "name": r["name"],
                "created_at": r["created_at"],
                "item_count": len(json.loads(r["items"])),
            }
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

    async def get_completed_evaluations_for_dashboard(
        self, dataset_id: str, collection: str
    ) -> list[dict]:
        """Completed compact runs for dashboard summaries, ordered ASC."""
        cursor = await self._db.execute(
            "SELECT * FROM evaluations "
            "WHERE dataset_id = ? AND collection = ? AND status = 'completed' "
            "ORDER BY created_at ASC",
            (dataset_id, collection),
        )
        rows = await cursor.fetchall()
        return [self._row_to_dict(r, include_results=False) for r in rows]

    def _experiment_row_to_dict(self, row, *, runs: list[dict] | None = None) -> dict:
        out = {
            "id": row["id"],
            "name": row["name"],
            "hypothesis": row["hypothesis"],
            "dataset_id": row["dataset_id"],
            "collection": row["collection"],
            "baseline_eval_id": row["baseline_eval_id"],
            "focus_metric": row["focus_metric"],
            "status": row["status"],
            "decision": row["decision"],
            "conclusion": row["conclusion"],
            "evidence": json.loads(row["evidence"]) if row["evidence"] else None,
            "notes": row["notes"],
            "created_at": row["created_at"],
            "updated_at": row["updated_at"],
        }
        if runs is not None:
            out["runs"] = runs
        return out

    async def create_experiment(
        self,
        name: str,
        hypothesis: str,
        dataset_id: str,
        collection: str,
        baseline_eval_id: str | None = None,
        focus_metric: str = "context_precision",
        status: str = "planned",
        notes: str | None = None,
    ) -> str:
        exp_id = str(uuid.uuid4())
        now = datetime.now(_UTC).isoformat()
        await self._db.execute(
            "INSERT INTO experiments "
            "(id, name, hypothesis, dataset_id, collection, baseline_eval_id, "
            "focus_metric, status, decision, notes, created_at, updated_at) "
            "VALUES (?, ?, ?, ?, ?, ?, ?, ?, NULL, ?, ?, ?)",
            (
                exp_id,
                name,
                hypothesis,
                dataset_id,
                collection,
                baseline_eval_id,
                focus_metric,
                status,
                notes,
                now,
                now,
            ),
        )
        await self._db.commit()
        return exp_id

    async def get_experiment(self, experiment_id: str) -> dict | None:
        cursor = await self._db.execute(
            "SELECT * FROM experiments WHERE id = ?", (experiment_id,)
        )
        row = await cursor.fetchone()
        if not row:
            return None
        runs = await self.list_experiment_runs(experiment_id)
        return self._experiment_row_to_dict(row, runs=runs)

    async def list_experiments(
        self,
        dataset_id: str | None = None,
        collection: str | None = None,
        status: str | None = None,
    ) -> list[dict]:
        clauses = []
        params: list[str] = []
        if dataset_id is not None:
            clauses.append("dataset_id = ?")
            params.append(dataset_id)
        if collection is not None:
            clauses.append("collection = ?")
            params.append(collection)
        if status is not None:
            clauses.append("status = ?")
            params.append(status)

        where = f"WHERE {' AND '.join(clauses)}" if clauses else ""
        cursor = await self._db.execute(
            f"SELECT * FROM experiments {where} ORDER BY created_at DESC",  # nosec B608
            tuple(params),
        )
        rows = await cursor.fetchall()
        return [self._experiment_row_to_dict(r) for r in rows]

    async def update_experiment(
        self,
        experiment_id: str,
        *,
        hypothesis: str | None = None,
        baseline_eval_id: str | None = None,
        focus_metric: str | None = None,
        status: str | None = None,
        decision: str | None = None,
        conclusion: str | None = None,
        evidence: dict | None = None,
        notes: str | None = None,
    ) -> None:
        now = datetime.now(_UTC).isoformat()
        await self._db.execute(
            "UPDATE experiments "
            "SET hypothesis = COALESCE(?, hypothesis), "
            "baseline_eval_id = ?, "
            "focus_metric = COALESCE(?, focus_metric), "
            "status = COALESCE(?, status), "
            "decision = ?, "
            "conclusion = ?, "
            "evidence = ?, "
            "notes = COALESCE(?, notes), "
            "updated_at = ? "
            "WHERE id = ?",
            (
                hypothesis,
                baseline_eval_id,
                focus_metric,
                status,
                decision,
                conclusion,
                json.dumps(evidence) if evidence is not None else None,
                notes,
                now,
                experiment_id,
            ),
        )
        await self._db.commit()

    def _experiment_run_row_to_dict(self, row) -> dict:
        evaluation = self._row_to_dict(row, include_results=False)
        return {
            "evaluation_id": row["evaluation_id"],
            "label": row["label"],
            "notes": row["run_notes"],
            "attached_at": row["attached_at"],
            "evaluation": evaluation,
        }

    async def attach_experiment_run(
        self,
        experiment_id: str,
        evaluation_id: str,
        label: str,
        notes: str | None = None,
    ) -> dict | None:
        experiment_cursor = await self._db.execute(
            "SELECT * FROM experiments WHERE id = ?", (experiment_id,)
        )
        experiment = await experiment_cursor.fetchone()
        if not experiment:
            return None

        evaluation = await self.get_evaluation(evaluation_id)
        if not evaluation:
            return None

        if experiment["status"] == "completed":
            raise ValueError("completed experiments cannot accept runs")
        if experiment["dataset_id"] != evaluation["dataset_id"]:
            raise ValueError("experiment run must use the same dataset")
        if experiment["collection"] != evaluation["collection"]:
            raise ValueError("experiment run must use the same collection")

        now = datetime.now(_UTC).isoformat()
        try:
            await self._db.execute(
                "INSERT INTO experiment_runs "
                "(experiment_id, evaluation_id, label, notes, created_at) "
                "VALUES (?, ?, ?, ?, ?)",
                (experiment_id, evaluation_id, label, notes, now),
            )
        except aiosqlite.IntegrityError as exc:
            if "experiment_runs.experiment_id, experiment_runs.label" in str(exc):
                raise ValueError("duplicate experiment run label") from exc
            raise
        await self._db.commit()
        return await self.get_experiment(experiment_id)

    async def list_experiment_runs(self, experiment_id: str) -> list[dict]:
        cursor = await self._db.execute(
            "SELECT "
            "er.evaluation_id, er.label, er.notes AS run_notes, "
            "er.created_at AS attached_at, "
            "e.* "
            "FROM experiment_runs er "
            "JOIN evaluations e ON e.id = er.evaluation_id "
            "WHERE er.experiment_id = ? "
            "ORDER BY er.created_at ASC",
            (experiment_id,),
        )
        rows = await cursor.fetchall()
        return [self._experiment_run_row_to_dict(r) for r in rows]

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

    async def fail_stale_running_evaluations(self, max_age_seconds: float) -> int:
        now = datetime.now(_UTC)
        cutoff = (now - timedelta(seconds=max_age_seconds)).isoformat()
        error = "evaluation exceeded max runtime and was recovered as stale"
        cursor = await self._db.execute(
            "UPDATE evaluations "
            "SET status = 'failed', error = ?, completed_at = ? "
            "WHERE status = 'running' AND created_at < ?",
            (error, now.isoformat(), cutoff),
        )
        await self._db.commit()
        return cursor.rowcount

    async def count_stale_running_evaluations(self, max_age_seconds: float) -> int:
        cutoff = (datetime.now(_UTC) - timedelta(seconds=max_age_seconds)).isoformat()
        cursor = await self._db.execute(
            "SELECT COUNT(*) AS count FROM evaluations "
            "WHERE status = 'running' AND created_at < ?",
            (cutoff,),
        )
        row = await cursor.fetchone()
        return int(row["count"])
