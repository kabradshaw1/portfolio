import os
import re
import uuid
from io import BytesIO

import httpx
import structlog
from fastapi import Depends, FastAPI, File, HTTPException, Query, UploadFile
from fastapi.middleware.cors import CORSMiddleware
from fastapi.responses import JSONResponse
from llm.factory import get_embedding_provider
from qdrant_client import QdrantClient
from shared.auth import create_auth_dependency
from shared.logging import RequestLoggingMiddleware, configure_logging
from shared.tracing import configure_tracing, instrument_app
from slowapi import Limiter
from slowapi.errors import RateLimitExceeded
from slowapi.util import get_remote_address
from starlette.requests import Request

from app.chunker import chunk_pages
from app.collection_meta import CollectionMetaDB
from app.config import settings
from app.embedder import embed_texts
from app.metrics import CHUNKS_CREATED, instrumentator
from app.pdf_parser import extract_pages
from app.store import QdrantStore

configure_logging(service_name="ingestion")
configure_tracing(service_name="ingestion")

logger = structlog.get_logger()

app = FastAPI(title="Ingestion API")

_COLLECTION_NAME_RE = re.compile(r"^[a-zA-Z0-9_-]{1,100}$")

app.add_middleware(
    CORSMiddleware,
    allow_origins=settings.allowed_origins.split(","),
    allow_methods=["GET", "POST", "DELETE"],
    allow_headers=["Authorization", "Content-Type"],
)
app.add_middleware(RequestLoggingMiddleware)

instrumentator.instrument(app).expose(app, include_in_schema=False)
instrument_app(app)

limiter = Limiter(key_func=get_remote_address)
app.state.limiter = limiter

require_auth = create_auth_dependency(settings.jwt_secret)


@app.exception_handler(RateLimitExceeded)
async def rate_limit_handler(request: Request, exc: RateLimitExceeded):
    return JSONResponse(status_code=429, content={"detail": "Rate limit exceeded"})


_embedding_provider = get_embedding_provider(
    provider=settings.embedding_provider,
    base_url=settings.get_embedding_base_url(),
    api_key=settings.embedding_api_key,
    model=settings.embedding_model,
)

_store: QdrantStore | None = None
_meta_db: CollectionMetaDB | None = None


def get_store() -> QdrantStore:
    global _store
    if _store is None:
        _store = QdrantStore(
            host=settings.qdrant_host,
            port=settings.qdrant_port,
            collection_name=settings.collection_name,
        )
    return _store


async def get_meta_db() -> CollectionMetaDB:
    global _meta_db
    if _meta_db is None:
        os.makedirs(
            os.path.dirname(settings.collection_meta_db_path) or ".", exist_ok=True
        )
        _meta_db = CollectionMetaDB(settings.collection_meta_db_path)
        await _meta_db.init()
    return _meta_db


@app.on_event("shutdown")
async def shutdown_meta_db():
    if _meta_db is not None:
        await _meta_db.close()


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
        llm_ok = await _embedding_provider.check_health()
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


@app.get("/collections")
@limiter.limit("30/minute")
async def list_collections(request: Request, user_id: str = Depends(require_auth)):
    store = get_store()
    try:
        collections = store.list_collections()
    except Exception as e:
        logger.error("Qdrant error listing collections: %s", e, exc_info=True)
        raise HTTPException(status_code=503, detail="Vector store unavailable")
    return {"collections": collections}


@app.get("/collections/{name}/config")
@limiter.limit("30/minute")
async def get_collection_config(
    request: Request, name: str, user_id: str = Depends(require_auth)
):
    """Return the chunk params and embedding model used to populate `name`.

    Read by the eval service at evaluation start so each run is
    self-describing. 404 if the collection has no recorded metadata
    (either the name is unknown or it predates this code).
    """
    if not _COLLECTION_NAME_RE.match(name):
        raise HTTPException(status_code=422, detail="Invalid collection name")
    db = await get_meta_db()
    cfg = await db.get(name)
    if cfg is None:
        raise HTTPException(status_code=404, detail="collection not found")
    return cfg


