# Go Backpressure Admission Control

- **Date:** 2026-05-07
- **Status:** Accepted

## Context

The Go ecommerce services already use several resilience controls: HTTP server
timeouts, bounded PostgreSQL pools, Redis rate limits on selected endpoints,
circuit breakers, retries, and graceful shutdown. Those controls do not fully
protect each pod from aggregate expensive work after a request has passed
caller-level rate limits.

The highest-risk path is `ai-service` `POST /chat`. A single accepted chat
request can hold a streaming HTTP connection while running an LLM/tool loop and
calling RAG, ecommerce, PostgreSQL, Loki, Jaeger, and Kafka-adjacent analytics
paths. If too many chats run concurrently on one pod, downstream dependencies
and goroutines can saturate before clients get a clear overload signal.

Kafka producer behavior also needed a clearer contract. Existing Go analytics
producers use async kafka-go writers, which is acceptable for non-critical
analytics signals only if the code and metrics make the best-effort semantics
visible. Kafka consumer reliability and RabbitMQ saga reliability are handled
by separate work and are intentionally out of scope for this decision.

## Decision

Add a shared `go/pkg/admission` limiter for bounded in-flight work. The limiter
supports non-blocking admission, context-aware waiting for internal callers, and
defer-friendly permit release. It is intentionally small so handlers can place
admission at the correct point in request flow rather than relying only on
blanket middleware.

Apply the limiter to `ai-service` `POST /chat` after body/auth/rate-limit
validation and before writing SSE headers. Saturated chat requests return a
normal JSON overload error with `503 Service Unavailable` and `Retry-After`
instead of starting an SSE stream. Configure this with:

- `CHAT_MAX_IN_FLIGHT`, default `4`
- `CHAT_OVERLOAD_RETRY_AFTER`, default `5s`

Expose low-cardinality saturation metrics:

- `ai_chat_in_flight`
- `ai_chat_admission_rejections_total`
- `ai_chat_duration_seconds`
- `grpc_server_in_flight{service,method}`
- `grpc_server_admission_rejections_total{service,method}`
- `kafka_producer_messages_total{service,topic,outcome}`
- `kafka_producer_write_duration_seconds{service,topic}`
- `kafka_producer_async_mode{service}`
- `payment_outbox_unpublished`

Keep Kafka analytics producers best-effort and make that explicit through
constructor naming/comments and producer outcome metrics. Publish failures are
logged and counted, but they do not fail the primary business operation.

Add reusable gRPC unary server admission support that returns
`codes.ResourceExhausted` when saturated, while preserving the existing gRPC
requirements for OpenTelemetry stats handlers, mTLS opt-in, and graceful
shutdown. Broad service-level gRPC limits remain opt-in rather than enabled by
default.

## Consequences

Positive outcomes:

- Expensive AI chat work is bounded per pod before SSE response headers commit.
- Overload is explicit and retryable instead of manifesting as slow streams,
  downstream failures, or goroutine pressure.
- Best-effort Kafka producer semantics are visible in code and metrics.
- Dashboards and future autoscaling work can use saturation and backlog signals
  beyond CPU.
- The shared limiter and gRPC interceptor give future expensive paths a common
  implementation pattern.

Trade-offs:

- A saturated pod now rejects some valid chat requests with `503`, so clients
  need to retry according to `Retry-After`.
- The default chat limit of `4` is conservative and may need tuning under real
  load tests.
- Producer metrics improve visibility into async analytics loss, but they do
  not make those events durable.
- gRPC admission support is available but not broadly enabled until each
  service has a justified concurrency limit.
