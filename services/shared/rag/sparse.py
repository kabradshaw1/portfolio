from __future__ import annotations

from collections.abc import Sequence
from typing import Any

from fastembed import SparseTextEmbedding
from qdrant_client.models import SparseVector


def _as_list(value: Any) -> list:
    if hasattr(value, "tolist"):
        return value.tolist()
    return list(value)


def normalize_sparse_vector(raw: Any) -> SparseVector:
    return SparseVector(
        indices=[int(index) for index in _as_list(raw.indices)],
        values=[float(value) for value in _as_list(raw.values)],
    )


class SparseVectorEncoder:
    def __init__(self, model_name: str = "Qdrant/bm25", batch_size: int = 256):
        self.model_name = model_name
        self.batch_size = batch_size
        self._model: SparseTextEmbedding | None = None

    def _get_model(self) -> SparseTextEmbedding:
        if self._model is None:
            self._model = SparseTextEmbedding(
                model_name=self.model_name,
            )
        return self._model

    def embed(self, texts: Sequence[str]) -> list[SparseVector]:
        if not texts:
            return []
        return [
            normalize_sparse_vector(vector)
            for vector in self._get_model().embed(
                list(texts),
                batch_size=self.batch_size,
            )
        ]
