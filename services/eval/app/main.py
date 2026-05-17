from __future__ import annotations

import asyncio
import logging
import os
import time

import httpx
from fastapi import BackgroundTasks, Depends, FastAPI, HTTPException
from fastapi.middleware.cors import CORSMiddleware
from prometheus_fastapi_instrumentator import Instrumentator
from shared.auth import AuthContext, create_auth_context_dependency
from shared.rate_limits import FixedWindowRateLimiter, policies_from_settings
from slowapi import Limiter, _rate_limit_exceeded_handler
from slowapi.errors import RateLimitExceeded
from slowapi.util import get_remote_address
from starlette.requests import Request
from starlette.responses import JSONResponse

from app.collection_validation import validate_collection_exists
from app.config import settings
from app.config_capture import capture_run_config
from app.db import EvalDB
from app.evaluator import EvalRunContext, run_evaluation
from app.metrics import (
    eval_quality_score,
    eval_queries_total,
    eval_run_duration_seconds,
    eval_runs_total,
    eval_stale_running_runs,
)
from app.models import (
    AttachExperimentRunRequest,
    CreateDatasetRequest,
    CreateExperimentRequest,
    DashboardBaselineDeltas,
    DashboardDatasetSummary,
    DashboardRunSummary,
    EvaluationDashboard,
    MetricTrendPoint,
    QueryScore,
    RetrievalConfig,
    StartEvaluationRequest,
    UpdateExperimentRequest,
)
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

_db: EvalDB | None = None


def build_eval_rate_limiter() -> FixedWindowRateLimiter:
    limiter = FixedWindowRateLimiter(
        policies_from_settings(
            {
                "eval_run_create": {
                    "operator": settings.eval_rate_limit_run_create_operator,
                    "user": settings.eval_rate_limit_run_create_user,
                    "anonymous": settings.eval_rate_limit_run_create_anonymous,
                },
                "eval_read": {
                    "operator": settings.eval_rate_limit_read_operator,
                    "user": settings.eval_rate_limit_read_user,
                    "anonymous": settings.eval_rate_limit_read_anonymous,
                },
                "eval_write": {
                    "operator": settings.eval_rate_limit_write_operator,
                    "user": settings.eval_rate_limit_write_user,
                    "anonymous": settings.eval_rate_limit_write_anonymous,
                },
            }
        )
    )
    limiter.enabled = True
    return limiter


eval_rate_limiter = build_eval_rate_limiter()


async def _resolve_auth_context(request: Request) -> AuthContext:
    dependency = create_auth_context_dependency(settings.jwt_secret)
    return await dependency(request, None)


async def _enforce_eval_rate_limit(group: str, request: Request) -> AuthContext:
    context = await _resolve_auth_context(request)
    if not getattr(eval_rate_limiter, "enabled", True):
        return context
    decision = eval_rate_limiter.check(group, context)
    if not decision.allowed:
        raise HTTPException(
            status_code=429,
            detail="Rate limit exceeded",
            headers={"Retry-After": str(decision.retry_after_seconds)},
        )
    return context


async def enforce_eval_run_create(request: Request) -> AuthContext:
    return await _enforce_eval_rate_limit("eval_run_create", request)


async def enforce_eval_read(request: Request) -> AuthContext:
    return await _enforce_eval_rate_limit("eval_read", request)


async def enforce_eval_write(request: Request) -> AuthContext:
    return await _enforce_eval_rate_limit("eval_write", request)


async def get_db() -> EvalDB:
    global _db
    if _db is None:
        os.makedirs(os.path.dirname(settings.db_path) or ".", exist_ok=True)
        _db = EvalDB(settings.db_path)
        await _db.init()
    return _db


