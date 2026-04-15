import json
import logging
import os

from fastapi import FastAPI, HTTPException
from fastapi.middleware.cors import CORSMiddleware
from fastapi.responses import JSONResponse
from llm.factory import get_embedding_provider, get_llm_provider
from pydantic import BaseModel, Field
from qdrant_client import QdrantClient
from slowapi import Limiter
from slowapi.errors import RateLimitExceeded
from slowapi.util import get_remote_address
from sse_starlette.sse import EventSourceResponse
from starlette.requests import Request

from app.agent import run_agent_loop
from app.config import settings
from app.indexer import index_project
from app.metrics import instrumentator

logger = logging.getLogger(__name__)

app = FastAPI(title="Debug Assistant API")

app.add_middleware(
    CORSMiddleware,
    allow_origins=settings.allowed_origins.split(","),
    allow_methods=["GET", "POST"],
    allow_headers=["Authorization", "Content-Type"],
)

instrumentator.instrument(app).expose(app, include_in_schema=False)

limiter = Limiter(key_func=get_remote_address)
app.state.limiter = limiter


@app.exception_handler(RateLimitExceeded)
async def rate_limit_handler(request: Request, exc: RateLimitExceeded):
    return JSONResponse(status_code=429, content={"error": "Rate limit exceeded"})


_llm_provider = get_llm_provider(
    provider=settings.llm_provider,
    base_url=settings.get_llm_base_url(),
    api_key=settings.llm_api_key,
    model=settings.get_llm_model(),
)

_embedding_provider = get_embedding_provider(
    provider=settings.embedding_provider,
    base_url=settings.get_embedding_base_url(),
    api_key=settings.embedding_api_key,
    model=settings.embedding_model,
)

_project_paths: dict[str, str] = {}


class IndexRequest(BaseModel):
    path: str


class DebugRequest(BaseModel):
    collection: str = Field(pattern=r"^[a-zA-Z0-9_-]{1,100}$")
    description: str = Field(max_length=5000)
    error_output: str | None = Field(default=None, max_length=10000)


@app.get("/health")
async def health():
    qdrant_ok = False
    llm_ok = False

    try:
        qd = QdrantClient(
            host=settings.qdrant_host, port=settings.qdrant_port, timeout=3
        )
        qd.get_collections()
        qdrant_ok = True
    except Exception:
        pass

    try:
        llm_ok = await _llm_provider.check_health()
    except Exception:
        pass

    status = "healthy" if (qdrant_ok and llm_ok) else "degraded"
    status_code = 200 if (qdrant_ok and llm_ok) else 503

    return JSONResponse(
        status_code=status_code,
        content={
            "status": status,
            "qdrant": "connected" if qdrant_ok else "disconnected",
            "llm": "connected" if llm_ok else "disconnected",
        },
    )


@app.post("/index")
@limiter.limit("5/minute")
async def index(request: Request, body: IndexRequest):
    if not os.path.isdir(body.path):
        raise HTTPException(status_code=400, detail=f"Directory not found: {body.path}")

    # Validate path is in allowlist
    allowed = [
        p.strip() for p in settings.allowed_project_paths.split(",") if p.strip()
    ]
    if not allowed:
        raise HTTPException(
            status_code=403, detail="No project paths configured for indexing"
        )
    abs_path = os.path.realpath(body.path)
    if not any(
        abs_path.startswith(os.path.realpath(a) + os.sep)
        or abs_path == os.path.realpath(a)
        for a in allowed
    ):
        raise HTTPException(status_code=403, detail="Path not allowed for indexing")

    try:
        result = await index_project(
            project_path=body.path,
            embedding_provider=_embedding_provider,
            qdrant_host=settings.qdrant_host,
            qdrant_port=settings.qdrant_port,
        )
    except Exception as e:
        logger.error("Indexing failed: %s", e, exc_info=True)
        raise HTTPException(status_code=500, detail="Indexing failed")

    _project_paths[result["collection"]] = body.path
    return result


@app.post("/debug")
@limiter.limit("10/minute")
async def debug(request: Request, body: DebugRequest):
    project_path = _project_paths.get(body.collection)
    if not project_path:
        raise HTTPException(
            status_code=400,
            detail=f"Collection '{body.collection}' not indexed. Call /index first.",
        )

    async def event_generator():
        try:
            async for event in run_agent_loop(
                description=body.description,
                error_output=body.error_output,
                collection=body.collection,
                project_path=project_path,
                llm_provider=_llm_provider,
                embedding_provider=_embedding_provider,
                chat_model=settings.get_llm_model(),
                qdrant_host=settings.qdrant_host,
                qdrant_port=settings.qdrant_port,
                max_steps=settings.max_agent_steps,
            ):
                yield {"event": event["event"], "data": json.dumps(event["data"])}
        except Exception as e:
            logger.error("Debug session error: %s", e, exc_info=True)
            yield {
                "event": "diagnosis",
                "data": json.dumps({"content": "Internal error during debug session."}),
            }
            yield {"event": "done", "data": json.dumps({})}

    return EventSourceResponse(event_generator())
