from __future__ import annotations

import json

import aiosqlite


class CollectionMetaDB:
    """Per-collection metadata store backed by SQLite.

    Qdrant collections do not carry arbitrary metadata, so we keep our own
    record of the chunk params and embedding model that produced each
    collection. The eval service reads this back at run start to snapshot
    what RAG configuration the evaluation was run against.
    """

    def __init__(self, db_path: str):
        self.db_path = db_path
        self._db: aiosqlite.Connection | None = None

    async def init(self) -> None:
        self._db = await aiosqlite.connect(self.db_path)
        self._db.row_factory = aiosqlite.Row
        await self._db.execute(
            """
            CREATE TABLE IF NOT EXISTS collection_meta (
                collection TEXT PRIMARY KEY,
                chunk_size INTEGER NOT NULL,
                chunk_overlap INTEGER NOT NULL,
                embedding_model TEXT NOT NULL,
                hybrid_enabled INTEGER NOT NULL DEFAULT 1,
                dense_vector_name TEXT NOT NULL DEFAULT 'dense',
                sparse_vector_name TEXT NOT NULL DEFAULT 'sparse',
                sparse_model TEXT
            )
            """
        )
        existing_columns = await self._db.execute("PRAGMA table_info(collection_meta)")
        column_names = {row["name"] for row in await existing_columns.fetchall()}
        for column_name, definition in {
            "hybrid_enabled": "INTEGER NOT NULL DEFAULT 1",
            "dense_vector_name": "TEXT NOT NULL DEFAULT 'dense'",
            "sparse_vector_name": "TEXT NOT NULL DEFAULT 'sparse'",
            "sparse_model": "TEXT",
        }.items():
            if column_name not in column_names:
                await self._db.execute(
                    f"ALTER TABLE collection_meta ADD COLUMN {column_name} {definition}"
                )
        await self._db.execute(
            """
            CREATE TABLE IF NOT EXISTS collection_manifests (
                collection TEXT PRIMARY KEY,
                manifest_json TEXT NOT NULL
            )
            """
        )
        await self._db.commit()

    async def close(self) -> None:
        if self._db:
            await self._db.close()
            self._db = None

    async def upsert(
        self,
        collection: str,
        chunk_size: int,
        chunk_overlap: int,
        embedding_model: str,
        sparse_model: str | None = None,
        hybrid_enabled: bool = True,
    ) -> None:
        await self._db.execute(
            "INSERT INTO collection_meta "
            "(collection, chunk_size, chunk_overlap, embedding_model, "
            "hybrid_enabled, dense_vector_name, sparse_vector_name, sparse_model) "
            "VALUES (?, ?, ?, ?, ?, ?, ?, ?) "
            "ON CONFLICT(collection) DO UPDATE SET "
            "chunk_size=excluded.chunk_size, "
            "chunk_overlap=excluded.chunk_overlap, "
            "embedding_model=excluded.embedding_model, "
            "hybrid_enabled=excluded.hybrid_enabled, "
            "dense_vector_name=excluded.dense_vector_name, "
            "sparse_vector_name=excluded.sparse_vector_name, "
            "sparse_model=excluded.sparse_model",
            (
                collection,
                chunk_size,
                chunk_overlap,
                embedding_model,
                int(hybrid_enabled),
                "dense",
                "sparse",
                sparse_model,
            ),
        )
        await self._db.commit()

    async def get(self, collection: str) -> dict | None:
        cursor = await self._db.execute(
            "SELECT chunk_size, chunk_overlap, embedding_model, hybrid_enabled, "
            "dense_vector_name, sparse_vector_name, sparse_model "
            "FROM collection_meta WHERE collection = ?",
            (collection,),
        )
        row = await cursor.fetchone()
        if not row:
            return None
        config = {
            "chunk_size": row["chunk_size"],
            "chunk_overlap": row["chunk_overlap"],
            "embedding_model": row["embedding_model"],
        }
        if row["sparse_model"] is not None:
            config.update(
                {
                    "hybrid_enabled": bool(row["hybrid_enabled"]),
                    "dense_vector_name": row["dense_vector_name"],
                    "sparse_vector_name": row["sparse_vector_name"],
                    "sparse_model": row["sparse_model"],
                }
            )
        return config

    async def upsert_manifest(self, collection: str, manifest: dict) -> None:
        await self._db.execute(
            "INSERT INTO collection_manifests (collection, manifest_json) "
            "VALUES (?, ?) "
            "ON CONFLICT(collection) DO UPDATE SET "
            "manifest_json=excluded.manifest_json",
            (collection, json.dumps(manifest, sort_keys=True)),
        )
        await self._db.commit()

    async def get_manifest(self, collection: str) -> dict | None:
        cursor = await self._db.execute(
            "SELECT manifest_json FROM collection_manifests WHERE collection = ?",
            (collection,),
        )
        row = await cursor.fetchone()
        if not row:
            return None
        return json.loads(row["manifest_json"])
