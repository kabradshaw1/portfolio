import json

from fastapi import FastAPI
from fastapi.middleware.cors import CORSMiddleware
from pydantic import BaseModel
from sse_starlette.sse import EventSourceResponse

from app.chain import rag_query
from app.config import settings

app = FastAPI(title="Chat API")

app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_methods=["*"],
    allow_headers=["*"],
)


class ChatRequest(BaseModel):
    question: str
    collection: str | None = None


@app.get("/health")
async def health():
    return {"status": "healthy"}


@app.post("/chat")
async def chat(request: ChatRequest):
    async def event_generator():
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

    return EventSourceResponse(event_generator())
