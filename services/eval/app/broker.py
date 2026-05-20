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


@dataclass(frozen=True)
class DLQRoutingMetadata:
    exchange: str
    routing_key: str
    queue: str
    death_count: int
    death_reason: str


@dataclass(frozen=True)
class DLQEntry:
    index: int
    delivery_tag: str
    redelivered: bool
    payload: dict[str, Any] | None
    routing: DLQRoutingMetadata
    invalid_payload: str | None = None


def safe_x_death_metadata(headers: dict[str, Any], dlq_name: str) -> DLQRoutingMetadata:
    deaths = headers.get("x-death") or []
    first = deaths[0] if deaths else {}
    routing_keys = first.get("routing-keys") or []
    routing_key = str(routing_keys[0]) if routing_keys else ""
    return DLQRoutingMetadata(
        exchange=str(first.get("exchange") or ""),
        routing_key=routing_key,
        queue=dlq_name,
        death_count=int(first.get("count") or 0),
        death_reason=str(first.get("reason") or ""),
    )


def _safe_payload_dict(decoded: EvalItemMessage) -> dict[str, Any]:
    return {
        "message_version": decoded.message_version,
        "evaluation_id": decoded.evaluation_id,
        "item_id": decoded.item_id,
        "item_index": decoded.item_index,
        "attempt": decoded.attempt,
    }


def build_dlq_entry(index: int, message: Any, dlq_name: str) -> DLQEntry:
    routing = safe_x_death_metadata(getattr(message, "headers", {}) or {}, dlq_name)
    try:
        decoded = decode_eval_item_message(message.body)
    except json.JSONDecodeError:
        return DLQEntry(
            index=index,
            delivery_tag=str(getattr(message, "delivery_tag", "")),
            redelivered=bool(getattr(message, "redelivered", False)),
            payload=None,
            routing=routing,
            invalid_payload="invalid_json",
        )
    except (KeyError, TypeError, ValueError):
        return DLQEntry(
            index=index,
            delivery_tag=str(getattr(message, "delivery_tag", "")),
            redelivered=bool(getattr(message, "redelivered", False)),
            payload=None,
            routing=routing,
            invalid_payload="invalid_schema",
        )
    return DLQEntry(
        index=index,
        delivery_tag=str(getattr(message, "delivery_tag", "")),
        redelivered=bool(getattr(message, "redelivered", False)),
        payload=_safe_payload_dict(decoded),
        routing=routing,
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


@dataclass(frozen=True)
class TakenDLQMessage:
    entry: DLQEntry
    message: Any


class EvalItemDLQClient:
    def __init__(self, rabbitmq_url: str, dlq_name: str):
        self.rabbitmq_url = rabbitmq_url
        self.dlq_name = dlq_name
        self._conn: Any | None = None
        self._channel: Any | None = None

    async def connect(self) -> None:
        import aio_pika

        self._conn = await aio_pika.connect_robust(self.rabbitmq_url)
        self._channel = await self._conn.channel()
        await self._channel.declare_queue(self.dlq_name, durable=True)

    async def _dlq_queue(self) -> Any:
        if self._channel is None:
            await self.connect()
        assert self._channel is not None
        return await self._channel.declare_queue(self.dlq_name, durable=True)

    async def list(self, limit: int) -> list[DLQEntry]:
        queue = await self._dlq_queue()
        entries: list[DLQEntry] = []
        messages: list[Any] = []
        for index in range(limit):
            message = await queue.get(fail=False, no_ack=False)
            if message is None:
                break
            messages.append(message)
            entries.append(build_dlq_entry(index, message, self.dlq_name))
        for message in messages:
            await message.nack(requeue=True)
        return entries

    async def take(
        self, *, item_id: str | None, index: int | None, scan_limit: int
    ) -> TakenDLQMessage | None:
        queue = await self._dlq_queue()
        for current_index in range(scan_limit):
            message = await queue.get(fail=False, no_ack=False)
            if message is None:
                return None
            entry = build_dlq_entry(current_index, message, self.dlq_name)
            matches_index = index is not None and current_index == index
            matches_item = (
                item_id is not None
                and entry.payload is not None
                and entry.payload.get("item_id") == item_id
            )
            if matches_index or matches_item:
                await message.ack()
                return TakenDLQMessage(entry=entry, message=message)
            await message.nack(requeue=True)
        return None

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
