# AGENTS.md

## Project Intent

Portfolio project for a Golang Engineer job applications.  

## Search Scope

Respect `.ignore` for repo searches. Do not scan or summarize ignored paths
unless the user explicitly asks for those files or the task cannot be completed
without them. In normal work, avoid `frontend/node_modules/`, generated build
output, caches, Jupyter checkpoints, `docs/superpowers/`, and `docs/adr/`.

## Quality Bar

This portfolio must demonstrate production-grade engineering, not just working demos. Every component should be something a hiring manager would feel safe dropping into a real production system. Err on the side of overly polished — if a shortcut wouldn't pass code review at a serious company, don't take it. The constraint is cost (no paid cloud services), not effort. Where a production system would use a managed service (RDS, Cloud SQL, S3 backups), implement the self-hosted equivalent with the same operational rigor: automated backups, recovery procedures, health checks, and alerting that doesn't cry wolf.

## Tech Stack

- **Python:** FastAPI microservices (ingestion, chat, debug), LangChain text splitters, Qdrant vector DB
- **Java:** Spring Boot microservices (task, activity, notification, gateway), PostgreSQL, MongoDB, Redis, RabbitMQ, GraphQL
- **Go:** Auth, product, ecommerce, AI agent, and analytics services, gRPC + REST, PostgreSQL, Redis, RabbitMQ, Kafka, protobuf/buf, shared `go/pkg/` module (see `go/AGENTS.md`)
- **AI/ML:** Ollama (Qwen 2.5 14B for chat/debug, nomic-embed-text for embeddings)
- **Frontend:** Next.js + TypeScript + shadcn/ui, Apollo Client (GraphQL)
- **Testing:** pytest, JUnit, Go test/benchmarks, Playwright (E2E)
- **Infra:** Docker Compose, Minikube (K8s), NGINX Ingress, Cloudflare Tunnel, GitHub Actions

## Infrastructure

- **Mac (dev machine):** Code editing, frontend dev server, no GPU. Docker runtime is Colima (start with `colima start` before `docker compose`).
- **Debian 13 (kyle@100.82.52.82 via Tailscale):** Ollama (RTX 3090), Minikube (all backend services). Do not use Debian as a general-purpose CI substitute; it hosts runtime services only.
- **SSH:** `ssh debian` (configured in `~/.ssh/config`, key-based auth)
- **Minikube:** All backend services run in Kubernetes on the Debian server
  - `ai-services` namespace: Python AI services + Qdrant
  - `java-tasks` namespace: Java microservices + databases
  - `go-ecommerce` namespace: Go auth + ecommerce services
  - `monitoring` namespace: Prometheus, Grafana, Loki, Promtail, Jaeger, kube-state-metrics, node-exporter
  - `ai-services-qa` namespace: QA copies of Python AI services (shared Qdrant with prod)
  - `java-tasks-qa` namespace: QA copies of Java services (shared infra with prod)
  - `go-ecommerce-qa` namespace: QA copies of Go services (shared infra with prod)
  - NGINX Ingress Controller routes all traffic by path
  - `minikube tunnel` runs as systemd service (auto-starts on boot)
- **Ollama:** Runs natively on Debian (GPU access), reached from K8s via ExternalName service (`host.minikube.internal`)
- **Local dev:** Docker Compose for both stacks (no Minikube needed for development)
  - SSH tunnel forwards `localhost:8000` to Debian nginx gateway
  ```bash
  ssh -f -N -L 8000:localhost:8000 debian
  ```
- **Frontend:** `npm run dev` in `frontend/`, points to `localhost:8000` via tunnel
- **Production:** Frontend on Vercel (`https://kylebradshaw.dev`), backend via Cloudflare Tunnel:
  - `https://api.kylebradshaw.dev` → Debian Minikube Ingress (192.168.49.2:80)
  - Ingress routes by path: `/ingestion/*`, `/chat/*`, `/debug/*` → Python services; `/graphql`, `/api/auth/*` → Java services; `/go-api/*`, `/go-auth/*`, `/go-products/*` → Go services; `/grafana/*` → monitoring
  - Cloudflared runs as systemd service (auto-starts on boot)
  - `minikube tunnel` runs as systemd service (auto-starts on boot)

## Execution Locality

The Debian server is runtime infrastructure, not a build or test worker.

Agents must not run tests, linters, compilers, package managers, or ad hoc
build verification on Debian. Debian should only be used for runtime/deployment
operations that actually belong there: Minikube/Kubernetes diagnostics, image
pulls during deploys, Ollama runtime checks, observability queries, and
read-only service health inspection.

