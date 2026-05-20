import httpx
from fastapi import FastAPI, HTTPException
from fastapi.middleware.cors import CORSMiddleware

from app.config import settings
from app.eval_client import EvalAPIError, EvalClient
from app.metrics import instrumentator, triage_requests_total
from app.models import TriageComparisonRequest, TriageEvalRunRequest
from app.service import RAGTriageService

app = FastAPI(title="RAG Triage API")

app.add_middleware(
    CORSMiddleware,
    allow_origins=[
        origin.strip()
        for origin in settings.allowed_origins.split(",")
        if origin.strip()
    ],
    allow_credentials=True,
    allow_methods=["GET", "POST"],
    allow_headers=["Authorization", "Content-Type"],
)

instrumentator.instrument(app).expose(app, include_in_schema=False)


@app.get("/health")
async def health():
    return {"status": "healthy"}


def build_service() -> RAGTriageService:
    client = EvalClient(
        base_url=settings.eval_api_url,
        token=settings.eval_api_token,
        timeout_seconds=settings.request_timeout_seconds,
    )
    return RAGTriageService(
        eval_client=client,
        default_metric=settings.default_metric,
        default_limit=settings.default_limit,
        max_limit=settings.max_limit,
    )


@app.post("/triage/eval-run")
async def triage_eval_run(body: TriageEvalRunRequest):
    service = build_service()
    try:
        result = await service.triage_eval_run(
            eval_id=body.eval_id,
            metric=body.metric,
            limit=body.limit,
        )
    except EvalAPIError as exc:
        triage_requests_total.labels(
            endpoint="eval-run",
            outcome="eval_api_error",
        ).inc()
        raise HTTPException(status_code=exc.status_code, detail=str(exc)) from exc
    except (httpx.HTTPError, ValueError) as exc:
        triage_requests_total.labels(
            endpoint="eval-run",
            outcome="upstream_error",
        ).inc()
        raise HTTPException(
            status_code=502,
            detail="eval API request failed",
        ) from exc
    finally:
        await service.close()

    triage_requests_total.labels(endpoint="eval-run", outcome="success").inc()
    return result


@app.post("/triage/comparison")
async def triage_comparison(body: TriageComparisonRequest):
    service = build_service()
    try:
        result = await service.triage_comparison(
            baseline_eval_id=body.baseline_eval_id,
            candidate_eval_id=body.candidate_eval_id,
            metric=body.metric,
            limit=body.limit,
        )
    except EvalAPIError as exc:
        triage_requests_total.labels(
            endpoint="comparison",
            outcome="eval_api_error",
        ).inc()
        raise HTTPException(status_code=exc.status_code, detail=str(exc)) from exc
    except (httpx.HTTPError, ValueError) as exc:
        triage_requests_total.labels(
            endpoint="comparison",
            outcome="upstream_error",
        ).inc()
        raise HTTPException(
            status_code=502,
            detail="eval API request failed",
        ) from exc
    finally:
        await service.close()

    triage_requests_total.labels(endpoint="comparison", outcome="success").inc()
    return result
