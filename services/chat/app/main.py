import json

import httpx
import structlog
from fastapi import Depends, FastAPI, HTTPException
from fastapi.middleware.cors import CORSMiddleware
from fastapi.responses import JSONResponse
from llm.factory import get_embedding_provider, get_llm_provider
from pydantic import BaseModel, Field
from qdrant_client import QdrantClient
from shared.auth import AuthContext, create_auth_context_dependency
from shared.llm.admission import AdmissionRejected
from shared.logging import RequestLoggingMiddleware, configure_logging
from shared.rate_limits import FixedWindowRateLimiter, policies_from_settings
from shared.tracing import configure_tracing, instrument_app
from slowapi import Limiter
from slowapi.errors import RateLimitExceeded
from slowapi.util import get_remote_address
from sse_starlette.sse import EventSourceResponse
from starlette.requests import Request

from app.chain import rag_query, retrieve_chunks
from app.config import settings
from app.metrics import instrumentator

logger = structlog.get_logger()

configure_logging(service_name="chat")
configure_tracing(service_name="chat")

app = FastAPI(title="Chat API")

app.add_middleware(
    CORSMiddleware,
    allow_origins=settings.allowed_origins.split(","),
    allow_methods=["GET", "POST"],
    allow_headers=["Authorization", "Content-Type", "X-RAG-Internal-Token"],
)
app.add_middleware(RequestLoggingMiddleware)

instrumentator.instrument(app).expose(app, include_in_schema=False)
instrument_app(app)

limiter = Limiter(key_func=get_remote_address)
app.state.limiter = limiter


@app.exception_handler(RateLimitExceeded)
async def rate_limit_handler(request: Request, exc: RateLimitExceeded):
    return JSONResponse(status_code=429, content={"detail": "Rate limit exceeded"})


def build_chat_rate_limiter() -> FixedWindowRateLimiter:
    limiter = FixedWindowRateLimiter(
        policies_from_settings(
            {
                "chat_ask": {
                    "operator": settings.chat_rate_limit_ask_operator,
                    "user": settings.chat_rate_limit_ask_user,
                    "anonymous": settings.chat_rate_limit_ask_anonymous,
                    "internal_eval": settings.chat_rate_limit_ask_internal_eval,
                },
                "chat_search": {
                    "operator": settings.chat_rate_limit_search_operator,
                    "user": settings.chat_rate_limit_search_user,
                    "anonymous": settings.chat_rate_limit_search_anonymous,
                    "internal_eval": settings.chat_rate_limit_search_internal_eval,
                },
            }
        )
    )
    limiter.enabled = True
    return limiter


chat_rate_limiter = build_chat_rate_limiter()


async def _resolve_auth_context(request: Request) -> AuthContext:
    internal_token = request.headers.get("x-rag-internal-token")
    if (
        settings.rag_internal_eval_token
        and internal_token == settings.rag_internal_eval_token
    ):
        return AuthContext(subject="internal_eval", email=None, tier="internal_eval")
    dependency = create_auth_context_dependency(settings.jwt_secret)
    return await dependency(request, None)


async def _enforce_rate_limit(group: str, request: Request) -> AuthContext:
    context = await _resolve_auth_context(request)
    if not getattr(chat_rate_limiter, "enabled", True):
        return context
    decision = chat_rate_limiter.check(group, context)
    if not decision.allowed:
        raise HTTPException(
            status_code=429,
            detail="Rate limit exceeded",
            headers={"Retry-After": str(decision.retry_after_seconds)},
        )
    return context


async def enforce_chat_ask(request: Request) -> AuthContext:
    return await _enforce_rate_limit("chat_ask", request)


async def enforce_chat_search(request: Request) -> AuthContext:
    return await _enforce_rate_limit("chat_search", request)


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
    rerank: bool = False


class SearchRequest(BaseModel):
    query: str = Field(max_length=2000)
    collection: str | None = Field(default=None, pattern=r"^[a-zA-Z0-9_-]{1,100}$")
    limit: int = Field(default=5, ge=1, le=20)
    rerank: bool = False


