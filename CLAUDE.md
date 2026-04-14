# CLAUDE.md

## Project Intent

Portfolio project for a Gen AI Engineer job application — demonstrating RAG architecture, agentic AI, prompt engineering, and Python API development through two AI tools: a Document Q&A Assistant and a Debug Assistant.

## Tech Stack

- **Python:** FastAPI microservices (ingestion, chat, debug), LangChain text splitters, Qdrant vector DB
- **Java:** Spring Boot microservices (task, activity, notification, gateway), PostgreSQL, MongoDB, Redis, RabbitMQ, GraphQL
- **Go:** Auth, ecommerce, and AI agent services, PostgreSQL, Redis, RabbitMQ, shared `go/pkg/` module (see `go/CLAUDE.md`)
- **AI/ML:** Ollama (Qwen 2.5 14B for chat/debug, nomic-embed-text for embeddings)
- **Frontend:** Next.js + TypeScript + shadcn/ui, Apollo Client (GraphQL)
- **Testing:** pytest, JUnit, Go test/benchmarks, Playwright (E2E)
- **Infra:** Docker Compose, Minikube (K8s), NGINX Ingress, Cloudflare Tunnel, GitHub Actions

## Infrastructure

- **Mac (dev machine):** Code editing, frontend dev server, no GPU. Docker runtime is Colima (start with `colima start` before `docker compose`).
- **Windows (PC@100.79.113.84 via Tailscale):** Ollama (RTX 3090), Minikube (all backend services)
- **SSH:** `ssh PC@100.79.113.84` — key-based auth configured
- **Minikube:** All backend services run in Kubernetes on the Windows PC
  - `ai-services` namespace: Python AI services + Qdrant
  - `java-tasks` namespace: Java microservices + databases
  - `go-ecommerce` namespace: Go auth + ecommerce services
  - `monitoring` namespace: Prometheus + Grafana
  - `ai-services-qa` namespace: QA copies of Python AI services (shared Qdrant with prod)
  - `java-tasks-qa` namespace: QA copies of Java services (shared infra with prod)
  - `go-ecommerce-qa` namespace: QA copies of Go services (shared infra with prod)
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
  - Ingress routes by path: `/ingestion/*`, `/chat/*`, `/debug/*` → Python services; `/graphql`, `/api/auth/*` → Java services; `/go-api/*`, `/go-auth/*` → Go services; `/grafana/*` → monitoring
  - Cloudflared installed as Windows service (auto-starts on boot)
  - `minikube tunnel` must be running as background process

### Vercel CLI

The Vercel CLI is installed and `frontend/.vercel/` is linked to the `frontend` project under team `kabradshaw1s-projects`. Claude can manage env vars and trigger redeploys directly from the Mac.

```bash
# List production env vars
cd frontend && vercel env ls production

# Add a new public env var (NEXT_PUBLIC_*) to production
printf 'https://api.kylebradshaw.dev/go-api' | vercel env add NEXT_PUBLIC_GO_ECOMMERCE_URL production

# Redeploy the latest production deployment so new env vars take effect
# (env var changes require a rebuild — they don't apply to existing deployments)
vercel switch kabradshaw1s-projects
LATEST=$(vercel ls --prod 2>&1 | awk '/Production/ {print $4; exit}')
vercel redeploy "$LATEST" --target production
```

Frontend env vars currently set in Vercel production:
- `NEXT_PUBLIC_API_URL`, `NEXT_PUBLIC_GATEWAY_URL` — Java gateway
- `NEXT_PUBLIC_INGESTION_API_URL`, `NEXT_PUBLIC_CHAT_API_URL` — Python AI services
- `NEXT_PUBLIC_GO_AUTH_URL=https://api.kylebradshaw.dev/go-auth`
- `NEXT_PUBLIC_GO_ECOMMERCE_URL=https://api.kylebradshaw.dev/go-api`
- `NEXT_PUBLIC_AI_SERVICE_URL=https://api.kylebradshaw.dev/ai-api` (add in Vercel before merge — localhost fallback will otherwise bake into the production bundle)

If frontend code adds a new `NEXT_PUBLIC_*` env var with a `localhost` fallback, **Vercel will silently bake the localhost fallback into the production bundle** unless the env var is added in Vercel and a redeploy is triggered. Always add the var to Vercel before merging.

### Migrations

