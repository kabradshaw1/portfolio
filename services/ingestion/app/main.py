import uuid
from io import BytesIO

from fastapi import FastAPI, File, HTTPException, UploadFile
from fastapi.middleware.cors import CORSMiddleware

from app.chunker import chunk_pages
from app.config import settings
from app.embedder import embed_texts
from app.pdf_parser import extract_pages
from app.store import QdrantStore

app = FastAPI(title="Ingestion API")

app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
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
    return {"status": "ok"}


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
    vectors = await embed_texts(
        texts=texts,
        ollama_base_url=settings.ollama_base_url,
        model=settings.embedding_model,
    )

    document_id = str(uuid.uuid4())
    store = get_store()
    store.upsert(
        chunks=chunks,
        vectors=vectors,
        document_id=document_id,
        filename=file.filename,
    )

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
