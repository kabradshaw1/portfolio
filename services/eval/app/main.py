import logging

import httpx
from fastapi import FastAPI
from fastapi.middleware.cors import CORSMiddleware
from prometheus_fastapi_instrumentator import Instrumentator
from shared.auth import create_auth_dependency
from slowapi import Limiter, _rate_limit_exceeded_handler
from slowapi.errors import RateLimitExceeded
from slowapi.util import get_remote_address
from starlette.responses import JSONResponse

from app.config import settings

logger = logging.getLogger(__name__)

app = FastAPI(title="Eval API")

app.add_middleware(
    CORSMiddleware,
    allow_origins=settings.allowed_origins.split(","),
    allow_methods=["GET", "POST"],
    allow_headers=["Authorization", "Content-Type"],
)

instrumentator = Instrumentator()
instrumentator.instrument(app).expose(app, include_in_schema=False)

limiter = Limiter(key_func=get_remote_address)
app.state.limiter = limiter
app.add_exception_handler(RateLimitExceeded, _rate_limit_exceeded_handler)

require_auth = create_auth_dependency(settings.jwt_secret)


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
