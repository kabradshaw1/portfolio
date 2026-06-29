import json

import structlog
from fastapi import Depends, FastAPI, HTTPException
from fastapi.middleware.cors import CORSMiddleware
from fastapi.responses import JSONResponse
from pydantic import BaseModel, Field
from shared.auth import create_auth_dependency
from shared.host_validation import HostHeaderValidationMiddleware
from shared.logging import RequestLoggingMiddleware, configure_logging
from shared.tracing import configure_tracing, instrument_app
from slowapi import Limiter
from slowapi.errors import RateLimitExceeded
from slowapi.util import get_remote_address
from sse_starlette.sse import EventSourceResponse
from starlette.requests import Request

from app import fixtures_store
from app.config import settings
from app.graph import build_graph
from app.metrics import INVESTIGATE_ATTEMPTS, instrumentator
from app.roles import model_for
from app.usage import RunAccountant

logger = structlog.get_logger()

configure_logging(service_name="ir-agent")
configure_tracing(service_name="ir-agent")

app = FastAPI(title="IR Agent API")

app.add_middleware(
    CORSMiddleware,
    allow_origins=settings.allowed_origins.split(","),
    allow_methods=["GET", "POST"],
    allow_headers=["Authorization", "Content-Type"],
)
app.add_middleware(RequestLoggingMiddleware)
app.add_middleware(HostHeaderValidationMiddleware)

instrumentator.instrument(app).expose(app, include_in_schema=False)
instrument_app(app)

limiter = Limiter(key_func=get_remote_address)
app.state.limiter = limiter

require_auth = create_auth_dependency(settings.jwt_secret)

# Lazily-built compiled graph (None until first /investigate or test injection).
_graph_app = None


def _build_graph_app():
    settings.validate()
    models = {
        role: model_for(role)
        for role in ("triage", "investigate", "validate", "report")
    }
    return build_graph(
        models,
        max_tool_steps=settings.max_tool_steps,
        max_attempts=settings.max_investigate_attempts,
    )


def _get_graph_app():
    global _graph_app
    if _graph_app is None:
        _graph_app = _build_graph_app()
    return _graph_app


@app.exception_handler(RateLimitExceeded)
async def rate_limit_handler(request: Request, exc: RateLimitExceeded):
    return JSONResponse(status_code=429, content={"detail": "Rate limit exceeded"})


class InvestigateRequest(BaseModel):
    incident_id: str = Field(pattern=r"^[A-Za-z0-9_-]{1,64}$")


@app.get("/health")
async def health():
    api_key_ok = bool(settings.anthropic_api_key)
    fixtures_ok = len(fixtures_store.list_incident_ids()) > 0
    healthy = api_key_ok and fixtures_ok
    return JSONResponse(
        status_code=200 if healthy else 503,
        content={
            "status": "healthy" if healthy else "degraded",
            "anthropic_key": "set" if api_key_ok else "missing",
            "fixtures": "loaded" if fixtures_ok else "missing",
        },
    )


def _serialize(value) -> str:
    return (
        value.model_dump_json()
        if hasattr(value, "model_dump_json")
        else json.dumps(value)
    )


# State keys that map to SSE events streamed to the client.
_STREAM_KEYS = ("triage", "evidence", "findings", "verdict", "report")

# Role -> configured model, for per-role cost attribution in the run summary.
ROLE_MODELS = {
    "triage": settings.triage_model,
    "investigate": settings.investigate_model,
    "validate": settings.validate_model,
    "report": settings.report_model,
}


def _iter_updates(chunk: dict):
    """Yield (state_key, value) pairs from one stream chunk.

    LangGraph's ``stream`` yields ``{node_name: {state_key: value}}`` (the inner
    value is the node's state-update dict). Tests inject a fake graph that yields
    ``{state_key: value}`` directly. This normalizes both shapes: when the value
    is a dict it is a node update to unpack, otherwise the outer key is already a
    state key.
    """
    for outer_key, value in chunk.items():
        if isinstance(value, dict):
            yield from value.items()
        else:
            yield outer_key, value


@app.post("/investigate")
@limiter.limit("10/minute")
async def investigate(
    request: Request, body: InvestigateRequest, user_id: str = Depends(require_auth)
):
    if body.incident_id not in fixtures_store.list_incident_ids():
        raise HTTPException(status_code=400, detail="Unknown incident_id")

    incident = fixtures_store.load_incident(body.incident_id)
    start_state = {"incident": incident, "evidence": [], "investigate_attempts": 0}
    graph_app = _get_graph_app()

    accountant = RunAccountant(ROLE_MODELS)
    config = {"callbacks": [accountant]}

    async def event_generator():
        tool_calls = 0
        attempts = 0
        try:
            for chunk in graph_app.stream(start_state, config=config):
                for key, payload in _iter_updates(chunk):
                    if payload is None:
                        continue
                    if key in _STREAM_KEYS:
                        if key == "evidence":
                            tool_calls = len(payload)
                            data = json.dumps([e.model_dump() for e in payload])
                        else:
                            data = _serialize(payload)
                        yield {"event": key, "data": data}
                    elif key == "investigate_attempts":
                        attempts = payload
                        INVESTIGATE_ATTEMPTS.observe(payload)
            summary = accountant.summary()
            summary["tool_calls"] = tool_calls
            summary["investigate_attempts"] = attempts
            yield {"event": "summary", "data": json.dumps(summary)}
            yield {"event": "done", "data": json.dumps({})}
        except Exception as e:  # noqa: BLE001
            logger.error("investigation_error", error=str(e), exc_info=True)
            yield {
                "event": "error",
                "data": json.dumps({"detail": "Internal error during investigation."}),
            }
            yield {"event": "done", "data": json.dumps({})}

    return EventSourceResponse(event_generator())
