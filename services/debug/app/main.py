import logging

import httpx
from fastapi import FastAPI
from fastapi.middleware.cors import CORSMiddleware
from fastapi.responses import JSONResponse
from qdrant_client import QdrantClient

from app.config import settings

logger = logging.getLogger(__name__)

app = FastAPI(title="Debug Assistant API")

app.add_middleware(
    CORSMiddleware,
    allow_origins=settings.allowed_origins.split(","),
    allow_methods=["GET", "POST"],
    allow_headers=["*"],
)


@app.get("/health")
async def health():
    try:
        qd = QdrantClient(host=settings.qdrant_host, port=settings.qdrant_port)
        qd.get_collections()
    except Exception:
        logger.error("Qdrant health check failed", exc_info=True)
        return JSONResponse(
            status_code=503,
            content={"status": "unhealthy", "detail": "Qdrant unavailable"},
        )

    try:
        async with httpx.AsyncClient() as client:
            resp = await client.get(f"{settings.ollama_base_url}/api/tags", timeout=5.0)
            resp.raise_for_status()
    except Exception:
        logger.error("Ollama health check failed", exc_info=True)
        return JSONResponse(
            status_code=503,
            content={"status": "unhealthy", "detail": "Ollama unavailable"},
        )

    return {"status": "ok"}
