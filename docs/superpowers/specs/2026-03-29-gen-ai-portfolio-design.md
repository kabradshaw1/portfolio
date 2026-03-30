# Gen AI Engineer Portfolio — Design Spec

## Context

This repo serves as a portfolio project for a Gen AI Engineer job application. It demonstrates proficiency across the job's core requirements: Python, FastAPI, LangChain, RAG architecture, prompt engineering, hybrid prompting, NLP fundamentals, and Docker containerization. It also references prior work (a Go-based LLM workflow with Ollama and RAG) to show breadth of experience.

## Repo Structure

Numbered top-level directories provide a natural reading order. Each section is self-contained with its own `requirements.txt`.

```
gen_ai_engineer/
├── README.md
├── .gitignore
├── 01_python_refresher/
├── 02_nlp_fundamentals/
├── 03_rag_app/
└── docs/
```

## README.md

The top-level README is the entry point for a hiring manager. It will:

- State the repo's purpose (portfolio for Gen AI Engineer role)
- Map each section to specific job requirements with brief explanations
- Link to the prior Go project (Ollama + RAG) with a description of what it demonstrates
- Include per-section setup/run instructions
- Keep prose concise — this is a skills demo, not a tutorial

## Section 1: Python Refresher (01_python_refresher/)

Quick, standalone Python scripts (50-100 lines each) covering core competencies:

| Script | Covers | Job Relevance |
|--------|--------|---------------|
| `data_structures.py` | Lists, dicts, sets, comprehensions, generators | Core Python |
| `oop_patterns.py` | Classes, inheritance, ABCs, dataclasses | Code structure |
| `async_basics.py` | asyncio, async/await, concurrent tasks | FastAPI uses async |
| `type_hints.py` | Type annotations, generics, Protocol | Code quality |
| `data_processing.py` | pandas/numpy data manipulation | "Data Processing" requirement |

Each script has a docstring, working examples, and visible output when run.

## Section 2: NLP Fundamentals (02_nlp_fundamentals/)

### Individual scripts (one per concept):

- `tokenization.py` — word/subword tokenization using HuggingFace tokenizers
- `embeddings.py` — generating and comparing embeddings (sentence-transformers)
- `cosine_similarity.py` — computing similarity between text pairs
- `ner.py` — Named Entity Recognition using spaCy
- `text_classification.py` — sentiment/topic classification using HuggingFace transformers

### Unified notebook:

`nlp_unified_notebook.ipynb` — walks through all concepts in sequence with markdown explanations, visualizations, and a cohesive narrative.

### Libraries:

spaCy, HuggingFace transformers, sentence-transformers, scikit-learn (for metrics)

## Section 3: RAG App (03_rag_app/)

The main showcase. A FastAPI backend demonstrating RAG architecture with hybrid prompting.

### Architecture

```
User Query → FastAPI API → RAG Chain → Ollama LLM → Response
                              ↓
                    ChromaDB Vector Store
                    (embeddings + chunks)
```

### Components

```
03_rag_app/
├── app/
│   ├── main.py            # FastAPI entry point
│   ├── config.py          # Settings/env config
│   ├── api/
│   │   └── routes.py      # API endpoints
│   ├── rag/
│   │   ├── chain.py       # LangChain RAG chain with hybrid prompting
│   │   ├── embeddings.py  # Embedding generation
│   │   ├── vectorstore.py # ChromaDB integration
│   │   └── prompts.py     # Hybrid prompt templates
│   └── ingestion/
│       └── loader.py      # Document loading and chunking
├── data/                  # Sample documents
├── Dockerfile
├── docker-compose.yml
└── requirements.txt
```

### API Endpoints

- `POST /ingest` — upload and process documents into ChromaDB
- `POST /query` — ask a question, get a RAG-powered answer
- `GET /health` — health check

### Hybrid Prompting Strategy

The RAG chain implements hybrid prompting by combining three layers:
1. **System instructions** — role definition, output format, behavioral constraints
2. **Retrieved context** — relevant document chunks from ChromaDB
3. **Dynamic user input** — the user's actual query

These are structured in a prompt template that LangChain assembles per request.

### Tech Stack

- **Framework**: FastAPI
- **LLM orchestration**: LangChain
- **Vector DB**: ChromaDB
- **LLM**: Ollama (local, open-source models)
- **Containerization**: Docker + docker-compose

### Docker Setup

- `Dockerfile` — multi-stage build for the FastAPI app
- `docker-compose.yml` — orchestrates app + ChromaDB (Ollama assumed running on host)

## Job Requirements Coverage

| Requirement | Where Demonstrated |
|-------------|-------------------|
| Strong Python knowledge | 01_python_refresher/, all sections |
| FastAPI API development | 03_rag_app/ |
| Data Processing with Python | 01_python_refresher/data_processing.py |
| LLM, Prompt Engineering, RAG | 03_rag_app/ |
| Hybrid prompting | 03_rag_app/app/rag/prompts.py |
| LangChain | 03_rag_app/ |
| HuggingFace | 02_nlp_fundamentals/ |
| NLP | 02_nlp_fundamentals/ |
| Docker | 03_rag_app/Dockerfile, docker-compose.yml |
| Git | This repo itself |
| Prior RAG/agent work | Linked Go project |

## Verification

1. **Python refresher**: Each script runs standalone (`python data_structures.py`) and produces output
2. **NLP demos**: Each script runs standalone; notebook runs end-to-end in Jupyter
3. **RAG app**: `docker-compose up` starts the app; POST to `/ingest` then `/query` returns RAG-powered responses
4. **README**: Review for clarity, completeness, and professional tone
