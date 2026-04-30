# Python AI Services

All backend services under `services/` are Python/FastAPI microservices.

## AI Service Topology

- `ingestion` - PDF upload, parse, chunk, embed, store, delete
- `chat` - question embed, search, RAG prompt, stream
- `debug` - code indexing, agent loop, tool execution, debug streaming
- `eval` - RAG evaluation metrics and experiment support when present

The services use Qdrant for vector storage. Ollama runs on Debian with GPU
access; chat/debug use Qwen 2.5 14B where configured, and embeddings use
`nomic-embed-text`.

Shared LLM and model-loading code lives under `services/shared/llm/`.

## Package Selection

- Prefer minimal, focused packages over large frameworks, such as
  `langchain-text-splitters` instead of the full `langchain` framework.
- When adding or updating dependencies, verify the package is current, not
  deprecated or renamed, and that import paths match the installed version.

Known deprecation: PyPDF2 was renamed to `pypdf` by the same authors.

## Local And Runtime Notes

Local development can use Docker Compose for Python services, nginx, and
Qdrant. Minikube is not required for local Python development.

Production and QA Kubernetes service configuration should stay aligned with
`docker-compose.yml` when changing Python service env vars, ports,
`depends_on`, or `env_file` references. The CI `compose-smoke` job depends on
that realism.

## Adding A New Python Service

When adding a service under `services/`, update:

1. `.github/workflows/ci.yml` backend test matrix
2. `.github/workflows/ci.yml` docker build matrix
3. `.github/workflows/ci.yml` pip-audit matrix
4. `.github/workflows/ci.yml` Hadolint Dockerfile matrix
5. `docker-compose.yml`
6. CI deploy pull commands
7. A companion ADR or notebook under `docs/adr/<service-name>/` when the design
   needs explanation

## Verification

Before committing Python changes, run:

```bash
make preflight-python
make preflight-security
```

Pre-commit hooks run ruff lint and format checks for relevant Python changes.
If pre-commit auto-fixes files, stage the fixed files and re-commit.
