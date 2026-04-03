import json
import logging

import httpx
from fastapi import FastAPI
from fastapi.middleware.cors import CORSMiddleware
from pydantic import BaseModel, Field
from qdrant_client import QdrantClient
from sse_starlette.sse import EventSourceResponse

from app.chain import rag_query
from app.config import settings

logger = logging.getLogger(__name__)

app = FastAPI(title="Chat API")

app.add_middleware(
    CORSMiddleware,
    allow_origins=settings.allowed_origins.split(","),
    allow_methods=["*"],
    allow_headers=["*"],
)


class ChatRequest(BaseModel):
    question: str = Field(max_length=2000)
    collection: str | None = Field(default=None, pattern=r"^[a-zA-Z0-9_-]{1,100}$")


@app.get("/health")
async def health():
    qdrant_ok = False
    ollama_ok = False

    try:
        qclient = QdrantClient(
            host=settings.qdrant_host, port=settings.qdrant_port, timeout=3
        )
        qclient.get_collections()
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

    from fastapi.responses import JSONResponse

    return JSONResponse(
        status_code=status_code,
        content={
            "status": status,
            "qdrant": "connected" if qdrant_ok else "disconnected",
            "ollama": "connected" if ollama_ok else "disconnected",
        },
    )


@app.post("/chat")
async def chat(request: ChatRequest):
    async def event_generator():
        try:
            async for event in rag_query(
                question=request.question,
                ollama_base_url=settings.ollama_base_url,
                chat_model=settings.chat_model,
                embedding_model=settings.embedding_model,
                qdrant_host=settings.qdrant_host,
                qdrant_port=settings.qdrant_port,
                collection_name=request.collection or settings.collection_name,
            ):
                yield {"data": json.dumps(event)}
        except (httpx.ConnectError, httpx.TimeoutException) as e:
            logger.error("Backend service error: %s", e)
            yield {"data": json.dumps({"error": "Service unavailable"})}
        except Exception as e:
            logger.error("Internal error: %s", e, exc_info=True)
            yield {"data": json.dumps({"error": "Internal error"})}

    return EventSourceResponse(event_generator())
