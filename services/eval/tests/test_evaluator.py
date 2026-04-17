from unittest.mock import AsyncMock, MagicMock, patch

import pytest
from app.evaluator import build_ragas_dataset, run_evaluation
from app.rag_client import RAGClient


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


@pytest.mark.asyncio
async def test_build_ragas_dataset(golden_items, mock_search_results, mock_chat_answer):
    rag_client = MagicMock(spec=RAGClient)
    rag_client.search = AsyncMock(return_value=mock_search_results)
    rag_client.ask = AsyncMock(return_value=mock_chat_answer)

    dataset = await build_ragas_dataset(
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

    assert rag_client.search.call_count == 2
    assert rag_client.ask.call_count == 2


@pytest.mark.asyncio
async def test_build_ragas_dataset_with_collection(
    golden_items, mock_search_results, mock_chat_answer
):
    rag_client = MagicMock(spec=RAGClient)
    rag_client.search = AsyncMock(return_value=mock_search_results)
    rag_client.ask = AsyncMock(return_value=mock_chat_answer)

    await build_ragas_dataset(
        items=golden_items,
        rag_client=rag_client,
        collection="my-docs",
    )

    call_args = rag_client.search.call_args_list[0]
    assert (
        call_args.kwargs.get("collection") == "my-docs" or call_args[0][1] == "my-docs"
    )


@pytest.mark.asyncio
@patch("app.evaluator._create_llm")
@patch("ragas.evaluate")
async def test_run_evaluation(
    mock_ragas_evaluate,
    mock_create_llm,
    golden_items,
    mock_search_results,
    mock_chat_answer,
):
    rag_client = MagicMock(spec=RAGClient)
    rag_client.search = AsyncMock(return_value=mock_search_results)
    rag_client.ask = AsyncMock(return_value=mock_chat_answer)

    # Mock RAGAS evaluate to return fake scores
    mock_result = MagicMock()
    mock_result.scores = [
        {
            "faithfulness": 0.9,
            "answer_relevancy": 0.85,
            "context_precision": 0.8,
            "context_recall": 0.88,
        },
        {
            "faithfulness": 0.82,
            "answer_relevancy": 0.9,
            "context_precision": 0.75,
            "context_recall": 0.8,
        },
    ]
    mock_ragas_evaluate.return_value = mock_result

    aggregate, results = await run_evaluation(
        items=golden_items,
        rag_client=rag_client,
        collection=None,
        llm_provider="ollama",
        llm_base_url="http://localhost:11434",
        llm_model="qwen2.5:14b",
        llm_api_key="",
    )

    assert "faithfulness" in aggregate
    assert "answer_relevancy" in aggregate
    assert len(results) == 2
    assert results[0]["query"] == "What is chunking?"
    assert results[0]["scores"]["faithfulness"] == 0.9

    mock_ragas_evaluate.assert_called_once()
