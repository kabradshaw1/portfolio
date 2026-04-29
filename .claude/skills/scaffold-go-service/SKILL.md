---
name: scaffold-go-service
description: Scaffold a new decomposed Go microservice with full observability, gRPC, K8s manifests, and CI/CD integration. Use when creating a new Go service or extracting a service from the ecommerce monolith.
disable-model-invocation: true
---

# Scaffold New Go Microservice

This skill guides the creation of a new Go microservice with all required infrastructure. Every item must be addressed or the QA/prod deploy will fail.

**Before any infra mutation, invoke `ops-as-code`.** New services almost always need bootstrap state in shared environments (databases, roles, queues, secrets). The rule is: any mutating action against a shared environment must exist as committed code (a K8s Job manifest or a `scripts/ops/` script) before it runs. Don't tell the user to `ssh debian` and `kubectl exec ... CREATE DATABASE` — that's the exact anti-pattern this project moved away from. See `docs/superpowers/specs/2026-04-28-ops-as-code-design.md`.

## Service Code

Create `go/<service>/` with this structure:

```
go/<service>/
  cmd/server/
    main.go       # Entry point: tracing, slog, buildinfo, infra connections, servers, shutdown
    config.go     # loadConfig() from env vars
    routes.go     # setupRouter() with gin, otelgin, apperror, prometheus /metrics
  internal/
    handler/      # HTTP handlers (use c.Error(apperror.X()) pattern)
    service/      # Business logic
    repository/   # Database access with circuit breaker + retry
    model/        # Domain types
    middleware/    # HTTP metrics middleware (http_requests_total, http_request_duration_seconds)
    metrics/      # Business-specific Prometheus metrics (promauto)
  migrations/     # golang-migrate SQL files (NNN_name.up.sql + .down.sql pairs)
  go.mod          # With `replace ../pkg => ../pkg` directive
  Dockerfile      # Multi-stage build with -ldflags for buildinfo
```

## main.go Boilerplate

Every service main.go must include:

```go
package main

import (
    "context"
    "log"
    "log/slog"
    "os"
    "time"

    "github.com/kabradshaw1/portfolio/go/pkg/buildinfo"
    "github.com/kabradshaw1/portfolio/go/pkg/resilience"
    "github.com/kabradshaw1/portfolio/go/pkg/shutdown"
    "github.com/kabradshaw1/portfolio/go/pkg/tracing"
)

func main() {
    cfg := loadConfig()
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    // 1. Initialize tracing
    shutdownTracer, err := tracing.Init(ctx, "<service-name>", cfg.OTELEndpoint)
    if err != nil {
        log.Fatalf("tracing init: %v", err)
    }

    // 2. Structured JSON logging with traceID injection
    slog.SetDefault(slog.New(
        tracing.NewLogHandler(slog.NewJSONHandler(os.Stdout, nil)),
    ))
    buildinfo.Log()

    // 3. Infrastructure connections (postgres, redis, rabbitmq, etc.)
    // 4. Circuit breakers for each dependency
    // 5. Repositories, services, handlers
    // 6. REST server with timeouts
    httpSrv := &http.Server{
        Addr:         ":" + cfg.Port,
        Handler:      router,
        ReadTimeout:  10 * time.Second,
        WriteTimeout: 30 * time.Second,
        IdleTimeout:  60 * time.Second,
    }
    // 7. gRPC server (if applicable) with mTLS opt-in
    // 8. Graceful shutdown manager
    sm := shutdown.New(15 * time.Second)
    // Register shutdown hooks in priority order
}
```

## Observability Checklist

Every new service MUST include:

- [ ] **slog + tracing** — `tracing.NewLogHandler(slog.NewJSONHandler(...))` in main.go
- [ ] **buildinfo** — `buildinfo.Log()` after slog.SetDefault
- [ ] **HTTP metrics middleware** — `http_requests_total` and `http_request_duration_seconds` (copy pattern from `go/order-service/internal/middleware/metrics.go`)
- [ ] **Business metrics** — Service-specific counters/histograms in `internal/metrics/metrics.go`
- [ ] **gRPC client interceptor** — If calling other services via gRPC, add `grpc.WithUnaryInterceptor(grpcmetrics.UnaryClientInterceptor("<target>"))` to every `grpc.NewClient` call
- [ ] **gRPC server OTel** — If serving gRPC, add `grpc.StatsHandler(otelgrpc.NewServerHandler())`
- [ ] **mTLS opt-in** — If serving gRPC, check `TLS_CERT_DIR` env var and call `tlsconfig.ServerTLS(certDir)` (see `go/payment-service/cmd/server/main.go` for pattern)
- [ ] **Circuit breakers** — Every outbound dependency (Postgres, Redis, HTTP, RabbitMQ) wrapped in `resilience.NewBreaker`
- [ ] **Prometheus /metrics endpoint** — `router.GET("/metrics", gin.WrapH(promhttp.Handler()))`
- [ ] **Dockerfile ldflags** — Build with `-ldflags="-X .../buildinfo.Version=${BUILD_VERSION} -X .../buildinfo.GitSHA=${BUILD_COMMIT}"`

