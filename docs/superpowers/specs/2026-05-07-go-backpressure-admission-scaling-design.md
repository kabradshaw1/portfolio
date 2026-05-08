# Go Back Pressure Admission And Scaling Design

Date: 2026-05-07

## TL;DR

The Go services already have good basic resilience: HTTP timeouts, bounded DB
pools, Redis rate limits on several endpoints, circuit breakers, and async
consumer hardening plans. The remaining back pressure work is mostly admission
control and operational scaling around expensive paths and producers.

This spec covers:

- per-pod max in-flight admission for expensive HTTP paths, especially
  `ai-service` `/chat`
- explicit Kafka producer durability contracts
- optional gRPC server admission limits
- backlog, saturation, and producer metrics that can drive future autoscaling

Kafka consumer reliability and RabbitMQ saga reliability are not repeated here;
they are covered by the dedicated 2026-05-07 Kafka consumer and RabbitMQ Go
reliability specs/plans.

## Current State

The Go services have several back pressure primitives already:

- every Go HTTP server sets read, write, and idle timeouts
- PostgreSQL clients use bounded connection pools
- auth/cart/order/AI expose Redis-backed request rate limits
- outbound dependencies use local circuit breaker and retry helpers
- Kubernetes manifests include resource requests/limits and HPAs for some
  request-serving services

The remaining gaps are at service admission boundaries and producer semantics:

- `ai-service` `/chat` can run many concurrent long-lived LLM/tool workflows
  per pod as long as callers pass the rate limit.
- Kafka producers use `Async: true` in several services. That is acceptable for
  non-critical analytics events, but the code should make the lossy contract
  explicit and expose visibility into publish failures or dropped events.
- gRPC servers do not currently set explicit concurrent stream admission limits.
- Existing metrics expose some service health, but not enough saturation and
  backlog signals to drive operational decisions.

## Goals

- Bound expensive per-pod work before it consumes all goroutines, DB
  connections, LLM capacity, or downstream service capacity.
- Return clear overload responses with `Retry-After` where callers can retry.
- Make Kafka producer durability explicit: durable business events must not use
  fire-and-forget semantics, while analytics events may stay lossy by design.
- Add low-cardinality metrics for in-flight work, admission rejections, queue or
  lag backlog, producer outcomes, and DB pool pressure.
- Keep changes repo-local and free of paid cloud dependencies.
- Preserve local development defaults.

## Non-Goals

- Do not reimplement Kafka consumer reliability here.
- Do not reimplement RabbitMQ saga retry, DLQ, or publisher confirm behavior
  here.
- Do not introduce Kafka transactions.
- Do not add KEDA or another cluster add-on unless a later implementation plan
  explicitly approves it.
- Do not change API response schemas except for standard overload errors where
  they are currently missing.

## HTTP Admission Control

Add a small shared admission primitive, preferably under `go/pkg/admission`, for
bounded in-flight work:

- fixed maximum concurrent permits
- non-blocking acquire for request admission
- optional bounded wait with context cancellation for internal callers
- `Release` must be defer-friendly
- Prometheus metrics for current in-flight work and rejected admissions

Suggested behavior for HTTP handlers:

1. Try to acquire a permit at the start of the expensive path.
2. If the permit is unavailable, return `429 Too Many Requests` or
   `503 Service Unavailable`.
3. Include `Retry-After` when the handler can provide a useful retry hint.
4. Release the permit when the request completes or the context is cancelled.

Use `429` when the limit is caller-facing quota style. Use `503` when the pod is
temporarily saturated and the request should be retried by infrastructure or a
client.

### AI Chat

`ai-service` `/chat` is the highest priority because it streams long-running
responses and can call LLM, RAG, ecommerce, PostgreSQL, Loki, and Jaeger
dependencies.

Add config:

- `CHAT_MAX_IN_FLIGHT`, default `4` locally
- `CHAT_OVERLOAD_RETRY_AFTER`, default `5s`

Apply the limiter after auth/rate-limit/request-size validation and before
writing SSE headers. If the request is rejected, return a normal JSON error
instead of starting an SSE stream.

Metrics:

- `ai_chat_in_flight`
- `ai_chat_admission_rejections_total`
- `ai_chat_duration_seconds`

The rate limiter still protects per-client abuse. The in-flight limiter protects
the pod and downstream dependencies from aggregate load.

### Other HTTP Services

Do not add blanket admission control to every cheap endpoint in this pass.
Prioritize endpoints that can fan out, lock rows, or perform long-running work:

- order checkout / saga-starting paths
- admin/reporting refresh endpoints
- product search if it becomes expensive under load

Each service should use config defaults that are conservative but not hostile to
local development.

## Kafka Producer Semantics