@app.on_event("startup")
async def recover_stale_evaluations():
    db = await get_db()
    max_age = settings.eval_run_max_seconds + settings.eval_stale_grace_seconds
    stale_count = await db.count_stale_running_evaluations(max_age)
    eval_stale_running_runs.set(stale_count)
    recovered = await db.fail_stale_running_evaluations(max_age)
    if recovered:
        eval_stale_running_runs.set(max(stale_count - recovered, 0))
        logger.warning("Recovered %s stale running evaluation(s)", recovered)


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

    return JSONResponse(
        status_code=200,
        content={
            "status": "healthy",
            "chat_service": "ok" if chat_ok else "unreachable",
        },
    )


# --- Datasets ---


@app.post(
    "/datasets",
    status_code=201,
    dependencies=[Depends(enforce_eval_write)],
)
async def create_dataset(
    request: Request,
    body: CreateDatasetRequest,
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


@app.get("/datasets", dependencies=[Depends(enforce_eval_read)])
async def list_datasets(request: Request):
    db = await get_db()
    datasets = await db.list_datasets()
    return {"datasets": datasets}


# --- Evaluations ---


def _failure_message(
    eval_id: str,
    collection: str,
    rerank: bool,
    elapsed_seconds: float,
    reason: str,
) -> str:
    return (
        f"evaluation {eval_id} failed for collection={collection} "
        f"rerank={str(rerank).lower()} after {elapsed_seconds:.2f}s: {reason}"
    )


def _requested_retrieval_config(config: RetrievalConfig | None) -> dict:
    return config.model_dump(exclude_none=True) if config else {}


def _effective_top_k(
    config: RetrievalConfig | None, captured_config: dict | None
) -> int:
    if config and config.top_k is not None:
        return config.top_k

    effective_config = (captured_config or {}).get("effective_retrieval_config", {})
    effective_top_k = effective_config.get("top_k")
    if type(effective_top_k) is int:
        return effective_top_k

    return 5


async def _run_evaluation_task(
    eval_id: str,
    items: list[dict],
    collection: str | None,
    rerank: bool = False,
    retrieval_config: RetrievalConfig | None = None,
):
    """Background task that runs the RAG quality evaluation."""
    db = await get_db()
    rag_client = RAGClient(
        base_url=settings.chat_service_url,
        internal_token=settings.rag_internal_eval_token,
    )
    start = time.perf_counter()
    coll_name = collection or "documents"
    run_context = EvalRunContext(
        eval_id=eval_id,
        collection=coll_name,
        requested_rerank=rerank,
    )
    requested_rerank = str(rerank).lower()

    try:
        logger.info(
            "evaluation_task_started eval_id=%s collection=%s rerank=%s item_count=%s",
            eval_id,
            coll_name,
            requested_rerank,
            len(items),
        )
        # Snapshot the RAG configuration that produced this run before we
        # invoke retrieval. capture_run_config never raises; failures are
        # recorded under _capture_error so the eval still completes.
        config = await capture_run_config(
            chat_url=settings.chat_service_url,
            ingestion_url=settings.ingestion_service_url,
            collection=coll_name,
            requested_rerank=rerank,
            requested_retrieval_config=_requested_retrieval_config(retrieval_config),
        )
        await db.set_evaluation_config(eval_id, config)
        effective_top_k = _effective_top_k(retrieval_config, config)

        aggregate, results = await asyncio.wait_for(
            run_evaluation(
                items=items,
                rag_client=rag_client,
                collection=collection,
                llm_provider=settings.llm_provider,
                llm_base_url=settings.llm_base_url,
                llm_model=settings.llm_model,
                llm_api_key=settings.llm_api_key,
                rerank=rerank,
                top_k=effective_top_k,
                run_context=run_context,
            ),
            timeout=settings.eval_run_max_seconds,
        )
        await db.complete_evaluation(
            eval_id, aggregate_scores=aggregate, results=results
        )

        # Update metrics
        eval_run_duration_seconds.observe(time.perf_counter() - start)
        eval_runs_total.labels(
            status="completed", requested_rerank=requested_rerank
        ).inc()
        eval_queries_total.inc(len(items))
        for metric_name, score in aggregate.items():
            if score is not None:
                eval_quality_score.labels(metric=metric_name).set(score)

        logger.info(
            "evaluation_completed eval_id=%s collection=%s rerank=%s aggregate=%s",
            eval_id,
            coll_name,
            requested_rerank,
            aggregate,
        )
    except TimeoutError:
        elapsed = time.perf_counter() - start
        error = _failure_message(
            eval_id,
            coll_name,
            rerank,
            elapsed,
            f"timed out after {settings.eval_run_max_seconds:.2f}s max runtime",
        )
        logger.error("%s", error)
        eval_runs_total.labels(status="failed", requested_rerank=requested_rerank).inc()
        await db.fail_evaluation(eval_id, error)
    except asyncio.CancelledError:
        elapsed = time.perf_counter() - start
        error = _failure_message(eval_id, coll_name, rerank, elapsed, "cancelled")
        logger.error("%s", error)
        eval_runs_total.labels(status="failed", requested_rerank=requested_rerank).inc()
        await db.fail_evaluation(eval_id, error)
        raise
    except Exception as e:
        logger.error("Evaluation %s failed: %s", eval_id, e, exc_info=True)
        error = _failure_message(
            eval_id,
            coll_name,
            rerank,
            time.perf_counter() - start,
            str(e),
        )
        eval_runs_total.labels(status="failed", requested_rerank=requested_rerank).inc()
        await db.fail_evaluation(eval_id, error)
    finally:
        await rag_client.close()


async def _validate_baseline(
    db: EvalDB, baseline_eval_id: str, dataset_id: str, collection: str
) -> None:
    baseline = await db.get_evaluation(baseline_eval_id)
    if not baseline:
        raise HTTPException(status_code=404, detail="Baseline evaluation not found")
    if baseline["status"] != "completed":
        raise HTTPException(
            status_code=400, detail="Baseline evaluation must be completed"
        )
    if baseline["dataset_id"] != dataset_id:
        raise HTTPException(
            status_code=400,
            detail="Baseline evaluation must use the same dataset",
        )
    if baseline["collection"] != collection:
        raise HTTPException(
            status_code=400,
            detail="Baseline evaluation must use the same collection",
        )


async def _validate_experiment_baseline(
    db: EvalDB, baseline_eval_id: str, dataset_id: str, collection: str
) -> None:
    baseline = await db.get_evaluation(baseline_eval_id)
    if not baseline:
        raise HTTPException(status_code=404, detail="Baseline evaluation not found")
    if baseline["dataset_id"] != dataset_id:
        raise HTTPException(
            status_code=400,
            detail="Baseline evaluation must use the same dataset",
        )
    if baseline["collection"] != collection:
        raise HTTPException(
            status_code=400,
            detail="Baseline evaluation must use the same collection",
        )


async def _validate_experiment_for_run(
    db: EvalDB, experiment_id: str, dataset_id: str, collection: str
) -> None:
    experiment = await db.get_experiment(experiment_id)
    if not experiment:
        raise HTTPException(status_code=404, detail="Experiment not found")
    if experiment["dataset_id"] != dataset_id:
        raise HTTPException(
            status_code=400, detail="Experiment must use the same dataset"
        )
    if experiment["collection"] != collection:
        raise HTTPException(
            status_code=400, detail="Experiment must use the same collection"
        )
    if experiment["status"] == "completed":
        raise HTTPException(
            status_code=400, detail="completed experiments cannot accept runs"
        )


@app.post(
    "/evaluations",
    status_code=202,
    dependencies=[Depends(enforce_eval_run_create)],
)
async def start_evaluation(
    request: Request,
    body: StartEvaluationRequest,
    background_tasks: BackgroundTasks,
):
    db = await get_db()
    dataset = await db.get_dataset(body.dataset_id)
    if not dataset:
        raise HTTPException(status_code=404, detail="Dataset not found")

    collection = body.collection or "documents"
    if body.baseline_eval_id is not None:
        await _validate_baseline(db, body.baseline_eval_id, body.dataset_id, collection)
    if body.experiment_id is not None:
        if body.experiment_label is None:
            raise HTTPException(
                status_code=400,
                detail="experiment_label is required with experiment_id",
            )
        await _validate_experiment_for_run(
            db, body.experiment_id, body.dataset_id, collection
        )

    await validate_collection_exists(settings.ingestion_service_url, collection)

    eval_id = await db.create_evaluation(
        dataset_id=body.dataset_id,
        collection=collection,
        notes=body.notes,
        baseline_eval_id=body.baseline_eval_id,
    )
    if body.experiment_id is not None and body.experiment_label is not None:
        try:
            await db.attach_experiment_run(
                body.experiment_id,
                eval_id,
                label=body.experiment_label,
                notes=body.notes,
            )
        except ValueError as exc:
            detail = str(exc)
            status_code = 409 if "duplicate" in detail else 400
            raise HTTPException(status_code=status_code, detail=detail) from exc

    logger.info(
        "evaluation_start_accepted eval_id=%s dataset_id=%s collection=%s rerank=%s",
        eval_id,
        body.dataset_id,
        collection,
        str(body.rerank).lower(),
    )

    background_tasks.add_task(
        _run_evaluation_task,
        eval_id,
        dataset["items"],
        collection,
        body.rerank,
        body.retrieval_config,
    )

    return {"id": eval_id, "status": "running"}


@app.post(
    "/experiments",
    status_code=201,
    dependencies=[Depends(enforce_eval_write)],
)
async def create_experiment(
    request: Request,
    body: CreateExperimentRequest,
):
    db = await get_db()
    dataset = await db.get_dataset(body.dataset_id)
    if not dataset:
        raise HTTPException(status_code=404, detail="Dataset not found")
    if body.baseline_eval_id is not None:
        await _validate_experiment_baseline(
            db, body.baseline_eval_id, body.dataset_id, body.collection
        )

    exp_id = await db.create_experiment(
        name=body.name,
        hypothesis=body.hypothesis,
        dataset_id=body.dataset_id,
        collection=body.collection,
        baseline_eval_id=body.baseline_eval_id,
        focus_metric=body.focus_metric,
        status=body.status,
        notes=body.notes,
    )
    if body.baseline_eval_id is not None:
        await db.attach_experiment_run(
            exp_id, body.baseline_eval_id, label="baseline", notes="baseline"
        )
    experiment = await db.get_experiment(exp_id)
    return experiment


@app.get("/experiments", dependencies=[Depends(enforce_eval_read)])
async def list_experiments(
    request: Request,
    dataset_id: str | None = None,
    collection: str | None = None,
    status: str | None = None,
):
    db = await get_db()
    experiments = await db.list_experiments(
        dataset_id=dataset_id, collection=collection, status=status
    )
    return {"experiments": experiments}


@app.get("/experiments/{experiment_id}", dependencies=[Depends(enforce_eval_read)])
async def get_experiment(request: Request, experiment_id: str):
    db = await get_db()
    experiment = await db.get_experiment(experiment_id)
    if not experiment:
        raise HTTPException(status_code=404, detail="Experiment not found")
    return experiment


@app.patch(
    "/experiments/{experiment_id}",
    dependencies=[Depends(enforce_eval_write)],
)
async def update_experiment(
    request: Request,
    experiment_id: str,
    body: UpdateExperimentRequest,
):
    db = await get_db()
    experiment = await db.get_experiment(experiment_id)
    if not experiment:
        raise HTTPException(status_code=404, detail="Experiment not found")

    final_status = body.status or experiment["status"]
    if body.decision is not None and final_status != "completed":
        raise HTTPException(
            status_code=400, detail="decision requires completed status"
        )
    final_decision = (
        body.decision if body.decision is not None else experiment["decision"]
    )
    final_conclusion = (
        body.conclusion if body.conclusion is not None else experiment["conclusion"]
    )
    final_evidence = (
        body.evidence if body.evidence is not None else experiment["evidence"]
    )
    if final_status == "completed" and final_decision is None:
        raise HTTPException(
            status_code=400, detail="completed experiments require a decision"
        )
    if final_status == "completed" and final_conclusion is None:
        raise HTTPException(
            status_code=400, detail="completed experiments require a conclusion"
        )
    if final_status == "completed" and final_evidence is None:
        raise HTTPException(
            status_code=400, detail="completed experiments require evidence"
        )
    baseline_eval_id = body.baseline_eval_id
    if baseline_eval_id is not None:
        await _validate_experiment_baseline(
            db,
            baseline_eval_id,
            experiment["dataset_id"],
            experiment["collection"],
        )

    await db.update_experiment(
        experiment_id,
        hypothesis=body.hypothesis,
        baseline_eval_id=baseline_eval_id,
        focus_metric=body.focus_metric,
        status=body.status,
        decision=body.decision,
        conclusion=body.conclusion,
        evidence=body.evidence,
        notes=body.notes,
    )
    updated = await db.get_experiment(experiment_id)
    return updated


@app.get(
    "/experiments/{experiment_id}/runs",
    dependencies=[Depends(enforce_eval_read)],
)
async def list_experiment_runs(request: Request, experiment_id: str):
    db = await get_db()
    experiment = await db.get_experiment(experiment_id)
    if not experiment:
        raise HTTPException(status_code=404, detail="Experiment not found")
    return {"runs": experiment["runs"]}


@app.post(
    "/experiments/{experiment_id}/runs",
    dependencies=[Depends(enforce_eval_write)],
)
async def attach_experiment_run(
    request: Request,
    experiment_id: str,
    body: AttachExperimentRunRequest,
):
    db = await get_db()
    try:
        experiment = await db.attach_experiment_run(
            experiment_id, body.evaluation_id, label=body.label, notes=body.notes
        )
    except ValueError as exc:
        detail = str(exc)
        status_code = 409 if "duplicate" in detail else 400
        raise HTTPException(status_code=status_code, detail=detail) from exc
    if not experiment:
        raise HTTPException(
            status_code=404, detail="Experiment or evaluation not found"
        )
    return experiment


@app.get("/evaluations", dependencies=[Depends(enforce_eval_read)])
async def list_evaluations(
    request: Request,
    limit: int = 20,
    offset: int = 0,
):
    db = await get_db()
    evaluations = await db.list_evaluations(limit=limit, offset=offset)
    return {"evaluations": evaluations}


_EVAL_METRICS = (
    "faithfulness",
    "answer_relevancy",
    "context_precision",
    "context_recall",
)


def _dashboard_run_summary(run: dict) -> DashboardRunSummary:
    return DashboardRunSummary(
        id=run["id"],
        created_at=run["created_at"],
        completed_at=run["completed_at"],
        notes=run.get("notes"),
        config_captured=run.get("config") is not None,
        aggregate_scores=run.get("aggregate_scores"),
        baseline_eval_id=run.get("baseline_eval_id"),
    )


def _metric_trends(runs: list[dict]) -> dict[str, list[MetricTrendPoint]]:
    trends: dict[str, list[MetricTrendPoint]] = {}
    for metric in _EVAL_METRICS:
        trends[metric] = [
            MetricTrendPoint(
                evaluation_id=run["id"],
                completed_at=run.get("completed_at"),
                score=(run.get("aggregate_scores") or {}).get(metric),
            )
            for run in runs
        ]
    return trends


def _baseline_to_latest_deltas(runs: list[dict]) -> DashboardBaselineDeltas | None:
    if len(runs) < 2:
        return None

    baseline = runs[0]
    latest = runs[-1]
    baseline_scores = baseline.get("aggregate_scores") or {}
    latest_scores = latest.get("aggregate_scores") or {}
    deltas: dict[str, float | None] = {}
    for metric in _EVAL_METRICS:
        baseline_score = baseline_scores.get(metric)
        latest_score = latest_scores.get(metric)
        if baseline_score is None or latest_score is None:
            deltas[metric] = None
        else:
            deltas[metric] = round(latest_score - baseline_score, 6)

    return DashboardBaselineDeltas(
        baseline_eval_id=baseline["id"],
        latest_eval_id=latest["id"],
        deltas=QueryScore(**deltas),
    )


def _empty_metric_trends() -> dict[str, list[MetricTrendPoint]]:
    return {metric: [] for metric in _EVAL_METRICS}


# NOTE: /evaluations/compare and /evaluations/history must be defined BEFORE
# /evaluations/{eval_id} so FastAPI matches the literal paths first instead
# of treating "compare"/"history"/"dashboard" as an eval_id.


@app.get("/evaluations/compare", dependencies=[Depends(enforce_eval_read)])
async def compare_evaluations(
    request: Request,
    ids: str,
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

    invalid_statuses = [
        f"{r['id']}={r.get('status')}" for r in runs if r.get("status") != "completed"
    ]
    if invalid_statuses:
        raise HTTPException(
            status_code=400,
            detail=(
                "compare requires completed runs; invalid statuses: "
                + ", ".join(invalid_statuses)
            ),
        )

    deltas: dict[str, list[float]] = {}
    for metric in _EVAL_METRICS:
        baseline = (runs[0].get("aggregate_scores") or {}).get(metric)
        deltas[metric] = []
        for r in runs:
            score = (r.get("aggregate_scores") or {}).get(metric)
            if baseline is None or score is None:
                deltas[metric].append(0.0)
            else:
                deltas[metric].append(round(score - baseline, 6))

    return {"runs": runs, "deltas": deltas}


@app.get("/evaluations/history", dependencies=[Depends(enforce_eval_read)])
async def get_history(
    request: Request,
    dataset_id: str | None = None,
    collection: str | None = None,
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


@app.get(
    "/evaluations/dashboard",
    response_model=EvaluationDashboard,
    dependencies=[Depends(enforce_eval_read)],
)
async def get_dashboard(
    request: Request,
    dataset_id: str | None = None,
    collection: str | None = None,
    recent_limit: int = 10,
):
    """Compact dashboard summary for completed runs on one dataset+collection."""
    if not dataset_id or not collection:
        raise HTTPException(
            status_code=400,
            detail="dataset_id and collection are both required",
        )
    if not (1 <= recent_limit <= 100):
        raise HTTPException(
            status_code=400,
            detail="recent_limit must be between 1 and 100",
        )

    db = await get_db()
    dataset = await db.get_dataset(dataset_id)
    if not dataset:
        raise HTTPException(status_code=404, detail="Dataset not found")

    runs = await db.get_completed_evaluations_for_dashboard(
        dataset_id=dataset_id,
        collection=collection,
    )
    run_summaries = [_dashboard_run_summary(run) for run in runs]
    recent_runs = list(reversed(run_summaries))[:recent_limit]

    return EvaluationDashboard(
        dataset=DashboardDatasetSummary(
            id=dataset["id"],
            name=dataset["name"],
            item_count=len(dataset["items"]),
        ),
        collection=collection,
        completed_run_count=len(runs),
        first_completed_run=run_summaries[0] if run_summaries else None,
        latest_completed_run=run_summaries[-1] if run_summaries else None,
        metric_trends=_metric_trends(runs) if runs else _empty_metric_trends(),
        recent_runs=recent_runs,
        baseline_to_latest_deltas=_baseline_to_latest_deltas(runs),
    )


@app.get("/evaluations/{eval_id}", dependencies=[Depends(enforce_eval_read)])
async def get_evaluation(request: Request, eval_id: str):
    db = await get_db()
    evaluation = await db.get_evaluation(eval_id)
    if not evaluation:
        raise HTTPException(status_code=404, detail="Evaluation not found")
    return evaluation