@app.post("/ingest")
@limiter.limit("5/minute")
async def ingest(
    request: Request,
    file: UploadFile = File(...),
    collection: str | None = Query(default=None),
    user_id: str = Depends(require_auth),
):
    if collection is not None and not _COLLECTION_NAME_RE.match(collection):
        raise HTTPException(status_code=422, detail="Invalid collection name")

    if not file.filename or not file.filename.lower().endswith(".pdf"):
        raise HTTPException(status_code=422, detail="Only PDF files are accepted")

    content = await file.read()
    # Validate PDF magic bytes
    if not content[:5] == b"%PDF-":
        raise HTTPException(status_code=422, detail="File is not a valid PDF")
    max_bytes = settings.max_file_size_mb * 1024 * 1024
    if len(content) > max_bytes:
        raise HTTPException(
            status_code=422,
            detail=f"File exceeds {settings.max_file_size_mb}MB limit",
        )

    try:
        pages = extract_pages(BytesIO(content))
    except ValueError as e:
        # User-facing parse errors (e.g., "No extractable text") — safe to expose
        raise HTTPException(status_code=422, detail=str(e))

    chunks = chunk_pages(
        pages,
        chunk_size=settings.chunk_size,
        chunk_overlap=settings.chunk_overlap,
    )

    if not chunks:
        raise HTTPException(status_code=422, detail="No text content found in PDF")

    texts = [c["text"] for c in chunks]
    try:
        vectors = await embed_texts(
            texts=texts,
            provider=_embedding_provider,
            model=settings.embedding_model,
        )
    except (httpx.ConnectError, httpx.TimeoutException) as e:
        logger.error("embedding_service_error", error=str(e))
        raise HTTPException(status_code=503, detail="Embedding service unavailable")

    document_id = str(uuid.uuid4())
    target_collection = collection or settings.collection_name
    try:
        if collection:
            store = QdrantStore(
                host=settings.qdrant_host,
                port=settings.qdrant_port,
                collection_name=collection,
            )
        else:
            store = get_store()
        store.upsert(
            chunks=chunks,
            vectors=vectors,
            document_id=document_id,
            filename=file.filename,
        )
    except Exception as e:
        logger.error("vector_store_error", error=str(e), exc_info=True)
        raise HTTPException(status_code=503, detail="Vector store unavailable")

    # Record the chunk params and embedding model that produced this
    # collection so the eval service can reproduce/explain its scores later.
    # Idempotent: ON CONFLICT DO UPDATE means re-uploading with new params
    # overwrites — last-writer-wins matches Qdrant's actual on-disk state.
    try:
        meta_db = await get_meta_db()
        await meta_db.upsert(
            collection=target_collection,
            chunk_size=settings.chunk_size,
            chunk_overlap=settings.chunk_overlap,
            embedding_model=settings.embedding_model,
        )
    except Exception as e:
        # Metadata write failure must not poison a successful upload — log it
        # and continue. Eval service treats a missing collection config as
        # "config unavailable" rather than a hard failure.
        logger.error("collection_meta_write_failed", error=str(e), exc_info=True)

    CHUNKS_CREATED.labels(service="ingestion").inc(len(chunks))

    return {
        "status": "success",
        "document_id": document_id,
        "chunks_created": len(chunks),
        "filename": file.filename,
    }


@app.get("/documents")
@limiter.limit("30/minute")
async def list_documents(request: Request, user_id: str = Depends(require_auth)):
    store = get_store()
    return {"documents": store.list_documents()}


@app.delete("/documents/{document_id}")
@limiter.limit("30/minute")
async def delete_document(
    request: Request, document_id: str, user_id: str = Depends(require_auth)
):
    store = get_store()
    chunks_deleted = store.delete_document(document_id)
    if chunks_deleted == 0:
        raise HTTPException(
            status_code=404, detail=f"No document found with id {document_id}"
        )
    return {
        "status": "deleted",
        "document_id": document_id,
        "chunks_deleted": chunks_deleted,
    }


@app.delete("/collections/{collection_name}")
@limiter.limit("30/minute")
async def delete_collection(
    request: Request, collection_name: str, user_id: str = Depends(require_auth)
):
    if not _COLLECTION_NAME_RE.match(collection_name):
        raise HTTPException(status_code=422, detail="Invalid collection name")
    store = get_store()
    try:
        store.delete_collection(collection_name)
    except ValueError as e:
        raise HTTPException(status_code=404, detail=str(e))
    return {"status": "deleted", "collection": collection_name}