- **Go services (`go/auth-service`, `go/ecommerce-service`):** schema changes use `golang-migrate`. Migration files live in `go/<service>/migrations/` and use the strict `NNN_name.up.sql` / `NNN_name.down.sql` pair format. The `migrate` binary is baked into each service image; a Kubernetes `Job` per service (`go/k8s/jobs/*-migrate.yml`) runs `migrate up` on every deploy before the deployments are rolled.
- **To add a schema change:** create a new `NNN_name.up.sql` + matching `.down.sql` in the right `migrations/` directory. Commit. The next deploy runs it automatically.
- **Seed data (ecommerce only):** lives in `go/ecommerce-service/seed.sql`, applied by the ecommerce Job after `migrate up`. Must be idempotent (guard every INSERT with `WHERE NOT EXISTS`).
- **Java services:** schema is owned by Spring/JPA at service startup. No separate migration step.
- **Python AI services:** no relational schema (Qdrant is schemaless).
- **`postgres-initdb` ConfigMap:** only creates the `ecommercedb` database on first boot of a fresh PVC. It does NOT own any schemas — those are owned by the per-service migration Jobs.
- **`sslmode=disable` is required on Go `DATABASE_URL`s.** The shared postgres has no SSL. `pgxpool` (used by the services) defaults to `sslmode=prefer` so it works either way, but `golang-migrate`'s `pq` driver defaults to `sslmode=require` and will fail with `pq: SSL is not enabled on the server`. Always include `?sslmode=disable` in the connection string.

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
├── ai-service/             # Agent loop over Ollama tool-calling, 9 tools wrapping ecommerce
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
├── go-ai-service/          # 1 markdown ADR (agent harness in Go, tool registry, evals)
├── *.md                    # Standalone ADRs (CI/CD, deployment, K8s migration, analytics, auth, RAG re-evaluation, etc.)
.github/workflows/          # CI/CD: ci.yml (unified), aws-deploy.yml (manual EKS)
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
- `qa` — pre-production QA environment. PRs trigger quality checks. Pushes trigger build + deploy to QA + smoke tests.
- Feature branches (`agent/feat-*`) — created by agents from `main`, short-lived, deleted after merge.
- `staging` — retired. Replaced by `qa`.

**Per-branch rules for Claude Code:**

- **On a feature branch:** implement, commit, push, create PR to `qa`. Don't ask before pushing or creating the PR — just do it. The CI pipeline catches problems.
- **On `qa`:** commit and push autonomously. Don't ask before pushing. Watch CI after pushing and debug failures. For CI fixes: lint errors, formatting, type errors, and config issues are fine to fix autonomously. For anything that changes application behavior (logic, API contracts, data flow), stop and check with Kyle before fixing.
- **On `main`:** never push autonomously. When Kyle explicitly says to merge/ship to main, handle the full flow: merge `qa` into `main`, push, watch CI, debug minor failures, clean up worktree, delete feature branch (local + remote).

Claude Code determines the current branch via `git branch --show-current` and follows the rules for that branch. No special mode or prompt needed.

**Agent worktrees:** Agents create worktrees in `.claude/worktrees/<branch-name>/` for feature work. Worktrees are cleaned up as part of the "ship to main" flow.

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

Single unified workflow (`.github/workflows/ci.yml`) handles all CI/CD:

| Trigger | What runs |
|---------|-----------|
| PR to `qa` | All quality checks (lint, test, security, k8s validation) |
| Push to `qa` | Quality checks + build all images + deploy to QA + smoke QA |
| Push to `main` | Quality checks + build all images + deploy to prod + smoke prod |

**Quality:** ruff lint/format, pytest + coverage, tsc, Next.js build, checkstyle, golangci-lint, Go tests
**Security:** Bandit (SAST), pip-audit, npm audit, gitleaks, Hadolint, CORS guardrail
**Deploy:** GHCR images built in CI → SSH to Windows PC → kubectl apply Kustomize overlays
**QA:** `qa-api.kylebradshaw.dev` (backend), `qa.kylebradshaw.dev` (frontend on Vercel)

**Compose-smoke realism:** Job 3 (`compose-smoke`) runs the Python AI stack via `docker-compose.yml` with a mocked Ollama. Any change to Python service configuration (env vars, ports, depends_on, env_file references) must be reflected in BOTH `docker-compose.yml` and the corresponding k8s manifests under `k8s/ai-services/`, or compose-smoke will drift from prod and stop catching real regressions.

**Tailscale authkey:** Expires every 90 days (free plan). Regenerate at Tailscale admin → Keys and update `TAILSCALE_AUTHKEY` in GitHub repo secrets.

## Browser Console Configuration
When I need to configure something in a console, first check to see if you can do it from the command line tool.  If not, then please give me a link to the exact pages I will need to visit, and as much details as you can about what I will need to do.  Consoles I have visited in c