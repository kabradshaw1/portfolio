import json
from unittest.mock import AsyncMock, MagicMock, patch

import pytest
from app.evaluator import (
    EvalRunContext,
    EvaluationError,
    JudgeScores,
    build_evaluation_dataset,
    judge_generation_scores,
    parse_judge_scores,
    run_evaluation,
    score_context_precision,
    score_context_recall,
)
from app.rag_client import RAGClient


class FakePermit:
    def __init__(self, events: list[str]):
        self._events = events

    def release(self):
        self._events.append("release")


def _judge_row():
    return {
        "user_input": "What is chunking?",
        "reference": "Splitting text into smaller pieces.",
        "retrieved_contexts": ["Chunking splits documents."],
        "response": "Chunking splits documents into smaller pieces.",
    }


@pytest.fixture
def golden_items():
    return [
        {
            "query": "What is chunking?",
            "expected_answer": "Splitting text into smaller pieces for embedding.",
            "expected_sources": ["ingestion.pdf"],
        },
        {
            "query": "What model is used for embeddings?",
            "expected_answer": "nomic-embed-text produces 768-dimensional vectors.",
            "expected_sources": ["chat.pdf"],
        },
    ]


@pytest.fixture
def mock_search_results():
    return [
        {
            "text": "Text chunking splits documents into smaller pieces.",
            "filename": "ingestion.pdf",
            "page_number": 1,
            "score": 0.92,
        },
        {
            "text": "Chunk sizes of 1000 with 200 overlap are used.",
            "filename": "ingestion.pdf",
            "page_number": 2,
            "score": 0.85,
        },
    ]


@pytest.fixture
def mock_chat_answer():
    return {
        "answer": (
            "Chunking splits text into smaller pieces for embedding and retrieval."
        ),
        "sources": [{"file": "ingestion.pdf", "page": 1}],
    }


@pytest.fixture
def mock_chat_answer_with_retrieval(mock_chat_answer):
    return {
        **mock_chat_answer,
        "retrieval": {
            "retrieval_mode": "hybrid",
            "retrieval_fallback": False,
            "rerank_requested": True,
            "rerank_enabled": True,
            "rerank_applied": True,
            "rerank_fallback": False,
            "rerank_model": "cross-encoder/ms-marco-MiniLM-L6-v2",
            "rerank_candidate_count": 20,
            "rerank_returned_count": 5,
        },
    }


@pytest.mark.asyncio
async def test_build_evaluation_dataset(
    golden_items, mock_search_results, mock_chat_answer
):
    rag_client = MagicMock(spec=RAGClient)
    rag_client.search = AsyncMock(return_value=mock_search_results)
    rag_client.ask = AsyncMock(return_value=mock_chat_answer)

    dataset = await build_evaluation_dataset(
        items=golden_items,
        rag_client=rag_client,
        collection=None,
    )

    assert len(dataset) == 2
    assert dataset[0]["user_input"] == "What is chunking?"
    assert dataset[0]["response"] == (
        "Chunking splits text into smaller pieces for embedding and retrieval."
    )
    assert len(dataset[0]["retrieved_contexts"]) == 2
    assert dataset[0]["reference"] == (
        "Splitting text into smaller pieces for embedding."
    )
    assert dataset[0]["expected_sources"] == ["ingestion.pdf"]

    assert rag_client.search.call_count == 2
    assert rag_client.ask.call_count == 2


@pytest.mark.asyncio
async def test_build_evaluation_dataset_with_collection(
    golden_items, mock_search_results, mock_chat_answer
):
    rag_client = MagicMock(spec=RAGClient)
    rag_client.search = AsyncMock(return_value=mock_search_results)
    rag_client.ask = AsyncMock(return_value=mock_chat_answer)

    await build_evaluation_dataset(
        items=golden_items,
        rag_client=rag_client,
        collection="my-docs",
    )

    call_args = rag_client.search.call_args_list[0]
    assert (
        call_args.kwargs.get("collection") == "my-docs" or call_args[0][1] == "my-docs"
    )


