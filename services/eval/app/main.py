from __future__ import annotations

import logging
import os
import time

import httpx
from fastapi import BackgroundTasks, Depends, FastAPI, HTTPException
from fastapi.middleware.cors import CORSMiddleware
from prometheus_fastapi_instrumentator import Instrumentator
from shared.auth import create_auth_dependency
from slowapi import Limiter, _rate_limit_exceeded_handler
from slowapi.errors import RateLimitExceeded
from slowapi.util import get_remote_address
from starlette.requests import Request
from starlette.responses import JSONResponse

from app.config import settings
from app.db import EvalDB
from app.evaluator import run_evaluation
from app.metrics import eval_queries_total, eval_ragas_score, eval_run_duration_seconds
from app.models import CreateDatasetRequest, StartEvaluationRequest
from app.rag_client import RAGClient

logger = logging.getLogger(__name__)

app = FastAPI(title="Eval API")

app.add_middleware(
    CORSMiddleware,
    allow_origins=settings.allowed_origins.split(","),
    allow_credentials=True,
    allow_methods=["GET", "POST"],
    allow_headers=["Authorization", "Content-Type"],
)

instrumentator = Instrumentator()
instrumentator.instrument(app).expose(app, include_in_schema=False)

limiter = Limiter(key_func=get_remote_address)
app.state.limiter = limiter
app.add_exception_handler(RateLimitExceeded, _rate_limit_exceeded_handler)

require_auth = create_auth_dependency(settings.jwt_secret)

_db: EvalDB | None = None


async def get_db() -> EvalDB:
    global _db
    if _db is None:
        os.makedirs(os.path.dirname(settings.db_path) or ".", exist_ok=True)
        _db = EvalDB(settings.db_path)
        await _db.init()
    return _db


@app.on_event("shutdown")
async def shutdown():
    if _db:
        await _db.close()


# --- Health ---


@app.get("/health")
async def health():
    """Health check — verifies chat service is reachable."""
    chat_ok = True
    try:
        async with httpx.AsyncClient(timeout=5.0) as client:
            resp = await client.get(f"{settings.chat_service_url}/health")
            if resp.status_code != 200:
                chat_ok = False
    except Exception:
        chat_ok = False

    status = "healthy" if chat_ok else "degraded"
    code = 200 if chat_ok else 503
    return JSONResponse(
        status_code=code,
        content={"status": status, "chat_service": "ok" if chat_ok else "unreachable"},
    )


# --- Datasets ---


@app.post("/datasets", status_code=201)
@limiter.limit("10/minute")
async def create_dataset(
    request: Request, body: CreateDatasetRequest, user_id: str = Depends(require_auth)
):
    db = await get_db()
    try:
        ds_id = await db.create_dataset(
            name=body.name,
            items=[item.model_dump() for item in body.items],
        )
    except ValueError as e:
        raise HTTPException(status_code=409, detail=str(e))
    return {"id": ds_id}


@app.get("/datasets")
@limiter.limit("30/minute")
async def list_datasets(request: Request, user_id: str = Depends(require_auth)):
    db = await get_db()
    datasets = await db.list_datasets()
    return {"datasets": datasets}


# --- Evaluations ---


async def _run_evaluation_task(eval_id: str, items: list[dict], collection: str | None):
    """Background task that runs the RAGAS evaluation."""
    db = await get_db()
    rag_client = RAGClient(base_url=settings.chat_service_url)
    start = time.perf_counter()

    try:
        aggregate, results = await run_evaluation(
            items=items,
            rag_client=rag_client,
            collection=collection,
            llm_provider=settings.llm_provider,
            llm_base_url=settings.llm_base_url,
            llm_model=settings.llm_model,
            llm_api_key=settings.llm_api_key,
        )
        await db.complete_evaluation(
            eval_id, aggregate_scores=aggregate, results=results
        )

        # Update metrics
        eval_run_duration_seconds.observe(time.perf_counter() - start)
        eval_queries_total.inc(len(items))
        for metric_name, score in aggregate.items():
            if score is not None:
                eval_ragas_score.labels(metric=metric_name).set(score)

        logger.info("Evaluation %s completed: %s", eval_id, aggregate)
    except Exception as e:
        logger.error("Evaluation %s failed: %s", eval_id, e, exc_info=True)
        await db.fail_evaluation(eval_id, error=str(e))
    finally:
        await rag_client.close()


@app.post("/evaluations", status_code=202)
@limiter.limit("5/minute")
async def start_evaluation(
    request: Request,
    body: StartEvaluationRequest,
    background_tasks: BackgroundTasks,
    user_id: str = Depends(require_auth),
):
    db = await get_db()
    dataset = await db.get_dataset(body.dataset_id)
    if not dataset:
        raise HTTPException(status_code=404, detail="Dataset not found")

    eval_id = await db.create_evaluation(
        dataset_id=body.dataset_id, collection=body.collection or "documents"
    )

    background_tasks.add_task(
        _run_evaluation_task, eval_id, dataset["items"], body.collection
    )

    return {"id": eval_id, "status": "running"}


@app.get("/evaluations/{eval_id}")
@limiter.limit("30/minute")
async def get_evaluation(
    request: Request, eval_id: str, user_id: str = Depends(require_auth)
):
    db = await get_db()
    evaluation = await db.get_evaluation(eval_id)
    if not evaluation:
        raise HTTPException(status_code=404, detail="Evaluation not found")
    return evaluation


@app.get("/evaluations")
@limiter.limit("30/minute")
async def list_evaluations(
    request: Request,
    limit: int = 20,
    offset: int = 0,
    user_id: str = Depends(require_auth),
):
    db = await get_db()
    evaluations = await db.list_evaluations(limit=limit, offset=offset)
    return {"evaluations": evaluations}
