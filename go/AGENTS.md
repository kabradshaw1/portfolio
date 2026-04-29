# Go Services

Go services live under `go/` and use REST for frontend traffic plus gRPC for
service-to-service calls.

## Go Service Inventory

- `auth-service` - JWT auth, Google OAuth, PostgreSQL
- `product-service` - product catalog CRUD, REST `:8095`, gRPC `:9095`, `productdb`
- `cart-service` - cart CRUD plus reserve, release, and clear saga steps, gRPC `:9096`, `cartdb`
- `order-service` - order CRUD and saga orchestration, REST `:8092`, `orderdb`
- `payment-service` - Stripe checkout, webhooks, outbox, REST `:8098`, gRPC `:9098`, `paymentdb`
- `ai-service` - agent loop over Ollama tool calling, ecommerce tools, RAG bridge, REST `:8093`
- `analytics-service` - Kafka consumer and streaming analytics, REST `:8094`

Use the `scaffold-go-service` skill when creating or extracting a Go service.

## Shared Package (`go/pkg/`)

`go/pkg/` is its own Go module. Each service uses a `replace` directive:

```go
require github.com/kabradshaw1/portfolio/go/pkg v0.0.0
replace github.com/kabradshaw1/portfolio/go/pkg => ../pkg
```

Shared packages include:

- `apperror` - `AppError` helpers and Gin `ErrorHandler()` middleware
- `resilience` - circuit breakers, retry helpers, and retryability checks
- `tracing` - OpenTelemetry init plus RabbitMQ and Kafka trace propagation helpers
- `tlsconfig` - server and client TLS helpers for gRPC mTLS

When modifying `go/pkg/`, run `go mod tidy` in `go/pkg/` and every service
directory that depends on it.

## Proto And gRPC Toolchain

`buf` v2 manages protobuf definitions at the `go/` level.

- Config: `go/buf.yaml`, `go/buf.gen.yaml`, and service-specific templates
- Proto files: `go/proto/<service>/v1/<service>.proto`
- Generated code: `go/<service>/pb/<service>/v1/`
- Lint: `cd go && buf lint`
- Generate all service stubs from `go/` using the matching `buf generate`
  commands for the changed proto path and template.

Generated code must stay outside `internal/` so other services can import it.
Inter-service clients use `grpc.NewClient(addr, ...)` with plaintext inside
local dev and mTLS in Kubernetes when `TLS_CERT_DIR` is configured.

## Docker Build Context

Dockerfiles expect build context `go/`, not `go/<service>/`, because services
depend on `../pkg`. CI and compose workflows should use `context: go` with
`file: go/<service>/Dockerfile`.

## Architecture Patterns

Handlers call `c.Error(apperror.<Kind>(...))` and return. The error middleware
converts application errors into the standard JSON envelope and hides unknown
internal messages.

Every outbound dependency should be wrapped with the local resilience helpers.
Redis and rate-limiter paths fail open when breakers trip. PostgreSQL and HTTP
calls use retry with exponential backoff where retryable.

OpenTelemetry is exported through OTLP gRPC when `OTEL_EXPORTER_OTLP_ENDPOINT`
is set. HTTP, agent loop, RabbitMQ, Kafka, Redis, and gRPC paths should preserve
trace context.

## Server Configuration

Every `http.Server` must set `ReadTimeout`, `WriteTimeout`, and `IdleTimeout`.
Standard values are 10s read, 30s write, and 60s idle. Streaming services such
as `ai-service` use a 120s write timeout.

PostgreSQL pools must use `pgxpool.ParseConfig` plus `pgxpool.NewWithConfig`.
Do not use bare `pgxpool.New()` defaults. Avoid magic numbers for durations and
configuration values; use named constants.

## Migrations

Go services use `golang-migrate`. Migration files live in
`go/<service>/migrations/` as `NNN_name.up.sql` and `NNN_name.down.sql` pairs.

Kubernetes migration Jobs run `migrate up` on deploy and must use
`DATABASE_URL_DIRECT`, which points directly to Postgres. App Deployments use
`DATABASE_URL` through PgBouncer. This avoids PgBouncer transaction-pool issues
with migration session features such as advisory locks.

`sslmode=disable` is required on Go `DATABASE_URL` values because
`golang-migrate`'s `pq` driver defaults to `sslmode=require`.

For migration changes, run:

```bash
make preflight-go-migrations
```

This requires Docker via Colima and `golang-migrate`.

## Ecommerce And Kafka

The decomposed ecommerce path uses RabbitMQ for checkout saga messages and
Kafka for analytics events.

Kafka runs in KRaft mode. Topics:

- `ecommerce.orders`
- `ecommerce.cart`
- `ecommerce.views`

`analytics-service` consumes as group `analytics-group` and exposes metrics
including `analytics_events_consumed_total`, `analytics_aggregation_latency_seconds`,
`kafka_consumer_lag`, and `kafka_consumer_errors_total`.

QA uses a separate RabbitMQ `/qa` vhost for queue isolation.

## AI Service Gateway

`go/ai-service/` is the MCP gateway for AI functionality. It fronts ecommerce
tools and RAG tools through a unified ReAct-style agent loop with SSE events:
`tool_call`, `tool_result`, `tool_error`, `final`, and `error`.

Important environment variables:

- `RAG_CHAT_URL`
- `RAG_INGESTION_URL`
- `OLLAMA_URL`
- `REDIS_URL`
- `ECOMMERCE_URL`

The RAG bridge calls Python chat and ingestion services through clients under
`go/ai-service/internal/tools/clients/`.

## Kubernetes Requirements

Every deployment must include container security context, readiness probe,
liveness probe, and a Pod Disruption Budget in `go/k8s/pdb/` with
`maxUnavailable: 1`. Use startup probes for slow-starting services.

## Testing

Before committing Go changes, run:

```bash
make preflight-go
```

Handler tests should include `apperror.ErrorHandler()` middleware. Constructor
calls in tests need a test breaker from `resilience.NewBreaker(...)`. Tracing
tests use `tracetest.NewInMemoryExporter()` for span assertions.
