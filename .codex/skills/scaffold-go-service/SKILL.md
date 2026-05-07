---
name: scaffold-go-service
description: Scaffold a new decomposed Go microservice with baseline observability, K8s manifests, CI/CD integration, and conditional hooks for gRPC or event consumers. Use when creating a new Go service or extracting a service from the ecommerce monolith.
---

# Scaffold New Go Microservice

This skill guides the creation of a new Go microservice with all required infrastructure. Every item must be addressed or the QA/prod deploy will fail.

**Before any infra mutation, invoke `ops-as-code`.** New services almost always need bootstrap state in shared environments. Any mutating action against a shared environment must exist as committed code, such as a K8s Job manifest or `scripts/ops/` script, before it runs.

## Service Code

Create `go/<service>/` with this structure:

```text
go/<service>/
  cmd/server/
    main.go
    config.go
    routes.go
  internal/
    handler/
    service/
    repository/
    model/
    middleware/
    metrics/
  migrations/
  go.mod
  Dockerfile
```

Service defaults:

- `main.go` initializes tracing, JSON `slog` with trace injection, `buildinfo.Log()`, infrastructure connections, circuit breakers, HTTP server timeouts, conditional interface wiring, and graceful shutdown.
- `routes.go` exposes `/health` and `/metrics`.
- handlers use `c.Error(apperror.<Kind>(...))` and return.
- repositories wrap outbound dependencies with `resilience.NewBreaker` and retry helpers.
- Dockerfiles build from the `go/` context and include buildinfo ldflags.

## Observability Checklist

Every new service must include:

- [ ] `tracing.Init(ctx, "<service-name>", cfg.OTELEndpoint)`
- [ ] `tracing.NewLogHandler(slog.NewJSONHandler(os.Stdout, nil))`
- [ ] `buildinfo.Log()` after `slog.SetDefault`
- [ ] HTTP metrics middleware for `http_requests_total` and `http_request_duration_seconds`
- [ ] business metrics in `internal/metrics/metrics.go`
- [ ] circuit breakers for every outbound dependency
- [ ] Prometheus `/metrics` endpoint with `promhttp.Handler()`
- [ ] Dockerfile ldflags for `buildinfo.Version` and `buildinfo.GitSHA`

## Conditional Interfaces

Invoke the narrow reliability skill before designing or editing conditional interfaces:

- `go-grpc-service-reliability` for protobuf, buf, generated code, gRPC clients/servers, mTLS, gRPC K8s ports, or gRPC CI configuration.
- `go-kafka-consumer-reliability` for Kafka consumers, projection/read-model consumers, Kafka DLQs, retry/backoff, offset commits, or idempotent event processing.
- `go-rabbitmq-reliability` for Go RabbitMQ publishers/consumers, saga commands/replies, AMQP DLQs, retry headers, publisher confirms, mandatory publishing, prefetch/QoS, or duplicate-safe saga handling.

## Kubernetes Manifests

Create or update manifests under `go/k8s/`:

- Deployment with non-root security context, read-only root filesystem, readiness/liveness probes, resource requests/limits, and Prometheus scrape annotations.
- Service with REST port and any conditional interface ports.
- ConfigMap with service env vars such as `DATABASE_URL`, `REDIS_URL`, `RABBITMQ_URL`, `OTEL_EXPORTER_OTLP_ENDPOINT`, and `ALLOWED_ORIGINS`.
- Migration Job at `go/k8s/jobs/<service>-migrate.yml`.
- HPA and PDB with `maxUnavailable: 1`.

For QA, provision service databases with committed idempotent Jobs under `go/k8s/jobs/ops/`, not manual shell commands. QA RabbitMQ URLs must include the `/qa` vhost so QA consumers never compete with production consumers.

## CI/CD And Smoke Tests

Add the service to:

- `.github/workflows/ci.yml` Go lint, test, image build, and hadolint matrices.
- deploy migration job steps for QA and prod.
- `deploy.sh` deployment waits.
- `go/docker-compose.ci.yml` with correct service env vars and health dependencies.
- `go/ci-init.sql` if the service owns a separate database.
- compose-smoke health checks in `frontend/e2e/smoke-go-compose/smoke-go-ci.spec.ts`.
- prod health checks in `frontend/e2e/smoke-prod/smoke-health.spec.ts`.
- `make preflight-go` coverage where applicable.

Use `--entrypoint migrate` for compose migration runs. Without it, Docker passes migration args to the Go binary's `ENTRYPOINT` and starts the HTTP server instead.

## Verification Gate

Before committing a new Go service:

- [ ] service builds locally
- [ ] service tests pass
- [ ] Dockerfile builds from `go/` context
- [ ] K8s manifests are valid YAML
- [ ] CI matrices include the service
- [ ] QA database bootstrap exists if needed
- [ ] QA overlay has required ConfigMap patches
- [ ] conditional gRPC, Kafka, or RabbitMQ checklist is complete if applicable
- [ ] ingress and frontend env vars are updated if the service is frontend-facing
- [ ] compose-smoke and prod health checks include the service
- [ ] `make preflight-go` passes
- [ ] `make preflight-compose-config` passes
