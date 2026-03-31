import uuid

from qdrant_client import QdrantClient
from qdrant_client.models import (
    Distance,
    PointStruct,
    VectorParams,
)


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
        self.client.upsert(
            collection_name=self.collection_name,
            points=points,
        )

    def list_documents(self) -> list[dict]:
        records, _ = self.client.scroll(
            collection_name=self.collection_name,
            limit=10000,
            with_payload=True,
            with_vectors=False,
        )

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
