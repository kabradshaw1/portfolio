from types import SimpleNamespace

from qdrant_client.models import SparseVector
from rag.sparse import SparseVectorEncoder, normalize_sparse_vector


def test_normalize_sparse_vector_converts_numpy_like_values():
    raw = SimpleNamespace(
        indices=SimpleNamespace(tolist=lambda: [2, 7]),
        values=SimpleNamespace(tolist=lambda: [0.5, 1.25]),
    )

    vector = normalize_sparse_vector(raw)

    assert vector == SparseVector(indices=[2, 7], values=[0.5, 1.25])


def test_sparse_vector_encoder_returns_one_vector_per_text(monkeypatch):
    class FakeSparseTextEmbedding:
        def __init__(self, model_name: str):
            self.model_name = model_name

        def embed(self, texts, batch_size: int):
            assert texts == ["RFC 7231", "section 4.2"]
            assert batch_size == 16
            return [
                SimpleNamespace(indices=[1], values=[0.7]),
                SimpleNamespace(indices=[3, 4], values=[0.2, 0.9]),
            ]

    monkeypatch.setattr("rag.sparse.SparseTextEmbedding", FakeSparseTextEmbedding)

    encoder = SparseVectorEncoder(model_name="Qdrant/bm25", batch_size=16)
    vectors = encoder.embed(["RFC 7231", "section 4.2"])

    assert vectors == [
        SparseVector(indices=[1], values=[0.7]),
        SparseVector(indices=[3, 4], values=[0.2, 0.9]),
    ]


def test_sparse_vector_encoder_returns_empty_list_for_empty_input(monkeypatch):
    class FailingSparseTextEmbedding:
        def __init__(self, model_name: str):
            raise AssertionError("model should not load for empty input")

    monkeypatch.setattr("rag.sparse.SparseTextEmbedding", FailingSparseTextEmbedding)

    encoder = SparseVectorEncoder(model_name="Qdrant/bm25", batch_size=16)

    assert encoder.embed([]) == []
