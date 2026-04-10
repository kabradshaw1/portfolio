# Go Services

Three microservices sharing a common `go/pkg/` module:

- **auth-service** (8091) — JWT auth, Google OAuth, PostgreSQL
- **ecommerce-service** (8092) — products, cart, orders, returns, PostgreSQL, Redis, RabbitMQ
- **ai-service** (8093) — LLM agent loop with tool calling, Ollama/OpenAI/Anthropic, Redis

## Shared Package (`go/pkg/`)

`go/pkg/` is its own Go module. Each service uses a `replace` directive:

```
require github.com/kabradshaw1/portfolio/go/pkg v0.0.0
replace github.com/kabradshaw1/portfolio/go/pkg => ../pkg
```

Three packages live here:

- **`apperror`** — `AppError` struct, constructor helpers (`NotFound`, `BadRequest`, etc.), Gin `ErrorHandler()` middleware. All handlers use `c.Error()` instead of `c.JSON()` for errors.
- **`resilience`** — `sony/gobreaker` circuit breakers, exponential retry with jitter, combined `Call[T]`/`Do` wrappers. `IsPgRetryable` skips domain errors. Prometheus gauge tracks breaker state.
- **`tracing`** — OpenTelemetry init (`Init(ctx, serviceName, endpoint)`), `RedisSpan` helper, `InjectAMQP`/`ExtractAMQP` for RabbitMQ trace propagation.

**When modifying `go/pkg/`:** run `go mod tidy` in `go/pkg/` and all three service directories, since they all depend on it.

## Docker Build Context

Dockerfiles expect the build context to be `go/` (not `go/<service>/`) because of the `../pkg` replace directive. The Dockerfiles use `WORKDIR /app/<service>` and `COPY pkg/ /app/pkg/` to make this work.

All CI workflows (`go-ci.yml`, `ci.yml`, `aws-deploy.yml`) and `go/docker-compose.yml` use `context: go` with `file: go/<service>/Dockerfile`.

## Architecture Patterns

**Error handling:** Handlers call `c.Error(apperror.NotFound(...))` and return. The `ErrorHandler()` middleware converts these to `{"error": {"code": "...", "message": "...", "request_id": "..."}}` JSON responses. Unknown errors become 500 `INTERNAL_ERROR` with hidden messages.

**Resilience:** Every outbound dependency (PostgreSQL, Redis, HTTP, RabbitMQ publish) is wrapped in a circuit breaker. PostgreSQL and HTTP calls also get retry with exponential backoff. Redis and rate limiter fail open when the breaker trips. Constructor functions accept `*gobreaker.CircuitBreaker[any]` — breakers are created in `main.go` and injected.

**Tracing:** OpenTelemetry with OTLP gRPC to Jaeger. `otelgin` auto-instruments HTTP handlers. `otelhttp` transport on ai-service HTTP clients propagates `traceparent`. Agent loop has parent/child spans. RabbitMQ messages carry trace context in headers. Redis operations get manual spans. Set `OTEL_EXPORTER_OTLP_ENDPOINT` to enable; empty disables tracing (no-op).

## Adding a New Dependency to a Service

1. `cd go/<service> && go get <package>`
2. `go mod tidy` in the service directory
3. If the dependency is also used in `go/pkg/`, add it there too and tidy all services

## Testing

- `make preflight-go` runs golangci-lint + `go test -race` across all three services
- Test routers must include `apperror.ErrorHandler()` middleware for handler tests to work
- Constructor calls in tests need a breaker: `resilience.NewBreaker(resilience.BreakerConfig{Name: "test"})`
- Tracing tests use `tracetest.NewInMemoryExporter()` for span assertions
