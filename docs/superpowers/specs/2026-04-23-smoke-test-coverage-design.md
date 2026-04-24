# Smoke Test Coverage Expansion

## Problem

The project has grown to include Go ecommerce services (auth, order, product, cart, payment, analytics, ai-service), Kafka streaming analytics, gRPC inter-service communication, and expanded Java services (activity, notification) — but smoke test coverage hasn't kept pace. Current smoke tests only cover:

- **CI compose-smoke:** Python RAG stack (ingestion, chat, debug health checks + upload/query flow)
- **Smoke-prod:** Python RAG flow, Go auth/products/cart/checkout, Java register/project/task, Grafana health

Services with no smoke coverage: payment-service, analytics-service, ai-service (direct), order-projector, activity-service, notification-service, debug service (prod), and all service health endpoints beyond Python.

## Approach

Playwright-only — extend the existing Playwright test infrastructure for both CI compose-smoke and smoke-prod. No shell scripts or separate test binaries.

## New CI Compose-Smoke Jobs

### Go Compose-Smoke (`compose-smoke-go`)

New CI job that mirrors the existing Python `compose-smoke` pattern:

1. Start Go `docker-compose.yml` with a `docker-compose.ci.yml` overlay
2. Services started: postgres, redis, rabbitmq, kafka, auth-service, order-service, product-service, cart-service, analytics-service
3. **Skips ai-service** — depends on external Ollama and Python RAG services
4. CI overlay sets `JWT_SECRET=ci-test-secret`, `ALLOWED_ORIGINS=*`
5. Seed data: SQL init script mounted into Postgres to run migrations and insert a "Smoke Test Widget" product
6. Wait for auth-service `/health` on port 8091
7. Run `playwright.smoke-go.config.ts`

**Test cases (`e2e/smoke-go-compose/smoke-go-ci.spec.ts`):**

| Test | What it validates |
|------|-------------------|
| Health checks pass | Hit `/health` on auth (8091), order (8092), product (8095), cart (8096), analytics (8094) — assert 200 |
| Auth flow | Register with unique email → login → verify httpOnly `access_token` cookie |
| Product catalog | `GET /products` returns non-empty array, `GET /categories` returns non-empty array |
| Checkout lifecycle with analytics | Add to cart → create order → poll cart empty → poll analytics stats endpoint for order event consumed |

Cleanup: compose teardown destroys all ephemeral data.

### Java Compose-Smoke (`compose-smoke-java`)

New CI job, same pattern:

1. Start Java `docker-compose.yml` with a `docker-compose.ci.yml` overlay
2. Services started: postgres, mongodb, redis, rabbitmq, task-service, activity-service, notification-service, gateway-service
3. CI overlay sets `JWT_SECRET=ci-test-secret-at-least-32-characters-long`, `ALLOWED_ORIGINS=*`
4. Wait for gateway-service health on port 8080
5. Run `playwright.smoke-java.config.ts`

**Test cases (`e2e/smoke-java-compose/smoke-java-ci.spec.ts`):**

| Test | What it validates |
|------|-------------------|
| Gateway health check | Hit gateway health endpoint — assert 200 |
| GraphQL schema loads | Introspection query succeeds |
| Register → project → task → activity | Register user via GraphQL, create project, create task, query activity feed for entries — proves async RabbitMQ flow (task-service → notification-service → activity-service) works |

Cleanup: `afterAll` deletes task, project, user via GraphQL mutations. Compose teardown handles the rest.

## Expanded Smoke-Prod Tests

### New File: `smoke-health.spec.ts`

Health checks for all services not currently covered in prod:

| Test | Endpoints |
|------|-----------|
| Go service health | `/go-auth/health`, `/go-orders/health`, `/go-products/health`, `/go-cart/health`, `/go-analytics/health` — assert 200 |
| Go AI service readiness | `/go-ai/health` (200), `/go-ai/ready` (200, verify checks object reports dependency status) |
| Debug service health | `/debug/health` — assert `status: "healthy"` |

### New File: `smoke-debug.spec.ts`

Light functional test for the debug service pipeline:

| Test | What it validates |
|------|-------------------|
| Debug query returns SSE stream | `POST /debug/debug` with `{ code: "def hello(): return 'world'", question: "What does this do?" }` — assert response contains `data:` events. No LLM output validation — just proves the pipeline streams a response. |

### Extensions to Existing `smoke.spec.ts`

**Go checkout test** — add analytics verification after cart-empty polling:

- Poll `GET /go-analytics/stats/orders` with retries (up to 15s, 500ms intervals)
- Assert order count > 0 or a recent event timestamp is present
- Proves Kafka consumer is alive and processing events end-to-end

**Java task test** — add activity feed verification before cleanup:

- Query the activity feed for the test project via GraphQL
- Assert at least one activity entry exists
- Proves the async flow through RabbitMQ (task-service publishes event → notification-service consumes → activity-service records) is working

## File Structure

```
frontend/
├── e2e/
│   ├── smoke-go-compose/
│   │   └── smoke-go-ci.spec.ts
│   ├── smoke-java-compose/
│   │   └── smoke-java-ci.spec.ts
│   └── smoke-prod/
│       ├── smoke.spec.ts              # existing — extended with analytics + activity checks
│       ├── smoke-health.spec.ts       # new — health checks for uncovered services
│       └── smoke-debug.spec.ts        # new — debug service functional test
├── playwright.smoke-go.config.ts
├── playwright.smoke-java.config.ts
go/
├── docker-compose.ci.yml
java/
├── docker-compose.ci.yml
```

## CI Workflow Changes

Two new jobs in `.github/workflows/ci.yml`, both following the existing `compose-smoke` pattern:

```
compose-smoke-go:
  name: Compose Smoke (Go stack)
  runs-on: ubuntu-latest
  steps:
    - checkout
    - GHCR login
    - build Go images (docker compose build)
    - start compose stack (skip ai-service)
    - wait for auth-service /health
    - setup Node.js + install frontend deps + Playwright
    - run playwright.smoke-go.config.ts
    - dump logs on failure
    - teardown

compose-smoke-java:
  name: Compose Smoke (Java stack)
  runs-on: ubuntu-latest
  steps:
    - checkout
    - GHCR login
    - build Java images (docker compose build)
    - start compose stack
    - wait for gateway-service health
    - setup Node.js + install frontend deps + Playwright
    - run playwright.smoke-java.config.ts
    - dump logs on failure
    - teardown
```

Both jobs run on the same triggers as existing `compose-smoke`: push to `qa`/`main`, PRs to `qa`.

## Playwright Config Patterns

All new configs follow existing conventions:

- `fullyParallel: false`
- `retries: 1`
- `workers: 1`
- `trace: "on-first-retry"`

Go compose tests hit services directly on their ports (no gateway in Go compose):
- `process.env.SMOKE_AUTH_URL || "http://localhost:8091"`
- `process.env.SMOKE_ORDER_URL || "http://localhost:8092"`
- `process.env.SMOKE_PRODUCT_URL || "http://localhost:8095"`
- `process.env.SMOKE_CART_URL || "http://localhost:8096"`
- `process.env.SMOKE_ANALYTICS_URL || "http://localhost:8094"`

Java compose tests go through the gateway: `process.env.SMOKE_API_URL || "http://localhost:8080"`.
Prod tests use `process.env.SMOKE_API_URL || "https://api.kylebradshaw.dev"` (ingress routes by path).

## What This Does NOT Cover

- **gRPC endpoint validation** — REST `/health` endpoints that check dependency connectivity are sufficient for smoke tests. gRPC issues surface in integration tests.
- **Payment-service webhook flows** — Stripe webhooks require external integration. Payment-service health is validated indirectly through the checkout saga (order completes = payment path worked).
- **ai-service in compose-smoke** — depends on Ollama and Python RAG services. Covered in smoke-prod via `/go-ai/health` and `/go-ai/ready`.
- **order-projector** — gRPC-only service, no REST health endpoint. Validated indirectly through order queries that read projected data.
- **eval service** — not a production-facing service. No smoke coverage needed.