All verification must run either:

- locally on the Mac dev machine, or
- in the GitHub Actions CI/CD pipeline.

If local verification is blocked by missing tools, disk pressure, platform
limits, or other workstation issues, report the blocker clearly and leave the
remaining verification to CI. Do not move the test run to Debian as a workaround
unless Kyle explicitly authorizes that specific exception.

### Vercel CLI

Vercel CLI is installed, linked to `kabradshaw1s-projects`. Use `vercel env ls production` to list vars, `vercel env add` to add, and `vercel redeploy` to apply.

**Critical rule:** If frontend code adds a new `NEXT_PUBLIC_*` env var with a `localhost` fallback, **Vercel will silently bake the localhost fallback into the production bundle** unless the env var is added in Vercel and a redeploy is triggered. Always add the var to Vercel before merging.

### Secrets and configuration

- **Application Secrets are committed `SealedSecret` resources** at `k8s/secrets/<namespace>/<name>.sealed.yml`. The Sealed Secrets controller in `kube-system` decrypts them on apply. The committed sealed file is the single source of truth — don't `kubectl edit` Secrets, don't `kubectl create secret generic` in CI for app Secrets, don't ship `*.template.yml` files. Use `scripts/seal-from-cluster.sh` to re-seal from cluster state.
- **Credentials never go in ConfigMap data.** DSN strings split into ConfigMap (host/port/db/options) + Secret (user/password); the application assembles the connection string at startup. The DSN-split rollout is in flight (Phase 4 of the migration plan); don't introduce new `user:pass` strings inside ConfigMap values.
- **Shared-infra services must exist in their prod namespace before QA can ExternalName-route to them.** The QA deploy job doesn't apply prod-namespace manifests; QA pointing at a not-yet-deployed prod Service produces the same DNS-resolution failure that auth-service hit when pgbouncer landed in QA before prod.
- The full ruleset is in [`docs/adr/security/secrets-and-config-practices.md`](docs/adr/security/secrets-and-config-practices.md); the migration decision is in [`docs/adr/security/secrets-management.md`](docs/adr/security/secrets-management.md).

### Migrations

