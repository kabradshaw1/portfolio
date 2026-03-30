# Gen AI Engineer Portfolio

A portfolio project demonstrating proficiency in Python, AI/ML workflows, and Generative AI — built to showcase skills relevant to a Gen AI Engineer role.

## Project Structure

| Section | Description | Key Skills |
|---------|-------------|------------|
| [01_python_refresher](./01_python_refresher/) | Core Python exercises covering data structures, OOP, async, type hints, and data processing | Python, pandas, numpy |
| [02_nlp_fundamentals](./02_nlp_fundamentals/) | NLP concept demos: tokenization, embeddings, cosine similarity, NER, text classification | HuggingFace, spaCy, NLP |
| [03_rag_app](./03_rag_app/) | RAG-powered Q&A app with hybrid prompting, built with FastAPI, LangChain, and ChromaDB | FastAPI, LangChain, RAG, Prompt Engineering, Docker |

## Prior Work

I previously built an LLM-powered workflow using **Ollama** with **RAG** in a larger Go-based application. That project demonstrates agent-like system design and prompt engineering strategies — retrieving information from a database based on user prompts and structuring prompts for the LLM.

> Repository: *[Link to Go project — coming soon]*

## Skills Demonstrated

### Core Requirements

- **Python** — exercises and applications throughout all sections
- **API Development (FastAPI)** — REST API backend in the RAG app
- **Data Processing** — pandas/numpy workflows in the Python refresher
- **LLM & Prompt Engineering** — LangChain-based RAG pipeline with structured prompts
- **RAG Architecture** — document ingestion, vector storage, retrieval-augmented generation
- **Hybrid Prompting** — combining system instructions, retrieved context, and dynamic user input
- **LangChain** — orchestration framework for the RAG pipeline
- **HuggingFace** — tokenizers, transformers, and sentence-transformers in NLP demos

### Good-to-Have

- **NLP Fundamentals** — tokenization, embeddings, cosine similarity, NER, text classification
- **Docker** — containerized RAG app with docker-compose
- **Git** — version-controlled development throughout

## Getting Started

Each section is self-contained with its own `requirements.txt` and README.

### Python Refresher

```bash
cd 01_python_refresher
pip install -r requirements.txt
python data_structures.py  # Run any script directly
```

### NLP Fundamentals

```bash
cd 02_nlp_fundamentals
pip install -r requirements.txt
python -m spacy download en_core_web_sm
python tokenization.py     # Run individual scripts
jupyter notebook           # Or open the unified notebook
```

### RAG App

```bash
cd 03_rag_app
docker-compose up          # Starts FastAPI app + ChromaDB
# Requires Ollama running locally: https://ollama.ai
```

Or run without Docker:

```bash
cd 03_rag_app
pip install -r requirements.txt
uvicorn app.main:app --reload
```
