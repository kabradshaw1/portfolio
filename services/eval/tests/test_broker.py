import json

from app.broker import (
    EvalItemMessage,
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
