import pytest
import respx
from app.eval_client import EvalAPIError, EvalClient
from httpx import Response


@pytest.mark.asyncio
@respx.mock
async def test_get_evaluation_sends_bearer_token():
    route = respx.get("http://eval:8000/evaluations/eval-1").mock(
        return_value=Response(
            200,
            json={
                "id": "eval-1",
                "dataset_id": "dataset-1",
                "status": "completed",
                "collection": "documents",
                "aggregate_scores": {"context_precision": 0.4},
                "results": [],
                "config": {"effective_retrieval_config": {"top_k": 5}},
            },
        )
    )

    client = EvalClient(base_url="http://eval:8000", token="token-1")
    try:
        result = await client.get_evaluation("eval-1")
    finally:
        await client.close()

    assert result.id == "eval-1"
    assert route.calls[0].request.headers["authorization"] == "Bearer token-1"


@pytest.mark.asyncio
@respx.mock
async def test_get_evaluation_raises_for_non_200():
    respx.get("http://eval:8000/evaluations/missing").mock(
        return_value=Response(404, json={"detail": "Evaluation not found"})
    )

    client = EvalClient(base_url="http://eval:8000", token="")
    try:
        with pytest.raises(EvalAPIError) as exc:
            await client.get_evaluation("missing")
    finally:
        await client.close()

    assert exc.value.status_code == 404
    assert "Evaluation not found" in str(exc.value)


@pytest.mark.asyncio
@respx.mock
async def test_get_evaluation_raises_for_redirect_status():
    respx.get("http://eval:8000/evaluations/redirect").mock(
        return_value=Response(302, text="redirect")
    )

    client = EvalClient(base_url="http://eval:8000", token="")
    try:
        with pytest.raises(EvalAPIError) as exc:
            await client.get_evaluation("redirect")
    finally:
        await client.close()

    assert exc.value.status_code == 302
    assert "redirect" in str(exc.value)


@pytest.mark.asyncio
@respx.mock
async def test_get_evaluation_handles_json_string_error_body():
    respx.get("http://eval:8000/evaluations/bad").mock(
        return_value=Response(500, json="upstream unavailable")
    )

    client = EvalClient(base_url="http://eval:8000", token="")
    try:
        with pytest.raises(EvalAPIError) as exc:
            await client.get_evaluation("bad")
    finally:
        await client.close()

    assert exc.value.status_code == 500
    assert "upstream unavailable" in str(exc.value)
