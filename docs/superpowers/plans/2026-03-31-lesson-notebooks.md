# Lesson Notebooks Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Create 7 Jupyter notebooks that walk through rebuilding both Python backend services from scratch, with package explanations, Go/TS comparisons, and experiment cells.

**Architecture:** Each notebook is a standalone `.ipynb` file in `lessons/`. A shared `requirements.txt` covers all dependencies. Notebooks follow a consistent structure: intro → prerequisites → package introductions → Go/TS comparison → build it → experiment → check your understanding.

**Tech Stack:** Jupyter, FastAPI, PyPDF2, LangChain, httpx, Qdrant, Ollama, pydantic-settings, sse-starlette, numpy, fpdf2

**Note on notebook creation:** Each task creates one `.ipynb` file using the NotebookEdit tool. Notebooks contain a mix of markdown and code cells. Code cells should be runnable — they produce the actual working code from the services.

---

## File Structure

```
lessons/
├── requirements.txt
├── 01_python_fastapi_basics.ipynb
├── 02_pdf_parsing_and_chunking.ipynb
├── 03_embeddings_and_vectors.ipynb
├── 04_qdrant_vector_storage.ipynb
├── 05_rag_chain_and_prompts.ipynb
├── 06_streaming_and_sse.ipynb
└── 07_wiring_the_endpoints.ipynb
```

---

### Task 1: Scaffolding + requirements.txt

**Files:**
- Create: `lessons/requirements.txt`

- [ ] **Step 1: Create lessons directory and requirements.txt**

`lessons/requirements.txt`:
```
fastapi==0.115.0
uvicorn[standard]==0.30.0
python-multipart==0.0.9
pypdf2==3.0.1
langchain-text-splitters==0.2.0
qdrant-client==1.9.0
httpx==0.27.0
pydantic-settings==2.3.0
sse-starlette==2.1.0
numpy
fpdf2
jupyter
```

- [ ] **Step 2: Install dependencies**

Run: `cd lessons && pip install -r requirements.txt`

- [ ] **Step 3: Commit**

```bash
git add lessons/requirements.txt
git commit -m "scaffold: add lessons directory with requirements.txt"
```

---

### Task 2: Lesson 01 — Python & FastAPI Basics

**Files:**
- Create: `lessons/01_python_fastapi_basics.ipynb`

This notebook covers: FastAPI app creation, decorator routing, Pydantic models, async handlers, type hints, and pydantic-settings config. No external services needed.

- [ ] **Step 1: Create the notebook**

The notebook should contain these cells in order:

**Cell 1 (markdown):**
```markdown
# Lesson 01: Python & FastAPI Basics

## What You're Building

In this lesson you'll build a minimal FastAPI web service from scratch — the same foundation used by both the Ingestion API and Chat API in our Document Q&A app. By the end, you'll have a running service with a health endpoint, request validation via Pydantic, and environment-driven configuration.

FastAPI is the framework we chose for both backend services. It's a modern Python web framework that gives you automatic request validation, API documentation, and async support out of the box. If you've built HTTP services in Go with chi/gin or in TypeScript with Express, the concepts are the same — the syntax is just different.

## How This Fits in the App

Every endpoint in both services (POST /ingest, GET /documents, POST /chat, GET /health) is a FastAPI route. The patterns you learn here — route decorators, Pydantic models, async handlers, and settings — appear in every file you'll write in lessons 02-07.
```

**Cell 2 (code):**
```python
# Prerequisites — nothing external needed for this lesson
# Just make sure you have the packages installed:
# pip install fastapi uvicorn pydantic-settings
import fastapi
import pydantic_settings
print(f"FastAPI version: {fastapi.__version__}")
print("Ready to go!")
```

**Cell 3 (markdown):**
```markdown
## Package Introductions

### FastAPI
FastAPI is a Python web framework built on top of Starlette (for HTTP) and Pydantic (for data validation). It was chosen for this project because:
- **Automatic validation** — request bodies are validated against Pydantic models before your code runs. In Go, you'd need to manually unmarshal and validate. In Express, you'd add middleware like `joi` or `zod`.
- **Async native** — route handlers can be `async def`, which matters when calling external services like Ollama and Qdrant.
- **Auto-generated docs** — visit `/docs` on any FastAPI app to get interactive Swagger UI. Free.

Key APIs you'll use:
- `FastAPI()` — create an app instance
- `@app.get("/path")`, `@app.post("/path")` — route decorators
- `HTTPException` — raise HTTP errors with status codes

### Pydantic
Pydantic is a data validation library. You define a class with type annotations and Pydantic ensures incoming data matches. Think of it as Go structs with `json` tags and built-in validation, or TypeScript interfaces that are enforced at runtime.

Key APIs:
- `BaseModel` — base class for request/response models
- `BaseSettings` (from pydantic-settings) — reads config from environment variables automatically

### Uvicorn
Uvicorn is an ASGI server — it's what actually runs your FastAPI app and handles HTTP connections. Think of it as the equivalent of Go's `http.ListenAndServe()` or Node's `app.listen()`. You won't interact with its API directly; you just point it at your app.
```

**Cell 4 (markdown):**
```markdown
## Go/TS Comparison

| Concept | Go | TypeScript/Express | Python/FastAPI |
|---------|----|--------------------|----------------|
| Define a route | `r.Get("/health", handler)` | `app.get('/health', handler)` | `@app.get("/health")` decorator |
| Request body | `json.Unmarshal` into struct | `req.body` (unvalidated) or zod | Pydantic model as param (auto-validated) |
| Response | `json.NewEncoder(w).Encode(data)` | `res.json(data)` | `return data` (auto-serialized) |
| Config from env | `envconfig.Process(&cfg)` | `process.env.VAR` | `BaseSettings` class (auto-reads env) |
| Async I/O | goroutines (implicit) | `async/await` or callbacks | `async def` + `await` |

The biggest shift: In Go, concurrency is built into the runtime (goroutines). In Python, you explicitly mark functions as `async` and use `await` for I/O operations. If you forget `async`, the function blocks the entire server while waiting for a response.
```

**Cell 5 (markdown):**
```markdown
## Build It

### Step 1: Create a minimal FastAPI app

This is the equivalent of `http.NewServeMux()` in Go or `express()` in Node.
```

**Cell 6 (code):**
```python
from fastapi import FastAPI

app = FastAPI(title="Ingestion API")

@app.get("/health")
async def health():
    return {"status": "ok"}

# In a real server, you'd run: uvicorn main:app --port 8000
# In a notebook, we can test using FastAPI's test client:
from fastapi.testclient import TestClient
client = TestClient(app)

response = client.get("/health")
print(f"Status: {response.status_code}")
print(f"Body: {response.json()}")
```

**Cell 7 (markdown):**
```markdown
### Step 2: Add a Pydantic request model

Pydantic models define the shape of your request data. When a request comes in, FastAPI automatically:
1. Parses the JSON body
2. Validates it against your model
3. Returns a 422 error if validation fails

This is like defining a Go struct with `json` tags — but validation happens automatically. No `if err := json.Unmarshal(...); err != nil` needed.
```

**Cell 8 (code):**
```python
from pydantic import BaseModel

class ChatRequest(BaseModel):
    question: str
    collection: str | None = None  # Optional with default None

# Test it — valid input
valid = ChatRequest(question="What is revenue?")
print(f"Question: {valid.question}")
print(f"Collection: {valid.collection}")  # None (default)

# Test it — with collection
with_collection = ChatRequest(question="What is revenue?", collection="documents")
print(f"Collection: {with_collection.collection}")

# Test it — invalid input (missing required field)
try:
    bad = ChatRequest()
except Exception as e:
    print(f"Validation error: {e}")
```

**Cell 9 (markdown):**
```markdown
### Step 3: Use the model in a route

When you put a Pydantic model as a function parameter, FastAPI knows it's the request body. It parses, validates, and passes it to your function — or returns 422 if invalid.
```

**Cell 10 (code):**
```python
from fastapi import FastAPI
from pydantic import BaseModel
from fastapi.testclient import TestClient

app = FastAPI(title="Chat API")

class ChatRequest(BaseModel):
    question: str
    collection: str | None = None

@app.get("/health")
async def health():
    return {"status": "ok"}

@app.post("/chat")
async def chat(request: ChatRequest):
    return {
        "echo": request.question,
        "collection": request.collection or "default",
    }

client = TestClient(app)

# Valid request
response = client.post("/chat", json={"question": "What is revenue?"})
print(f"Status: {response.status_code}")
print(f"Body: {response.json()}")

# Invalid request — missing question
response = client.post("/chat", json={})
print(f"\nInvalid request status: {response.status_code}")
print(f"Error detail: {response.json()['detail'][0]['msg']}")
```

**Cell 11 (markdown):**
```markdown
### Step 4: Environment-driven configuration with pydantic-settings

In Go you might use `envconfig` or `viper` to read environment variables into a config struct. Python's `pydantic-settings` does the same thing — you define a class with defaults, and it automatically reads matching environment variables.

The field name `qdrant_host` matches the env var `QDRANT_HOST` (case-insensitive). This is how both services get their config.
```

**Cell 12 (code):**
```python
from pydantic_settings import BaseSettings

class Settings(BaseSettings):
    ollama_base_url: str = "http://host.docker.internal:11434"
    embedding_model: str = "nomic-embed-text"
    qdrant_host: str = "qdrant"
    qdrant_port: int = 6333
    collection_name: str = "documents"
    chunk_size: int = 1000
    chunk_overlap: int = 200
    max_file_size_mb: int = 50

settings = Settings()

print(f"Ollama URL: {settings.ollama_base_url}")
print(f"Qdrant: {settings.qdrant_host}:{settings.qdrant_port}")
print(f"Chunk size: {settings.chunk_size}")
print(f"Collection: {settings.collection_name}")

# If you set CHUNK_SIZE=500 in your environment, it would override the default.
# Try it: import os; os.environ["CHUNK_SIZE"] = "500"
# Then re-create Settings() and check settings.chunk_size
```

