import json
import logging
import os

import httpx
from fastapi import FastAPI, HTTPException
from fastapi.middleware.cors import CORSMiddleware
from fastapi.responses import JSONResponse
from pydantic import BaseModel, Field
from qdrant_client import QdrantClient
from sse_starlette.sse import EventSourceResponse

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
    allow_headers=["*"],
)

instrumentator.instrument(app).expose(app, include_in_schema=False)

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
    ollama_ok = False

    try:
        qd = QdrantClient(
            host=settings.qdrant_host, port=settings.qdrant_port, timeout=3
        )
        qd.get_collections()
        qdrant_ok = True
    except Exception:
        pass

    try:
        async with httpx.AsyncClient() as client:
            resp = await client.get(f"{settings.ollama_base_url}/api/tags", timeout=3.0)
            if resp.status_code == 200:
                ollama_ok = True
    except Exception:
        pass

    status = "healthy" if (qdrant_ok and ollama_ok) else "degraded"
    status_code = 200 if (qdrant_ok and ollama_ok) else 503

    return JSONResponse(
        status_code=status_code,
        content={
            "status": status,
            "qdrant": "connected" if qdrant_ok else "disconnected",
            "ollama": "connected" if ollama_ok else "disconnected",
        },
    )


@app.post("/index")
async def index(request: IndexRequest):
    if not os.path.isdir(request.path):
        raise HTTPException(
            status_code=400, detail=f"Directory not found: {request.path}"
        )

    try:
        result = await index_project(
            project_path=request.path,
            ollama_base_url=settings.ollama_base_url,
            embedding_model=settings.embedding_model,
            qdrant_host=settings.qdrant_host,
            qdrant_port=settings.qdrant_port,
        )
    except Exception as e:
        logger.error("Indexing failed: %s", e, exc_info=True)
        raise HTTPException(status_code=500, detail="Indexing failed")

    _project_paths[result["collection"]] = request.path
    return result


@app.post("/debug")
async def debug(request: DebugRequest):
    project_path = _project_paths.get(request.collection)
    if not project_path:
        raise HTTPException(
            status_code=400,
            detail=f"Collection '{request.collection}' not indexed. Call /index first.",
        )

    async def event_generator():
        try:
            async for event in run_agent_loop(
                description=request.description,
                error_output=request.error_output,
                collection=request.collection,
                project_path=project_path,
                ollama_base_url=settings.ollama_base_url,
                chat_model=settings.chat_model,
                embedding_model=settings.embedding_model,
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
