# Go Ecommerce Stress Testing & Scalability Analysis

**Date:** 2026-04-10
**Status:** Approved
**Scope:** Stress testing the Go ecommerce, auth, and AI agent services using k6, with Prometheus/Grafana integration, targeted performance fixes, and an ADR documenting findings.

---

## Goals

1. Find real performance bottlenecks in the Go services under load
2. Fix what the data shows — config tuning, code fixes, K8s scaling manifests
3. Document the process as a portfolio piece demonstrating scalability thinking
4. Add a Grafana dashboard correlating load test metrics with service internals

## Tool Choice: k6

k6 (Grafana Labs) is the load testing tool. Rationale:

- Lightweight Go binary, ~50-100MB RAM for hundreds of virtual users
- JavaScript test scripts — easy to read and version control
- Native Prometheus remote-write pushes k6 metrics into existing Grafana
- Supports multi-step scenarios (login → cart → checkout)
- Industry-standard tool recognized by interviewers
- Built-in pass/fail thresholds for automated validation

Runs on the Mac, hits services through the SSH tunnel to match real-world access patterns.

## Test Structure

```
loadtest/
├── scripts/
│   ├── phase1-ecommerce.js    # Products, cart, checkout flows
│   ├── phase2-auth.js         # Register, login, refresh under load
│   └── phase3-ai-agent.js     # AI agent turns with tool calling
├── lib/
│   └── helpers.js             # Shared auth helpers, random data generators
├── dashboards/
│   └── k6-load-test.json      # Grafana dashboard for k6 metrics
└── README.md                  # How to run, interpret results
```

## Phase 1: Ecommerce Service

### Scenario A — Browse Products
- Ramp 1→50 VUs over 2 minutes, hold for 2 minutes, ramp down over 1 minute
- Hits: `GET /go-api/products`, `GET /go-api/products/{id}`, `GET /go-api/products/categories`
- Measures: response time percentiles, cache hit ratio, DB connection pool pressure

### Scenario B — Cart Operations
- 20 VUs sustained for 3 minutes
- Flow per VU: login → add 2-3 random items to cart → update quantity → view cart → clear cart
- Measures: response time by endpoint, error rate, DB connection usage

### Scenario C — Checkout Flow
- 30 VUs sustained for 3 minutes
- Flow per VU: login → add items to cart → checkout → verify order status
- Measures: order throughput, RabbitMQ queue depth, worker processing lag

### Scenario D — Stock Contention
- 50 VUs all attempt to checkout the same item with stock=10
- Measures: final stock count vs. successful orders (detects overselling), error handling behavior
- Expected result: race condition may allow overselling — this is a known code issue

## Phase 2: Auth Service

### Scenario A — Registration Burst
- 50 concurrent registration requests
- Measures: bcrypt CPU saturation, response time under load, error rate

### Scenario B — Login Sustained Load
- Constant 20 req/s for 3 minutes
- Measures: p95/p99 latency, CPU utilization, connection pool behavior

### Scenario C — Token Refresh
- 30 VUs refreshing tokens concurrently
- Measures: JWT validation throughput, response time stability

## Phase 3: AI Agent Service

### Scenario A — Simple Queries (Cached Tools)
- 10 VUs sending product search queries for 3 minutes
- Measures: agent turn duration, tool cache hit rate, Ollama token throughput

### Scenario B — Multi-Step Flows
- 5 VUs sending "search then add to cart" queries requiring auth + multiple tool calls
- Measures: end-to-end agent turn time, tool call latency breakdown

### Scenario C — Rate Limiter Behavior
- 5 VUs exceeding 20 req/min threshold
- Measures: 429 response rate, rate limiter accuracy, recovery behavior after window reset

## Load Profiles

All profiles are modest — single-replica Minikube on a PC, not cloud-scale. Goal is finding breaking points, not simulating production traffic.