- **Go services:** `golang-migrate`. Files in `go/<service>/migrations/` as `NNN_name.up.sql` / `NNN_name.down.sql` pairs. K8s Jobs run `migrate up` on every deploy.
- **To add a schema change:** create the migration pair, commit. The next deploy runs it automatically.
- **Java services:** schema owned by Spring/JPA at startup. No separate migration step.
- **Python AI services:** no relational schema (Qdrant is schemaless).
- **`sslmode=disable` is required on Go `DATABASE_URL`s.** `golang-migrate`'s `pq` driver defaults to `sslmode=require` and will fail without it.
- **Go migration Jobs bypass PgBouncer.** Each Go service ConfigMap defines two keys: `DATABASE_URL` (routes through `pgbouncer.java-tasks.svc.cluster.local:6432`, transaction-pooled) and `DATABASE_URL_DIRECT` (direct `postgres.java-tasks.svc.cluster.local:5432`, session-level). Migration Jobs reference `DATABASE_URL_DIRECT`; app Deployments read `DATABASE_URL`. Reason: `golang-migrate` uses session-level features (advisory locks, transaction wrapping) that PgBouncer's transaction-pool mode doesn't preserve. When adding a new Go service, define both keys in its ConfigMap and point its migrate Job at `DATABASE_URL_DIRECT`.

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
├── product-service/        # Product catalog CRUD, REST :8095 + gRPC :9095, productdb
├── cart-service/           # Cart CRUD + saga reserve/release/clear, gRPC :9096, cartdb
├── order-service/          # Order CRUD + saga orchestrator, REST :8092, ecommercedb
├── payment-service/        # Stripe checkout + webhooks + outbox, REST :8098 + gRPC :9098, paymentdb
├── ai-service/             # Agent loop over Ollama tool-calling, 12 tools wrapping ecommerce
├── analytics-service/      # Kafka consumer — streaming analytics (orders, cart, views)
├── proto/                  # Protobuf definitions (product/v1/product.proto)
├── buf.yaml                # buf v2 config for proto linting and generation
├── k8s/                    # Go-specific K8s manifests
frontend/                   # Next.js + shadcn/ui
├── src/app/                # Pages: ai/ (rag, debug), java/ (tasks), go/ (ecommerce)
├── src/components/         # Shared UI + domain components (java/, go/)
├── src/lib/                # API clients, auth utils, Apollo GraphQL client
├── e2e/                    # Playwright E2E tests (mocked + production smoke)
nginx/                      # Reverse proxy — path-based routing to all backend stacks
k8s/                        # Kubernetes manifests — production deployment (Minikube)
├── ai-services/            # Python AI services + Qdrant
├── monitoring/             # Full observability stack (see Monitoring section below)
├── deploy.sh               # Unified deploy script for all namespaces
monitoring/                 # Local Docker Compose Prometheus + Grafana config files
docs/adr/                   # Architecture Decision Records
├── document-qa/            # 7 notebooks (Python/FastAPI, RAG pipeline)
├── document-debugger/      # 3 notebooks (code-aware chunking, agent loop, tools)
├── java-task-management/   # 7 markdown lessons (Spring Boot, JPA, GraphQL, etc.)
├── go-ai-service/          # 1 markdown ADR (agent harness in Go, tool registry, evals)
├── *.md                    # Standalone ADRs (CI/CD, deployment, K8s migration, analytics, auth, RAG re-evaluation, etc.)
.github/workflows/          # CI/CD: ci.yml (unified), aws-deploy.yml (manual EKS)
docker-compose.yml          # Python services + nginx + Qdrant
```

## AI Platform Architecture

The Go ai-service (`go/ai-service/`) is the MCP gateway for all AI functionality. It fronts 9 ecommerce tools and 3 RAG tools through a unified agent loop.

- **Tool registry:** 12 built-in tools in `go/ai-service/internal/tools/`, registered in `main.go`. Interface: Name/Description/Schema/Call. Cached via Redis wrapper (`tools/cached.go`).
- **Agent loop:** ReAct pattern in `go/ai-service/internal/agent/agent.go`. 8 steps max, 90s timeout. Streams SSE events (tool_call, tool_result, tool_error, final, error) from `internal/http/chat.go`.
- **RAG bridge:** Go calls Python chat service at `/search` and `/chat`, ingestion service at `/collections`. Client in `go/ai-service/internal/tools/clients/rag.go`. 30s timeout, circuit breaker, OTel trace propagation.
- **Python services:** ingestion (PDF→chunk→embed→Qdrant), chat (embed→search→RAG prompt→stream), debug (code indexing + agent loop), eval (RAGAS metrics). Shared LLM factory in `services/shared/llm/`.
- **Key env vars:** `RAG_CHAT_URL`, `RAG_INGESTION_URL`, `OLLAMA_URL`, `REDIS_URL`, `ECOMMERCE_URL`.
- **Frontend integration:** POST /chat with SSE streaming. Frontend client in `frontend/src/lib/ai-service.ts` parses event types.
- **Roadmap:** tracked in GitHub issues #75-#85 (AI platform phases, eval service, RAG improvements).

## Codex Skills

Use the installed project-specific Codex skills when their trigger conditions match:

- `debug-observability`: runtime errors, alerts, circuit breakers, saga issues, gRPC failures, incident verification, or service misbehavior in QA/prod.
- `ops-as-code`: before any mutating action against a shared environment, including `kubectl apply/exec/rollout/scale/delete`, database mutations, secret edits, queue purges, or one-off prod fixes.
- `scaffold-go-service`: when creating or extracting a decomposed Go microservice.

## gRPC & Proto Toolchain

Decomposed Go services use gRPC for inter-service communication and REST for frontend traffic. Each service runs a dual server (REST + gRPC) from a single binary.

- **Proto toolchain:** `buf` v2 at the `go/` level. `go/buf.yaml` (lint config), `go/buf.gen.yaml` (code generation). Proto files at `go/proto/<service>/v1/<service>.proto`.
- **Generated code:** lives at `go/<service>/pb/<service>/v1/` (NOT `internal/pb/` — Go's `internal` package visibility blocks cross-module imports).
- **gRPC features:** reflection (`grpc.reflection`), health checking (`grpc_health_v1`), OTel interceptors (`otelgrpc.NewServerHandler()`).
- **Inter-service calls:** `grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))` — plaintext within the cluster.
- **mTLS (cert-manager):** All gRPC services use mTLS in both QA and prod. cert-manager v1.17.2 is installed by CI and `deploy.sh`. Certificate resources live in `k8s/cert-manager/` (prod) and `k8s/cert-manager/qa-certificates.yml` (QA). Each service gets a per-service secret (e.g., `payment-grpc-tls`) mounted at `/etc/tls/`. Services check `TLS_CERT_DIR` env var at startup — if set, servers call `tlsconfig.ServerTLS(certDir)` and clients call `tlsconfig.ClientTLS(certDir)` from `go/pkg/tlsconfig/`. If unset, both fall back to `insecure.NewCredentials()`.
- **mTLS debugging:** If a gRPC call fails with `"transport: authentication handshake failed"`: (1) check cert-manager is running: `kubectl get pods -n cert-manager`, (2) check certificates are Ready: `kubectl get certificates -n <namespace>`, (3) check the service image is up to date with mTLS server code — grep startup logs for `"mTLS enabled"`. A stale image that predates the mTLS fix will serve insecure gRPC while the client connects with TLS.
- **CI:** `buf lint` runs on proto changes. `buf generate` produces `*.pb.go` and `*_grpc.pb.go`.

## Ecommerce Architecture

Decomposed ecommerce: order-service, cart-service, product-service, payment-service with gRPC inter-service communication and a RabbitMQ-based checkout saga. Database-per-service on shared Postgres (`authdb`, `orderdb`, `productdb`, `cartdb`, `paymentdb`). QA uses a separate RabbitMQ `/qa` vhost for queue isolation.

**Adding a new Go service:** Use the `scaffold-go-service` Codex skill for the full 15-item checklist.

## Monitoring & Observability

Three pillars in the `monitoring` namespace: Prometheus (metrics), Loki+Promtail (logs), Jaeger (traces). Dashboards and alerts in `k8s/monitoring/configmaps/`. Details in `docs/adr/observability/01-08`.

**Critical config — do not change:**
- Prometheus datasource uid: `PBFA97CFB590B2093` (referenced by all alert rules)
- Loki datasource uid: `loki`
- Jaeger datasource uid: `jaeger`

**Minikube:** 16Gi memory (cannot increase without `minikube delete` which wipes all cluster state).

**Debugging triage hierarchy** — follow this order:

1. **Pods down / CrashLoopBackOff:** `kubectl get pods` + `kubectl logs` directly. Observability can't help if the monitoring target is dead. Fix the pod first, then assess.
2. **Alerts firing:** Use the `debug-observability` Codex skill — it has a structured alert triage routine that classifies stale vs real alerts and identifies what actually needs attention.
3. **Pods running but errors:** Use the `debug-observability` Codex skill — Loki logs, Jaeger traces, circuit breaker queries. Don't SSH and grep logs manually.
4. **Post-incident verification:** Use the `debug-observability` Codex skill — it has a health verification checklist and stale alert cleanup procedure.

Rule: only skip the observability skill for step 1 (pods down). For everything else, use the skill first.


## Kafka Streaming Analytics

The Go analytics-service (`go/analytics-service/`, port 8094) consumes events from Kafka:

- **Broker:** Apache Kafka 3.7.0 (KRaft mode, no Zookeeper), StatefulSet in `go-ecommerce` namespace
- **Topics:** `ecommerce.orders`, `ecommerce.cart`, `ecommerce.views`
- **Producers:** ecommerce-service (orders, cart events), ai-service (product view events, optional)
- **Consumer:** analytics-service, consumer group `analytics-group`, aggregates into order/cart/trending metrics
- **Library:** `segmentio/kafka-go` v0.4.50
- **Prometheus metrics:** `analytics_events_consumed_total`, `analytics_aggregation_latency_seconds`, `kafka_consumer_lag`, `kafka_consumer_errors_total`
- **Trace propagation:** W3C trace context in Kafka message headers via `go/pkg/tracing/kafka.go`

## Java Service Resource Limits

Java services use `-Xmx512m` heap cap (set in Dockerfiles) with 768Mi container memory limits. Without the heap cap, JVM auto-sizing can cause OOM kills. If adding a new Java service, always include `-Xmx512m` in the `ENTRYPOINT`.

**`make preflight-java` fails on Mac** — requires JDK 21 which is not installed locally. Java compilation and tests run correctly in CI. Do not run Java tests on Debian as a workaround; this is a known limitation of the local dev setup.


## Architecture Decision Records (ADRs)

Design decisions documented in `docs/adr/`, organized by service. Jupyter notebooks for service-level walkthroughs, markdown for standalone decisions. Use `docs/adr/template-adr.md` for new ADRs.

## Branching & Workflow

- `main` — production. Pushes trigger deploy + post-deploy smoke tests.
- `qa` — pre-production QA environment. PRs trigger quality checks. Pushes trigger build + deploy to QA + smoke tests.
- Feature branches (`agent/feat-*`) — created by agents from `main`, short-lived, deleted after merge.
- `staging` — retired. Replaced by `qa`.

**Per-branch rules for Codex:**

- **On a feature branch:** The full autonomous flow is:
  1. **Spec approved** — Kyle reviews and approves the spec. This is the human gate. After writing the spec, update the status line marker so Kyle can see which spec is active:
     ```bash
     echo "spec-name-here" > ~/.codex/current-spec.txt
     ```
     Use the spec filename without the date prefix or `.md` extension (e.g., `restore-e2e-prestaging-design`).
  2. **Plan + execute** — Write the implementation plan and execute it. Don't ask to approve the plan — just do it.
  3. **Push** — Commit and push. Don't ask before pushing.
  4. **Create the PR** to `qa` and notify Kyle.
  
  Don't ask for approval at any point in this flow. The spec review is the gate — everything after that is autonomous. Do NOT watch or monitor CI — Kyle will check CI results himself and report back if there are failures to fix.
- **On `qa`:** commit and push autonomously. Don't ask before pushing. Do NOT watch CI after pushing. For CI fixes Kyle reports: lint errors, formatting, type errors, and config issues are fine to fix autonomously. For anything that changes application behavior (logic, API contracts, data flow), stop and check with Kyle before fixing.
  - **Doc-only changes:** Do NOT push commits that only touch docs (`AGENTS.md`, `docs/`, specs, plans, ADRs). Commit them locally — Kyle views them locally. Push them along with the next code change that has a reason to trigger CI. This avoids unnecessary CI/CD runs.
- **On `main`:** never push autonomously. When Kyle explicitly says to merge/ship to main, handle the full flow: merge `qa` into `main`, push, clean up worktree, delete feature branch (local + remote). Do NOT watch CI.

Codex determines the current branch via `git branch --show-current` and follows the rules for that branch. No special mode or prompt needed.

**Agent worktrees:** Agents create worktrees in `.codex/worktrees/<branch-name>/` for feature work when a separate worktree is needed. Worktrees are cleaned up as part of the "ship to main" flow.

## Pre-commit Requirements

**First-time setup:** New clones must install the pre-commit hook framework once:

```bash
make install-pre-commit
```

This installs both commit-stage hooks (gitleaks, bandit, hadolint, ruff, java-checkstyle, frontend tsc/lint, go-lint covering all 8 services) and pre-push-stage hooks (frontend `next build`). After this, every commit triggers the relevant subset based on what files changed.

Before every commit, run the relevant preflight checks locally and fix any failures. Only escalate to Kyle if you can't resolve the issue. If a local check cannot run because of missing tools, disk pressure, or platform limits, report the blocker and leave that verification to CI. Do not run tests or build verification on Debian unless Kyle explicitly authorizes that specific exception.

- **Python changes:** `make preflight-python` and `make preflight-security`
- **Frontend changes:** `make preflight-frontend` and `make preflight-e2e`
- **Java changes:** `make preflight-java` (checkstyle + unit tests, runs locally)
- **Java integration tests:** run in CI unless Kyle explicitly authorizes a specific local/remote exception.
- **Go changes:** `make preflight-go` (lint + tests)
- **Go migration changes:** `make preflight-go-migrations` (requires Docker via Colima + `golang-migrate`; spins up Postgres, runs all migrations, verifies tables)
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
**Deploy:** GHCR images built in CI → SSH to Debian server → kubectl apply Kustomize overlays
**QA:** `qa-api.kylebradshaw.dev` (backend), `qa.kylebradshaw.dev` (frontend on Vercel)

**Compose-smoke realism:** Job 3 (`compose-smoke`) runs the Python AI stack via `docker-compose.yml` with a mocked Ollama. Any change to Python service configuration (env vars, ports, depends_on, env_file references) must be reflected in BOTH `docker-compose.yml` and the corresponding k8s manifests under `k8s/ai-services/`, or compose-smoke will drift from prod and stop catching real regressions.

**Tailscale authkey:** Expires every 90 days (free plan). Regenerate at Tailscale admin → Keys and update `TAILSCALE_AUTHKEY` in GitHub repo secrets.

## Browser Console Configuration
When I need to configure something in a console, first check to see if you can do it from the command line tool. If not, give me a link to the exact pages I will need to visit, and as much detail as you can about what I will need to do.