@pytest.mark.asyncio
async def test_build_evaluation_dataset_passes_rerank(
    golden_items, mock_search_results, mock_chat_answer
):
    rag_client = AsyncMock()
    rag_client.search.return_value = mock_search_results
    rag_client.ask.return_value = mock_chat_answer

    await build_evaluation_dataset(
        items=golden_items,
        rag_client=rag_client,
        collection="documents",
        rerank=True,
    )

    assert rag_client.search.call_args_list[0].kwargs["rerank"] is True
    assert rag_client.ask.call_args_list[0].kwargs["rerank"] is True


@pytest.mark.asyncio
async def test_build_evaluation_dataset_uses_effective_top_k_for_search_and_chat(
    golden_items, mock_search_results, mock_chat_answer
):
    rag_client = AsyncMock()
    rag_client.search.return_value = mock_search_results
    rag_client.ask.return_value = mock_chat_answer

    await build_evaluation_dataset(
        items=golden_items,
        rag_client=rag_client,
        collection="documents",
        rerank=False,
        top_k=3,
    )

    assert rag_client.search.call_args_list[0].kwargs["limit"] == 3
    assert rag_client.ask.call_args_list[0].kwargs["retrieval_config"] == {"top_k": 3}


@pytest.mark.asyncio
async def test_build_evaluation_dataset_preserves_retrieval_metadata(
    golden_items, mock_search_results, mock_chat_answer_with_retrieval
):
    rag_client = AsyncMock()
    rag_client.search.return_value = mock_search_results
    rag_client.ask.return_value = mock_chat_answer_with_retrieval

    dataset = await build_evaluation_dataset(
        items=golden_items,
        rag_client=rag_client,
        collection="documents",
        rerank=True,
    )

    assert dataset[0]["retrieval"] == mock_chat_answer_with_retrieval["retrieval"]


@pytest.mark.asyncio
async def test_build_evaluation_dataset_logs_item_lifecycle(
    golden_items, mock_search_results, mock_chat_answer, caplog
):
    rag_client = AsyncMock()
    rag_client.search.return_value = mock_search_results
    rag_client.ask.return_value = mock_chat_answer

    with caplog.at_level("INFO", logger="app.evaluator"):
        await build_evaluation_dataset(
            items=golden_items[:1],
            rag_client=rag_client,
            collection="documents",
            rerank=True,
            run_context=EvalRunContext(
                eval_id="eval-123",
                collection="documents",
                requested_rerank=True,
            ),
        )

    assert "eval_item_start" in caplog.text
    assert "eval_item_completed" in caplog.text
    assert "eval-123" in caplog.text
    assert "documents" in caplog.text


def test_score_context_recall_counts_reference_terms_in_contexts():
    score = score_context_recall(
        reference="Splitting text into smaller pieces for embedding.",
        contexts=[
            "Text chunking splits documents into smaller pieces.",
            "Embedding stores chunks for retrieval.",
        ],
    )

    assert score == pytest.approx(0.8, abs=0.0001)


def test_score_context_precision_averages_context_usefulness():
    score = score_context_precision(
        query="What is chunking?",
        reference="Splitting text into smaller pieces for embedding.",
        contexts=[
            "Text chunking splits documents into smaller pieces.",
            "The deployment uses Kubernetes ingress.",
        ],
    )

    assert score == pytest.approx(0.3333, abs=0.0001)


def test_context_scores_are_zero_for_empty_inputs():
    assert score_context_recall(reference="", contexts=["anything"]) == 0.0
    assert score_context_recall(reference="answer", contexts=[]) == 0.0
    assert (
        score_context_precision(query="question", reference="answer", contexts=[])
        == 0.0
    )


def test_parse_judge_scores_accepts_json_and_clamps_scores():
    scores = parse_judge_scores(
        '{"faithfulness": {"score": 1.2, "reason": "supported"}, '
        '"answer_relevancy": {"score": -0.1, "reason": "off topic"}}'
    )

    assert scores == JudgeScores(
        faithfulness=1.0,
        answer_relevancy=0.0,
        reasons={
            "faithfulness": "supported",
            "answer_relevancy": "off topic",
        },
    )


def test_parse_judge_scores_rejects_malformed_json():
    with pytest.raises(EvaluationError, match="judge returned invalid JSON"):
        parse_judge_scores("not json")


def test_parse_judge_scores_rejects_missing_metric():
    with pytest.raises(EvaluationError, match="missing answer_relevancy"):
        parse_judge_scores('{"faithfulness": {"score": 0.5, "reason": "partial"}}')