| Scenario | Peak VUs | Duration | Target |
|----------|----------|----------|--------|
| Product browse | 50 | 5 min | Find cache/DB ceiling |
| Cart ops | 20 | 3 min | Connection pool pressure |
| Checkout | 30 | 3 min | Order throughput limit |
| Stock contention | 50 | 1 min | Race condition proof |
| Auth registration | 50 | 1 min | bcrypt CPU ceiling |
| Auth login | 20 req/s | 3 min | Sustained auth throughput |
| Token refresh | 30 | 2 min | JWT validation throughput |
| AI simple query | 10 | 3 min | Ollama throughput |
| AI multi-step | 5 | 3 min | Agent loop bottleneck |
| AI rate limit | 5 | 2 min | Rate limiter verification |

## Pass/Fail Thresholds

- Product endpoints: p95 < 500ms
- Checkout: p95 < 1s
- Auth login: p95 < 2s (bcrypt is intentionally slow)
- AI agent turns: p95 < 15s (LLM-bound)
- Error rate: < 1% for all non-contention scenarios
- Stock contention: zero overselling (successful orders <= available stock)

## Prometheus/Grafana Integration

k6 pushes metrics to Prometheus via remote-write through the SSH tunnel. A dedicated Grafana dashboard combines:

**k6 metrics (load generator side):**
- Virtual users over time
- Request rate by scenario
- Response time percentiles (p50/p95/p99)
- Error rate and HTTP status distribution

**Service metrics (existing instrumentation):**
- `http_requests_total` / `http_request_duration_seconds` by endpoint
- `ecommerce_cache_operations_total` (hit/miss ratio under load)
- `ecommerce_orders_placed_total` and `ecommerce_order_value_dollars`
- `ecommerce_rabbitmq_publish_total` (queue publish success/failure)
- `orders_total` and `rabbitmq_messages_processed_total` (worker side)
- `ai_agent_turn_duration_seconds` and `ai_tool_duration_seconds`
- `ollama_request_duration_seconds` and `ollama_tokens_total`

**Correlation panels:**
- k6 request rate vs. service p99 latency (shows where latency degrades)
- VU count vs. cache hit ratio (shows cache effectiveness under load)
- Order creation rate vs. RabbitMQ worker processing rate (shows queue backup)

## Targeted Improvements (Data-Driven)

Only implemented if load test data confirms the issue. Before/after metrics captured for each fix.

### Quick Wins (Config Tuning)
- **pgxpool configuration** — explicit `MaxConns`, `MinConns`, `MaxConnIdleTime` based on observed connection pressure
- **RabbitMQ worker concurrency** — increase from 3 based on queue depth data
- **HTTP server timeouts** — add read/write/idle timeouts to Gin server (currently none)

### Medium Effort (Code Fixes)
- **Stock decrement race condition** — replace bare `UPDATE ... WHERE stock >= qty` with `SELECT ... FOR UPDATE` inside a transaction
- **Query-level context timeouts** — add explicit timeouts in repository methods to prevent slow queries from holding connections

### K8s Scaling
- **HPA manifests** — add HorizontalPodAutoscaler for ecommerce-service and auth-service with CPU-based thresholds
- **Resource tuning** — adjust requests/limits based on observed memory and CPU under load

### Observability Gaps
- **Go runtime metrics** — expose goroutine count, GC pause duration, heap allocation via Prometheus
- **pgxpool metrics** — wire up pgx's built-in pool stats (in-use, idle, waiting connections)
- **Load test Grafana dashboard** — the k6 + service correlation dashboard described above

## Documentation Deliverable

An ADR (markdown or Jupyter notebook in `docs/adr/`) documenting:
- What we tested and why
- What broke and at what load level
- What we fixed and how
- Before/after Grafana screenshots
- Remaining known risks and recommendations

## Out of Scope

- CI integration for load tests (can be added later)
- Distributed load generation (single Mac is sufficient)
- Changes to AI service rate limiter (document behavior, don't remove)
- Multi-replica testing (add HPA manifests, note expected behavior, don't test at scale)
- Load testing Java services (separate effort)

## Execution Workflow

1. **Setup** — install k6, verify Prometheus connectivity, create scripts
2. **Baseline** — run each phase at 2-5 VUs to establish baseline metrics
3. **Stress test** — run phases sequentially, analyze between phases
4. **Fix** — implement data-driven improvements, re-run failing scenarios
5. **Document** — build dashboard, write ADR with before/after analysis, commit all artifacts
