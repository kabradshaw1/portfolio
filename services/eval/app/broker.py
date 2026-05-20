from __future__ import annotations

import json
from dataclasses import dataclass
from typing import TYPE_CHECKING, Any

if TYPE_CHECKING:
    from aio_pika.abc import AbstractQueue


@dataclass(frozen=True)
class EvalItemMessage:
    evaluation_id: str
    item_id: str
    item_index: int
    attempt: int
    message_version: int = 1


def encode_eval_item_message(message: EvalItemMessage) -> bytes:
    return json.dumps(
        {
            "message_version": message.message_version,
            "evaluation_id": message.evaluation_id,
            "item_id": message.item_id,
            "item_index": message.item_index,
            "attempt": message.attempt,
        }
    ).encode("utf-8")


def decode_eval_item_message(body: bytes) -> EvalItemMessage:
    payload = json.loads(body.decode("utf-8"))
    return EvalItemMessage(
        message_version=int(payload["message_version"]),
        evaluation_id=str(payload["evaluation_id"]),
        item_id=str(payload["item_id"]),
        item_index=int(payload["item_index"]),
        attempt=int(payload["attempt"]),
    )


class EvalItemPublisher:
    def __init__(self, rabbitmq_url: str, queue_name: str, dlq_name: str):
        self.rabbitmq_url = rabbitmq_url
        self.queue_name = queue_name
        self.dlq_name = dlq_name
        self._conn: Any | None = None
        self._channel: Any | None = None

    async def connect(self) -> None:
        import aio_pika

        self._conn = await aio_pika.connect_robust(self.rabbitmq_url)
        self._channel = await self._conn.channel(publisher_confirms=True)
        await self._channel.declare_queue(self.dlq_name, durable=True)
        await self._channel.declare_queue(
            self.queue_name,
            durable=True,
            arguments={
                "x-dead-letter-exchange": "",
                "x-dead-letter-routing-key": self.dlq_name,
            },
        )

    async def publish(self, message: EvalItemMessage) -> None:
        import aio_pika

        if self._channel is None:
            await self.connect()
        assert self._channel is not None
        await self._channel.default_exchange.publish(
            aio_pika.Message(
                body=encode_eval_item_message(message),
                delivery_mode=aio_pika.DeliveryMode.PERSISTENT,
                content_type="application/json",
            ),
            routing_key=self.queue_name,
        )

    async def close(self) -> None:
        if self._conn is not None:
            await self._conn.close()


class EvalItemConsumer:
    def __init__(
        self, rabbitmq_url: str, queue_name: str, dlq_name: str, prefetch: int
    ):
        self.rabbitmq_url = rabbitmq_url
        self.queue_name = queue_name
        self.dlq_name = dlq_name
        self.prefetch = prefetch
        self._conn: Any | None = None
        self._channel: Any | None = None

    async def connect(self) -> AbstractQueue:
        import aio_pika

        self._conn = await aio_pika.connect_robust(self.rabbitmq_url)
        self._channel = await self._conn.channel()
        await self._channel.set_qos(prefetch_count=self.prefetch)
        await self._channel.declare_queue(self.dlq_name, durable=True)
        return await self._channel.declare_queue(
            self.queue_name,
            durable=True,
            arguments={
                "x-dead-letter-exchange": "",
                "x-dead-letter-routing-key": self.dlq_name,
            },
        )

    async def close(self) -> None:
        if self._conn is not None:
            await self._conn.close()