**Cell 13 (markdown):**
```markdown
> **Pitfall:** Claude Code often generates synchronous route handlers (`def health()` instead of `async def health()`). For simple endpoints that don't do I/O, it doesn't matter. But for handlers that call Ollama or Qdrant (which we'll build later), forgetting `async` will block the entire server during the request. Always use `async def` for FastAPI routes.
```

**Cell 14 (markdown):**
```markdown
## Experiment

Try these modifications to build intuition:
```

**Cell 15 (code):**
```python
# Experiment 1: Add a query parameter
# In Go: r.URL.Query().Get("name")
# In Express: req.query.name
# In FastAPI: just add it as a function parameter

@app.get("/greet")
async def greet(name: str = "world"):
    return {"message": f"Hello, {name}!"}

client = TestClient(app)
print(client.get("/greet").json())
print(client.get("/greet?name=Kyle").json())
```

**Cell 16 (code):**
```python
# Experiment 2: Add validation constraints
# Pydantic lets you add constraints that Go would need custom validation for

from pydantic import BaseModel, Field

class IngestRequest(BaseModel):
    filename: str = Field(min_length=1, max_length=255)
    max_pages: int = Field(default=100, ge=1, le=1000)

# Valid
print(IngestRequest(filename="report.pdf").model_dump())

# Invalid — empty filename
try:
    IngestRequest(filename="")
except Exception as e:
    print(f"Validation error: {e}")
```

**Cell 17 (markdown):**
```markdown
## Check Your Understanding

1. **In your own words, what does a Pydantic model do that a plain Python dictionary doesn't?**

2. **Why do we use `async def` for route handlers instead of regular `def`? When does it matter?**

3. **How does `pydantic-settings` know to read `QDRANT_HOST` from the environment for the `qdrant_host` field?**
```

- [ ] **Step 2: Verify the notebook runs**

Open the notebook in Jupyter or VS Code and run all cells. All cells should execute without errors.

- [ ] **Step 3: Commit**

```bash
git add lessons/01_python_fastapi_basics.ipynb
git commit -m "lesson: add 01_python_fastapi_basics notebook"
```

---

### Task 3: Lesson 02 — PDF Parsing & Chunking

**Files:**
- Create: `lessons/02_pdf_parsing_and_chunking.ipynb`

This notebook covers: PyPDF2 for text extraction, LangChain text splitters, BytesIO, chunking strategies. No external services needed.

- [ ] **Step 1: Create the notebook**

The notebook should contain these cells in order:

**Cell 1 (markdown):**
```markdown
# Lesson 02: PDF Parsing & Chunking

## What You're Building

In this lesson you'll build two functions that form the first half of the ingestion pipeline: `extract_pages()` takes a PDF file and returns text organized by page, and `chunk_pages()` splits that text into overlapping segments suitable for embedding.

These two functions are the entry point for every document in the system. When a user uploads a PDF, the ingestion service runs it through extract → chunk → embed → store. You're building the first two steps.

## How This Fits in the App

In the ingestion service (`services/ingestion/app/`), these live in `pdf_parser.py` and `chunker.py`. The `/ingest` endpoint calls `extract_pages()` first, then passes the result to `chunk_pages()`, then sends chunks to the embedder.
```

**Cell 2 (code):**
```python
# Prerequisites — no external services needed
# pip install pypdf2 langchain-text-splitters fpdf2
from PyPDF2 import PdfReader, PdfWriter
from langchain_text_splitters import RecursiveCharacterTextSplitter
from fpdf import FPDF
print("All packages loaded!")
```

**Cell 3 (markdown):**
```markdown
## Package Introductions

### PyPDF2
PyPDF2 is a pure-Python library for reading and manipulating PDF files. We chose it because:
- **No system dependencies** — unlike `pdfminer` (complex) or `poppler` (requires C library install), PyPDF2 is pure Python and installs cleanly everywhere.
- **Simple API** — create a `PdfReader`, iterate `.pages`, call `.extract_text()` on each page.
- **Good enough for text-heavy PDFs** — it struggles with complex layouts, scanned images, or tables, but for documents with flowing text (reports, papers, manuals) it works well.

Note: PyPDF2 is technically deprecated in favor of `pypdf` (same authors, renamed). We're using PyPDF2 here because the API is identical and it's what the app uses. In a new project, use `pypdf`.

### LangChain Text Splitters
LangChain is a large framework for building LLM applications. We're only using one small piece: `langchain-text-splitters`. This sub-package provides text chunking strategies.

`RecursiveCharacterTextSplitter` is the one we use. It tries to split text at natural boundaries (paragraphs first, then sentences, then words) while keeping chunks under a target size. The "recursive" part means it tries multiple separators in order: `\n\n`, `\n`, ` `, then individual characters.

### fpdf2
A simple library for creating PDFs programmatically. We only use this to create test PDFs in the notebook — it's not part of the actual app.
```

**Cell 4 (markdown):**
```markdown
## Go/TS Comparison

| Concept | Go | Python |
|---------|-----|--------|
| In-memory file buffer | `bytes.Buffer` / `io.Reader` | `io.BytesIO` |
| Reading a file-like object | `io.ReadAll(reader)` | `file.read()` / `file.seek(0)` |
| Error for bad input | `return nil, fmt.Errorf(...)` | `raise ValueError(...)` |

The `BytesIO` pattern is important — in Python, many libraries expect a "file-like object" (something with `.read()` and `.seek()` methods). `BytesIO` wraps raw bytes to look like a file. This is exactly like wrapping `[]byte` in a `bytes.Reader` in Go.
```

**Cell 5 (markdown):**
```markdown
## Build It

### Step 1: Create a test PDF

First, let's create a PDF with known content so we can verify our extraction works.
```

**Cell 6 (code):**
```python
from fpdf import FPDF
from io import BytesIO

def create_test_pdf() -> bytes:
    """Create a 2-page PDF with known content."""
    pdf = FPDF()

    pdf.add_page()
    pdf.set_font("Helvetica", size=12)
    pdf.cell(0, 10, "Page 1: The quarterly revenue was 2.5 million dollars.")
    pdf.ln()
    pdf.cell(0, 10, "Operating margins improved to 23 percent.")
    pdf.ln()
    pdf.cell(0, 10, "The company expanded into three new markets.")

    pdf.add_page()
    pdf.cell(0, 10, "Page 2: The engineering team grew to 45 people.")
    pdf.ln()
    pdf.cell(0, 10, "Customer satisfaction scores reached 4.8 out of 5.")
    pdf.ln()
    pdf.cell(0, 10, "Infrastructure costs decreased by 12 percent.")

    buffer = BytesIO()
    pdf.output(buffer)
    return buffer.getvalue()

pdf_bytes = create_test_pdf()
print(f"Created test PDF: {len(pdf_bytes)} bytes")
```

**Cell 7 (markdown):**
```markdown
### Step 2: Extract text from each page

This function is the equivalent of `pdf_parser.py` in the ingestion service. It takes a file-like object (BytesIO) and returns a list of dicts, one per page.

Note the error handling pattern: we check for empty input first, then catch any PyPDF2 parsing errors and wrap them in a `ValueError`. This is similar to how you'd return `fmt.Errorf("invalid PDF: %w", err)` in Go.
```

**Cell 8 (code):**
```python
from io import BytesIO
from PyPDF2 import PdfReader

def extract_pages(pdf_file: BytesIO) -> list[dict]:
    """Extract text from each page of a PDF.

    Returns a list of dicts with 'page_number' (1-indexed) and 'text' keys.
    Raises ValueError if the file is empty or not a valid PDF.
    """
    try:
        content = pdf_file.read()
        if not content:
            raise ValueError("empty or invalid PDF")
        pdf_file.seek(0)
        reader = PdfReader(pdf_file)
    except Exception as e:
        if "empty or invalid" in str(e):
            raise
        raise ValueError(f"empty or invalid PDF: {e}")

    pages = []
    for i, page in enumerate(reader.pages):
        text = page.extract_text() or ""
        pages.append({"page_number": i + 1, "text": text})

    return pages

# Test it
pages = extract_pages(BytesIO(pdf_bytes))
for page in pages:
    print(f"\n--- Page {page['page_number']} ---")
    print(page["text"][:200])
```

**Cell 9 (code):**
```python
# Test error handling
import traceback

# Empty input
try:
    extract_pages(BytesIO(b""))
except ValueError as e:
    print(f"Empty input: {e}")

# Invalid PDF
try:
    extract_pages(BytesIO(b"not a pdf at all"))
except ValueError as e:
    print(f"Invalid PDF: {e}")
```

**Cell 10 (markdown):**
```markdown
### Step 3: Chunk the extracted text

Now we need to split the page text into smaller pieces for embedding. Why?

1. **LLM context windows** — embedding models have a maximum input length. Long pages need to be split.
2. **Retrieval precision** — smaller chunks mean more precise search results. A 100-word chunk about revenue is more useful than a 5000-word page that mentions revenue once.
3. **Overlap** — chunks overlap so that context isn't lost at boundaries. If a sentence spans two chunks, the overlap ensures both chunks contain the full sentence.

`RecursiveCharacterTextSplitter` handles all of this. You set `chunk_size` (target characters per chunk) and `chunk_overlap` (how many characters adjacent chunks share).
```

**Cell 11 (code):**
```python
from langchain_text_splitters import RecursiveCharacterTextSplitter

def chunk_pages(
    pages: list[dict],
    chunk_size: int = 1000,
    chunk_overlap: int = 200,
) -> list[dict]:
    """Split page text into overlapping chunks.

    Returns list of dicts with 'text', 'page_number', and 'chunk_index' keys.
    Empty pages are skipped.
    """
    splitter = RecursiveCharacterTextSplitter(
        chunk_size=chunk_size,
        chunk_overlap=chunk_overlap,
        length_function=len,
    )

    chunks = []
    index = 0
    for page in pages:
        text = page["text"].strip()
        if not text:
            continue

        splits = splitter.split_text(text)
        for split in splits:
            chunks.append({
                "text": split,
                "page_number": page["page_number"],
                "chunk_index": index,
            })
            index += 1

    return chunks

# Test with our PDF — use small chunk size to see splitting in action
chunks = chunk_pages(pages, chunk_size=100, chunk_overlap=20)
print(f"Pages: {len(pages)}, Chunks: {len(chunks)}\n")

for chunk in chunks:
    print(f"Chunk {chunk['chunk_index']} (page {chunk['page_number']}): {chunk['text'][:80]}...")
```

