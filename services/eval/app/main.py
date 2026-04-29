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
from app.config_capture import capture_run_config
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
    coll_name = collection or "documents"

    try:
        # Snapshot the RAG configuration that produced this run before we
        # invoke retrieval. capture_run_config never raises; failures are
        # recorded under _capture_error so the eval still completes.
        config = await capture_run_config(
            chat_url=settings.chat_service_url,
            ingestion_url=settings.ingestion_service_url,
            collection=coll_name,
        )
        await db.set_evaluation_config(eval_id, config)

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
        dataset_id=body.dataset_id,
        collection=body.collection or "documents",
        notes=body.notes,
        baseline_eval_id=body.baseline_eval_id,
    )

    background_tasks.add_task(
        _run_evaluation_task, eval_id, dataset["items"], body.collection
    )

    return {"id": eval_id, "status": "running"}


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


_RAGAS_METRICS = (
    "faithfulness",
    "answer_relevancy",
    "context_precision",
    "context_recall",
)


# NOTE: /evaluations/compare and /evaluations/history must be defined BEFORE
# /evaluations/{eval_id} so FastAPI matches the literal paths first instead
# of treating "compare"/"history" as an eval_id.


@app.get("/evaluations/compare")
@limiter.limit("30/minute")
async def compare_evaluations(
    request: Request,
    ids: str,
    user_id: str = Depends(require_auth),
):
    """Side-by-side comparison of 2-5 runs with deltas vs the first run.

    All runs must reference the same dataset_id (cross-dataset comparison
    is mathematically meaningless — different golden questions). Returns
    400 on cardinality or dataset-mismatch violations, 404 if any id is
    unknown.
    """
    id_list = [i for i in ids.split(",") if i]
    if not (2 <= len(id_list) <= 5):
        raise HTTPException(status_code=400, detail="compare requires 2-5 ids")

    db = await get_db()
    runs = await db.get_evaluations_by_ids(id_list)
    if len(runs) != len(id_list):
        missing = sorted(set(id_list) - {r["id"] for r in runs})
        raise HTTPException(
            status_code=404, detail=f"unknown evaluation id(s): {missing}"
        )

    datasets = {r["dataset_id"] for r in runs}
    if len(datasets) > 1:
        raise HTTPException(
            status_code=400, detail="all runs must belong to the same dataset"
        )

    deltas: dict[str, list[float]] = {}
    for metric in _RAGAS_METRICS:
        baseline = (runs[0].get("aggregate_scores") or {}).get(metric)
        deltas[metric] = []
        for r in runs:
            score = (r.get("aggregate_scores") or {}).get(metric)
            if baseline is None or score is None:
                deltas[metric].append(0.0)
            else:
                deltas[metric].append(round(score - baseline, 6))

    return {"runs": runs, "deltas": deltas}


@app.get("/evaluations/history")
@limiter.limit("30/minute")
async def get_history(
    request: Request,
    dataset_id: str | None = None,
    collection: str | None = None,
    user_id: str = Depends(require_auth),
):
    """Time-series of completed runs for a dataset+collection pair.

    Both query params are required so the response is unambiguous (a
    dataset evaluated against multiple collections has incomparable
    score curves). Empty result returns 200 with an empty list.
    """
    if not dataset_id or not collection:
        raise HTTPException(
            status_code=400,
            detail="dataset_id and collection are both required",
        )
    db = await get_db()
    runs = await db.get_history(dataset_id=dataset_id, collection=collection)
    return {"runs": runs}


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