## Dockerfile Template

```dockerfile
FROM golang:1.24-alpine AS builder
WORKDIR /app/<service>
COPY pkg/ /app/pkg/
COPY <service>/ /app/<service>/
# If importing another service's proto:
# COPY <other-service>/ /app/<other-service>/
RUN go mod download
ARG BUILD_VERSION=dev
ARG BUILD_COMMIT=unknown
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-X github.com/kabradshaw1/portfolio/go/pkg/buildinfo.Version=${BUILD_VERSION} -X github.com/kabradshaw1/portfolio/go/pkg/buildinfo.GitSHA=${BUILD_COMMIT}" \
    -o /<service> ./cmd/server

FROM alpine:3.21
RUN apk add --no-cache ca-certificates wget && adduser -D -u 1001 appuser
COPY --from=builder /<service> /<service>
COPY --from=builder /app/<service>/migrations /migrations
USER 1001
EXPOSE <rest-port> <grpc-port>
ENTRYPOINT ["/<service>"]
```

## Proto + buf (if using gRPC)

1. Create `go/proto/<service>/v1/<service>.proto`
2. Add generation config or use existing `buf.gen.yaml`
3. Run: `cd go && buf generate --path proto/<service>`
4. Generated code goes to `go/<service>/pb/<service>/v1/` (NOT `internal/pb/`)
5. If another service imports this proto, add `replace` directive in that service's go.mod AND `COPY` in its Dockerfile

## Kubernetes Manifests

Create in `go/k8s/`:

### Deployment
Must include:
- Security context: `runAsNonRoot: true`, `runAsUser: 1001`, `readOnlyRootFilesystem: true`, `allowPrivilegeEscalation: false`, `capabilities.drop: [ALL]`
- Readiness probe: `/health` endpoint
- Liveness probe: `/health` with longer intervals
- Resource limits and requests
- Volume mount for `grpc-tls` secret at `/etc/tls` (if gRPC)
- `prometheus.io/scrape: "true"` annotation

### Other manifests
- Service (ClusterIP, REST + gRPC ports)
- ConfigMap (env vars: DATABASE_URL, REDIS_URL, RABBITMQ_URL, OTEL endpoint, ALLOWED_ORIGINS, TLS_CERT_DIR)
- Migration Job (`go/k8s/jobs/<service>-migrate.yml`)
- HPA (horizontal pod autoscaler)
- PDB (`maxUnavailable: 1`)

### cert-manager Certificate
Add to `k8s/cert-manager/certificates.yml` (prod) and `k8s/cert-manager/qa-certificates.yml`:
```yaml
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: <service>-grpc-tls
spec:
  secretName: <service>-grpc-tls
  issuerRef:
    name: grpc-ca-issuer
    kind: Issuer
  dnsNames:
    - go-<service>
    - go-<service>.<namespace>
    - go-<service>.<namespace>.svc.cluster.local
```

## CI/CD Integration

### CI matrices (`.github/workflows/ci.yml`)

Add the service to these matrix jobs:
- `go-lint` — service name for golangci-lint
- `go-tests` — service name for `go test`
- `build-images` — service, context, file, image, paths
- `security-hadolint` — Dockerfile path

### CI deploy steps

**QA deploy section** (~line 886):
```yaml
# Delete old migration job, apply new one, wait for completion
sed 's/namespace: go-ecommerce$/namespace: go-ecommerce-qa/' go/k8s/jobs/<service>-migrate.yml | $SSH "kubectl delete -f - --ignore-not-found && kubectl apply -f -"
$SSH "kubectl wait --for=condition=complete job/<service>-migrate -n go-ecommerce-qa --timeout=60s"
```

**Prod deploy section** (~line 1003): Same pattern without the `sed` namespace swap.

### deploy.sh

Add `kubectl wait` for the new deployment in both QA and prod sections.

## QA Setup

1. **Provision the QA database via a committed Job, not by hand.** Per `ops-as-code`: the bootstrap action exists as code. Add a `<service>-db-bootstrap` Job at `go/k8s/jobs/ops/<service>-db-bootstrap-qa.yml` that runs `CREATE DATABASE IF NOT EXISTS` (or the Postgres equivalent: `SELECT 1 FROM pg_database WHERE datname='<dbname>_qa'` guarded `CREATE DATABASE`). Idempotent: re-running on an existing DB is a no-op. CI applies it as part of the QA deploy.

   Do NOT add an `ssh debian ... CREATE DATABASE ...` step to the runbook or PR description; that's the anti-pattern this project moved away from.

2. **QA Kustomize overlay** — Add ConfigMap patch in `k8s/overlays/qa-go/kustomization.yaml`:
```yaml
- target:
    kind: ConfigMap
    name: <service>-config
  patch: |
    - op: replace
      path: /data/ALLOWED_ORIGINS
      value: "https://qa.kylebradshaw.dev,http://localhost:3000"
    - op: replace
      path: /data/DATABASE_URL
      value: "postgres://taskuser:taskpass@postgres.java-tasks.svc.cluster.local:5432/<dbname>_qa?sslmode=disable"
    - op: replace
      path: /data/REDIS_URL
      value: "redis://redis.java-tasks.svc.cluster.local:6379/1"
    - op: replace
      path: /data/RABBITMQ_URL
      value: "amqp://guest:guest@rabbitmq.java-tasks.svc.cluster.local:5672/qa"
```

