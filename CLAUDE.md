# CLAUDE.md

## Project Intent

Portfolio project for a Gen AI Engineer job application — demonstrating RAG architecture, agentic AI, prompt engineering, and Python API development through two AI tools: a Document Q&A Assistant and a Debug Assistant.

## Tech Stack

- **Python:** FastAPI microservices (ingestion, chat, debug), LangChain text splitters, Qdrant vector DB
- **Java:** Spring Boot microservices (task, activity, notification, gateway), PostgreSQL, MongoDB, Redis, RabbitMQ, GraphQL
- **Go:** Auth, product, ecommerce, AI agent, and analytics services, gRPC + REST, PostgreSQL, Redis, RabbitMQ, Kafka, protobuf/buf, shared `go/pkg/` module (see `go/CLAUDE.md`)
- **AI/ML:** Ollama (Qwen 2.5 14B for chat/debug, nomic-embed-text for embeddings)
- **Frontend:** Next.js + TypeScript + shadcn/ui, Apollo Client (GraphQL)
- **Testing:** pytest, JUnit, Go test/benchmarks, Playwright (E2E)
- **Infra:** Docker Compose, Minikube (K8s), NGINX Ingress, Cloudflare Tunnel, GitHub Actions

## Infrastructure

- **Mac (dev machine):** Code editing, frontend dev server, no GPU. Docker runtime is Colima (start with `colima start` before `docker compose`).
- **Debian 13 (kyle@100.82.52.82 via Tailscale):** Ollama (RTX 3090), Minikube (all backend services)
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
- `NEXT_PUBLIC_AI_SERVICE_URL=https://api.kylebradshaw.dev/ai-api`
- `NEXT_PUBLIC_GO_PRODUCT_URL=https://api.kylebradshaw.dev/go-products`

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
├── product-service/        # Product catalog CRUD, REST :8095 + gRPC :9095, productdb
├── ecommerce-service/      # Cart, orders, returns, Redis, RabbitMQ (products extracted)
├── ai-service/             # Agent loop over Ollama tool-calling, 9 tools wrapping ecommerce
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
- **Roadmap (Q2 2026, issue #75):** Phase 1: architecture doc → Phase 2: unified AI assistant UI (#77) → Phase 3: Loki log aggregation (#78) → Phase 4a-c: RAG eval harness, hybrid search, cross-encoder re-ranking (#79-#81).

## gRPC & Proto Toolchain

Decomposed Go services use gRPC for inter-service communication and REST for frontend traffic. Each service runs a dual server (REST + gRPC) from a single binary.

- **Proto toolchain:** `buf` v2 at the `go/` level. `go/buf.yaml` (lint config), `go/buf.gen.yaml` (code generation). Proto files at `go/proto/<service>/v1/<service>.proto`.
- **Generated code:** lives at `go/<service>/pb/<service>/v1/` (NOT `internal/pb/` — Go's `internal` package visibility blocks cross-module imports).
- **gRPC features:** reflection (`grpc.reflection`), health checking (`grpc_health_v1`), OTel interceptors (`otelgrpc.NewServerHandler()`).
- **Inter-service calls:** `grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))` — plaintext within the cluster.
- **CI:** `buf lint` runs on proto changes. `buf generate` produces `*.pb.go` and `*_grpc.pb.go`.

## Ecommerce Decomposition

Splitting the monolithic `ecommerce-service` into product, cart, and order services with gRPC inter-service communication and a RabbitMQ-based checkout saga. Roadmap spec: `docs/superpowers/specs/2026-04-20-ecommerce-decomposition-grpc-design.md`.

- **Phase 1 (DONE):** Product-service extracted. REST :8095, gRPC :9095, `productdb`. Ecommerce order worker calls product-service via gRPC. Product routes removed from ecommerce-service.
- **Phase 2 (TODO):** Cart-service extraction. Calls product-service via gRPC for price validation. Own `cartdb`.
- **Phase 3 (TODO):** Order-service + saga orchestrator. RabbitMQ saga for checkout (reserve → validate → confirm → clear). DLQ with retry. Compensation flows. Retires ecommerce-service.
- **Database-per-service:** Each service gets its own database on the shared Postgres instance (`productdb`, `cartdb`, `orderdb`). Same logical isolation as enterprise, pragmatic for portfolio infra.
- **GitHub issues for future enhancements:** #96 (auth gRPC), #97 (DLQ replay), #98 (proto contract testing), #99 (async integration tests), #100 (graceful shutdown), #101 (mTLS).

### Adding a New Decomposed Go Service (checklist)

When extracting a service from ecommerce-service (or adding any new Go service), every item below must be addressed or the QA/prod deploy will fail:

1. **Service code:** Create `go/<service>/` with cmd/server, internal/, go.mod (with `replace ../pkg`), Dockerfile
2. **Proto:** Add `go/proto/<service>/v1/<service>.proto`, run `buf generate`. Generated code at `go/<service>/pb/<service>/v1/`
3. **Cross-module imports:** If service A imports service B's proto, add `replace` directive in A's go.mod AND `COPY <service-b>/ /app/<service-b>/` in A's Dockerfile
4. **Seed data:** Use explicit UUIDs shared across all seed files so FKs work during the transition phase (both databases need matching product/cart/order IDs)
5. **Kubernetes manifests:** deployment, service, configmap, migration job, HPA, PDB in `go/k8s/`
6. **QA database:** Create `<dbname>_qa` manually on the Debian server (`kubectl exec -n java-tasks deploy/postgres -- psql -U taskuser -d taskdb -c 'CREATE DATABASE <dbname>_qa OWNER taskuser;'`). The `postgres-initdb` ConfigMap only runs on fresh PVC.
7. **QA Kustomize overlay:** Add ConfigMap patch in `k8s/overlays/qa-go/kustomization.yaml` pointing DATABASE_URL to `*_qa`, CORS to `qa.kylebradshaw.dev`, Redis to DB index `/1`
8. **CI matrices:** Add to `go-lint`, `go-tests`, `build-images`, `security-hadolint` matrices in `.github/workflows/ci.yml`
9. **CI deploy steps:** Add migration job delete+apply+wait to both QA deploy (line ~886) and prod deploy (line ~1003) in ci.yml
10. **deploy.sh:** Add `kubectl wait` for the new deployment in both QA and prod sections
11. **Ingress:** Add path in `go/k8s/ingress.yml` and update `go/k8s/kustomization.yaml`
12. **Frontend:** Add `NEXT_PUBLIC_*` env var to Vercel (both production and preview/qa) BEFORE merging
13. **Smoke tests:** Update `frontend/e2e/smoke-prod/smoke.spec.ts` if endpoints moved
14. **Makefile:** Add to `preflight-go` target (lint + test)
15. **Migration state:** If tables are created manually before the migration job runs, set the `*_schema_migrations` table to the correct version (`INSERT INTO <svc>_schema_migrations (version, dirty) VALUES (<N>, false)`) or the job will fail with "dirty database"

## Monitoring & Observability

Three pillars deployed in the `monitoring` namespace (`k8s/monitoring/`):

- **Metrics (Prometheus):** Scrapes kube-state-metrics, node-exporter, GPU exporter, and all app pods via `prometheus.io/scrape` annotations. 15s scrape interval, 15-day retention, 8GB max storage.
- **Logs (Loki + Promtail):** Loki (single-binary, StatefulSet with 5Gi PVC) stores logs. Promtail (DaemonSet) tails `/var/log/pods/`, parses JSON logs, extracts `level` and `traceID` labels, ships to Loki. Go services emit structured JSON via `slog` + `tracing.NewLogHandler()`. Java services use `logstash-logback-encoder` for JSON output.
- **Traces (Jaeger):** OTLP gRPC collector on port 4317. Go services use OTel SDK with `otelgin` middleware. Trace context propagates across HTTP (W3C traceparent), Kafka headers (`go/pkg/tracing/kafka.go`), and RabbitMQ headers.
- **Correlation:** Loki datasource has derived fields that turn `traceID` in log lines into clickable Jaeger links. The "Observability Overview" dashboard shows metrics → logs → traces drill-down.

**Grafana dashboards** (5 total, embedded in `k8s/monitoring/configmaps/grafana-dashboards.yml`):
- `system-overview.json` — GPU, system, RAG pipeline
- `go-services.json` — RED metrics, Go runtime, ecommerce, cache, AI agent, streaming analytics
- `kubernetes.json` — Pod status, restarts, deployment replicas
- `ai-pipeline.json` — Ollama, RAG, embeddings, vector search
- `observability-overview.json` — Correlation dashboard (metrics/logs/traces)

**Alert rules** (4 groups in `k8s/monitoring/configmaps/grafana-alerting.yml`, all → Telegram):
- Infrastructure: GPU exporter down, AI service not ready, GPU temp, GPU VRAM
- Kubernetes Health: OOM killed, pod restart storm, container memory high, node memory/disk pressure, deployment replicas unavailable
- Application SLOs: Error rate + p95 latency for Go AI (5%/30s), Go ecommerce (2%/2s), Java gateway (2%/3s)
- Streaming Analytics: Kafka consumer lag > 1000

**Datasources** (`k8s/monitoring/configmaps/grafana-datasource.yml`):
- Prometheus: uid `PBFA97CFB590B2093` (referenced by all alert rules — do not change)
- Loki: uid `loki`
- Jaeger: uid `jaeger`

**Minikube:** Allocated 16Gi memory (cannot increase without `minikube delete` which wipes all cluster state). Currently sufficient.

## Debugging with Observability Tools

**Rule: Use Loki/Jaeger before SSH.** When debugging service issues, query the observability stack first. SSH + `kubectl logs` is a last resort for when the monitoring stack is unavailable.

### Loki Log Queries

Access via Grafana (Explore → Loki datasource) or via CLI:

```bash
# Port-forward to Loki
ssh debian 'kubectl port-forward svc/loki 3100:3100 -n monitoring &'

# Query by order ID across all services
curl -sG http://localhost:3100/loki/api/v1/query_range \
  --data-urlencode 'query={namespace=~"go-ecommerce.*"} |= "<orderID>" | json' \
  --data-urlencode 'limit=50'

# Query errors for a specific service
curl -sG http://localhost:3100/loki/api/v1/query_range \
  --data-urlencode 'query={namespace="go-ecommerce-qa",app="go-order-service"} | json | level="ERROR"' \
  --data-urlencode 'limit=20'
```

In Grafana Explore (Loki datasource):
- **By order ID:** `{namespace=~"go-ecommerce.*"} |= "<orderID>" | json`
- **By error level:** `{namespace="go-ecommerce-qa",app="go-order-service"} | json | level="ERROR"`
- **Readable output:** append `| line_format "{{.msg}}"`

### Jaeger Trace Lookup

Every structured log line includes `traceID`. Copy it from Loki, then look it up in Jaeger.

```bash
# Port-forward to Jaeger UI
ssh debian 'kubectl port-forward svc/jaeger 16686:16686 -n monitoring &'
# Open http://localhost:16686/jaeger in browser
```

A checkout trace spans: order-service → cart-service (gRPC) → product-service (gRPC) → payment-service (gRPC) → RabbitMQ publish → saga consumer.

In Grafana, Loki log entries with `traceID` have a clickable "View Trace" link that opens directly in Jaeger.

### Circuit Breaker Diagnosis

```bash
# Check breaker state via Prometheus (0=closed, 1=half-open, 2=open)
# In Grafana: query circuit_breaker_state{name="order-postgres"}

# Check for poison messages in RabbitMQ
ssh debian 'kubectl exec -n java-tasks deploy/rabbitmq -- rabbitmqctl list_queues name messages'

# If breaker is open: purge the offending queue + restart service
ssh debian 'kubectl scale deployment/go-order-service -n go-ecommerce-qa --replicas=0'
ssh debian 'kubectl exec -n java-tasks deploy/rabbitmq -- rabbitmqctl purge_queue saga.order.events'
ssh debian 'kubectl scale deployment/go-order-service -n go-ecommerce-qa --replicas=2'
```

### Common Debugging Patterns

| Symptom | Likely Cause | Fix |
|---------|-------------|-----|
| Saga stuck in COMPENSATING | Stale RabbitMQ messages looping | Purge queue, mark orders as FAILED in DB, restart service |
| 503 on order endpoints | Circuit breaker open from poison messages | Purge queue, restart service to reset breaker |
| Traces not appearing in Jaeger | Jaeger pod down or OTEL endpoint misconfigured | Check `kubectl get pod -n monitoring -l app=jaeger` |
| Loki returns empty results | Promtail not scraping | Check `kubectl port-forward daemonset/promtail 3101:3101 -n monitoring && curl localhost:3101/ready` |

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

**`make preflight-java` fails on Mac** — requires JDK 21 which is not installed locally. Java compilation and tests run correctly in CI (Debian server has JDK 21). This is a known limitation of the local dev setup.

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
- `docs/adr/observability/` — 6 markdown guides (three pillars, Prometheus, Loki, Jaeger, SLOs, correlation)
- Standalone markdown ADRs for CI/CD, deployment architecture, K8s migration, analytics, auth, etc.

## Branching & Workflow

- `main` — production. Pushes trigger deploy + post-deploy smoke tests.
- `qa` — pre-production QA environment. PRs trigger quality checks. Pushes trigger build + deploy to QA + smoke tests.
- Feature branches (`agent/feat-*`) — created by agents from `main`, short-lived, deleted after merge.
- `staging` — retired. Replaced by `qa`.

**Per-branch rules for Claude Code:**

- **On a feature branch:** The full autonomous flow is:
  1. **Spec approved** — Kyle reviews and approves the spec. This is the human gate. After writing the spec, update the status line marker so Kyle can see which spec is active:
     ```bash
     echo "spec-name-here" > ~/.claude/current-spec.txt
     ```
     Use the spec filename without the date prefix or `.md` extension (e.g., `restore-e2e-prestaging-design`).
  2. **Plan + execute** — Write the implementation plan and execute it. Don't ask to approve the plan — just do it.
  3. **Push** — Commit and push. Don't ask before pushing.
  4. **Create the PR** to `qa` and notify Kyle.
  
  Don't ask for approval at any point in this flow. The spec review is the gate — everything after that is autonomous. Do NOT watch or monitor CI — Kyle will check CI results himself and report back if there are failures to fix.
- **On `qa`:** commit and push autonomously. Don't ask before pushing. Do NOT watch CI after pushing. For CI fixes Kyle reports: lint errors, formatting, type errors, and config issues are fine to fix autonomously. For anything that changes application behavior (logic, API contracts, data flow), stop and check with Kyle before fixing.
- **On `main`:** never push autonomously. When Kyle explicitly says to merge/ship to main, handle the full flow: merge `qa` into `main`, push, clean up worktree, delete feature branch (local + remote). Do NOT watch CI.

Claude Code determines the current branch via `git branch --show-current` and follows the rules for that branch. No special mode or prompt needed.

**Agent worktrees:** Agents create worktrees in `.claude/worktrees/<branch-name>/` for feature work. Worktrees are cleaned up as part of the "ship to main" flow.

## Pre-commit Requirements

Before every commit, run the relevant preflight checks and fix any failures. Only escalate to Kyle if you can't resolve the issue.

- **Python changes:** `make preflight-python` and `make preflight-security`
- **Frontend changes:** `make preflight-frontend` and `make preflight-e2e`
- **Java changes:** `make preflight-java` (checkstyle + unit tests, runs locally)
- **Java integration tests:** `make preflight-java-integration` (runs over SSH on Debian server, on-demand)
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
When I need to configure something in a console, first check to see if you can do it from the command line tool.  If not, then please give me a link to the exact pages I will need to visit, and as much details as you can about what I will need to do.  Consoles I have visited in c