**Cell 12 (markdown):**
```markdown
> **Pitfall:** Claude Code sometimes imports from `langchain.text_splitter` (old, deprecated path) instead of `langchain_text_splitters` (the current standalone package). The old import may work if you have the full `langchain` package installed, but it pulls in hundreds of unnecessary dependencies. Always use `langchain-text-splitters` as a standalone package.
```

**Cell 13 (markdown):**
```markdown
## Experiment
```

**Cell 14 (code):**
```python
# Experiment 1: How does chunk_size affect the number of chunks?
for size in [50, 100, 200, 500, 1000]:
    chunks = chunk_pages(pages, chunk_size=size, chunk_overlap=0)
    print(f"chunk_size={size:>4}, overlap=0   → {len(chunks)} chunks")
```

**Cell 15 (code):**
```python
# Experiment 2: How does overlap affect chunks?
# With overlap, adjacent chunks share some text — notice the repeated content
chunks = chunk_pages(pages, chunk_size=100, chunk_overlap=50)
if len(chunks) >= 2:
    print(f"Chunk 0: ...{chunks[0]['text'][-60:]}")
    print(f"Chunk 1: {chunks[1]['text'][:60]}...")
    print(f"\nNotice the overlap — both chunks contain some of the same text.")
    print(f"This ensures context isn't lost at chunk boundaries.")
```

**Cell 16 (code):**
```python
# Experiment 3: What happens with an empty page?
pages_with_empty = [
    {"page_number": 1, "text": ""},
    {"page_number": 2, "text": "Only this page has content."},
]
chunks = chunk_pages(pages_with_empty, chunk_size=1000, chunk_overlap=0)
print(f"Chunks: {len(chunks)}")
print(f"All from page: {[c['page_number'] for c in chunks]}")
print("Empty pages are skipped — no empty chunks pollute the vector store.")
```

**Cell 17 (markdown):**
```markdown
## Check Your Understanding

1. **Why do we use `BytesIO` instead of reading the PDF file directly? What Go pattern is this similar to?**

2. **Why does chunk overlap matter for RAG? What would happen if overlap was 0?**

3. **The RecursiveCharacterTextSplitter tries separators in order: `\n\n`, `\n`, ` `, then characters. Why this order? What would happen if it only split on spaces?**
```

- [ ] **Step 2: Verify the notebook runs**

Run all cells. All should execute without errors.

- [ ] **Step 3: Commit**

```bash
git add lessons/02_pdf_parsing_and_chunking.ipynb
git commit -m "lesson: add 02_pdf_parsing_and_chunking notebook"
```

---

### Task 4: Lesson 03 — Embeddings & Vector Spaces

**Files:**
- Create: `lessons/03_embeddings_and_vectors.ipynb`

This notebook covers: what embeddings are, httpx async HTTP, calling Ollama's embed API, cosine similarity. Requires Ollama with nomic-embed-text.

- [ ] **Step 1: Create the notebook**

**Cell 1 (markdown):**
```markdown
# Lesson 03: Embeddings & Vector Spaces

## What You're Building

In this lesson you'll build the `embed_texts()` function — the bridge between human-readable text and the vector space where similarity search happens. This function takes a list of strings, sends them to Ollama's embedding model, and gets back numerical vectors that capture semantic meaning.

This is the key insight behind RAG: you can compare the *meaning* of a user's question to the *meaning* of document chunks by comparing their vector representations. Two texts about the same topic will have similar vectors, even if they use completely different words.

## How This Fits in the App

Both services use embeddings:
- **Ingestion API** embeds each text chunk before storing in Qdrant (`embedder.py`)
- **Chat API** embeds the user's question before searching Qdrant (`chain.py`)

The embedding model (nomic-embed-text) produces 768-dimensional vectors. Both services must use the same model — if you embed documents with one model and search with another, the vectors won't be comparable.
```

**Cell 2 (code):**
```python
# Prerequisites — requires Ollama running with nomic-embed-text
import httpx
import asyncio
import numpy as np

# Check Ollama connectivity
OLLAMA_BASE_URL = "http://localhost:11434"

async def check_ollama():
    try:
        async with httpx.AsyncClient() as client:
            response = await client.get(f"{OLLAMA_BASE_URL}/api/tags", timeout=5.0)
            models = [m["name"] for m in response.json().get("models", [])]
            if any("nomic-embed-text" in m for m in models):
                print(f"✓ Ollama is running with nomic-embed-text")
            else:
                print(f"✗ nomic-embed-text not found. Run: ollama pull nomic-embed-text")
                print(f"  Available models: {models}")
    except Exception as e:
        print(f"✗ Cannot reach Ollama at {OLLAMA_BASE_URL}: {e}")
        print(f"  Make sure Ollama is running (ollama serve)")

await check_ollama()
```

**Cell 3 (markdown):**
```markdown
## Package Introductions

### httpx
httpx is Python's modern HTTP client — think of it as Go's `http.Client` or Node's `fetch`/`axios`. We chose it over the older `requests` library because:
- **Async support** — `httpx.AsyncClient()` works with `async/await`, which we need inside FastAPI handlers. The `requests` library is sync-only.
- **Streaming support** — we'll use `client.stream()` in Lesson 06 for SSE. httpx handles this natively.
- **Similar API to requests** — if you've seen Python tutorials using `requests`, httpx is almost identical but adds async.

Key APIs:
- `httpx.AsyncClient()` — create an async client (use as context manager: `async with httpx.AsyncClient() as client:`)
- `client.post(url, json={...})` — send a POST with JSON body
- `response.json()` — parse JSON response
- `response.raise_for_status()` — raise exception on 4xx/5xx (like checking `resp.StatusCode` in Go)

### numpy
numpy is Python's numerical computing library. We only use it here for one thing: computing cosine similarity between vectors. It's the standard way to do vector math in Python.

Key API: `np.dot()` for dot product, `np.linalg.norm()` for vector magnitude.
```

**Cell 4 (markdown):**
```markdown
## Go/TS Comparison

| Concept | Go | Python |
|---------|-----|--------|
| HTTP client | `http.Client{}` | `httpx.AsyncClient()` |
| POST with JSON | `json.Marshal` + `http.NewRequest` | `client.post(url, json=data)` |
| Context manager | `defer resp.Body.Close()` | `async with client:` (auto-closes) |
| Async I/O | goroutines (implicit) | `async/await` (explicit) |

The biggest difference: In Go, every HTTP call is implicitly concurrent (goroutines handle it). In Python, you must explicitly use `async/await` to avoid blocking. `httpx.AsyncClient` is the async version — there's also a sync `httpx.Client` but we don't use it in FastAPI handlers.
```

**Cell 5 (markdown):**
```markdown
## Build It

### Step 1: Understand what embeddings are

An embedding is a list of numbers (a vector) that represents the *meaning* of a piece of text. The key property: **texts with similar meanings produce similar vectors.**

Let's see this in action.
```

**Cell 6 (code):**
```python
import httpx

OLLAMA_BASE_URL = "http://localhost:11434"

async def embed_texts(
    texts: list[str],
    ollama_base_url: str,
    model: str,
) -> list[list[float]]:
    """Embed a list of texts using Ollama's /api/embed endpoint.

    Returns a list of embedding vectors (list of floats).
    """
    if not texts:
        return []

    async with httpx.AsyncClient() as client:
        response = await client.post(
            f"{ollama_base_url}/api/embed",
            json={"model": model, "input": texts},
            timeout=120.0,
        )
        response.raise_for_status()
        data = response.json()

    return data["embeddings"]

# Embed a single sentence
vectors = await embed_texts(
    ["The quarterly revenue was 2.5 million dollars."],
    ollama_base_url=OLLAMA_BASE_URL,
    model="nomic-embed-text",
)

print(f"Number of vectors: {len(vectors)}")
print(f"Dimensions per vector: {len(vectors[0])}")
print(f"First 10 values: {vectors[0][:10]}")
```

**Cell 7 (markdown):**
```markdown
### Step 2: Compute cosine similarity

Cosine similarity measures how similar two vectors are, on a scale from -1 (opposite) to 1 (identical). For embeddings, similar meaning → score close to 1.

The formula: `similarity = dot(A, B) / (|A| * |B|)`

This is the same math that Qdrant uses internally for vector search.
```

**Cell 8 (code):**
```python
import numpy as np

def cosine_similarity(a: list[float], b: list[float]) -> float:
    """Compute cosine similarity between two vectors."""
    a, b = np.array(a), np.array(b)
    return float(np.dot(a, b) / (np.linalg.norm(a) * np.linalg.norm(b)))

# Embed several sentences and compare
sentences = [
    "The quarterly revenue was 2.5 million dollars.",  # About revenue
    "The company earned 2.5M in Q3.",                   # Same meaning, different words
    "The engineering team grew to 45 people.",           # Different topic
    "What was the revenue last quarter?",                # Question about revenue
]

vectors = await embed_texts(sentences, OLLAMA_BASE_URL, "nomic-embed-text")

print("Cosine similarity matrix:\n")
print(f"{'':>50}", end="")
for i in range(len(sentences)):
    print(f"  [{i}]", end="")
print()

for i, s1 in enumerate(sentences):
    print(f"[{i}] {s1[:48]:>50}", end="")
    for j in range(len(sentences)):
        sim = cosine_similarity(vectors[i], vectors[j])
        print(f" {sim:.2f}", end="")
    print()
```

**Cell 9 (markdown):**
```markdown
Look at the results:
- Sentences [0] and [1] should have **high similarity** (~0.85+) — they say the same thing in different words
- Sentences [0] and [2] should have **lower similarity** — different topics
- Sentences [0] and [3] should have **high similarity** — the question is about the same topic as the statement

This is why RAG works: embed the question, search for chunks with similar vectors, and you find relevant content regardless of exact wording.

> **Pitfall:** Claude Code sometimes hardcodes the model name (e.g., `model="nomic-embed-text"`) instead of reading it from config. In the real service, the model name comes from `settings.embedding_model` so it can be changed without editing code.
```