**CRITICAL:** QA RABBITMQ_URL must include the `/qa` vhost. Without it, QA consumers compete with production consumers for the same messages. This caused a checkout bug where production cart-service consumed QA saga commands.

3. **Ingress** — Add path in `go/k8s/ingress.yml` and update `go/k8s/kustomization.yaml`

## Frontend

- Add `NEXT_PUBLIC_<SERVICE>_URL` env var to Vercel (production + preview/qa) BEFORE merging
- Add to `make preflight-go` target

## Smoke Tests & Compose CI

Every new service needs smoke test coverage in three places: compose-smoke CI (tests service starts and responds), prod health checks (tests deployed service is reachable), and compose config validation (catches env var errors before pushing).

### 1. Database init (`go/ci-init.sql`)

If the service uses its own database (not `ecommercedb`), add:
```sql
CREATE DATABASE <dbname>;
GRANT ALL PRIVILEGES ON DATABASE <dbname> TO taskuser;
```

### 2. Compose CI overlay (`go/docker-compose.ci.yml`)

Add a service block:
```yaml
  <service>:
    build:
      context: .
      dockerfile: <service>/Dockerfile
    ports:
      - "<rest-port>:<rest-port>"
      - "<grpc-port>:<grpc-port>"  # if applicable
    environment:
      DATABASE_URL: postgres://taskuser:taskpass@postgres:5432/<dbname>?sslmode=disable
      JWT_SECRET: ci-test-secret
      ALLOWED_ORIGINS: "*"
      # Add gRPC addresses if calling other services:
      # AUTH_GRPC_URL: auth-service:9091
      # PRODUCT_GRPC_ADDR: product-service:9095
    depends_on:
      postgres:
        condition: service_healthy
      redis:
        condition: service_healthy
```

### 3. CI workflow migration step (`.github/workflows/ci.yml`)

In the `compose-smoke-go` job's "Run migrations and seed data" step, add:
```bash
$DC run --rm --no-deps --entrypoint migrate <service> \
  -path /migrations \
  -database "postgres://taskuser:taskpass@postgres:5432/<dbname>?sslmode=disable" up
```
Use `&x-migrations-table=<svc>_schema_migrations` if sharing a database with another service.

If the service has a `seed.sql`, also add:
```bash
$DC run --rm --no-deps --entrypoint sh <service> \
  -c 'PGPASSWORD=taskpass psql -h postgres -U taskuser -d <dbname> -f /seed.sql'
```

**CRITICAL:** Use `--entrypoint migrate` (or `--entrypoint sh` for seeds). Without it, `docker compose run` passes args to the Go binary's ENTRYPOINT, which starts the HTTP server instead of running migrations.

### 4. Compose-smoke health check (`frontend/e2e/smoke-go-compose/smoke-go-ci.spec.ts`)

Add to the `services` array in the health check test:
```typescript
{ name: "<service>", url: process.env.SMOKE_<SERVICE>_URL || "http://localhost:<port>" }
```

### 5. Prod health check (`frontend/e2e/smoke-prod/smoke-health.spec.ts`)

Add the service's ingress path to the `endpoints` array:
```typescript
"/go-<service>/health"
```

### 6. Compose config validation

No action needed — `make preflight-compose-config` runs automatically and validates the compose overlay merges without interpolation errors.

## Migration Notes

- `sslmode=disable` is REQUIRED on all Go `DATABASE_URL` values (shared Postgres has no SSL)
- If tables were created manually before the migration job runs, set the schema_migrations table: `INSERT INTO <svc>_schema_migrations (version, dirty) VALUES (<N>, false)`
- Use explicit UUIDs shared across seed files so FKs work during transition phases

## Verification Gate

Before merging, confirm ALL items:
- [ ] Service builds locally (`go build ./...`)
- [ ] Tests pass (`go test ./... -race`)
- [ ] Dockerfile builds (`docker build -f go/<service>/Dockerfile go/`)
- [ ] K8s manifests are valid YAML
- [ ] Service is in all CI matrix jobs
- [ ] QA database exists
- [ ] QA overlay has ConfigMap patches
- [ ] cert-manager Certificate added (if gRPC)
- [ ] Ingress path added
- [ ] Frontend env var set in Vercel (if applicable)
- [ ] `make preflight-go` passes
- [ ] Service added to `go/docker-compose.ci.yml` with correct env vars
- [ ] Service DB added to `go/ci-init.sql` (if separate DB)
- [ ] Migration step added to CI `compose-smoke-go` job
- [ ] Health check added to `smoke-go-ci.spec.ts`
- [ ] Health check added to `smoke-health.spec.ts` (prod)
- [ ] `make preflight-compose-config` passes