The repo should distinguish producer modes in code and docs:

### Best-Effort Analytics Producer

Current `Async: true` producers are acceptable for events that are explicitly
non-critical analytics signals. Keep the primary request path non-blocking, but
make the tradeoff visible:

- name constructors or types as best-effort where practical
- document that publish failure must not fail the business operation
- increment publish attempt and failure metrics
- ensure shutdown calls `Close()` so buffered async writes get a chance to flush

Metrics:

- `kafka_producer_messages_total{service,topic,outcome}`
- `kafka_producer_write_duration_seconds{service,topic}`
- `kafka_producer_async_mode{service}` as a gauge set to `1` for best-effort
  producers, if useful for dashboards

### Durable Business Producer

If a future Go path publishes business-critical Kafka events, it should not use
the best-effort producer. It should use synchronous writes, bounded context
timeouts, retries only when safe, and caller-visible failure semantics. For
state changes, prefer an outbox table so the database commit and publish intent
are durable together.

This spec does not require converting existing analytics events to durable
business events.

## gRPC Admission Control

Go gRPC servers already have tracing and mTLS support. Add explicit admission
limits only where concurrency can realistically overload the service or its DB.

Options:

- set `grpc.MaxConcurrentStreams(N)` for coarse transport-level control
- add a unary server interceptor using the shared `go/pkg/admission` limiter for
  application-level control and metrics

Prefer the interceptor when the service needs clear metrics or method-specific
behavior. Prefer `MaxConcurrentStreams` when a simple transport cap is enough.

Initial candidates:

- product-service stock/availability RPCs
- cart-service reservation RPCs
- payment-service payment creation/refund RPCs
- auth-service token checking if it becomes a hotspot

Metrics:

- `grpc_server_in_flight{service,method}`
- `grpc_server_admission_rejections_total{service,method}`

Rejected unary RPCs should return `codes.ResourceExhausted`.

## Saturation And Backlog Metrics

Add low-cardinality metrics that expose pressure before users see failures.

Recommended metrics:

- HTTP in-flight requests for limited paths
- admission rejections by service/path
- pgx pool acquired, idle, and acquire wait duration where pgx exposes useful
  stats
- outbox unpublished row count for services that use outbox publishing
- Kafka producer outcomes by service/topic
- Kafka consumer lag where consumers already expose it
- RabbitMQ queue depth and DLQ depth through broker/exporter metrics rather than
  app-side polling where possible

Avoid high-cardinality labels such as user IDs, order IDs, message keys,
offsets, raw SQL text, or raw error strings.

## Autoscaling Direction

The existing HPAs are CPU based. CPU is useful but incomplete for queue-backed
or LLM-heavy paths. Future scaling should consider:

- CPU and memory for baseline pod health
- Kafka consumer lag for consumer services
- RabbitMQ queue depth for saga workers
- HTTP/gRPC in-flight saturation and admission rejection rate for request
  services
- outbox unpublished row count for producer backlog

This spec does not mandate a specific scaler implementation. If the repo later
adds Prometheus Adapter or KEDA, use these metrics as inputs. Until then, the
metrics still improve dashboards and reviewability.

## Configuration

Add config through each service's `cmd/server/config.go` and corresponding K8s
ConfigMap where the feature is enabled.

Suggested defaults:

- `CHAT_MAX_IN_FLIGHT=4`
- `CHAT_OVERLOAD_RETRY_AFTER=5s`
- `GRPC_MAX_CONCURRENT_STREAMS` unset by default
- endpoint-specific HTTP admission limits unset by default unless the endpoint
  is known expensive

Config parsing should fail clearly for invalid numeric or duration values where
the service cannot safely continue.

## Tests

Add tests before implementation:

- admission limiter allows up to the configured limit and rejects the next
  acquire
- cancelled contexts do not leak permits
- AI `/chat` returns overload JSON before SSE headers when saturated
- AI `/chat` releases permits after runner completion and after runner error
- gRPC interceptor returns `codes.ResourceExhausted` when saturated
- Kafka best-effort producer records publish failures without failing
  `SafePublish`
- service config parses admission settings and rejects invalid values

Run targeted package tests first, then `make preflight-go` before committing.

## Implementation Approach

Implement in small slices:

1. Add shared `go/pkg/admission` limiter and tests.
2. Wire AI `/chat` admission control and metrics.
3. Add Kafka producer outcome metrics and make best-effort naming/docs explicit.
4. Add optional gRPC admission interceptor or `MaxConcurrentStreams` support for
   services that need it.
5. Add saturation/backlog metrics that are cheap and low cardinality.
6. Update K8s ConfigMaps for enabled limits.
7. Run targeted Go tests and `make preflight-go`.