**Cell 10 (markdown):**
```markdown
## Experiment
```

**Cell 11 (code):**
```python
# Experiment 1: Embed a question and find the most similar sentence
question = "How much money did the company make?"
passages = [
    "The quarterly revenue was 2.5 million dollars.",
    "The engineering team grew to 45 people.",
    "Customer satisfaction scores reached 4.8 out of 5.",
    "Infrastructure costs decreased by 12 percent.",
]

all_texts = [question] + passages
vecs = await embed_texts(all_texts, OLLAMA_BASE_URL, "nomic-embed-text")

q_vec = vecs[0]
print(f"Question: {question}\n")
for i, passage in enumerate(passages):
    sim = cosine_similarity(q_vec, vecs[i + 1])
    print(f"  {sim:.3f}  {passage}")

print(f"\nThe highest-scoring passage is the one about revenue — that's what")
print(f"Qdrant's similarity search does at scale.")
```

**Cell 12 (code):**
```python
# Experiment 2: What happens with empty input?
result = await embed_texts([], OLLAMA_BASE_URL, "nomic-embed-text")
print(f"Empty input returns: {result}")
print("We handle this edge case to avoid unnecessary API calls.")
```

**Cell 13 (markdown):**
```markdown
## Check Your Understanding

1. **Why must both services (ingestion and chat) use the same embedding model?**

2. **What does cosine similarity measure, and why is it better than Euclidean distance for comparing text embeddings?**

3. **The `embed_texts` function uses `async with httpx.AsyncClient()`. What's the Go equivalent of this pattern, and why is it important?**
```

- [ ] **Step 2: Verify the notebook runs**

Requires Ollama running with nomic-embed-text. Run all cells and verify output.

- [ ] **Step 3: Commit**

```bash
git add lessons/03_embeddings_and_vectors.ipynb
git commit -m "lesson: add 03_embeddings_and_vectors notebook"
```

---

### Task 5: Lesson 04 — Qdrant Vector Storage

**Files:**
- Create: `lessons/04_qdrant_vector_storage.ipynb`

This notebook covers: Qdrant's data model, creating collections, upserting vectors with metadata, similarity search. Requires Qdrant running.

- [ ] **Step 1: Create the notebook**

**Cell 1 (markdown):**
```markdown
# Lesson 04: Qdrant Vector Storage

## What You're Building

In this lesson you'll build two classes: `QdrantStore` (for the ingestion service — upserts vectors with metadata) and `QdrantRetriever` (for the chat service — searches for similar vectors). Together they're the persistence layer for the RAG pipeline.

A vector database is specialized for one job: finding the N most similar vectors to a query vector, fast. Regular databases (Postgres, MongoDB) can store vectors, but they can't do efficient similarity search at scale. Qdrant is purpose-built for this.

## How This Fits in the App

- **Ingestion:** After embedding text chunks, `QdrantStore.upsert()` stores the vectors + metadata (filename, page number, chunk text) in Qdrant
- **Chat:** When a user asks a question, `QdrantRetriever.search()` finds the top-K most similar chunks and returns them with their metadata for building the prompt
```

**Cell 2 (code):**
```python
# Prerequisites — requires Qdrant running
# docker run -d -p 6333:6333 qdrant/qdrant:latest
from qdrant_client import QdrantClient

QDRANT_HOST = "localhost"
QDRANT_PORT = 6333

try:
    client = QdrantClient(host=QDRANT_HOST, port=QDRANT_PORT)
    collections = client.get_collections()
    print(f"✓ Connected to Qdrant at {QDRANT_HOST}:{QDRANT_PORT}")
    print(f"  Existing collections: {[c.name for c in collections.collections]}")
except Exception as e:
    print(f"✗ Cannot reach Qdrant: {e}")
    print(f"  Start it: docker run -d -p 6333:6333 qdrant/qdrant:latest")
```

**Cell 3 (markdown):**
```markdown
## Package Introductions

### qdrant-client
The official Python client for Qdrant vector database. It provides a high-level API for creating collections, inserting vectors, and searching.

Key concepts in Qdrant's data model:
- **Collection** — like a database table. Has a name and a vector configuration (dimensions + distance metric).
- **Point** — like a row. Contains: an ID, a vector (list of floats), and a payload (arbitrary JSON metadata).
- **Payload** — metadata attached to each vector. We store `text`, `page_number`, `filename`, `document_id`, `chunk_index`. This is what lets us show "Source: report.pdf, page 3" in the UI.
- **Search** — given a query vector, find the N closest points by cosine similarity. Returns points sorted by score.

Key APIs:
- `QdrantClient(host, port)` — connect to Qdrant
- `client.create_collection(name, vectors_config)` — create a new collection
- `client.collection_exists(name)` — check if collection exists
- `client.upsert(collection_name, points)` — insert or update points
- `client.search(collection_name, query_vector, limit)` — similarity search
- `client.scroll(collection_name)` — iterate all points (for listing documents)
```

**Cell 4 (markdown):**
```markdown
## Go/TS Comparison

| Concept | Go | Python |
|---------|-----|--------|
| Struct with methods | `type Store struct{} + func (s *Store) Upsert()` | `class QdrantStore: def __init__(), def upsert()` |
| Constructor | `func NewStore() *Store` | `def __init__(self)` |
| Method receiver | `func (s *Store) method()` | `def method(self)` |
| Nil check | `if s.client == nil` | `if self.client is None` |

Python classes are more explicit than Go — every method takes `self` as the first parameter (equivalent to the method receiver in Go). `__init__` is the constructor (runs when you call `MyClass()`). There's no separate `New` function convention.
```

**Cell 5 (markdown):**
```markdown
## Build It

### Step 1: Create the QdrantStore class

This class handles the ingestion side: creating the collection (if it doesn't exist) and upserting vectors with metadata.
```

**Cell 6 (code):**
```python
import uuid
from qdrant_client import QdrantClient
from qdrant_client.models import Distance, PointStruct, VectorParams

class QdrantStore:
    def __init__(self, host: str, port: int, collection_name: str):
        self.client = QdrantClient(host=host, port=port)
        self.collection_name = collection_name
        self._ensure_collection()

    def _ensure_collection(self):
        """Create the collection if it doesn't exist."""
        if not self.client.collection_exists(self.collection_name):
            self.client.create_collection(
                collection_name=self.collection_name,
                vectors_config=VectorParams(
                    size=768,  # nomic-embed-text output dimensions
                    distance=Distance.COSINE,
                ),
            )
            print(f"Created collection: {self.collection_name}")
        else:
            print(f"Collection already exists: {self.collection_name}")

    def upsert(
        self,
        chunks: list[dict],
        vectors: list[list[float]],
        document_id: str,
        filename: str,
    ) -> None:
        """Store chunks with their vectors and metadata."""
        points = [
            PointStruct(
                id=str(uuid.uuid4()),
                vector=vector,
                payload={
                    "text": chunk["text"],
                    "page_number": chunk["page_number"],
                    "chunk_index": chunk["chunk_index"],
                    "document_id": document_id,
                    "filename": filename,
                },
            )
            for chunk, vector in zip(chunks, vectors)
        ]
        self.client.upsert(
            collection_name=self.collection_name,
            points=points,
        )
        print(f"Upserted {len(points)} points for '{filename}'")

    def list_documents(self) -> list[dict]:
        """List all unique documents in the collection."""
        records, _ = self.client.scroll(
            collection_name=self.collection_name,
            limit=10000,
            with_payload=True,
            with_vectors=False,
        )

        docs: dict[str, dict] = {}
        for record in records:
            doc_id = record.payload["document_id"]
            if doc_id not in docs:
                docs[doc_id] = {
                    "document_id": doc_id,
                    "filename": record.payload["filename"],
                    "chunks": 0,
                }
            docs[doc_id]["chunks"] += 1

        return list(docs.values())

# Test it — create a store with a test collection
store = QdrantStore(host="localhost", port=6333, collection_name="lesson_04_test")
```

**Cell 7 (markdown):**
```markdown
### Step 2: Insert some test data

Let's create fake embeddings and upsert them. In the real app, these would come from `embed_texts()`.
```

**Cell 8 (code):**
```python
import numpy as np

# Create fake chunks and embeddings for testing
chunks = [
    {"text": "Revenue was 2.5 million dollars.", "page_number": 1, "chunk_index": 0},
    {"text": "Operating margins improved to 23%.", "page_number": 1, "chunk_index": 1},
    {"text": "Engineering team grew to 45 people.", "page_number": 2, "chunk_index": 2},
]

# Fake 768-dimensional vectors (in reality, these come from Ollama)
np.random.seed(42)
vectors = [np.random.randn(768).tolist() for _ in chunks]

store.upsert(
    chunks=chunks,
    vectors=vectors,
    document_id="test-doc-001",
    filename="q3_report.pdf",
)

# List documents
docs = store.list_documents()
for doc in docs:
    print(f"  {doc['filename']}: {doc['chunks']} chunks (ID: {doc['document_id']})")
```

**Cell 9 (markdown):**
```markdown
### Step 3: Build the QdrantRetriever class

This class handles the chat side: searching for similar vectors.

> **Pitfall:** Claude Code sometimes creates the Qdrant collection with the wrong vector size (e.g., 384 or 1536 instead of 768). The size must match your embedding model's output dimensions. nomic-embed-text produces 768-dimensional vectors. If sizes don't match, upsert will fail silently or search will return garbage results.
```

