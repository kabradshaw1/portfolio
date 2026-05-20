from __future__ import annotations

import asyncio
import logging
import socket

from app.broker import EvalItemConsumer, EvalItemMessage, decode_eval_item_message
from app.config import settings
from app.db import EvalDB
from app.evaluator import (
    EvalRunContext,
    EvaluationError,
    evaluate_item,
    judge_generation_scores,
)
from app.metrics import (
    eval_item_dlq_total,
    eval_item_messages_total,
    eval_item_retries_total,
)
from app.rag_client import RAGClient

logger = logging.getLogger(__name__)


class RetryableEvalItemError(RuntimeError):
    pass


class PermanentEvalItemError(RuntimeError):
    pass


class CancelledEvaluationError(RuntimeError):
    pass


def classify_item_error(exc: Exception) -> tuple[str, bool]:
    if isinstance(exc, PermanentEvalItemError):
        return (type(exc).__name__, False)
    if isinstance(exc, EvaluationError):
        return (type(exc).__name__, False)
    return (type(exc).__name__, True)


class ItemProcessor:
    def __init__(
        self,
        db: EvalDB,
        evaluate_item_fn=evaluate_item,
        worker_id: str | None = None,
    ):
        self.db = db
        self.evaluate_item_fn = evaluate_item_fn
        self.worker_id = worker_id or socket.gethostname()

    async def _raise_if_cancelled(self, eval_id: str) -> None:
        evaluation = await self.db.get_evaluation(eval_id)
        if evaluation and evaluation.get("status") == "cancelled":
            raise CancelledEvaluationError(f"evaluation {eval_id} is cancelled")

    async def process(self, message: EvalItemMessage) -> None:
        evaluation = await self.db.get_evaluation(message.evaluation_id)
        if evaluation is None:
            await self.db.mark_evaluation_item_failed(
                message.item_id,
                {"error_type": "missing_evaluation", "retryable": False},
            )
            eval_item_dlq_total.labels(reason="missing_evaluation").inc()
            return
        if evaluation.get("status") == "cancelled":
            await self.db.mark_evaluation_item_cancelled(message.item_id)
            eval_item_messages_total.labels(outcome="cancelled").inc()
            return

        item = await self.db.claim_evaluation_item(
            message.item_id,
            worker_id=self.worker_id,
            lease_seconds=settings.eval_item_lease_seconds,
        )
        if item is None:
            eval_item_messages_total.labels(outcome="duplicate_or_terminal").inc()
            return
        evaluation = await self.db.get_evaluation(message.evaluation_id)
        if evaluation.get("status") == "cancelled":
            await self.db.mark_evaluation_item_cancelled(message.item_id)
            eval_item_messages_total.labels(outcome="cancelled").inc()
            return

        await self.db.mark_evaluation_running(message.evaluation_id)
        rag_client = RAGClient(
            base_url=settings.chat_service_url,
            internal_token=settings.rag_internal_eval_token,
        )
        try:
            top_k = (
                (evaluation.get("config") or {})
                .get("effective_retrieval_config", {})
                .get("top_k", 5)
            )

            async def judge(row: dict):
                return await judge_generation_scores(
                    row=row,
                    provider=settings.llm_provider,
                    base_url=settings.llm_base_url,
                    model=settings.llm_model,
                    api_key=settings.llm_api_key,
                )

            evaluated = await self.evaluate_item_fn(
                item=item,
                rag_client=rag_client,
                collection=evaluation["collection"],
                rerank=False,
                top_k=top_k,
                judge=judge,
                run_context=EvalRunContext(
                    eval_id=message.evaluation_id,
                    collection=evaluation["collection"],
                    requested_rerank=False,
                ),
                answer_model=None,
                item_index=message.item_index,
                check_cancelled=lambda: self._raise_if_cancelled(message.evaluation_id),
            )
            await self.db.mark_evaluation_item_completed(
                message.item_id,
                result=evaluated["result"],
                scores=evaluated["scores"],
                score_reasons=evaluated["score_reasons"],
            )
            eval_item_messages_total.labels(outcome="completed").inc()
        except CancelledEvaluationError:
            await self.db.mark_evaluation_item_cancelled(message.item_id)
            eval_item_messages_total.labels(outcome="cancelled").inc()
        except Exception as exc:
            error_type, retryable = classify_item_error(exc)
            attempts = item["attempt_count"]
            max_attempts = item["max_attempts"]
            if retryable and attempts < max_attempts:
                await self.db.release_evaluation_item_for_retry(
                    message.item_id,
                    {"error_type": error_type, "retryable": True},
                )
                eval_item_retries_total.labels(reason=error_type).inc()
                raise RetryableEvalItemError(str(exc)) from exc
            await self.db.mark_evaluation_item_failed(
                message.item_id,
                {"error_type": error_type, "retryable": False},
            )
            eval_item_dlq_total.labels(reason=error_type).inc()
        finally:
            await rag_client.close()

        await self.db.finalize_evaluation_if_terminal(message.evaluation_id)


async def run_worker() -> None:
    db = EvalDB(settings.db_path)
    await db.init()
    consumer = EvalItemConsumer(
        rabbitmq_url=settings.rabbitmq_url,
        queue_name=settings.eval_item_queue,
        dlq_name=settings.eval_item_dlq,
        prefetch=settings.eval_worker_prefetch,
    )
    queue = await consumer.connect()
    processor = ItemProcessor(db=db)
    semaphore = asyncio.Semaphore(settings.eval_worker_concurrency)

    async def handle(message):
        async with semaphore:
            try:
                decoded = decode_eval_item_message(message.body)
                await processor.process(decoded)
            except RetryableEvalItemError:
                await message.nack(requeue=True)
                return
            except Exception:
                logger.exception("eval_item_message_failed")
                await message.reject(requeue=False)
                return
            await message.ack()

    try:
        await queue.consume(handle)
        await asyncio.Future()
    finally:
        await consumer.close()
        await db.close()


def main() -> None:
    if not settings.rabbitmq_url:
        raise RuntimeError("RABBITMQ_URL is required for eval worker")
    asyncio.run(run_worker())


if __name__ == "__main__":
    main()
