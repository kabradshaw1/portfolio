# Architecture Decision Records (ADRs)

Design decision documentation for this project. Each document explains *why* things were built the way they were — package choices, architecture patterns, trade-offs considered.

## Structure

```
docs/adr/
├── README.md              # This file
├── template-adr.md        # Markdown template for standalone decisions
└── document-qa/           # ADR notebooks for the Document Q&A services
    ├── requirements.txt
    ├── 01_python_fastapi_basics.ipynb
    ├── 02_pdf_parsing_and_chunking.ipynb
    ├── 03_embeddings_and_vectors.ipynb
    ├── 04_qdrant_vector_storage.ipynb
    ├── 05_rag_chain_and_prompts.ipynb
    ├── 06_streaming_and_sse.ipynb
    └── 07_wiring_the_endpoints.ipynb
```

New services get their own subdirectory (e.g., `docs/adr/new-service/`).

## Two Formats

### Jupyter Notebooks — service-level documentation

For services or multi-file features. Each notebook walks through the service step-by-step, rebuilding it with explanations of every design decision.

**Sections:**
1. **Overview** — what the service does and why it exists
2. **Architecture Context** — where it fits in the larger system
3. **Package Introductions** — each dependency, why chosen over alternatives
4. **Go/TS Comparison** — maps concepts to other languages (when relevant)
5. **Build It** — step-by-step code cells with explanatory markdown
6. **Experiment** — parameter tweaks exploring trade-offs
7. **Check Your Understanding** — reflection questions about design decisions

**Supported languages:**
- Python — standard Jupyter kernel
- TypeScript — Deno kernel (`deno jupyter`)
- Go/Rust — use markdown format instead (no mature Jupyter kernel)

### Markdown ADRs — standalone decisions

For smaller, self-contained decisions. Use `template-adr.md` as a starting point.

Example topics: "Why Qdrant over Pinecone", "Why RecursiveCharacterTextSplitter over other strategies", "CORS policy design".

## Creating ADRs with Claude

### Automatic
After Claude finishes building a service, it will suggest creating ADR documentation. Accept the suggestion and it follows the standard structure.

### Manual
Tell Claude to invoke the skill:
```
/writing-adrs
```
Or simply ask: "Write ADR documentation for the [service-name] service."

### What Claude will do
1. Review the service code that was just built
2. Identify key design decisions (packages, patterns, data flow, error handling)
3. Choose format (notebook or markdown) based on scope
4. Write the document following the standard structure
5. Place it in `docs/adr/<service-name>/`

## Running Notebooks

Install dependencies first:
```bash
cd docs/adr/<service-name>
pip install -r requirements.txt
```

Open in VS Code (with Jupyter extension) or JupyterLab. Run cells top-to-bottom — each cell builds on the previous ones.

Some notebooks require external services (Ollama, Qdrant). Check the Prerequisites section at the top of each notebook.