**Cell 10 (code):**
```python
class QdrantRetriever:
    def __init__(self, host: str, port: int, collection_name: str):
        self.client = QdrantClient(host=host, port=port)
        self.collection_name = collection_name

    def search(
        self, query_vector: list[float], top_k: int = 5
    ) -> list[dict]:
        """Search for the most similar vectors."""
        results = self.client.search(
            collection_name=self.collection_name,
            query_vector=query_vector,
            limit=top_k,
        )

        return [
            {
                "text": hit.payload["text"],
                "page_number": hit.payload["page_number"],
                "filename": hit.payload["filename"],
                "document_id": hit.payload["document_id"],
                "score": hit.score,
            }
            for hit in results
        ]

# Test search
retriever = QdrantRetriever(host="localhost", port=6333, collection_name="lesson_04_test")

# Search with one of our vectors (should find itself as top result)
results = retriever.search(query_vector=vectors[0], top_k=3)
print("Search results:")
for r in results:
    print(f"  Score: {r['score']:.3f}  Page {r['page_number']}: {r['text']}")
```

**Cell 11 (markdown):**
```markdown
## Experiment
```

**Cell 12 (code):**
```python
# Experiment 1: Change top_k
results_1 = retriever.search(query_vector=vectors[0], top_k=1)
results_3 = retriever.search(query_vector=vectors[0], top_k=3)
print(f"top_k=1: {len(results_1)} results")
print(f"top_k=3: {len(results_3)} results")
```

**Cell 13 (code):**
```python
# Experiment 2: Search with a random vector (no good matches)
random_vector = np.random.randn(768).tolist()
results = retriever.search(query_vector=random_vector, top_k=3)
print("Search with random vector (expect low scores):")
for r in results:
    print(f"  Score: {r['score']:.3f}  {r['text']}")
```

**Cell 14 (code):**
```python
# Clean up — delete test collection
store.client.delete_collection("lesson_04_test")
print("Cleaned up test collection")
```

**Cell 15 (markdown):**
```markdown
## Check Your Understanding

1. **Why use a vector database instead of storing embeddings in Postgres with pgvector?** (Hint: think about search speed at scale and purpose-built indexing)

2. **What's stored in the Qdrant payload, and why? What would happen if we only stored the vector without metadata?**

3. **In the `QdrantStore.__init__`, we call `_ensure_collection()`. What's the Go equivalent of this initialization pattern?** (Hint: think about `sync.Once` or init functions)
```

- [ ] **Step 2: Verify the notebook runs**

Requires Qdrant running. Run all cells.

- [ ] **Step 3: Commit**

```bash
git add lessons/04_qdrant_vector_storage.ipynb
git commit -m "lesson: add 04_qdrant_vector_storage notebook"
```

---

### Task 6: Lesson 05 — RAG Chain & Prompt Engineering

**Files:**
- Create: `lessons/05_rag_chain_and_prompts.ipynb`

This notebook covers: RAG pattern, prompt templates, grounding, hallucination prevention, source attribution. Requires Ollama + Qdrant.

- [ ] **Step 1: Create the notebook**

**Cell 1 (markdown):**
```markdown
# Lesson 05: RAG Chain & Prompt Engineering

## What You're Building

In this lesson you'll build the prompt engineering layer — the part that turns retrieved document chunks into a well-structured prompt for the LLM. You'll also wire up the full RAG pipeline: embed the question → search Qdrant → build prompt → send to LLM → get answer.

This is where the "intelligence" of the system lives. The LLM doesn't know anything about your documents — it only knows what you put in the prompt. Good prompt engineering means grounded, cited answers. Bad prompt engineering means hallucination.

## How This Fits in the App

In the chat service, this logic lives in `prompt.py` (templates) and `chain.py` (the pipeline). The `/chat` endpoint calls `rag_query()` which orchestrates: embed → search → prompt → stream.
```

**Cell 2 (code):**
```python
# Prerequisites — requires Ollama (mistral + nomic-embed-text) and Qdrant
import httpx
import numpy as np

OLLAMA_BASE_URL = "http://localhost:11434"
QDRANT_HOST = "localhost"
QDRANT_PORT = 6333

async def check_services():
    # Check Ollama
    try:
        async with httpx.AsyncClient() as client:
            resp = await client.get(f"{OLLAMA_BASE_URL}/api/tags", timeout=5.0)
            models = [m["name"] for m in resp.json().get("models", [])]
            has_mistral = any("mistral" in m for m in models)
            has_embed = any("nomic-embed-text" in m for m in models)
            print(f"✓ Ollama: mistral={'✓' if has_mistral else '✗'}, nomic-embed-text={'✓' if has_embed else '✗'}")
    except Exception as e:
        print(f"✗ Ollama: {e}")

    # Check Qdrant
    try:
        from qdrant_client import QdrantClient
        client = QdrantClient(host=QDRANT_HOST, port=QDRANT_PORT)
        client.get_collections()
        print(f"✓ Qdrant connected")
    except Exception as e:
        print(f"✗ Qdrant: {e}")

await check_services()
```

**Cell 3 (markdown):**
```markdown
## Package Introductions

No new packages in this lesson — we're combining httpx (Lesson 03) and qdrant-client (Lesson 04). The focus here is on **prompt design**, which is about crafting text, not calling APIs.

The one new concept is **prompt templates** — string formatting patterns that structure how context and questions are presented to the LLM. These are just Python f-strings or `.format()` calls. No library needed.
```

**Cell 4 (markdown):**
```markdown
## Go/TS Comparison

Prompt engineering is language-agnostic — it's about the text you send to the LLM, not the code. The closest analogy in traditional web development is **template rendering** (like Go's `html/template` or Handlebars in JS). You have a template with placeholders, you fill in data, and the result goes to the consumer (LLM instead of browser).

The RAG pattern itself is similar to how you'd build a search feature:
1. User submits query → **embed** (like building a search index query)
2. Search the database → **retrieve** (like `SELECT ... WHERE similarity > threshold`)
3. Format results for display → **prompt** (like rendering a template)
4. Return to user → **generate** (except the LLM generates natural language, not HTML)
```

**Cell 5 (markdown):**
```markdown
## Build It

### Step 1: Define the prompt templates

These templates control what the LLM sees. The system prompt sets the LLM's behavior. The RAG template structures how context and questions are presented.
```

**Cell 6 (code):**
```python
SYSTEM_PROMPT = """You are a helpful document Q&A assistant. Answer questions based only on the provided context. If the context doesn't contain enough information to answer, say so honestly — do not make up information.

When referencing information, mention the source file and page number."""

RAG_TEMPLATE = """Context:
{context}

Question: {question}

Answer based only on the context above. Cite sources (filename, page) when possible."""

NO_CONTEXT_TEMPLATE = """The user asked: {question}

I don't have any relevant context from uploaded documents to answer this question. Please upload a relevant document first, or rephrase your question."""


def build_rag_prompt(question: str, chunks: list[dict]) -> str:
    """Build a prompt from retrieved chunks and a question."""
    if not chunks:
        return NO_CONTEXT_TEMPLATE.format(question=question)

    context_parts = []
    for chunk in chunks:
        source = f"[{chunk['filename']}, page {chunk['page_number']}]"
        context_parts.append(f"{source}\n{chunk['text']}")

    context = "\n\n".join(context_parts)
    return RAG_TEMPLATE.format(context=context, question=question)

# Test with sample chunks
sample_chunks = [
    {"text": "Revenue was 2.5 million dollars.", "filename": "report.pdf", "page_number": 1},
    {"text": "Operating margins improved to 23%.", "filename": "report.pdf", "page_number": 1},
]

prompt = build_rag_prompt("What was the revenue?", sample_chunks)
print("=== Generated Prompt ===")
print(prompt)
```

**Cell 7 (code):**
```python
# Test with no chunks
prompt = build_rag_prompt("What is quantum computing?", [])
print("=== No Context Prompt ===")
print(prompt)
```

**Cell 8 (markdown):**
```markdown
### Step 2: Wire up the full RAG pipeline

Now let's put it all together. We'll:
1. Seed Qdrant with some document chunks (reusing code from Lessons 02-04)
2. Embed a question
3. Search Qdrant for similar chunks
4. Build a prompt with the results
5. Send it to Ollama and get an answer
```

**Cell 9 (code):**
```python
# First, let's seed Qdrant with real embeddings
# (Combining what we built in Lessons 02, 03, and 04)

from qdrant_client import QdrantClient
from qdrant_client.models import Distance, PointStruct, VectorParams
import uuid

COLLECTION = "lesson_05_test"

# Embed function from Lesson 03
async def embed_texts(texts, ollama_base_url, model):
    if not texts:
        return []
    async with httpx.AsyncClient() as client:
        response = await client.post(
            f"{ollama_base_url}/api/embed",
            json={"model": model, "input": texts},
            timeout=120.0,
        )
        response.raise_for_status()
        return response.json()["embeddings"]

# Create collection and seed with chunks
qdrant = QdrantClient(host=QDRANT_HOST, port=QDRANT_PORT)
if qdrant.collection_exists(COLLECTION):
    qdrant.delete_collection(COLLECTION)

qdrant.create_collection(
    collection_name=COLLECTION,
    vectors_config=VectorParams(size=768, distance=Distance.COSINE),
)

chunks = [
    {"text": "The quarterly revenue was 2.5 million dollars, up 15% from last year.", "page_number": 1, "chunk_index": 0},
    {"text": "Operating margins improved to 23%, driven by cost optimization.", "page_number": 1, "chunk_index": 1},
    {"text": "The engineering team grew to 45 people. Three new products launched.", "page_number": 2, "chunk_index": 2},
    {"text": "Customer satisfaction scores reached 4.8 out of 5.", "page_number": 2, "chunk_index": 3},
    {"text": "Infrastructure costs decreased by 12% due to cloud migration.", "page_number": 2, "chunk_index": 4},
]

texts = [c["text"] for c in chunks]
vectors = await embed_texts(texts, OLLAMA_BASE_URL, "nomic-embed-text")

points = [
    PointStruct(
        id=str(uuid.uuid4()),
        vector=vec,
        payload={"text": c["text"], "page_number": c["page_number"],
                 "filename": "q3_report.pdf", "document_id": "doc-001",
                 "chunk_index": c["chunk_index"]},
    )
    for c, vec in zip(chunks, vectors)
]
qdrant.upsert(collection_name=COLLECTION, points=points)
print(f"Seeded {len(points)} chunks into '{COLLECTION}'")
```