@app.get("/config")
async def get_config():
    """Read-only RAG-config snapshot. Consumed by the eval service to record
    the parameters in effect for each evaluation run. Intentionally returns
    no secrets (no base URLs, no API keys).
    """
    return {
        "llm_model": settings.get_llm_model(),
        "embedding_model": settings.embedding_model,
        "top_k": settings.top_k,
        "prompt_version": settings.prompt_version,
        "retrieval_mode": settings.retrieval_mode,
        "hybrid_prefetch_limit": settings.hybrid_prefetch_limit,
        "dense_vector_name": settings.dense_vector_name,
        "sparse_vector_name": settings.sparse_vector_name,
        "sparse_model": settings.sparse_model,
        "fusion": "rrf" if settings.retrieval_mode == "hybrid" else None,
        "rerank_enabled": settings.rerank_enabled,
        "rerank_model": settings.rerank_model,
        "rerank_candidate_limit": settings.rerank_candidate_limit,
        "rerank_max_candidates": settings.rerank_max_candidates,
        "rerank_device": settings.rerank_device,
    }


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
async def chat(
    request: Request,
    body: ChatRequest,
    auth_context: AuthContext = Depends(enforce_chat_ask),
):
    wants_json = request.headers.get("accept", "").startswith("application/json")

    if wants_json:
        try:
            tokens = []
            sources = []
            retrieval = {}
            async for event in rag_query(
                question=body.question,
                llm_provider=_llm_provider,
                embedding_provider=_embedding_provider,
                chat_model=settings.get_llm_model(),
                embedding_model=settings.embedding_model,
                qdrant_host=settings.qdrant_host,
                qdrant_port=settings.qdrant_port,
                collection_name=body.collection or settings.collection_name,
                top_k=settings.top_k,
                rerank=body.rerank,
            ):
                if "token" in event:
                    tokens.append(event["token"])
                if event.get("done"):
                    sources = event.get("sources", [])
                    retrieval = event.get("retrieval", {})
            return {
                "answer": "".join(tokens),
                "sources": sources,
                "retrieval": retrieval,
            }
        except (httpx.ConnectError, httpx.TimeoutException) as e:
            logger.error("Backend service error: %s", e)
            raise HTTPException(status_code=503, detail="Service unavailable")
        except AdmissionRejected as e:
            raise HTTPException(
                status_code=503,
                detail="LLM service overloaded",
                headers={"Retry-After": str(e.retry_after_seconds)},
            ) from e
        except Exception as e:
            logger.error("Internal error: %s", e, exc_info=True)
            raise HTTPException(status_code=500, detail="Internal error")

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
                top_k=settings.top_k,
                rerank=body.rerank,
            ):
                yield {"data": json.dumps(event)}
        except (httpx.ConnectError, httpx.TimeoutException) as e:
            logger.error("backend_service_error", error=str(e))
            yield {"data": json.dumps({"error": "Service unavailable"})}
        except AdmissionRejected as e:
            yield {
                "data": json.dumps(
                    {
                        "error": "LLM service overloaded",
                        "retry_after_seconds": e.retry_after_seconds,
                    }
                )
            }
        except Exception as e:
            logger.error("internal_error", error=str(e), exc_info=True)
            yield {"data": json.dumps({"error": "Internal error"})}

    return EventSourceResponse(event_generator())


@app.post("/search")
async def search(
    request: Request,
    body: SearchRequest,
    auth_context: AuthContext = Depends(enforce_chat_search),
):
    try:
        retrieval = await retrieve_chunks(
            question=body.query,
            embedding_provider=_embedding_provider,
            embedding_model=settings.embedding_model,
            qdrant_host=settings.qdrant_host,
            qdrant_port=settings.qdrant_port,
            collection_name=body.collection or settings.collection_name,
            top_k=body.limit,
            rerank=body.rerank,
        )
    except (httpx.ConnectError, httpx.TimeoutException) as e:
        logger.error("Embedding service error: %s", e)
        raise HTTPException(status_code=503, detail="Embedding service unavailable")
    except AdmissionRejected as e:
        raise HTTPException(
            status_code=503,
            detail="LLM service overloaded",
            headers={"Retry-After": str(e.retry_after_seconds)},
        ) from e

    return {
        "results": [
            {
                "text": c["text"],
                "filename": c["filename"],
                "page_number": c["page_number"],
                "score": c["score"],
            }
            for c in retrieval.chunks
        ],
        "retrieval": retrieval.metadata,
    }
