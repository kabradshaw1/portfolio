import pytest
from app.evaluator import EvalRunContext, JudgeScores, evaluate_item


class FakeRAGClient:
    async def search(self, query, collection, limit, rerank=False):
        assert query == "What is RAG?"
        assert collection == "documents"
        assert limit == 3
        assert rerank is True
        return [{"text": "RAG combines retrieval with generation."}]

    async def ask(
        self,
        question,
        collection,
        rerank=False,
        retrieval_config=None,
        answer_model=None,
    ):
        assert retrieval_config == {"top_k": 3}
        return {
            "answer": "RAG combines retrieval with generation.",
            "retrieval": {"retrieval_mode": "hybrid"},
            "usage": {"answer_model": "qwen"},
        }


@pytest.mark.asyncio
async def test_evaluate_item_returns_result_scores_and_reasons():
    async def judge(row):
        assert row["user_input"] == "What is RAG?"
        return JudgeScores(
            faithfulness=1.0,
            answer_relevancy=0.9,
            reasons={"faithfulness": "supported", "answer_relevancy": "direct"},
        )

    result = await evaluate_item(
        item={
            "query": "What is RAG?",
            "expected_answer": "RAG combines retrieval with generation.",
            "expected_sources": [],
        },
        rag_client=FakeRAGClient(),
        collection="documents",
        rerank=True,
        top_k=3,
        judge=judge,
        run_context=EvalRunContext(
            eval_id="eval-1", collection="documents", requested_rerank=True
        ),
        answer_model=None,
        item_index=0,
    )

    assert result["result"]["query"] == "What is RAG?"
    assert result["result"]["answer"] == "RAG combines retrieval with generation."
    assert result["scores"]["faithfulness"] == 1.0
    assert result["scores"]["answer_relevancy"] == 0.9
    assert result["score_reasons"]["faithfulness"] == "supported"