**Cell 10 (code):**
```python
# Now: the full RAG pipeline

async def rag_answer(question: str) -> str:
    """Full RAG pipeline: embed → search → prompt → generate."""

    # 1. Embed the question
    q_vectors = await embed_texts([question], OLLAMA_BASE_URL, "nomic-embed-text")
    q_vector = q_vectors[0]

    # 2. Search Qdrant
    results = qdrant.search(
        collection_name=COLLECTION,
        query_vector=q_vector,
        limit=3,
    )
    retrieved_chunks = [
        {"text": hit.payload["text"],
         "filename": hit.payload["filename"],
         "page_number": hit.payload["page_number"],
         "score": hit.score}
        for hit in results
    ]

    print("Retrieved chunks:")
    for c in retrieved_chunks:
        print(f"  [{c['score']:.3f}] {c['text'][:60]}...")

    # 3. Build prompt
    prompt = build_rag_prompt(question, retrieved_chunks)

    # 4. Send to Ollama (non-streaming for simplicity)
    async with httpx.AsyncClient() as client:
        response = await client.post(
            f"{OLLAMA_BASE_URL}/api/generate",
            json={"model": "mistral", "prompt": prompt, "system": SYSTEM_PROMPT, "stream": False},
            timeout=120.0,
        )
        response.raise_for_status()
        return response.json()["response"]

# Try it!
answer = await rag_answer("What was the company's revenue?")
print(f"\n=== Answer ===\n{answer}")
```

**Cell 11 (markdown):**
```markdown
## Experiment
```

**Cell 12 (code):**
```python
# Experiment 1: Hallucination demo — ask about something NOT in the context
answer = await rag_answer("What is the CEO's name?")
print(f"Answer: {answer}")
print("\nThe LLM should say it doesn't have enough context.")
print("If it makes up a name, the prompt engineering needs work.")
```

**Cell 13 (code):**
```python
# Experiment 2: What happens WITHOUT grounding instructions?
BAD_TEMPLATE = """Here is some context: {context}

Question: {question}

Answer the question."""

def build_bad_prompt(question, chunks):
    context = "\n".join(c["text"] for c in chunks)
    return BAD_TEMPLATE.format(context=context, question=question)

# Ask the same question with a weaker prompt
q_vecs = await embed_texts(["What is the CEO's salary?"], OLLAMA_BASE_URL, "nomic-embed-text")
results = qdrant.search(collection_name=COLLECTION, query_vector=q_vecs[0], limit=3)
chunks_for_prompt = [{"text": h.payload["text"], "filename": h.payload["filename"], "page_number": h.payload["page_number"]} for h in results]
bad_prompt = build_bad_prompt("What is the CEO's salary?", chunks_for_prompt)

async with httpx.AsyncClient() as client:
    resp = await client.post(
        f"{OLLAMA_BASE_URL}/api/generate",
        json={"model": "mistral", "prompt": bad_prompt, "stream": False},
        timeout=120.0,
    )
    print(f"Without grounding: {resp.json()['response']}")
    print("\nNotice: without explicit instructions to only use context,")
    print("the LLM is more likely to hallucinate or speculate.")
```

**Cell 14 (code):**
```python
# Experiment 3: Change the system prompt personality
CASUAL_SYSTEM = "You are a casual, friendly assistant. Use simple language and emoji when appropriate. Answer based only on the provided context."

async with httpx.AsyncClient() as client:
    prompt = build_rag_prompt("How much revenue did they make?", chunks_for_prompt)
    resp = await client.post(
        f"{OLLAMA_BASE_URL}/api/generate",
        json={"model": "mistral", "prompt": prompt, "system": CASUAL_SYSTEM, "stream": False},
        timeout=120.0,
    )
    print(f"Casual style: {resp.json()['response']}")
```

**Cell 15 (code):**
```python
# Clean up
qdrant.delete_collection(COLLECTION)
print("Cleaned up test collection")
```

**Cell 16 (markdown):**
```markdown
## Check Your Understanding

1. **What is "grounding" in prompt engineering, and why is it critical for RAG applications?**

2. **Why do we include source attribution instructions in the system prompt? What would the user experience be without them?**

3. **The `build_rag_prompt` function has a separate template for when no chunks are found. Why not just send the question to the LLM directly?**
```

- [ ] **Step 2: Verify the notebook runs**

Requires Ollama (mistral + nomic-embed-text) and Qdrant. Run all cells.

- [ ] **Step 3: Commit**

```bash
git add lessons/05_rag_chain_and_prompts.ipynb
git commit -m "lesson: add 05_rag_chain_and_prompts notebook"
```

---

### Task 7: Lesson 06 — Streaming & SSE

**Files:**
- Create: `lessons/06_streaming_and_sse.ipynb`

This notebook covers: async generators, streaming from Ollama, SSE format, token-by-token display. Requires Ollama with mistral.

- [ ] **Step 1: Create the notebook**

**Cell 1 (markdown):**
```markdown
# Lesson 06: Streaming & SSE

## What You're Building

In this lesson you'll build `stream_ollama_response()` — an async generator that streams tokens from Ollama one at a time. This is what makes the chat UI feel responsive: instead of waiting 10+ seconds for a complete answer, users see words appearing as the LLM generates them.

## How This Fits in the App

In the chat service, `chain.py` uses `stream_ollama_response()` inside `rag_query()`. The `/chat` endpoint wraps this in an SSE (Server-Sent Events) response using `sse-starlette`. The frontend reads the SSE stream and appends each token to the message bubble.
```

**Cell 2 (code):**
```python
# Prerequisites — requires Ollama with mistral
import httpx
import json
import time

OLLAMA_BASE_URL = "http://localhost:11434"

async def check_ollama():
    try:
        async with httpx.AsyncClient() as client:
            resp = await client.get(f"{OLLAMA_BASE_URL}/api/tags", timeout=5.0)
            models = [m["name"] for m in resp.json().get("models", [])]
            has_mistral = any("mistral" in m for m in models)
            print(f"✓ Ollama: mistral={'✓' if has_mistral else '✗'}")
    except Exception as e:
        print(f"✗ Ollama: {e}")

await check_ollama()
```

**Cell 3 (markdown):**
```markdown
## Package Introductions

No new packages — we're using httpx's streaming capabilities, which we introduced in Lesson 03.

The new concept is **async generators** — Python's way of producing a stream of values lazily. If you've used Go channels to stream data between goroutines, async generators serve a similar purpose but with different syntax.

### Async Generators
```python
# Go channel pattern:
# ch := make(chan string)
# go func() { ch <- "hello"; ch <- "world"; close(ch) }()
# for msg := range ch { fmt.Println(msg) }

# Python async generator equivalent:
async def my_stream():
    yield "hello"
    yield "world"

async for msg in my_stream():
    print(msg)
```

Key difference: Go channels are concurrent (producer and consumer run in separate goroutines). Python async generators are cooperative — `yield` suspends the generator until the consumer asks for the next value with `async for`.
```

**Cell 4 (markdown):**
```markdown
## Go/TS Comparison

| Concept | Go | Python |
|---------|-----|--------|
| Streaming values | `chan T` | `async def f(): yield value` |
| Consuming stream | `for v := range ch` | `async for v in f()` |
| Stream done | `close(ch)` | function returns (implicit) |
| HTTP streaming | `resp.Body` (io.Reader) | `response.aiter_lines()` |

In Go, you'd read a streaming HTTP response with `bufio.NewScanner(resp.Body)` and iterate lines. In Python, httpx gives you `response.aiter_lines()` which is an async iterator over lines as they arrive.

> **Pitfall:** Claude Code sometimes generates non-streaming Ollama calls (`"stream": false`) when streaming is needed. The non-streaming version waits for the entire response before returning, which defeats the purpose. Always set `"stream": true` when building the chat endpoint.
```

**Cell 5 (markdown):**
```markdown
## Build It

### Step 1: Non-streaming baseline

First, let's see what a non-streaming call looks like and how long it takes.
```

**Cell 6 (code):**
```python
import httpx
import time

OLLAMA_BASE_URL = "http://localhost:11434"

# Non-streaming — wait for full response
start = time.time()
async with httpx.AsyncClient() as client:
    response = await client.post(
        f"{OLLAMA_BASE_URL}/api/generate",
        json={
            "model": "mistral",
            "prompt": "Explain what RAG means in AI, in 2 sentences.",
            "stream": False,
        },
        timeout=120.0,
    )
    data = response.json()
    elapsed = time.time() - start

print(f"Response ({elapsed:.1f}s wait):")
print(data["response"])
print(f"\nThe user sees NOTHING for {elapsed:.1f} seconds, then the full text appears at once.")
```

**Cell 7 (markdown):**
```markdown
### Step 2: Streaming — see tokens arrive

Now let's stream the same request. The LLM sends back JSON objects line by line, each containing one token.
```

**Cell 8 (code):**
```python
import json

# Streaming — tokens arrive one at a time
print("Streaming response:")
start = time.time()
first_token_time = None

async with httpx.AsyncClient() as client:
    async with client.stream(
        "POST",
        f"{OLLAMA_BASE_URL}/api/generate",
        json={
            "model": "mistral",
            "prompt": "Explain what RAG means in AI, in 2 sentences.",
            "stream": True,
        },
        timeout=120.0,
    ) as response:
        async for line in response.aiter_lines():
            if line.strip():
                data = json.loads(line)
                if data.get("response"):
                    if first_token_time is None:
                        first_token_time = time.time() - start
                    print(data["response"], end="", flush=True)
                if data.get("done"):
                    break

elapsed = time.time() - start
print(f"\n\nFirst token: {first_token_time:.1f}s, Total: {elapsed:.1f}s")
print(f"The user starts reading after {first_token_time:.1f}s instead of waiting {elapsed:.1f}s.")
```

**Cell 9 (markdown):**
```markdown
### Step 3: Build the reusable async generator

Now let's wrap this in a clean function — the `stream_ollama_response()` that the chat service uses.
```

