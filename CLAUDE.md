# CLAUDE.md

## Project Intent

Portfolio project for a Gen AI Engineer job application — a Document Q&A Assistant demonstrating RAG architecture, prompt engineering, and Python API development.

## Tech Stack

- FastAPI (Python backend microservices)
- Qdrant (vector database, Docker container)
- Ollama (mistral 7B for chat, nomic-embed-text for embeddings)
- LangChain text splitters (chunking only — not using the full LangChain framework)
- Next.js + TypeScript + shadcn/ui (frontend)
- Playwright (E2E testing)
- Docker Compose (backend orchestration)

## Infrastructure

- **Mac (dev machine):** Code editing, frontend dev server, no GPU
- **Windows (PC@100.79.113.84 via Tailscale):** Ollama (RTX 3090), Docker Compose (Qdrant + backend services)
- **SSH:** `ssh PC@100.79.113.84` — key-based auth configured
- **Local dev:** SSH tunnels forward `localhost:8001` and `localhost:8002` to Windows backend
  ```bash
  ssh -f -N -L 8001:localhost:8001 -L 8002:localhost:8002 PC@100.79.113.84
  ```
- **Frontend:** `npm run dev` in `frontend/`, points to `localhost:8001`/`8002` via tunnels
- **Production:** Frontend on Vercel (`https://kylebradshaw.dev`), backend via Cloudflare Tunnel:
  - `https://api-chat.kylebradshaw.dev` → Windows PC :8002
  - `https://api-ingestion.kylebradshaw.dev` → Windows PC :8001
  - Cloudflared installed as Windows service (auto-starts on boot)

## Project Structure

```
services/
├── ingestion/          # FastAPI — PDF upload, parse, chunk, embed, store, delete
│   ├── app/            # main.py, pdf_parser.py, chunker.py, embedder.py, store.py, config.py
│   └── tests/          # unit tests
├── chat/               # FastAPI — question embed, search, RAG prompt, stream
│   ├── app/            # main.py, retriever.py, prompt.py, chain.py, config.py
│   └── tests/          # unit tests
frontend/               # Next.js + shadcn/ui — chat UI, PDF upload, document management
├── e2e/                # Playwright E2E tests (mocked + production smoke)
├── src/components/     # ChatWindow, FileUpload, DocumentList, MessageInput, SourceBadge
docs/adr/               # Architecture Decision Records (notebooks per service + markdown ADRs)
├── document-qa/        # 7 notebooks explaining the Document Q&A services step-by-step
├── template-adr.md     # Lightweight ADR template for smaller decisions
.github/workflows/      # CI/CD pipeline with security scanning
docker-compose.yml      # Qdrant + ingestion + chat services
.env.example            # Config template
```

## Kyle's Background

- Strong in Go and TypeScript (full-stack web apps)
- Experienced with Docker, Kubernetes, GitHub Actions, SQL/NoSQL
- Has used Ollama and built web services to interact with it
- Limited hands-on experience with Python data processing, LLM workflows, RAG, prompt engineering
- Has written Python for Django, taken tutorials, but limited production Python experience

## Architecture Decision Records (ADRs)

Design decision documentation lives in `docs/adr/`, organized by service. Two formats:

**Jupyter notebooks** — for service-level documentation. Each notebook walks through how a service was built step-by-step, explaining design decisions along the way. Sections: Overview, Architecture Context, Package Introductions, Go/TS Comparison, Build It, Experiment, Check Your Understanding.

**Markdown ADRs** — for smaller, standalone decisions (e.g., "Why Qdrant over Pinecone"). Use `docs/adr/template-adr.md` as a starting point.

Current notebooks: `docs/adr/document-qa/` (7 notebooks covering the ingestion and chat services).

## Branching & Workflow

- `main` — production. Pushes trigger deploy + post-deploy smoke tests.
- `staging` — integration branch. Pushes trigger mocked Playwright E2E tests.
- `feat/*`, `fix/*` — feature branches merged into `staging` first.
- **Kyle handles all git push and merge operations.** Claude should commit locally but never push to remote.

