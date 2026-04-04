# CLAUDE.md

## Project Intent

Portfolio project for a Gen AI Engineer job application — demonstrating RAG architecture, agentic AI, prompt engineering, and Python API development through two AI tools: a Document Q&A Assistant and a Debug Assistant.

## Tech Stack

- FastAPI (Python backend microservices)
- Qdrant (vector database, Docker container)
- Ollama (Qwen 2.5 14B for chat/debug, nomic-embed-text for embeddings)
- LangChain text splitters
- Next.js + TypeScript + shadcn/ui (frontend)
- Playwright (E2E testing)
- Docker Compose (backend orchestration)

## Infrastructure

- **Mac (dev machine):** Code editing, frontend dev server, no GPU
- **Windows (PC@100.79.113.84 via Tailscale):** Ollama (RTX 3090), Minikube (all backend services)
- **SSH:** `ssh PC@100.79.113.84` — key-based auth configured
- **Minikube:** All backend services run in Kubernetes on the Windows PC
  - `ai-services` namespace: Python AI services + Qdrant
  - `java-tasks` namespace: Java microservices + databases
  - `monitoring` namespace: Prometheus + Grafana
  - NGINX Ingress Controller routes all traffic by path
  - `minikube tunnel` exposes Ingress on localhost:80
- **Ollama:** Runs natively on Windows (GPU access), reached from K8s via ExternalName service
- **Local dev:** Docker Compose for both stacks (no Minikube needed for development)
  - SSH tunnel forwards `localhost:8000` to Windows nginx gateway
  ```bash
  ssh -f -N -L 8000:localhost:8000 PC@100.79.113.84
  ```
- **Frontend:** `npm run dev` in `frontend/`, points to `localhost:8000` via tunnel
- **Production:** Frontend on Vercel (`https://kylebradshaw.dev`), backend via Cloudflare Tunnel:
  - `https://api.kylebradshaw.dev` → Windows PC localhost:80 (Minikube Ingress)
  - Ingress routes by path: `/ingestion/*`, `/chat/*`, `/debug/*` → Python services; `/graphql`, `/api/auth/*` → Java services; `/grafana/*` → monitoring
  - Cloudflared installed as Windows service (auto-starts on boot)
  - `minikube tunnel` must be running as background process

## Project Structure

```
services/
├── ingestion/          # FastAPI — PDF upload, parse, chunk, embed, store, delete
│   ├── app/            # main.py, pdf_parser.py, chunker.py, embedder.py, store.py, config.py
│   └── tests/          # unit tests
├── chat/               # FastAPI — question embed, search, RAG prompt, stream
│   ├── app/            # main.py, retriever.py, prompt.py, chain.py, config.py
│   └── tests/          # unit tests
├── debug/              # FastAPI — code indexing, agent loop, tool execution, debug streaming
│   ├── app/            # main.py, agent.py, tools.py, indexer.py, prompts.py, config.py
│   └── tests/          # unit tests
nginx/                  # Reverse proxy — path-based routing to backend services
├── nginx.conf          # /ingestion/*, /chat/*, /debug/* → respective services
k8s/                    # Kubernetes manifests — production deployment (Minikube)
├── ai-services/        # Python AI services + Qdrant namespace
├── monitoring/         # Prometheus + Grafana namespace
└── deploy.sh           # Unified deploy script for all namespaces
frontend/               # Next.js + shadcn/ui — chat UI, PDF upload, document management, debug
├── e2e/                # Playwright E2E tests (mocked + production smoke)
├── src/components/     # ChatWindow, FileUpload, DocumentList, DebugForm, AgentTimeline, etc.
docs/adr/               # Architecture Decision Records (notebooks per service + markdown ADRs)
├── document-qa/        # 7 notebooks explaining the Document Q&A services step-by-step
├── template-adr.md     # Lightweight ADR template for smaller decisions
.github/workflows/      # CI/CD pipeline with security scanning
docker-compose.yml      # nginx gateway + Qdrant + ingestion + chat + debug services
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
- **Python:** see `services/CLAUDE.md`
- **Frontend:** `cd frontend && npx tsc --noEmit` must pass

## CI/CD Pipeline

All jobs run on every push. Security + E2E jobs gate deployment.

**Quality:** ruff lint/format, pytest + coverage, tsc, Next.js build
**Security:** Bandit (SAST), pip-audit, npm audit, gitleaks, Hadolint, CORS guardrail
**E2E:** Playwright mocked tests (staging), production smoke tests (post-deploy)
**Deploy:** GHCR images built in CI → SSH to Windows PC → `docker compose pull && up -d`

**Tailscale authkey:** Expires every 90 days (free plan). Regenerate at Tailscale admin → Keys and update `TAILSCALE_AUTHKEY` in GitHub repo secrets.

## Design Specs

- `docs/superpowers/specs/2026-03-31-document-qa-assistant-design.md` — full system architecture
- `docs/superpowers/specs/2026-03-31-frontend-design.md` — frontend design
- `docs/superpowers/specs/2026-03-31-lesson-notebooks-design.md` — ADR notebook design
- `docs/superpowers/specs/2026-03-31-devsecops-design.md` — security hardening
- `docs/superpowers/specs/2026-04-01-e2e-testing-and-staging-design.md` — E2E testing & staging workflow
- `docs/superpowers/specs/2026-04-03-debug-assistant-design.md` — debug assistant architecture

## Current State

- **Backend:** Ingestion + chat + debug services, all with unit tests, Docker Compose on Windows
- **Frontend:** Document Q&A chat UI + Debug Assistant with agent timeline
- **Gateway:** nginx reverse proxy — single API domain with path-based routing
- **E2E Tests:** 9 mocked Playwright tests + production smoke suite
- **ADRs:** 7 notebooks in `docs/adr/document-qa/`
- **Deployed:** Frontend on Vercel, backend via Cloudflare Tunnel (`api.kylebradshaw.dev`)
- **Security:** Automated scanning in CI (Bandit, pip-audit, npm audit, gitleaks, Hadolint)
- **K8s Deployment:** All services in Minikube (3 namespaces), NGINX Ingress Controller, unified deploy script