**Cell 10 (code):**
```python
from typing import AsyncGenerator

SYSTEM_PROMPT = """You are a helpful document Q&A assistant. Answer questions based only on the provided context."""

async def stream_ollama_response(
    prompt: str,
    model: str,
    base_url: str,
) -> AsyncGenerator[dict, None]:
    """Stream tokens from Ollama as an async generator.

    Yields dicts like {"token": "The"} for each token.
    """
    async with httpx.AsyncClient() as client:
        async with client.stream(
            "POST",
            f"{base_url}/api/generate",
            json={
                "model": model,
                "prompt": prompt,
                "system": SYSTEM_PROMPT,
                "stream": True,
            },
            timeout=300.0,
        ) as response:
            response.raise_for_status()

            async for line in response.aiter_lines():
                if line.strip():
                    data = json.loads(line)
                    if data.get("response"):
                        yield {"token": data["response"]}
                    if data.get("done"):
                        break

# Use the generator
print("Using stream_ollama_response():")
async for event in stream_ollama_response(
    prompt="What is machine learning? One sentence.",
    model="mistral",
    base_url=OLLAMA_BASE_URL,
):
    print(event["token"], end="", flush=True)
print()
```

**Cell 11 (markdown):**
```markdown
### Step 4: SSE format

When we serve this from FastAPI, each event gets wrapped in SSE format: `data: {"token": "..."}\n\n`. This is the format the browser's `EventSource` API (and our frontend's `fetch` + `ReadableStream`) expects.

Here's what the raw SSE stream looks like:
```

**Cell 12 (code):**
```python
# Simulate what the /chat endpoint sends to the browser
print("Raw SSE format:\n")
token_count = 0
async for event in stream_ollama_response(
    prompt="What is Python? One sentence.",
    model="mistral",
    base_url=OLLAMA_BASE_URL,
):
    sse_line = f"data: {json.dumps(event)}"
    print(sse_line)
    token_count += 1
    if token_count > 8:
        print("...")
        break

# Final event with sources
final = {"done": True, "sources": [{"file": "docs.pdf", "page": 1}]}
print(f"data: {json.dumps(final)}")
print("\nEach line is a separate SSE event. The frontend parses each 'data:' line as JSON.")
```

**Cell 13 (markdown):**
```markdown
## Experiment
```

**Cell 14 (code):**
```python
# Experiment 1: Count tokens and measure throughput
token_count = 0
start = time.time()

async for event in stream_ollama_response(
    prompt="Write a short paragraph about vector databases.",
    model="mistral",
    base_url=OLLAMA_BASE_URL,
):
    token_count += 1

elapsed = time.time() - start
print(f"Tokens: {token_count}")
print(f"Time: {elapsed:.1f}s")
print(f"Speed: {token_count/elapsed:.1f} tokens/sec")
```

**Cell 15 (markdown):**
```markdown
## Check Your Understanding

1. **Why stream responses instead of waiting for the complete text? What's the UX difference?**

2. **How is a Python async generator different from a Go channel? What does `yield` do?**

3. **Why does the SSE stream end with a `{"done": true, "sources": [...]}` event instead of just stopping?**
```

- [ ] **Step 2: Verify the notebook runs**

Requires Ollama with mistral. Run all cells.

- [ ] **Step 3: Commit**

```bash
git add lessons/06_streaming_and_sse.ipynb
git commit -m "lesson: add 06_streaming_and_sse notebook"
```

---

### Task 8: Lesson 07 — Wiring the Endpoints

**Files:**
- Create: `lessons/07_wiring_the_endpoints.ipynb`

This notebook covers: assembling all pieces into /ingest and /chat endpoints, file upload, CORS, error handling, lazy singletons. Requires Ollama + Qdrant.

- [ ] **Step 1: Create the notebook**

**Cell 1 (markdown):**
```markdown
# Lesson 07: Wiring the Endpoints

## What You're Building

In this lesson you'll assemble everything from lessons 01-06 into two complete FastAPI services: the Ingestion API (POST /ingest, GET /documents) and the Chat API (POST /chat). This is the final lesson — by the end, you'll have rebuilt both services from scratch.

The code in this notebook is functionally equivalent to what's in `services/ingestion/app/main.py` and `services/chat/app/main.py`. Compare your version to the production code when you're done.

## How This Fits in the App

This IS the app. The Ingestion API is the entry point for documents (PDF → text → chunks → embeddings → Qdrant). The Chat API is the entry point for questions (question → embedding → search → prompt → streamed answer).
```

**Cell 2 (code):**
```python
# Prerequisites — requires Ollama and Qdrant
import httpx
from qdrant_client import QdrantClient

OLLAMA_BASE_URL = "http://localhost:11434"
QDRANT_HOST = "localhost"
QDRANT_PORT = 6333

# Quick connectivity check
try:
    async with httpx.AsyncClient() as client:
        await client.get(f"{OLLAMA_BASE_URL}/api/tags", timeout=5.0)
    print("✓ Ollama")
except:
    print("✗ Ollama")

try:
    QdrantClient(host=QDRANT_HOST, port=QDRANT_PORT).get_collections()
    print("✓ Qdrant")
except:
    print("✗ Qdrant")
```

**Cell 3 (markdown):**
```markdown
## Package Introductions

### python-multipart
Handles multipart form data — the format browsers use for file uploads. FastAPI needs this to parse `UploadFile` parameters. In Go, `multipart.Reader` does the same thing. You never call python-multipart directly — FastAPI uses it internally when you declare an `UploadFile` parameter.

### sse-starlette
Wraps an async generator into an SSE (Server-Sent Events) HTTP response. Instead of manually formatting `data: ...\n\n` lines, you yield dicts and `EventSourceResponse` handles the formatting. It's built on Starlette (the ASGI framework under FastAPI).

Key API: `EventSourceResponse(generator)` — takes an async generator, returns a streaming HTTP response.

### CORS Middleware (from FastAPI/Starlette)
CORS (Cross-Origin Resource Sharing) is the browser security mechanism that blocks requests from one origin (e.g., `localhost:3000`) to another (e.g., `localhost:8001`). The `CORSMiddleware` adds the HTTP headers that tell the browser "yes, this other origin is allowed to call me."

In Go, you'd use `rs/cors` or write the headers manually. In Express, you'd use the `cors` package. Same concept, same headers, different framework glue.
```

**Cell 4 (markdown):**
```markdown
## Go/TS Comparison

| Concept | Go | Python/FastAPI |
|---------|-----|----------------|
| File upload | `r.FormFile("file")` | `file: UploadFile = File(...)` |
| Read file bytes | `io.ReadAll(file)` | `await file.read()` |
| CORS | `cors.New(cors.Options{...})` middleware | `CORSMiddleware(allow_origins=["*"])` |
| Singleton | `sync.Once` + package-level var | Module-level var + lazy init function |
| HTTP error | `http.Error(w, msg, 422)` | `raise HTTPException(status_code=422, detail=msg)` |

The **lazy singleton** pattern deserves attention: in Go, you'd use `sync.Once` to initialize a database connection exactly once. In Python, we use a module-level variable + a function that initializes it on first call. It's simpler but serves the same purpose.
```

**Cell 5 (markdown):**
```markdown
## Build It

### Step 1: Define all the building blocks

Let's define all the functions from previous lessons in one place, so our endpoints can use them.
```

**Cell 6 (code):**
```python
# All building blocks from Lessons 02-06, assembled together

import uuid
import json
from io import BytesIO
from typing import AsyncGenerator

from PyPDF2 import PdfReader
from langchain_text_splitters import RecursiveCharacterTextSplitter
import httpx
from qdrant_client import QdrantClient
from qdrant_client.models import Distance, PointStruct, VectorParams

# --- Config ---
OLLAMA_BASE_URL = "http://localhost:11434"
EMBEDDING_MODEL = "nomic-embed-text"
CHAT_MODEL = "mistral"
QDRANT_HOST = "localhost"
QDRANT_PORT = 6333
COLLECTION_NAME = "lesson_07_test"
CHUNK_SIZE = 1000
CHUNK_OVERLAP = 200

# --- PDF Parser (Lesson 02) ---
def extract_pages(pdf_file: BytesIO) -> list[dict]:
    try:
        content = pdf_file.read()
        if not content:
            raise ValueError("empty or invalid PDF")
        pdf_file.seek(0)
        reader = PdfReader(pdf_file)
    except Exception as e:
        if "empty or invalid" in str(e):
            raise
        raise ValueError(f"empty or invalid PDF: {e}")
    return [{"page_number": i + 1, "text": page.extract_text() or ""} for i, page in enumerate(reader.pages)]

# --- Chunker (Lesson 02) ---
def chunk_pages(pages, chunk_size=1000, chunk_overlap=200):
    splitter = RecursiveCharacterTextSplitter(chunk_size=chunk_size, chunk_overlap=chunk_overlap, length_function=len)
    chunks, index = [], 0
    for page in pages:
        text = page["text"].strip()
        if not text:
            continue
        for split in splitter.split_text(text):
            chunks.append({"text": split, "page_number": page["page_number"], "chunk_index": index})
            index += 1
    return chunks

# --- Embedder (Lesson 03) ---
async def embed_texts(texts, ollama_base_url, model):
    if not texts:
        return []
    async with httpx.AsyncClient() as client:
        response = await client.post(f"{ollama_base_url}/api/embed", json={"model": model, "input": texts}, timeout=120.0)
        response.raise_for_status()
        return response.json()["embeddings"]

# --- Store (Lesson 04) ---
class QdrantStore:
    def __init__(self, host, port, collection_name):
        self.client = QdrantClient(host=host, port=port)
        self.collection_name = collection_name
        if not self.client.collection_exists(collection_name):
            self.client.create_collection(collection_name=collection_name, vectors_config=VectorParams(size=768, distance=Distance.COSINE))

    def upsert(self, chunks, vectors, document_id, filename):
        points = [PointStruct(id=str(uuid.uuid4()), vector=v, payload={"text": c["text"], "page_number": c["page_number"], "chunk_index": c["chunk_index"], "document_id": document_id, "filename": filename}) for c, v in zip(chunks, vectors)]
        self.client.upsert(collection_name=self.collection_name, points=points)

    def list_documents(self):
        records, _ = self.client.scroll(collection_name=self.collection_name, limit=10000, with_payload=True, with_vectors=False)
        docs = {}
        for r in records:
            did = r.payload["document_id"]
            if did not in docs:
                docs[did] = {"document_id": did, "filename": r.payload["filename"], "chunks": 0}
            docs[did]["chunks"] += 1
        return list(docs.values())

# --- Prompt (Lesson 05) ---
SYSTEM_PROMPT = """You are a helpful document Q&A assistant. Answer questions based only on the provided context. If the context doesn't contain enough information to answer, say so honestly — do not make up information.\n\nWhen referencing information, mention the source file and page number."""
RAG_TEMPLATE = "Context:\n{context}\n\nQuestion: {question}\n\nAnswer based only on the context above. Cite sources (filename, page) when possible."
NO_CONTEXT_TEMPLATE = "The user asked: {question}\n\nI don't have any relevant context from uploaded documents to answer this question."

def build_rag_prompt(question, chunks):
    if not chunks:
        return NO_CONTEXT_TEMPLATE.format(question=question)
    context = "\n\n".join(f"[{c['filename']}, page {c['page_number']}]\n{c['text']}" for c in chunks)
    return RAG_TEMPLATE.format(context=context, question=question)

# --- Streaming (Lesson 06) ---
async def stream_ollama_response(prompt, model, base_url):
    async with httpx.AsyncClient() as client:
        async with client.stream("POST", f"{base_url}/api/generate", json={"model": model, "prompt": prompt, "system": SYSTEM_PROMPT, "stream": True}, timeout=300.0) as response:
            response.raise_for_status()
            async for line in response.aiter_lines():
                if line.strip():
                    data = json.loads(line)
                    if data.get("response"):
                        yield {"token": data["response"]}
                    if data.get("done"):
                        break

print("All building blocks loaded!")
```

