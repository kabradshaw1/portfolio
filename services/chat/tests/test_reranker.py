from unittest.mock import MagicMock

from app.reranker import CrossEncoderReranker, rerank_chunks


def _chunk(text: str, score: float = 0.1) -> dict:
    return {
        "text": text,
        "page_number": 1,
        "filename": "doc.pdf",
        "document_id": "doc",
        "score": score,
    }


def test_rerank_chunks_sorts_by_cross_encoder_score_descending():
    model = MagicMock()
    model.predict.return_value = [0.2, 0.9, 0.4]
    reranker = CrossEncoderReranker(model_loader=lambda: model)

    result = reranker.rerank(
        query="where is the answer?",
        chunks=[_chunk("weak"), _chunk("best"), _chunk("middle")],
        top_k=2,
        model_name="test-model",
    )

    assert [c["text"] for c in result.chunks] == ["best", "middle"]
    assert [c["rerank_score"] for c in result.chunks] == [0.9, 0.4]
    model.predict.assert_called_once_with(
        [
            ("where is the answer?", "weak"),
            ("where is the answer?", "best"),
            ("where is the answer?", "middle"),
        ],
        show_progress_bar=False,
    )


def test_rerank_chunks_preserves_original_order_for_equal_scores():
    model = MagicMock()
    model.predict.return_value = [0.5, 0.5, 0.5]
    reranker = CrossEncoderReranker(model_loader=lambda: model)

    result = reranker.rerank(
        query="same score",
        chunks=[_chunk("first"), _chunk("second"), _chunk("third")],
        top_k=3,
        model_name="test-model",
    )

    assert [c["text"] for c in result.chunks] == ["first", "second", "third"]


def test_rerank_chunks_empty_input_does_not_load_model():
    model_loader = MagicMock()
    reranker = CrossEncoderReranker(model_loader=model_loader)

    result = reranker.rerank(
        query="anything",
        chunks=[],
        top_k=5,
        model_name="test-model",
    )

    assert result.chunks == []
    assert result.metadata == {
        "rerank_applied": False,
        "rerank_model": "test-model",
        "rerank_candidate_count": 0,
        "rerank_returned_count": 0,
    }
    model_loader.assert_not_called()


def test_rerank_chunks_uses_default_singleton_loader(monkeypatch):
    model = MagicMock()
    model.predict.return_value = [0.8]
    loader = MagicMock(return_value=model)
    monkeypatch.setattr("app.reranker.get_cross_encoder", loader)

    result = rerank_chunks(
        query="q",
        chunks=[_chunk("answer")],
        top_k=1,
        model_name="test-model",
    )

    assert result.chunks[0]["text"] == "answer"
    loader.assert_called_once_with("test-model", "cpu")
