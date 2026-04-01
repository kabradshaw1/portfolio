import uuid
from io import BytesIO

import httpx
from fastapi import FastAPI, File, HTTPException, UploadFile
from fastapi.middleware.cors import CORSMiddleware
from qdrant_client import QdrantClient

from app.chunker import chunk_pages
from app.config import settings
from app.embedder import embed_texts
from app.pdf_parser import extract_pages
from app.store import QdrantStore

app = FastAPI(title="Ingestion API")

app.add_middleware(
    CORSMiddleware,
    allow_origins=settings.allowed_origins.split(","),
    allow_methods=["*"],
    allow_headers=["*"],
)

_store: QdrantStore | None = None


def get_store() -> QdrantStore:
    global _store
    if _store is None:
        _store = QdrantStore(
            host=settings.qdrant_host,
            port=settings.qdrant_port,
            collection_name=settings.collection_name,
        )
    return _store


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


@app.post("/ingest")
async def ingest(file: UploadFile = File(...)):
    if not file.filename or not file.filename.lower().endswith(".pdf"):
        raise HTTPException(status_code=422, detail="Only PDF files are accepted")

    content = await file.read()
    max_bytes = settings.max_file_size_mb * 1024 * 1024
    if len(content) > max_bytes:
        raise HTTPException(
            status_code=422,
            detail=f"File exceeds {settings.max_file_size_mb}MB limit",
        )

    try:
        pages = extract_pages(BytesIO(content))
    except ValueError as e:
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
            ollama_base_url=settings.ollama_base_url,
            model=settings.embedding_model,
        )
    except (httpx.ConnectError, httpx.TimeoutException) as e:
        raise HTTPException(
            status_code=503, detail=f"Embedding service unavailable: {e}"
        )

    document_id = str(uuid.uuid4())
    try:
        store = get_store()
        store.upsert(
            chunks=chunks,
            vectors=vectors,
            document_id=document_id,
            filename=file.filename,
        )
    except Exception as e:
        raise HTTPException(status_code=503, detail=f"Vector store unavailable: {e}")

    return {
        "status": "success",
        "document_id": document_id,
        "chunks_created": len(chunks),
        "filename": file.filename,
    }


@app.get("/documents")
async def list_documents():
    store = get_store()
    return {"documents": store.list_documents()}
