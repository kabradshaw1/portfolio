import pytest
from app.broker import EvalItemMessage
from app.worker import ItemProcessor, RetryableEvalItemError


class FakeDB:
    def __init__(self):
        self.item = {
            "id": "item-1",
            "evaluation_id": "eval-1",
            "item_index": 0,
            "query": "q",
            "expected_answer": "a",
            "expected_sources": [],
            "attempt_count": 0,
            "max_attempts": 3,
        }
        self.completed = None
        self.failed = None
        self.finalized = False

    async def claim_evaluation_item(self, item_id, worker_id, lease_seconds):
        assert item_id == "item-1"
        return self.item

    async def get_evaluation(self, eval_id):
        return {
            "id": eval_id,
            "collection": "documents",
            "config": {"effective_retrieval_config": {"top_k": 5}},
        }

    async def mark_evaluation_running(self, eval_id):
        self.running = eval_id

    async def mark_evaluation_item_completed(
        self, item_id, result, scores, score_reasons
    ):
        self.completed = (item_id, result, scores, score_reasons)

    async def mark_evaluation_item_failed(self, item_id, error):
        self.failed = (item_id, error)

    async def release_evaluation_item_for_retry(self, item_id, error):
        self.released = (item_id, error)

    async def finalize_evaluation_if_terminal(self, eval_id):
        self.finalized = True
        return True


@pytest.mark.asyncio
async def test_item_processor_completes_claimed_item():
    db = FakeDB()

    async def evaluate_item(**kwargs):
        return {
            "result": {"query": "q", "answer": "a", "contexts": []},
            "scores": {"faithfulness": 1.0},
            "score_reasons": {"faithfulness": "ok"},
        }

    processor = ItemProcessor(db=db, evaluate_item_fn=evaluate_item, worker_id="w1")

    await processor.process(
        EvalItemMessage(
            evaluation_id="eval-1",
            item_id="item-1",
            item_index=0,
            attempt=1,
        )
    )

    assert db.completed[0] == "item-1"
    assert db.finalized is True
    assert db.failed is None


@pytest.mark.asyncio
async def test_item_processor_marks_failed_after_retry_exhaustion():
    db = FakeDB()
    db.item["attempt_count"] = 3
    db.item["max_attempts"] = 3

    async def evaluate_item(**kwargs):
        raise RetryableEvalItemError("chat timeout")

    processor = ItemProcessor(db=db, evaluate_item_fn=evaluate_item, worker_id="w1")

    await processor.process(
        EvalItemMessage(
            evaluation_id="eval-1",
            item_id="item-1",
            item_index=0,
            attempt=3,
        )
    )

    assert db.failed[0] == "item-1"
    assert db.failed[1]["retryable"] is False


@pytest.mark.asyncio
async def test_item_processor_releases_retryable_item_before_requeue():
    db = FakeDB()
    db.item["attempt_count"] = 1
    db.item["max_attempts"] = 3

    async def evaluate_item(**kwargs):
        raise TimeoutError("chat timeout")

    processor = ItemProcessor(db=db, evaluate_item_fn=evaluate_item, worker_id="w1")

    with pytest.raises(RetryableEvalItemError):
        await processor.process(
            EvalItemMessage(
                evaluation_id="eval-1", item_id="item-1", item_index=0, attempt=1
            )
        )

    assert db.released[0] == "item-1"
    assert db.released[1]["retryable"] is True
