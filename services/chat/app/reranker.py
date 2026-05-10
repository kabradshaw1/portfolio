from __future__ import annotations

import time
from collections.abc import Callable
from dataclasses import dataclass
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from sentence_transformers import CrossEncoder

from app.metrics import RERANK_CANDIDATES, RERANK_DURATION


@dataclass(frozen=True)
class RerankResult:
    chunks: list[dict]
    metadata: dict


_models: dict[tuple[str, str], CrossEncoder] = {}


def get_cross_encoder(model_name: str, device: str) -> CrossEncoder:
    from sentence_transformers import CrossEncoder

    key = (model_name, device)
    if key not in _models:
        _models[key] = CrossEncoder(model_name, device=device)
    return _models[key]


class CrossEncoderReranker:
    def __init__(self, model_loader: Callable[[], object]):
        self._model_loader = model_loader

    def rerank(
        self,
        query: str,
        chunks: list[dict],
        top_k: int,
        model_name: str,
    ) -> RerankResult:
        if not chunks:
            return RerankResult(
                chunks=[],
                metadata={
                    "rerank_applied": False,
                    "rerank_model": model_name,
                    "rerank_candidate_count": 0,
                    "rerank_returned_count": 0,
                },
            )

        start = time.perf_counter()
        model = self._model_loader()
        pairs = [(query, chunk["text"]) for chunk in chunks]
        raw_scores = model.predict(pairs, show_progress_bar=False)
        scores = [float(score) for score in raw_scores]
        ranked = sorted(
            enumerate(zip(chunks, scores, strict=True)),
            key=lambda item: (-item[1][1], item[0]),
        )
        reranked = []
        for _, (chunk, score) in ranked[:top_k]:
            updated = dict(chunk)
            updated["rerank_score"] = score
            reranked.append(updated)

        RERANK_DURATION.labels(model=model_name, outcome="applied").observe(
            time.perf_counter() - start
        )
        RERANK_CANDIDATES.labels(model=model_name).observe(len(chunks))

        return RerankResult(
            chunks=reranked,
            metadata={
                "rerank_applied": True,
                "rerank_model": model_name,
                "rerank_candidate_count": len(chunks),
                "rerank_returned_count": len(reranked),
            },
        )


def rerank_chunks(
    query: str,
    chunks: list[dict],
    top_k: int,
    model_name: str,
    device: str = "cpu",
) -> RerankResult:
    reranker = CrossEncoderReranker(
        model_loader=lambda: get_cross_encoder(model_name, device)
    )
    return reranker.rerank(
        query=query,
        chunks=chunks,
        top_k=top_k,
        model_name=model_name,
    )
