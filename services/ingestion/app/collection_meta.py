from __future__ import annotations

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
                embedding_model TEXT NOT NULL
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
    ) -> None:
        await self._db.execute(
            "INSERT INTO collection_meta "
            "(collection, chunk_size, chunk_overlap, embedding_model) "
            "VALUES (?, ?, ?, ?) "
            "ON CONFLICT(collection) DO UPDATE SET "
            "chunk_size=excluded.chunk_size, "
            "chunk_overlap=excluded.chunk_overlap, "
            "embedding_model=excluded.embedding_model",
            (collection, chunk_size, chunk_overlap, embedding_model),
        )
        await self._db.commit()

    async def get(self, collection: str) -> dict | None:
        cursor = await self._db.execute(
            "SELECT chunk_size, chunk_overlap, embedding_model "
            "FROM collection_meta WHERE collection = ?",
            (collection,),
        )
        row = await cursor.fetchone()
        if not row:
            return None
        return {
            "chunk_size": row["chunk_size"],
            "chunk_overlap": row["chunk_overlap"],
            "embedding_model": row["embedding_model"],
        }
