# CLAUDE.md

## Project Intent

Portfolio project for a Gen AI Engineer job application — demonstrating RAG architecture, agentic AI, prompt engineering, and Python API development through two AI tools: a Document Q&A Assistant and a Debug Assistant.

## Tech Stack

- **Python:** FastAPI microservices (ingestion, chat, debug), LangChain text splitters, Qdrant vector DB
- **Java:** Spring Boot microservices (task, activity, notification, gateway), PostgreSQL, MongoDB, Redis, RabbitMQ, GraphQL
- **Go:** Auth service + ecommerce service, PostgreSQL, Redis, RabbitMQ
- **AI/ML:** Ollama (Qwen 2.5 14B for chat/debug, nomic-embed-text for embeddings)
- **Frontend:** Next.js + TypeScript + shadcn/ui, Apollo Client (GraphQL)
- **Testing:** pytest, JUnit, Go test/benchmarks, Playwright (E2E)
- **Infra:** Docker Compose, Minikube (K8s), NGINX Ingress, Cloudflare Tunnel, GitHub Actions

## Infrastructure

- **Mac (dev machine):** Code editing, frontend dev server, no GPU
- **Windows (PC@100.79.113.84 via Tailscale):** Ollama (RTX 3090), Minikube (all backend services)
- **SSH:** `ssh PC@100.79.113.84` — key-based auth configured
- **Minikube:** All backend services run in Kubernetes on the Windows PC
  - `ai-services` namespace: Python AI services + Qdrant
  - `java-tasks` namespace: Java microservices + databases
  - `go-ecommerce` namespace: Go auth + ecommerce services
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
  - Ingress routes by path: `/ingestion/*`, `/chat/*`, `/debug/*` → Python services; `/graphql`, `/api/auth/*` → Java services; `/go/*` → Go services; `/grafana/*` → monitoring
  - Cloudflared installed as Windows service (auto-starts on boot)
  - `minikube tunnel` must be running as background process

## Project Structure

```
services/                   # Python AI microservices
├── ingestion/              # FastAPI — PDF upload, parse, chunk, embed, store, delete
├── chat/                   # FastAPI — question embed, search, RAG prompt, stream
├── debug/                  # FastAPI — code indexing, agent loop, tool execution, debug streaming
java/                       # Java microservices (Spring Boot, Gradle multi-project)
├── task-service/           # Task/project CRUD, PostgreSQL, JPA
├── activity-service/       # Activity feed, MongoDB, Redis caching, analytics aggregation
├── notification-service/   # Event-driven notifications, RabbitMQ consumer
├── gateway-service/        # GraphQL gateway, routes to task/activity/notification
├── k8s/                    # Java-specific K8s manifests
go/                         # Go microservices
├── auth-service/           # JWT auth (register, login, refresh), PostgreSQL
├── ecommerce-service/      # Products, cart, orders, Redis caching, RabbitMQ worker pool
├── k8s/                    # Go-specific K8s manifests
frontend/                   # Next.js + shadcn/ui
├── src/app/                # Pages: ai/ (rag, debug), java/ (tasks), go/ (ecommerce)
├── src/components/         # Shared UI + domain components (java/, go/)
├── src/lib/                # API clients, auth utils, Apollo GraphQL client
├── e2e/                    # Playwright E2E tests (mocked + production smoke)
nginx/                      # Reverse proxy — path-based routing to all backend stacks
k8s/                        # Kubernetes manifests — production deployment (Minikube)
├── ai-services/            # Python AI services + Qdrant
├── monitoring/             # Prometheus + Grafana
├── deploy.sh               # Unified deploy script for all namespaces
monitoring/                 # Prometheus + Grafana config files
docs/adr/                   # Architecture Decision Records
├── document-qa/            # 7 notebooks (Python/FastAPI, RAG pipeline)
├── document-debugger/      # 3 notebooks (code-aware chunking, agent loop, tools)
├── java-task-management/   # 7 markdown lessons (Spring Boot, JPA, GraphQL, etc.)
├── *.md                    # Standalone ADRs (CI/CD, deployment, K8s migration, etc.)
.github/workflows/          # CI/CD: ci.yml (Python + frontend), java-ci.yml, go-ci.yml
docker-compose.yml          # Python services + nginx + Qdrant
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

Current ADRs:
- `docs/adr/document-qa/` — 7 Jupyter notebooks covering the ingestion and chat services
- `docs/adr/document-debugger/` — 3 Jupyter notebooks covering the debug assistant
- `docs/adr/java-task-management/` — 7 markdown lessons covering the Java microservices stack
- Standalone markdown ADRs for CI/CD, deployment architecture, K8s migration, analytics, auth, etc.

## Branching & Workflow

- `main` — production. Pushes trigger deploy + post-deploy smoke tests.
- `staging` — integration branch. Pushes trigger mocked Playwright E2E tests.
- **Kyle handles all git push and merge operations.** Claude should commit locally but never push to remote.

**New features and non-urgent changes happen on a feature branch:**

1. Claude creates a feature branch from `main` (e.g., `fix-task-403`, `add-analytics`)
2. Claude makes changes and commits on the feature branch
3. Kyle reviews the diff, pushes the feature branch, and watches CI
4. Kyle merges the feature branch into `staging` and pushes — watches CI (lint, tests, security, E2E)
5. If all pass, Kyle merges `staging` into `main` and pushes — watches CI (deploy + smoke tests)
6. Kyle deletes the feature branch after merge

**Fixes for things already in production can go straight to `main`.**

**Do not use git worktrees or the EnterWorktree/ExitWorktree tools.**

## Pre-commit Requirements

Before every commit, run the relevant preflight checks and fix any failures. Only escalate to Kyle if you can't resolve the issue.

- **Python changes:** `make preflight-python` and `make preflight-security`
- **Frontend changes:** `make preflight-frontend` and `make preflight-e2e`
- **Java changes:** `make preflight-java` (checkstyle + unit tests, runs locally)
- **Java integration tests:** `make preflight-java-integration` (runs over SSH on Windows PC, on-demand)
- **Go changes:** `make preflight-go` (lint + tests)
- **Full sweep:** `make preflight` (runs Python + frontend + security + Java + Go locally)

If a check fails, fix it before committing. If you can't fix it, explain the failure to Kyle before suggesting a push.

## CI/CD Pipeline

All jobs run on every push. Security + E2E jobs gate deployment.

**Quality:** ruff lint/format, pytest + coverage, tsc, Next.js build, checkstyle, golangci-lint, Go tests
**Security:** Bandit (SAST), pip-audit, npm audit, gitleaks, Hadolint, CORS guardrail
**E2E:** Playwright mocked tests (staging), production smoke tests (post-deploy)
**Deploy:** GHCR images built in CI → SSH to Windows PC → `docker compose pull && up -d`
**Separate CI workflows:** `ci.yml` (Python + frontend + security), `java-ci.yml`, `go-ci.yml`

**Tailscale authkey:** Expires every 90 days (free plan). Regenerate at Tailscale admin → Keys and update `TAILSCALE_AUTHKEY` in GitHub repo secrets.
