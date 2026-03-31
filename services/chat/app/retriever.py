from qdrant_client import QdrantClient


class QdrantRetriever:
    def __init__(self, host: str, port: int, collection_name: str):
        self.client = QdrantClient(host=host, port=port)
        self.collection_name = collection_name

    def search(self, query_vector: list[float], top_k: int = 5) -> list[dict]:
        results = self.client.search(
            collection_name=self.collection_name,
            query_vector=query_vector,
            limit=top_k,
        )

        return [
            {
                "text": hit.payload["text"],
                "page_number": hit.payload["page_number"],
                "filename": hit.payload["filename"],
                "document_id": hit.payload["document_id"],
                "score": hit.score,
            }
            for hit in results
        ]