@pytest.mark.asyncio
@patch("app.evaluator.get_llm_provider")
async def test_judge_generation_scores_acquires_generation_admission(
    mock_get_provider, monkeypatch
):
    events = []

    async def acquire():
        events.append("acquire")
        return FakePermit(events)

    monkeypatch.setattr("app.evaluator.generate_limiter.acquire", acquire)
    provider = AsyncMock()
    provider.chat.return_value = {
        "message": {
            "content": json.dumps(
                {
                    "faithfulness": {"score": 0.9, "reason": "grounded"},
                    "answer_relevancy": {"score": 0.8, "reason": "answers"},
                }
            )
        }
    }
    mock_get_provider.return_value = provider

    scores = await judge_generation_scores(
        row=_judge_row(),
        provider="ollama",
        base_url="http://ollama",
        model="qwen",
        api_key="",
    )

    assert scores.faithfulness == 0.9
    assert events == ["acquire", "release"]


@pytest.mark.asyncio
async def test_run_evaluation_preserves_result_shape(
    golden_items,
    mock_search_results,
    mock_chat_answer,
):
    rag_client = MagicMock(spec=RAGClient)
    rag_client.search = AsyncMock(return_value=mock_search_results)
    rag_client.ask = AsyncMock(return_value=mock_chat_answer)

    judge = AsyncMock(
        side_effect=[
            JudgeScores(
                faithfulness=0.9,
                answer_relevancy=0.85,
                reasons={
                    "faithfulness": "answer is supported",
                    "answer_relevancy": "answer addresses the question",
                },
            ),
            JudgeScores(
                faithfulness=0.82,
                answer_relevancy=0.9,
                reasons={
                    "faithfulness": "mostly supported",
                    "answer_relevancy": "directly answers",
                },
            ),
        ]
    )

    aggregate, results = await run_evaluation(
        items=golden_items,
        rag_client=rag_client,
        collection=None,
        llm_provider="ollama",
        llm_base_url="http://localhost:11434",
        llm_model="qwen2.5:14b",
        llm_api_key="",
        judge=judge,
    )

    assert aggregate["faithfulness"] == 0.86
    assert aggregate["answer_relevancy"] == 0.875
    assert "context_precision" in aggregate
    assert "context_recall" in aggregate
    assert len(results) == 2
    assert results[0]["query"] == "What is chunking?"
    assert results[0]["scores"]["faithfulness"] == 0.9
    assert results[0]["score_reasons"]["faithfulness"] == "answer is supported"
    assert "retrieval" not in results[0]


@pytest.mark.asyncio
async def test_run_evaluation_persists_retrieval_metadata_in_results(
    golden_items,
    mock_search_results,
    mock_chat_answer_with_retrieval,
):
    rag_client = MagicMock(spec=RAGClient)
    rag_client.search = AsyncMock(return_value=mock_search_results)
    rag_client.ask = AsyncMock(return_value=mock_chat_answer_with_retrieval)
    judge = AsyncMock(
        return_value=JudgeScores(
            faithfulness=1.0,
            answer_relevancy=1.0,
            reasons={
                "faithfulness": "supported",
                "answer_relevancy": "direct",
            },
        )
    )

    aggregate, results = await run_evaluation(
        items=golden_items,
        rag_client=rag_client,
        collection="documents",
        llm_provider="ollama",
        llm_base_url="http://localhost:11434",
        llm_model="qwen2.5:14b",
        llm_api_key="",
        rerank=True,
        judge=judge,
    )

    assert aggregate["faithfulness"] == 1.0
    assert aggregate["answer_relevancy"] == 1.0
    assert results[0]["retrieval"] == mock_chat_answer_with_retrieval["retrieval"]


@pytest.mark.asyncio
async def test_run_evaluation_empty_items_returns_empty_results():
    rag_client = MagicMock(spec=RAGClient)

    aggregate, results = await run_evaluation(
        items=[],
        rag_client=rag_client,
        collection=None,
        llm_provider="ollama",
        llm_base_url="http://localhost:11434",
        llm_model="qwen2.5:14b",
        llm_api_key="",
        judge=AsyncMock(),
    )

    assert aggregate == {
        "faithfulness": None,
        "answer_relevancy": None,
        "context_precision": None,
        "context_recall": None,
    }
    assert results == []
