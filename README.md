# Kyle Bradshaw — Portfolio

I'm a developer who primarily focuses on **backend and AI integration**. This repo is a polyglot, production-style portfolio that shows how I design services, wire them together, and integrate LLMs into real applications.

**Live demo:** [kylebradshaw.dev](https://kylebradshaw.dev) · **API:** `api.kylebradshaw.dev`

---

## What this project demonstrates

- **Backend engineering across three languages** — Go, Java (Spring Boot), and Python (FastAPI), each chosen where it fits best
- **AI integration** — a full RAG pipeline and an agentic debugging assistant, both backed by locally-hosted LLMs
- **Production infrastructure** — Kubernetes, NGINX Ingress, Cloudflare Tunnel, Prometheus/Grafana, and a full CI/CD pipeline deploying to a home-lab cluster
- **Linux server administration** — hand-installed Debian 13 on the home-lab box, hardened the OS (firewall, SSH lockdown, sudo policy, auditd, kernel sysctls), wrote systemd units for service auto-start, and recovered cleanly from a power outage and a botched SSH config
- **A working full-stack surface** — Next.js + TypeScript frontend on Vercel, talking to REST, GraphQL, and streaming AI endpoints

---

## Backend services

### Go — where I'm strongest
- **`auth-service`** — JWT auth (register / login / refresh), PostgreSQL
- **`ecommerce-service`** — products, cart, orders, Redis caching, RabbitMQ worker pool
- **`ai-service`** — LLM-powered shopping assistant, tool-calling agent loop over Ollama (Qwen 2.5 14B), 9 tools wrapping the ecommerce API

### Java (Spring Boot)
- **`task-service`** — task & project CRUD, PostgreSQL + JPA, Flyway migrations
- **`activity-service`** — activity feed, MongoDB, Redis caching, analytics aggregation with HikariCP tuning
- **`notification-service`** — event-driven notifications via RabbitMQ
- **`gateway-service`** — GraphQL gateway federating the three above

### Python (FastAPI) — the AI layer
- **`ingestion`** — PDF upload, parse, chunk (LangChain splitters), embed, store, delete
- **`chat`** — question embedding, vector search, RAG prompt assembly, streaming responses
- **`debug`** — codebase indexing, agent loop, tool execution, streamed reasoning

---

## AI integration

Two LLM-powered tools are featured in the frontend:

1. **Document Q&A Assistant** — Upload a PDF, ask questions. Full RAG pipeline: parse → chunk → embed (`nomic-embed-text`) → store in Qdrant → retrieve → stream answers from **Qwen 2.5 14B** via Ollama.
2. **Debug Assistant** — An agentic loop over an indexed codebase. The model plans, calls tools (search / read / inspect), and streams its reasoning while it works through a bug.

Both services are documented end-to-end as Jupyter ADR notebooks in `docs/adr/document-qa/` and `docs/adr/document-debugger/` — the best place to see how I think about prompt design, chunking strategies, and agent tool design.

---

## Frontend

Next.js + TypeScript + shadcn/ui + Apollo Client, deployed on Vercel. Sections for the AI tools (`ai/rag`, `ai/debug`), Java task management (`java/tasks`), and the Go ecommerce store (`go/`). Playwright covers both mocked E2E and post-deploy production smoke tests.

---

## QA Environment

Every change goes through a QA branch before reaching production. Feature branches merge into `qa`, which auto-deploys to a parallel set of Kubernetes namespaces (`ai-services-qa`, `java-tasks-qa`, `go-ecommerce-qa`) and a separate Vercel frontend build. Once visually inspected, `qa` merges into `main` for production deploy.

- **QA frontend:** [qa.kylebradshaw.dev](https://qa.kylebradshaw.dev)
- **QA API:** `qa-api.kylebradshaw.dev`
- **Production:** [kylebradshaw.dev](https://kylebradshaw.dev) / `api.kylebradshaw.dev`

The `/cicd` page on the live site shows what's currently staged on QA vs production.

---

## Infrastructure & DevOps

- **Kubernetes (Minikube)** on a self-installed **Debian 13 server** with an RTX 3090 running Ollama natively for GPU inference
- **Namespaces:** `ai-services`, `java-tasks`, `go-ecommerce`, `monitoring`
- **NGINX Ingress** with path-based routing across all stacks
- **Cloudflare Tunnel** exposes the home-lab cluster publicly as `api.kylebradshaw.dev`
- **Docker Compose** for local development
- **Prometheus + Grafana** for metrics and dashboards
- **GitHub Actions CI/CD** — ruff, pytest, tsc, Next.js build, checkstyle, golangci-lint, Bandit, pip-audit, npm audit, gitleaks, Hadolint, Playwright E2E, and auto-deploy to the cluster via SSH
- **Hardened Linux host** — UFW default-deny firewall, SSH locked to Tailscale only, narrow passwordless sudo allowlist, auditd + persistent journald, sysctl kernel hardening, lynis baseline score 77. See [`docs/security/linux-server-hardening.md`](docs/security/linux-server-hardening.md).

---

## Security

For a full security assessment of the controls implemented in this project — shift-left CI gates, Kubernetes policy-as-code, auth architecture, supply chain posture, and explicitly accepted risks — see **[`docs/security/security-assessment.md`](docs/security/security-assessment.md)**. For host-level hardening of the Debian 13 server (firewall, SSH lockdown, sudo policy, auditd, kernel sysctls, patch hygiene, lynis baseline), see **[`docs/security/linux-server-hardening.md`](docs/security/linux-server-hardening.md)**. Each finding cites the exact file(s) that implement it so the claims can be independently verified.

---

## Tech stack

| Layer | Tools |
|---|---|
| **Backend** | Go, Java (Spring Boot, Gradle, JPA, GraphQL), Python (FastAPI) |
| **AI/ML** | Ollama, Qwen 2.5 14B, nomic-embed-text, Qdrant, LangChain |
| **Data** | PostgreSQL, MongoDB, Redis, Qdrant, RabbitMQ |
| **Frontend** | Next.js, TypeScript, shadcn/ui, Apollo Client, Playwright |
| **Infra** | Docker, Kubernetes, NGINX Ingress, Cloudflare Tunnel, Prometheus, Grafana |
| **CI/CD** | GitHub Actions, ruff, pytest, golangci-lint, checkstyle, Bandit, gitleaks |

---

## Repository layout

```
services/          Python AI microservices (ingestion, chat, debug)
java/              Spring Boot microservices (task, activity, notification, gateway)
go/                Go microservices (auth, ecommerce, ai-service)
frontend/          Next.js app
k8s/               Kubernetes manifests for all namespaces
nginx/             Reverse-proxy config (local dev)
monitoring/        Prometheus + Grafana
docs/adr/          Architecture Decision Records (notebooks + markdown)
.github/workflows/ CI/CD pipelines
```

---

## Architecture Decision Records

Every major design choice is written up in `docs/adr/`:

- [`docs/architecture.md`](docs/architecture.md) — System architecture overview and AI platform deep dive
- **`document-qa/`** — 7 notebooks walking through the RAG pipeline build
- **`document-debugger/`** — 3 notebooks on code-aware chunking, the agent loop, and tool design
- **`java-task-management/`** — 7 lessons on the Spring Boot stack (JPA, GraphQL, caching, analytics)
- **`go-stress-testing.md`** — k6 load testing, bottleneck analysis, and data-driven performance fixes (stock race condition, HPA autoscaling, connection pool tuning)
- Standalone markdown ADRs covering CI/CD, deployment topology, K8s migration, auth, and more

---

## For hiring managers

If you're short on time:

1. **`go/`** — my strongest language; start here for backend style
2. **`docs/adr/go-stress-testing.md`** — k6 load testing that found real bugs (stock overselling), with before/after metrics
3. **`services/`** — FastAPI + RAG + agent implementation
4. **`docs/adr/document-qa/`** and **`docs/adr/document-debugger/`** — how and why the AI services were built
5. **`docs/security/`** — security assessments for the application stack and the hardened Debian 13 host (lynis 77)
6. **`.github/workflows/`** — CI/CD, security scanning, and deployment
7. **`k8s/`** — production Kubernetes topology

Thanks for taking a look. — Kyle
