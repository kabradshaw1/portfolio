import time
import uuid

from qdrant_client import QdrantClient
from qdrant_client.models import (
    Distance,
    FieldCondition,
    Filter,
    MatchValue,
    PointStruct,
    VectorParams,
)

from app.metrics import QDRANT_OPERATION_DURATION


class QdrantStore:
    def __init__(self, host: str, port: int, collection_name: str):
        self.client = QdrantClient(host=host, port=port)
        self.collection_name = collection_name
        self._ensure_collection()

    def _ensure_collection(self):
        if not self.client.collection_exists(self.collection_name):
            self.client.create_collection(
                collection_name=self.collection_name,
                vectors_config=VectorParams(
                    size=768,
                    distance=Distance.COSINE,
                ),
            )

    def upsert(
        self,
        chunks: list[dict],
        vectors: list[list[float]],
        document_id: str,
        filename: str,
    ) -> None:
        points = [
            PointStruct(
                id=str(uuid.uuid4()),
                vector=vector,
                payload={
                    "text": chunk["text"],
                    "page_number": chunk["page_number"],
                    "chunk_index": chunk["chunk_index"],
                    "document_id": document_id,
                    "filename": filename,
                },
            )
            for chunk, vector in zip(chunks, vectors)
        ]
        start = time.perf_counter()
        self.client.upsert(
            collection_name=self.collection_name,
            points=points,
        )
        QDRANT_OPERATION_DURATION.labels(
            service="ingestion", operation="upsert"
        ).observe(time.perf_counter() - start)

    def list_documents(self) -> list[dict]:
        start = time.perf_counter()
        records, _ = self.client.scroll(
            collection_name=self.collection_name,
            limit=10000,
            with_payload=True,
            with_vectors=False,
        )
        QDRANT_OPERATION_DURATION.labels(
            service="ingestion", operation="scroll"
        ).observe(time.perf_counter() - start)

        docs: dict[str, dict] = {}
        for record in records:
            doc_id = record.payload["document_id"]
            if doc_id not in docs:
                docs[doc_id] = {
                    "document_id": doc_id,
                    "filename": record.payload["filename"],
                    "chunks": 0,
                }
            docs[doc_id]["chunks"] += 1

        return list(docs.values())

    def delete_document(self, document_id: str) -> int:
        records, _ = self.client.scroll(
            collection_name=self.collection_name,
            scroll_filter=Filter(
                must=[
                    FieldCondition(
                        key="document_id",
                        match=MatchValue(value=document_id),
                    )
                ]
            ),
            limit=10000,
            with_payload=True,
            with_vectors=False,
        )

        if not records:
            return 0

        start = time.perf_counter()
        self.client.delete(
            collection_name=self.collection_name,
            points_selector=Filter(
                must=[
                    FieldCondition(
                        key="document_id",
                        match=MatchValue(value=document_id),
                    )
                ]
            ),
        )
        QDRANT_OPERATION_DURATION.labels(
            service="ingestion", operation="delete"
        ).observe(time.perf_counter() - start)
        return len(records)

    def list_collections(self) -> list[dict]:
        """List all Qdrant collections with point counts."""
        response = self.client.get_collections()
        result = []
        for col in response.collections:
            info = self.client.get_collection(col.name)
            result.append(
                {
                    "name": col.name,
                    "point_count": info.points_count,
                }
            )
        return result

    def delete_collection(self, collection_name: str) -> None:
        if not self.client.collection_exists(collection_name):
            raise ValueError(f"Collection {collection_name} not found")
        self.client.delete_collection(collection_name=collection_name)
