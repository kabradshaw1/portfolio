# RAG Application

A retrieval-augmented generation (RAG) Q&A application built with FastAPI, LangChain, ChromaDB, and Ollama. Demonstrates RAG architecture, hybrid prompting strategies, and containerized deployment.

## Architecture

```
User Query → FastAPI API → RAG Chain → Ollama LLM → Response
                              ↓
                    ChromaDB Vector Store
                    (embeddings + chunks)
```

## Hybrid Prompting

The RAG chain implements hybrid prompting by combining three layers:

1. **System instructions** — role definition, output format, behavioral constraints
2. **Retrieved context** — relevant document chunks from ChromaDB
3. **Dynamic user input** — the user's actual query

## API Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/ingest` | Upload and process documents into ChromaDB |
| POST | `/query` | Ask a question, get a RAG-powered answer |
| GET | `/health` | Health check |

## Setup

### With Docker (recommended)

```bash
# Requires Ollama running locally
docker-compose up
```

### Without Docker

```bash
pip install -r requirements.txt
uvicorn app.main:app --reload
```

## Tech Stack

- **FastAPI** — async web framework
- **LangChain** — LLM orchestration
- **ChromaDB** — vector database
- **Ollama** — local LLM inference
- **Docker** — containerization
