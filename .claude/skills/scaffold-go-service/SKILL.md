---
name: scaffold-go-service
description: Scaffold a new decomposed Go microservice with full observability, gRPC, K8s manifests, and CI/CD integration. Use when creating a new Go service or extracting a service from the ecommerce monolith.
disable-model-invocation: true
---

# Scaffold New Go Microservice

This skill guides the creation of a new Go microservice with all required infrastructure. Every item must be addressed or the QA/prod deploy will fail.

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

1. **Create QA database manually:**
```bash
ssh debian 'kubectl exec -n java-tasks deploy/postgres -- psql -U taskuser -d taskdb -c "CREATE DATABASE <dbname>_qa OWNER taskuser;"'
```

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
```

3. **Ingress** — Add path in `go/k8s/ingress.yml` and update `go/k8s/kustomization.yaml`

## Frontend + Smoke Tests

- Add `NEXT_PUBLIC_<SERVICE>_URL` env var to Vercel (production + preview/qa) BEFORE merging
- Update `frontend/e2e/smoke-prod/smoke.spec.ts` if endpoints moved
- Add to `make preflight-go` target

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