**Developer workflow:**
1. Create feature branch from `staging`
2. Push — CI runs lint, unit tests, security scans
3. Merge into `staging` — CI runs mocked E2E tests
4. If all pass, merge `staging` into `main`
5. CI deploys to production, runs smoke tests against live URLs

**Exception:** Hotfixes for CI/production breakage can go straight to `main`.

**Git commands (Kyle runs all push/merge):**
```bash
# Start feature work
git checkout staging && git pull origin staging
git checkout -b feat/my-feature

# Work and commit (Claude does this part)
git add <files> && git commit -m "feat: description"

# Merge feature → staging (Kyle does this)
git checkout staging
git merge feat/my-feature
git push origin staging
# Wait for staging CI (lint + tests + security + E2E) to pass

# Promote staging → main (Kyle does this)
git checkout main && git pull origin main
git merge staging
git push origin main
# Wait for deploy + smoke tests to pass

# Clean up
git branch -d feat/my-feature
```

## Pre-commit Requirements

Before every commit, ensure code passes CI checks:
- **Python:** `ruff check services/` and `ruff format --check services/` must pass
- **Frontend:** `cd frontend && npx tsc --noEmit` must pass
- Pre-commit hooks run automatically (ruff lint + format on `services/`)
- If pre-commit rejects a commit, stage the auto-fixed files and re-commit

## CI/CD Pipeline

All jobs run on every push. Security + E2E jobs gate deployment.

**Quality:** ruff lint/format, pytest + coverage, tsc, Next.js build
**Security:** Bandit (SAST), pip-audit, npm audit, gitleaks, Hadolint, CORS guardrail
**E2E:** Playwright mocked tests (staging), production smoke tests (post-deploy)
**Deploy:** GHCR images built in CI → SSH to Windows PC → `docker compose pull && up -d`

**Known:** langchain 0.2.x has 5 CVEs that require 0.3.x migration (ignored in pip-audit). Migration tracked as future work.

**Tailscale authkey:** Expires every 90 days (free plan). Regenerate at Tailscale admin → Keys and update `TAILSCALE_AUTHKEY` in GitHub repo secrets.

## Adding a New Service

When adding a new service under `services/`, update these:
1. `ci.yml` — add to `backend-tests.strategy.matrix.service`
2. `ci.yml` — add to `docker-build.strategy.matrix.service`
3. `ci.yml` — add to `security-pip-audit.strategy.matrix.service`
4. `ci.yml` — add Dockerfile path to `security-hadolint.strategy.matrix.dockerfile`
5. `docker-compose.yml` — add service with GHCR image
6. `ci.yml` deploy step — add service name to `docker compose pull` command
7. `docs/adr/<service-name>/` — create companion ADR notebooks explaining the service's design decisions step-by-step

## Design Specs

- `docs/superpowers/specs/2026-03-31-document-qa-assistant-design.md` — full system architecture
- `docs/superpowers/specs/2026-03-31-frontend-design.md` — frontend design
- `docs/superpowers/specs/2026-03-31-lesson-notebooks-design.md` — ADR notebook design
- `docs/superpowers/specs/2026-03-31-devsecops-design.md` — security hardening
- `docs/superpowers/specs/2026-04-01-e2e-testing-and-staging-design.md` — E2E testing & staging workflow

## Current State

- **Backend:** Complete — unit tests passing, Docker Compose on Windows, CORS hardened via env var
- **Frontend:** Complete — chat UI, PDF upload, document management with delete
- **E2E Tests:** 9 mocked Playwright tests + production smoke suite
- **ADRs:** Complete — 7 notebooks in `docs/adr/document-qa/`
- **Deployed:** Frontend on Vercel, backend via Cloudflare Tunnel
- **Security:** Automated scanning in CI (Bandit, pip-audit, npm audit, gitleaks, Hadolint)
