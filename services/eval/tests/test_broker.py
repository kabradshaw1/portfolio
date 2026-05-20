import json
from dataclasses import asdict

import pytest
from app.broker import (
    DLQRoutingMetadata,
    EvalItemDLQClient,
    EvalItemMessage,
    build_dlq_entry,
    decode_eval_item_message,
    encode_eval_item_message,
)


def test_encode_eval_item_message_contains_only_identifiers():
    payload = encode_eval_item_message(
        EvalItemMessage(
            evaluation_id="eval-1",
            item_id="item-1",
            item_index=3,
            attempt=1,
        )
    )

    decoded = json.loads(payload)
    assert decoded == {
        "message_version": 1,
        "evaluation_id": "eval-1",
        "item_id": "item-1",
        "item_index": 3,
        "attempt": 1,
    }
    assert "query" not in decoded
    assert "expected_answer" not in decoded
    assert "api_key" not in decoded


def test_decode_eval_item_message_round_trips():
    encoded = encode_eval_item_message(
        EvalItemMessage(
            evaluation_id="eval-1",
            item_id="item-1",
            item_index=2,
            attempt=4,
        )
    )

    decoded = decode_eval_item_message(encoded)

    assert decoded.evaluation_id == "eval-1"
    assert decoded.item_id == "item-1"
    assert decoded.item_index == 2
    assert decoded.attempt == 4


class FakeIncomingMessage:
    def __init__(self, body, headers=None, redelivered=False, delivery_tag=7):
        self.body = body
        self.headers = headers or {}
        self.redelivered = redelivered
        self.delivery_tag = delivery_tag


def test_build_dlq_entry_redacts_payload_to_identifiers():
    message = FakeIncomingMessage(
        body=json.dumps(
            {
                "message_version": 1,
                "evaluation_id": "eval-1",
                "item_id": "item-1",
                "item_index": 2,
                "attempt": 3,
                "query": "secret query",
                "expected_answer": "secret answer",
            }
        ).encode("utf-8"),
        headers={
            "x-death": [
                {
                    "count": 2,
                    "reason": "rejected",
                    "exchange": "",
                    "routing-keys": ["eval.item.requested"],
                }
            ]
        },
    )

    entry = build_dlq_entry(
        index=0, message=message, dlq_name="eval.item.requested.dlq"
    )

    assert entry.payload == {
        "message_version": 1,
        "evaluation_id": "eval-1",
        "item_id": "item-1",
        "item_index": 2,
        "attempt": 3,
    }
    assert entry.invalid_payload is None
    assert entry.routing == DLQRoutingMetadata(
        exchange="",
        routing_key="eval.item.requested",
        queue="eval.item.requested.dlq",
        death_count=2,
        death_reason="rejected",
    )
    assert "secret query" not in json.dumps(asdict(entry))
    assert "secret answer" not in json.dumps(asdict(entry))


def test_build_dlq_entry_marks_invalid_payload_without_body_text():
    entry = build_dlq_entry(
        index=0,
        message=FakeIncomingMessage(body=b"{not-json"),
        dlq_name="eval.item.requested.dlq",
    )

    assert entry.payload is None
    assert entry.invalid_payload == "invalid_json"
    assert "{not-json" not in json.dumps(asdict(entry))


class FakeQueue:
    def __init__(self, messages):
        self.messages = list(messages)
        self.calls = 0

    async def get(self, fail=True, no_ack=False):
        assert no_ack is False
        if self.calls >= len(self.messages):
            return None
        message = self.messages[self.calls]
        self.calls += 1
        return message


class AckableMessage(FakeIncomingMessage):
    def __init__(self, body, headers=None):
        super().__init__(body=body, headers=headers)
        self.acked = False
        self.nacked = False

    async def ack(self):
        self.acked = True

    async def nack(self, requeue=True):
        assert requeue is True
        self.nacked = True


class FakeDLQClient(EvalItemDLQClient):
    def __init__(self, queue):
        self.dlq_name = "eval.item.requested.dlq"
        self._queue = queue

    async def _dlq_queue(self):
        return self._queue


@pytest.mark.asyncio
async def test_list_peeks_and_requeues_messages():
    body = encode_eval_item_message(EvalItemMessage("eval-1", "item-1", 0, 3))
    msg = AckableMessage(body=body)
    client = FakeDLQClient(FakeQueue([msg]))

    entries = await client.list(limit=10)

    assert len(entries) == 1
    assert entries[0].payload["item_id"] == "item-1"
    assert msg.nacked is True
    assert msg.acked is False


@pytest.mark.asyncio
async def test_take_by_item_id_acks_only_selected_message_and_requeues_others():
    first = AckableMessage(
        body=encode_eval_item_message(EvalItemMessage("eval-1", "item-1", 0, 3))
    )
    second = AckableMessage(
        body=encode_eval_item_message(EvalItemMessage("eval-1", "item-2", 1, 3))
    )
    client = FakeDLQClient(FakeQueue([first, second]))

    taken = await client.take(item_id="item-2", index=None, scan_limit=10)

    assert taken is not None
    assert taken.entry.payload["item_id"] == "item-2"
    assert first.nacked is True
    assert second.acked is True