**Cell 7 (markdown):**
```markdown
### Step 2: Build the Ingestion API

This is `services/ingestion/app/main.py`. Note the patterns:
- **Lazy singleton** for the Qdrant store — created on first request, reused after
- **File validation** — check extension and size before processing
- **Error handling** — `HTTPException` for client errors (422), propagated errors from `extract_pages`
```

**Cell 8 (code):**
```python
from fastapi import FastAPI, File, HTTPException, UploadFile
from fastapi.middleware.cors import CORSMiddleware
from fastapi.testclient import TestClient

ingestion_app = FastAPI(title="Ingestion API")

ingestion_app.add_middleware(CORSMiddleware, allow_origins=["*"], allow_methods=["*"], allow_headers=["*"])

# Lazy singleton — initialized on first request
_store = None

def get_store():
    global _store
    if _store is None:
        _store = QdrantStore(host=QDRANT_HOST, port=QDRANT_PORT, collection_name=COLLECTION_NAME)
    return _store

@ingestion_app.get("/health")
async def ingestion_health():
    return {"status": "ok"}

@ingestion_app.post("/ingest")
async def ingest(file: UploadFile = File(...)):
    # Validate file type
    if not file.filename or not file.filename.lower().endswith(".pdf"):
        raise HTTPException(status_code=422, detail="Only PDF files are accepted")

    # Read and validate size
    content = await file.read()
    max_bytes = 50 * 1024 * 1024  # 50MB
    if len(content) > max_bytes:
        raise HTTPException(status_code=422, detail="File exceeds 50MB limit")

    # Extract pages
    try:
        pages = extract_pages(BytesIO(content))
    except ValueError as e:
        raise HTTPException(status_code=422, detail=str(e))

    # Chunk
    chunks = chunk_pages(pages, chunk_size=CHUNK_SIZE, chunk_overlap=CHUNK_OVERLAP)
    if not chunks:
        raise HTTPException(status_code=422, detail="No text content found in PDF")

    # Embed
    texts = [c["text"] for c in chunks]
    vectors = await embed_texts(texts, OLLAMA_BASE_URL, EMBEDDING_MODEL)

    # Store
    document_id = str(uuid.uuid4())
    store = get_store()
    store.upsert(chunks=chunks, vectors=vectors, document_id=document_id, filename=file.filename)

    return {"status": "success", "document_id": document_id, "chunks_created": len(chunks), "filename": file.filename}

@ingestion_app.get("/documents")
async def list_documents():
    store = get_store()
    return {"documents": store.list_documents()}

print("Ingestion API defined!")
```

**Cell 9 (code):**
```python
# Test the ingestion API using FastAPI's test client
from fpdf import FPDF

# Create a test PDF
pdf = FPDF()
pdf.add_page()
pdf.set_font("Helvetica", size=12)
pdf.cell(0, 10, "The quarterly revenue was 2.5 million dollars.")
pdf.ln()
pdf.cell(0, 10, "Operating margins improved to 23 percent.")
test_pdf = BytesIO()
pdf.output(test_pdf)
test_pdf.seek(0)

client = TestClient(ingestion_app)

# Test health
print("Health:", client.get("/health").json())

# Test ingest
response = client.post("/ingest", files={"file": ("report.pdf", test_pdf, "application/pdf")})
print(f"Ingest: {response.json()}")

# Test documents list
print(f"Documents: {client.get('/documents').json()}")

# Test rejection of non-PDF
response = client.post("/ingest", files={"file": ("report.txt", BytesIO(b"hello"), "text/plain")})
print(f"Non-PDF rejected: {response.status_code}")
```

**Cell 10 (markdown):**
```markdown
### Step 3: Build the Chat API

This is `services/chat/app/main.py`. The key new pieces:
- **SSE response** via `EventSourceResponse` — wraps our async generator
- **The full RAG flow** in `rag_query()` — embed → search → prompt → stream
```

**Cell 11 (code):**
```python
from pydantic import BaseModel
from sse_starlette.sse import EventSourceResponse

chat_app = FastAPI(title="Chat API")
chat_app.add_middleware(CORSMiddleware, allow_origins=["*"], allow_methods=["*"], allow_headers=["*"])

class ChatRequest(BaseModel):
    question: str
    collection: str | None = None

async def rag_query(question, ollama_base_url, chat_model, embedding_model, qdrant_host, qdrant_port, collection_name, top_k=5):
    """Full RAG pipeline as an async generator."""
    # Embed
    vectors = await embed_texts([question], ollama_base_url, embedding_model)
    query_vector = vectors[0]

    # Retrieve
    retriever_client = QdrantClient(host=qdrant_host, port=qdrant_port)
    results = retriever_client.search(collection_name=collection_name, query_vector=query_vector, limit=top_k)
    chunks = [{"text": h.payload["text"], "page_number": h.payload["page_number"], "filename": h.payload["filename"], "document_id": h.payload["document_id"], "score": h.score} for h in results]

    # Prompt
    prompt = build_rag_prompt(question, chunks)

    # Sources
    seen = set()
    sources = []
    for c in chunks:
        key = (c["filename"], c["page_number"])
        if key not in seen:
            seen.add(key)
            sources.append({"file": c["filename"], "page": c["page_number"]})

    # Stream
    async for event in stream_ollama_response(prompt, chat_model, ollama_base_url):
        yield event
    yield {"done": True, "sources": sources}

@chat_app.get("/health")
async def chat_health():
    return {"status": "healthy"}

@chat_app.post("/chat")
async def chat(request: ChatRequest):
    async def event_generator():
        async for event in rag_query(
            question=request.question,
            ollama_base_url=OLLAMA_BASE_URL,
            chat_model=CHAT_MODEL,
            embedding_model=EMBEDDING_MODEL,
            qdrant_host=QDRANT_HOST,
            qdrant_port=QDRANT_PORT,
            collection_name=request.collection or COLLECTION_NAME,
        ):
            yield {"data": json.dumps(event)}
    return EventSourceResponse(event_generator())

print("Chat API defined!")
```

**Cell 12 (code):**
```python
# Test the chat API
chat_client = TestClient(chat_app)

# Health check
print("Health:", chat_client.get("/health").json())

# Chat — this will stream an SSE response
response = chat_client.post("/chat", json={"question": "What was the revenue?"})
print(f"\nStatus: {response.status_code}")
print(f"Content-Type: {response.headers.get('content-type')}")

# Parse SSE events
events = []
for line in response.text.strip().split("\n"):
    if line.startswith("data: "):
        events.append(json.loads(line[6:]))

tokens = [e["token"] for e in events if "token" in e]
done = [e for e in events if e.get("done")]

print(f"\nTokens received: {len(tokens)}")
print(f"Answer: {''.join(tokens)}")
if done:
    print(f"Sources: {done[0]['sources']}")
```

**Cell 13 (markdown):**
```markdown
### Congratulations!

You've rebuilt both backend services from scratch. Compare your code to:
- `services/ingestion/app/main.py` — your Ingestion API
- `services/chat/app/main.py` — your Chat API

The production code splits things into separate files (pdf_parser.py, chunker.py, embedder.py, etc.) but the logic is identical to what you've built here.
```

**Cell 14 (code):**
```python
# Clean up
store = get_store()
store.client.delete_collection(COLLECTION_NAME)
_store = None
print("Cleaned up test collection")
```

**Cell 15 (markdown):**
```markdown
## Check Your Understanding

1. **Why do we use a lazy singleton for the Qdrant store instead of creating it at import time?** (Hint: think about what happens during testing and when the service first starts)

2. **What does `CORSMiddleware(allow_origins=["*"])` do, and why would you tighten it for production?**

3. **Trace the full data flow for a chat request: what happens from the moment the user hits Send to when they see the first token?** (List every step: frontend → backend → Ollama → Qdrant → etc.)
```

- [ ] **Step 2: Verify the notebook runs**

Requires Ollama (mistral + nomic-embed-text) and Qdrant. Run all cells.

- [ ] **Step 3: Commit**

```bash
git add lessons/07_wiring_the_endpoints.ipynb
git commit -m "lesson: add 07_wiring_the_endpoints notebook"
```
