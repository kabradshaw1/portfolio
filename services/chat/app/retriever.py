import time
from dataclasses import dataclass

from qdrant_client import QdrantClient
from qdrant_client.models import Fusion, FusionQuery, Prefetch, SparseVector

from app.config import settings
from app.metrics import QDRANT_SEARCH_DURATION, QDRANT_SEARCH_RESULTS


@dataclass(frozen=True)
class RetrievalResult:
    chunks: list[dict]
    metadata: dict


class QdrantRetriever:
    def __init__(self, host: str, port: int, collection_name: str):
        self.client = QdrantClient(host=host, port=port)
        self.collection_name = collection_name

    def _format_results(self, results) -> list[dict]:
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

    def _record_metrics(self, start: float, result_count: int) -> None:
        QDRANT_SEARCH_DURATION.labels(collection=self.collection_name).observe(
            time.perf_counter() - start
        )
        QDRANT_SEARCH_RESULTS.labels(collection=self.collection_name).observe(
            result_count
        )

    def search_semantic(
        self,
        query_vector: list[float],
        top_k: int = 5,
        *,
        legacy_vector: bool = False,
        fallback: bool = False,
    ) -> RetrievalResult:
        start = time.perf_counter()
        using = None if legacy_vector else settings.dense_vector_name
        response = self.client.query_points(
            collection_name=self.collection_name,
            query=query_vector,
            using=using,
            limit=top_k,
            with_payload=True,
        )
        results = response.points
        self._record_metrics(start, len(results))

        return RetrievalResult(
            chunks=self._format_results(results),
            metadata={
                "retrieval_mode": "semantic",
                "retrieval_fallback": fallback,
                "fusion": None,
            },
        )

    def search_hybrid(
        self,
        query_vector: list[float],
        sparse_vector: SparseVector,
        top_k: int = 5,
        prefetch_limit: int | None = None,
    ) -> RetrievalResult:
        start = time.perf_counter()
        effective_prefetch_limit = prefetch_limit or settings.hybrid_prefetch_limit
        response = self.client.query_points(
            collection_name=self.collection_name,
            prefetch=[
                Prefetch(
                    query=query_vector,
                    using=settings.dense_vector_name,
                    limit=effective_prefetch_limit,
                ),
                Prefetch(
                    query=sparse_vector,
                    using=settings.sparse_vector_name,
                    limit=effective_prefetch_limit,
                ),
            ],
            query=FusionQuery(fusion=Fusion.RRF),
            limit=top_k,
            with_payload=True,
        )
        results = response.points
        self._record_metrics(start, len(results))

        return RetrievalResult(
            chunks=self._format_results(results),
            metadata={
                "retrieval_mode": "hybrid",
                "retrieval_fallback": False,
                "fusion": "rrf",
            },
        )

    def search(self, query_vector: list[float], top_k: int = 5) -> list[dict]:
        return self.search_semantic(query_vector=query_vector, top_k=top_k).chunks
