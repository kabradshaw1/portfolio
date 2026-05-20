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
        await self._db.execute("PRAGMA journal_mode=WAL")
        await self._db.execute("PRAGMA busy_timeout=5000")
        await self._db.execute("PRAGMA foreign_keys=ON")
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
            CREATE TABLE IF NOT EXISTS evaluation_items (
                id TEXT PRIMARY KEY,
                evaluation_id TEXT NOT NULL REFERENCES evaluations(id),
                item_index INTEGER NOT NULL,
                query TEXT NOT NULL,
                expected_answer TEXT NOT NULL,
                expected_sources TEXT NOT NULL,
                status TEXT NOT NULL DEFAULT 'queued',
                attempt_count INTEGER NOT NULL DEFAULT 0,
                max_attempts INTEGER NOT NULL DEFAULT 3,
                lease_owner TEXT,
                lease_expires_at TEXT,
                last_error TEXT,
                replay_count INTEGER NOT NULL DEFAULT 0,
                last_replayed_at TEXT,
                result TEXT,
                scores TEXT,
                score_reasons TEXT,
                started_at TEXT,
                completed_at TEXT,
                created_at TEXT NOT NULL,
                updated_at TEXT NOT NULL,
                UNIQUE (evaluation_id, item_index)
            );
            CREATE INDEX IF NOT EXISTS idx_evaluation_items_eval_status
                ON evaluation_items(evaluation_id, status);
            CREATE INDEX IF NOT EXISTS idx_evaluation_items_status_lease
                ON evaluation_items(status, lease_expires_at);
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
            "ALTER TABLE evaluation_items "
            "ADD COLUMN replay_count INTEGER NOT NULL DEFAULT 0",
            "ALTER TABLE evaluation_items ADD COLUMN last_replayed_at TEXT",
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
        status: str = "running",
    ) -> str:
        eval_id = str(uuid.uuid4())
        now = datetime.now(_UTC).isoformat()
        await self._db.execute(
            "INSERT INTO evaluations "
            "(id, dataset_id, status, collection, created_at, notes, baseline_eval_id) "
            "VALUES (?, ?, ?, ?, ?, ?, ?)",
            (eval_id, dataset_id, status, collection, now, notes, baseline_eval_id),
        )
        await self._db.commit()
        return eval_id

    def _item_row_to_dict(self, row) -> dict:
        return {
            "id": row["id"],
            "evaluation_id": row["evaluation_id"],
            "item_index": row["item_index"],
            "query": row["query"],
            "expected_answer": row["expected_answer"],
            "expected_sources": json.loads(row["expected_sources"]),
            "status": row["status"],
            "attempt_count": row["attempt_count"],
            "max_attempts": row["max_attempts"],
            "lease_owner": row["lease_owner"],
            "lease_expires_at": row["lease_expires_at"],
            "last_error": json.loads(row["last_error"]) if row["last_error"] else None,
            "replay_count": row["replay_count"],
            "last_replayed_at": row["last_replayed_at"],
            "result": json.loads(row["result"]) if row["result"] else None,
            "scores": json.loads(row["scores"]) if row["scores"] else None,
            "score_reasons": (
                json.loads(row["score_reasons"]) if row["score_reasons"] else None
            ),
            "started_at": row["started_at"],
            "completed_at": row["completed_at"],
            "created_at": row["created_at"],
            "updated_at": row["updated_at"],
        }

    async def create_evaluation_items(
        self, eval_id: str, items: list[dict], max_attempts: int
    ) -> list[dict]:
        now = datetime.now(_UTC).isoformat()
        created = []
        for index, item in enumerate(items):
            item_id = str(uuid.uuid4())
            expected_sources = item.get("expected_sources", [])
            await self._db.execute(
                "INSERT INTO evaluation_items "
                "(id, evaluation_id, item_index, query, expected_answer, "
                "expected_sources, status, attempt_count, max_attempts, "
                "created_at, updated_at) "
                "VALUES (?, ?, ?, ?, ?, ?, 'queued', 0, ?, ?, ?)",
                (
                    item_id,
                    eval_id,
                    index,
                    item["query"],
                    item["expected_answer"],
                    json.dumps(expected_sources),
                    max_attempts,
                    now,
                    now,
                ),
            )
            created.append(
                {
                    "id": item_id,
                    "evaluation_id": eval_id,
                    "item_index": index,
                    "query": item["query"],
                    "expected_answer": item["expected_answer"],
                    "expected_sources": expected_sources,
                    "status": "queued",
                    "attempt_count": 0,
                    "max_attempts": max_attempts,
                }
            )
        await self._db.commit()
        return created

    async def list_evaluation_items(self, eval_id: str) -> list[dict]:
        cursor = await self._db.execute(
            "SELECT * FROM evaluation_items "
            "WHERE evaluation_id = ? ORDER BY item_index",
            (eval_id,),
        )
        rows = await cursor.fetchall()
        return [self._item_row_to_dict(row) for row in rows]

    async def get_evaluation_item(self, item_id: str) -> dict | None:
        cursor = await self._db.execute(
            "SELECT * FROM evaluation_items WHERE id = ?", (item_id,)
        )
        row = await cursor.fetchone()
        return self._item_row_to_dict(row) if row else None

    async def claim_evaluation_item(
        self, item_id: str, worker_id: str, lease_seconds: float
    ) -> dict | None:
        now = datetime.now(_UTC)
        now_text = now.isoformat()
        lease_until = (now + timedelta(seconds=lease_seconds)).isoformat()
        cursor = await self._db.execute(
            "UPDATE evaluation_items "
            "SET status = 'running', attempt_count = attempt_count + 1, "
            "lease_owner = ?, lease_expires_at = ?, "
            "started_at = COALESCE(started_at, ?), updated_at = ? "
            "WHERE id = ? AND status = 'queued'",
            (worker_id, lease_until, now_text, now_text, item_id),
        )
        await self._db.commit()
        if cursor.rowcount == 0:
            return None
        return await self.get_evaluation_item(item_id)

    async def mark_evaluation_running(self, eval_id: str) -> None:
        await self._db.execute(
            "UPDATE evaluations SET status = 'running', completed_at = NULL "
            "WHERE id = ? AND status = 'queued'",
            (eval_id,),
        )
        await self._db.commit()

    async def mark_evaluation_item_completed(
        self,
        item_id: str,
        result: dict,
        scores: dict,
        score_reasons: dict,
    ) -> None:
        now = datetime.now(_UTC).isoformat()
        await self._db.execute(
            "UPDATE evaluation_items "
            "SET status = 'completed', result = ?, scores = ?, score_reasons = ?, "
            "lease_owner = NULL, lease_expires_at = NULL, completed_at = ?, "
            "updated_at = ? WHERE id = ? AND status != 'completed'",
            (
                json.dumps(result),
                json.dumps(scores),
                json.dumps(score_reasons),
                now,
                now,
                item_id,
            ),
        )
        await self._db.commit()

    async def mark_evaluation_item_failed(self, item_id: str, error: dict) -> None:
        now = datetime.now(_UTC).isoformat()
        await self._db.execute(
            "UPDATE evaluation_items "
            "SET status = 'failed', last_error = ?, lease_owner = NULL, "
            "lease_expires_at = NULL, completed_at = ?, updated_at = ? "
            "WHERE id = ? AND status != 'completed'",
            (json.dumps(error), now, now, item_id),
        )
        await self._db.commit()

    async def mark_evaluation_item_cancelled(self, item_id: str) -> None:
        now = datetime.now(_UTC).isoformat()
        await self._db.execute(
            "UPDATE evaluation_items "
            "SET status = 'cancelled', lease_owner = NULL, lease_expires_at = NULL, "
            "completed_at = ?, updated_at = ? "
            "WHERE id = ? AND status NOT IN ('completed', 'failed', 'cancelled')",
            (now, now, item_id),
        )
        await self._db.commit()

    async def cancel_evaluation(self, eval_id: str) -> dict | None:
        now = datetime.now(_UTC).isoformat()
        cursor = await self._db.execute(
            "UPDATE evaluations "
            "SET status = 'cancelled', error = ?, completed_at = ? "
            "WHERE id = ? AND status IN ('queued', 'running')",
            ("evaluation cancelled by operator", now, eval_id),
        )
        if cursor.rowcount == 0:
            await self._db.commit()
            return None
        await self._db.execute(
            "UPDATE evaluation_items "
            "SET status = 'cancelled', lease_owner = NULL, lease_expires_at = NULL, "
            "completed_at = COALESCE(completed_at, ?), updated_at = ? "
            "WHERE evaluation_id = ? "
            "AND status NOT IN ('completed', 'failed', 'cancelled')",
            (now, now, eval_id),
        )
        await self._db.commit()
        return await self.get_evaluation(eval_id)

    async def release_evaluation_item_for_retry(
        self, item_id: str, error: dict
    ) -> None:
        now = datetime.now(_UTC).isoformat()
        await self._db.execute(
            "UPDATE evaluation_items "
            "SET status = 'queued', last_error = ?, lease_owner = NULL, "
            "lease_expires_at = NULL, updated_at = ? "
            "WHERE id = ? AND status = 'running'",
            (json.dumps(error), now, item_id),
        )
        await self._db.commit()

    async def requeue_failed_item_for_replay(self, item_id: str) -> dict | None:
        now = datetime.now(_UTC).isoformat()
        cursor = await self._db.execute(
            "UPDATE evaluation_items "
            "SET status = 'queued', lease_owner = NULL, lease_expires_at = NULL, "
            "replay_count = replay_count + 1, last_replayed_at = ?, updated_at = ? "
            "WHERE id = ? AND status = 'failed'",
            (now, now, item_id),
        )
        await self._db.commit()
        if cursor.rowcount == 0:
            return None
        return await self.get_evaluation_item(item_id)

    def _aggregate_item_scores(self, completed: list[dict]) -> dict:
        metric_names = (
            "faithfulness",
            "answer_relevancy",
            "context_precision",
            "context_recall",
        )
        aggregate = {}
        for name in metric_names:
            values = [
                item["scores"].get(name)
                for item in completed
                if item["scores"] and item["scores"].get(name) is not None
            ]
            aggregate[name] = round(sum(values) / len(values), 4) if values else None
        return aggregate

    async def finalize_evaluation_if_terminal(self, eval_id: str) -> bool:
        items = await self.list_evaluation_items(eval_id)
        if not items:
            return False
        if any(item["status"] in {"queued", "running"} for item in items):
            return False

        completed = [item for item in items if item["status"] == "completed"]
        failed = [item for item in items if item["status"] == "failed"]
        now = datetime.now(_UTC).isoformat()
        if completed:
            status = "completed" if not failed else "completed_with_failures"
            aggregate = self._aggregate_item_scores(completed)
            results = [
                item["result"] | {"scores": item["scores"]} for item in completed
            ]
            error = None if not failed else f"failed_items={len(failed)}"
            await self._db.execute(
                "UPDATE evaluations "
                "SET status = ?, aggregate_scores = ?, results = ?, error = ?, "
                "completed_at = ? WHERE id = ? AND status NOT IN "
                "('completed', 'completed_with_failures', 'failed', 'cancelled')",
                (
                    status,
                    json.dumps(aggregate),
                    json.dumps(results),
                    error,
                    now,
                    eval_id,
                ),
            )
        else:
            await self._db.execute(
                "UPDATE evaluations "
                "SET status = 'failed', error = ?, completed_at = ? "
                "WHERE id = ? AND status NOT IN ('failed', 'cancelled')",
                ("all evaluation items failed", now, eval_id),
            )
        await self._db.commit()
        return True

    async def count_evaluation_items_by_status(
        self, eval_id: str, stale_seconds: float = 900.0
    ) -> dict[str, int]:
        cursor = await self._db.execute(
            "SELECT status, COUNT(*) AS count FROM evaluation_items "
            "WHERE evaluation_id = ? GROUP BY status",
            (eval_id,),
        )
        rows = await cursor.fetchall()
        counts = {row["status"]: row["count"] for row in rows}
        stale_cutoff = (
            datetime.now(_UTC) - timedelta(seconds=stale_seconds)
        ).isoformat()
        retryable_cursor = await self._db.execute(
            "SELECT COUNT(*) AS count FROM evaluation_items "
            "WHERE evaluation_id = ? "
            "AND status IN ('queued', 'running', 'failed') "
            "AND attempt_count < max_attempts",
            (eval_id,),
        )
        stale_cursor = await self._db.execute(
            "SELECT COUNT(*) AS count FROM evaluation_items "
            "WHERE evaluation_id = ? "
            "AND status IN ('queued', 'running') "
            "AND (updated_at < ? OR lease_expires_at < ?)",
            (eval_id, stale_cutoff, datetime.now(_UTC).isoformat()),
        )
        retryable_row = await retryable_cursor.fetchone()
        stale_row = await stale_cursor.fetchone()
        counts["retryable"] = int(retryable_row["count"])
        counts["stale"] = int(stale_row["count"])
        return counts

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

    async def reset_expired_running_items(self, max_age_seconds: float) -> list[dict]:
        now = datetime.now(_UTC)
        cutoff = (now - timedelta(seconds=max_age_seconds)).isoformat()
        cursor = await self._db.execute(
            "SELECT * FROM evaluation_items "
            "WHERE status = 'running' AND lease_expires_at < ? "
            "AND attempt_count < max_attempts "
            "ORDER BY updated_at ASC",
            (cutoff,),
        )
        rows = await cursor.fetchall()
        items = [self._item_row_to_dict(row) for row in rows]
        await self._db.execute(
            "UPDATE evaluation_items "
            "SET status = 'queued', lease_owner = NULL, lease_expires_at = NULL, "
            "updated_at = ? "
            "WHERE status = 'running' AND lease_expires_at < ? "
            "AND attempt_count < max_attempts",
            (now.isoformat(), cutoff),
        )
        await self._db.commit()
        return items

    async def fail_expired_running_items(self, max_age_seconds: float) -> list[dict]:
        now = datetime.now(_UTC)
        cutoff = (now - timedelta(seconds=max_age_seconds)).isoformat()
        cursor = await self._db.execute(
            "SELECT * FROM evaluation_items "
            "WHERE status = 'running' AND lease_expires_at < ? "
            "AND attempt_count >= max_attempts "
            "ORDER BY updated_at ASC",
            (cutoff,),
        )
        rows = await cursor.fetchall()
        items = [self._item_row_to_dict(row) for row in rows]
        error = json.dumps({"error_type": "stale_item_lease", "retryable": False})
        await self._db.execute(
            "UPDATE evaluation_items "
            "SET status = 'failed', last_error = ?, lease_owner = NULL, "
            "lease_expires_at = NULL, completed_at = ?, updated_at = ? "
            "WHERE status = 'running' AND lease_expires_at < ? "
            "AND attempt_count >= max_attempts",
            (error, now.isoformat(), now.isoformat(), cutoff),
        )
        await self._db.commit()
        return items

    async def list_queued_items_for_republish(
        self, max_age_seconds: float
    ) -> list[dict]:
        cutoff = (datetime.now(_UTC) - timedelta(seconds=max_age_seconds)).isoformat()
        cursor = await self._db.execute(
            "SELECT i.* FROM evaluation_items i "
            "JOIN evaluations e ON e.id = i.evaluation_id "
            "WHERE i.status = 'queued' AND i.updated_at < ? "
            "AND e.status NOT IN "
            "('completed', 'completed_with_failures', 'failed', 'cancelled') "
            "ORDER BY i.updated_at ASC",
            (cutoff,),
        )
        rows = await cursor.fetchall()
        return [self._item_row_to_dict(row) for row in rows]
