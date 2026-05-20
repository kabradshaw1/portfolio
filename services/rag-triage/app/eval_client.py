from __future__ import annotations

import httpx

from app.models import EvaluationDetail


class EvalAPIError(Exception):
    def __init__(self, status_code: int, message: str):
        super().__init__(message)
        self.status_code = status_code


class EvalClient:
    def __init__(
        self,
        base_url: str,
        token: str,
        timeout_seconds: float = 30.0,
        transport: httpx.AsyncBaseTransport | None = None,
    ):
        kwargs: dict = {"base_url": base_url.rstrip("/"), "timeout": timeout_seconds}
        if transport is not None:
            kwargs["transport"] = transport
        self._client = httpx.AsyncClient(**kwargs)
        self._token = token

    def _headers(self) -> dict[str, str]:
        if not self._token:
            return {}
        return {"Authorization": f"Bearer {self._token}"}

    async def get_evaluation(self, eval_id: str) -> EvaluationDetail:
        response = await self._client.get(
            f"/evaluations/{eval_id}",
            headers=self._headers(),
        )
        if response.status_code >= 400:
            raise EvalAPIError(response.status_code, self._error_message(response))
        return EvaluationDetail.model_validate(response.json())

    @staticmethod
    def _error_message(response: httpx.Response) -> str:
        try:
            payload = response.json()
        except ValueError:
            return response.text[:256]
        detail = payload.get("detail")
        if isinstance(detail, str):
            return detail
        return str(payload)[:256]

    async def close(self) -> None:
        await self._client.aclose()
