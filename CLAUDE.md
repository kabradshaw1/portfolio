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
- **Kyle handles all git push and merge operations.** Claude should commit locally but never push to remote.

**All code changes must use git worktrees.** Never commit directly to `main` or `staging`. This applies to both agent work and main-conversation work.

Claude uses `isolation: "worktree"` to work in isolated copies of the repo. This avoids branch switching on the main working tree and lets multiple agents work in parallel without conflicts.

1. Agent spawned with `isolation: "worktree"` — gets a temporary worktree with its own branch
2. Agent makes changes and commits in the worktree
3. Kyle reviews the worktree diff, merges into `staging`, and pushes
4. CI runs lint, unit tests, security scans, E2E tests on `staging`
5. If all pass, Kyle merges `staging` into `main`
6. CI deploys to production, runs smoke tests against live URLs

**For main-conversation work (no subagent):** Use the `EnterWorktree` tool to create an isolated worktree before making changes. Commit there, then use `ExitWorktree` when done.

**Exception:** Hotfixes for CI/production breakage can go straight to `main`.

**When work is complete:** Claude must list all worktree branches created during the session, with a summary of what each contains. Format:
```
Worktree branches created:
- worktree-agent-XXXX — <summary of changes>
- worktree-agent-YYYY — <summary of changes>
```

**Kyle's worktree workflow (step by step):**

```bash
# ── Step 1: Review what the agent built ──
# List all worktrees to see what exists:
git worktree list

# See what changed on the worktree branch:
git log --oneline main..<worktree-branch>
git diff --stat main..<worktree-branch>

# ── Step 2: Push to staging to trigger CI ──
git checkout staging && git pull origin staging
git merge <worktree-branch>
git push origin staging
# CI runs: lint, unit tests, security scans, E2E tests

# ── Step 3: If CI passes, promote to main ──
git checkout main && git pull origin main
git merge staging
git push origin main
# CI runs: deploy + smoke tests

# ── Step 4: Clean up the worktree ──
# Remove the worktree directory (frees disk space):
git worktree remove .claude/worktrees/<worktree-dir>
# Delete the local branch:
git branch -d <worktree-branch>
# Delete the remote branch (if you pushed it directly):
git push origin --delete <worktree-branch>

# ── Bulk cleanup: remove all stale worktrees ──
git worktree list        # see what exists
git worktree prune       # remove entries for deleted directories
```

**How worktrees work (quick reference):**
- Each worktree is a separate checkout of the repo in `.claude/worktrees/`
- It has its own branch (e.g., `worktree-agent-a516b636`)
- Agents commit to that branch — main stays untouched
- You merge the branch into staging/main when ready
- After merging, clean up the worktree directory + branch to free disk space
- `git worktree list` shows all active worktrees at any time

## Pre-commit Requirements

Before every commit, run the relevant preflight checks and fix any failures. Only escalate to Kyle if you can't resolve the issue.

- **Python changes:** `make preflight-python` and `make preflight-security`
- **Frontend changes:** `make preflight-frontend` and `make preflight-e2e`
- **Java changes:** `make preflight-java` (checkstyle + unit tests, runs locally)
- **Java integration tests:** `make preflight-java-integration` (runs over SSH on Windows PC, on-demand)
- **Full sweep:** `make preflight` (runs Python + frontend + security + Java locally)

If a check fails, fix it before committing. If you can't fix it, explain the failure to Kyle before suggesting a push.

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
