import json
import logging

import httpx
from fastapi import FastAPI
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

from app.chain import rag_query
from app.config import settings
from app.metrics import instrumentator

logger = logging.getLogger(__name__)

app = FastAPI(title="Chat API")

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


class ChatRequest(BaseModel):
    question: str = Field(max_length=2000)
    collection: str | None = Field(default=None, pattern=r"^[a-zA-Z0-9_-]{1,100}$")


@app.get("/health")
async def health():
    qdrant_ok = False
    llm_ok = False

    try:
        qclient = QdrantClient(
            host=settings.qdrant_host, port=settings.qdrant_port, timeout=3
        )
        qclient.get_collections()
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


@app.post("/chat")
@limiter.limit("20/minute")
async def chat(request: Request, body: ChatRequest):
    async def event_generator():
        try:
            async for event in rag_query(
                question=body.question,
                llm_provider=_llm_provider,
                embedding_provider=_embedding_provider,
                chat_model=settings.get_llm_model(),
                embedding_model=settings.embedding_model,
                qdrant_host=settings.qdrant_host,
                qdrant_port=settings.qdrant_port,
                collection_name=body.collection or settings.collection_name,
            ):
                yield {"data": json.dumps(event)}
        except (httpx.ConnectError, httpx.TimeoutException) as e:
            logger.error("Backend service error: %s", e)
            yield {"data": json.dumps({"error": "Service unavailable"})}
        except Exception as e:
            logger.error("Internal error: %s", e, exc_info=True)
            yield {"data": json.dumps({"error": "Internal error"})}

    return EventSourceResponse(event_generator())
