# Go Back Pressure Admission And Scaling Implementation Plan

> **For agentic workers:** Implement task-by-task with TDD where practical. Keep this work separate from Kafka consumer reliability and RabbitMQ saga reliability.

**Goal:** Add production-grade admission control and saturation visibility for the remaining Go back pressure gaps, prioritizing `ai-service` `/chat` and shared primitives before optional broader rollout.

**Spec:** `docs/superpowers/specs/2026-05-07-go-backpressure-admission-scaling-design.md`

**Current branch at planning time:** `main`

**Scope boundaries:**
- Do not rework Kafka consumer behavior. Kafka consumer reliability belongs to `docs/superpowers/specs/2026-05-07-kafka-consumer-reliability-design.md` and branch `adr-kafka-consumer-reliability`.
- Do not rework Go RabbitMQ saga retries, DLQs, confirms, topology, or consumer reliability. That belongs to `docs/superpowers/plans/2026-05-07-rabbitmq-go-reliability.md` and branch `feature/rabbitmq-consumer-reliability`.
- Kafka producer changes here are limited to best-effort producer naming/docs, attempts/failures/duration metrics, and close-on-shutdown verification.

---

## Inspection Summary

- `go/pkg/` has shared modules for `apperror`, `grpcmetrics`, `resilience`, `shutdown`, `tlsconfig`, and `tracing`, but no admission package yet.
- `go/ai-service/internal/http/chat.go` validates body/auth first, then writes SSE headers before calling `runner.Run`; admission must be inserted before the SSE header block.
- `go/ai-service/cmd/server/config.go` has no typed chat admission settings yet; `go/analytics-service/cmd/server/config.go` has a good `time.ParseDuration` helper pattern.
- `go/ai-service/internal/metrics/metrics.go` already centralizes Prometheus collectors for AI metrics.
- `go/ai-service/internal/http/chat_test.go` has route tests that can be extended for overload-before-SSE and permit release behavior.
- Async Kafka producers exist in `go/ai-service/internal/kafka/producer.go`, `go/cart-service/internal/kafka/producer.go`, and `go/order-service/internal/kafka/producer.go`; all use `Async: true`, `RequiredAcks: RequireOne`, `SafePublish`, and `Close()`.
- gRPC servers in auth/product/cart/payment already use `grpc.StatsHandler(otelgrpc.NewServerHandler())` and mTLS opt-in. There is client metrics in `go/pkg/grpcmetrics`, but no server admission interceptor.
- `go/k8s/configmaps/ai-service-config.yml` lacks `CHAT_MAX_IN_FLIGHT` and `CHAT_OVERLOAD_RETRY_AFTER`.

---

## Task 1: Shared Admission Limiter

**Files:**
- Create `go/pkg/admission/limiter.go`
- Create `go/pkg/admission/limiter_test.go`
- Modify `go/pkg/go.mod` or tidy only if imports require it

- [ ] Write tests first for:
  - `TryAcquire` allows up to the configured limit and rejects the next acquire.
  - `Release` is defer-friendly and makes capacity available again.
  - `Acquire(ctx)` with bounded wait returns on context cancellation without leaking permits.
  - invalid max values are rejected clearly.
  - metrics hooks record in-flight and rejected admissions without forcing service-specific metric names.
- [ ] Implement a small semaphore-backed limiter:
  - `NewLimiter(max int, opts ...Option) (*Limiter, error)`
  - `TryAcquire(ctx context.Context) (Permit, bool)`
  - `Acquire(ctx context.Context) (Permit, error)`
  - `Permit.Release()` safe for defer and idempotent.
- [ ] Keep Prometheus support low-cardinality and reusable:
  - either package-level generic metrics with labels `service`, `path`/`operation`, or collector hooks passed from service packages
  - avoid user IDs, message keys, raw methods with unbounded shape, and raw errors.
- [ ] Run `cd go/pkg && go test ./admission`.

**Implementation note:** Prefer a tiny API over middleware-only design because the `/chat` handler needs admission after validation but before SSE headers.

---

## Task 2: AI `/chat` Admission Before SSE

**Files:**
- Modify `go/ai-service/cmd/server/config.go`
- Add or modify `go/ai-service/cmd/server/config_test.go`
- Modify `go/ai-service/cmd/server/routes.go`
- Modify `go/ai-service/internal/http/chat.go`
- Modify `go/ai-service/internal/http/chat_test.go`
- Modify `go/ai-service/internal/metrics/metrics.go`
- Modify `go/k8s/configmaps/ai-service-config.yml`

- [ ] Add config:
  - `CHAT_MAX_IN_FLIGHT`, default `4`
  - `CHAT_OVERLOAD_RETRY_AFTER`, default `5s`
  - invalid integer/duration values should fail startup clearly.
- [ ] Extend route wiring so `/chat` receives a chat admission limiter and retry-after duration.
- [ ] Add failing handler tests:
  - saturated limiter returns JSON overload through `apperror.ErrorHandler()`
  - response status is `503 Service Unavailable`
  - `Retry-After` is present
  - `Content-Type` is not `text/event-stream`
  - permit is released after successful runner completion
  - permit is released when `runner.Run` returns an error after streaming starts.
- [ ] Insert admission after request body/auth/rate-limit validation and before the comment currently marking the SSE boundary.
- [ ] Add AI-specific metrics:
  - `ai_chat_in_flight`
  - `ai_chat_admission_rejections_total`
  - `ai_chat_duration_seconds`
- [ ] Update K8s default config with:
  - `CHAT_MAX_IN_FLIGHT: "4"`
  - `CHAT_OVERLOAD_RETRY_AFTER: "5s"`
- [ ] Run:
  - `cd go/ai-service && go test ./cmd/server ./internal/http ./internal/metrics`

**Behavior choice:** Use `503` for pod saturation, not `429`, because Redis rate limiting already handles caller quota.

---

## Task 3: Best-Effort Kafka Producer Metrics And Naming

**Files:**
- Modify `go/ai-service/internal/kafka/producer.go` and tests
- Modify `go/cart-service/internal/kafka/producer.go` and tests
- Modify `go/order-service/internal/kafka/producer.go` and tests
- Optionally create a shared producer metrics helper under `go/pkg/kafkametrics` if it removes duplication without introducing service coupling

- [ ] Add tests around `SafePublish` with a fake producer:
  - publish attempts are counted.
  - publish failures are counted.
  - publish duration is observed.
  - `SafePublish` still swallows producer errors.
- [ ] Make best-effort semantics explicit:
  - rename constructors to `NewBestEffortProducer` or add wrapper aliases while preserving compatibility where needed.
  - update comments so async/lossy analytics semantics are unmistakable.
  - keep business operation success independent from analytics publish failure.
- [ ] Add metrics:
  - `kafka_producer_messages_total{service,topic,outcome}`
  - `kafka_producer_write_duration_seconds{service,topic}`
  - optional `kafka_producer_async_mode{service}` gauge set to `1`.
- [ ] Confirm existing `defer kafkaPub.Close()` remains in each service startup path.
- [ ] Run targeted tests for each producer package.

**Boundary:** Do not alter `analytics-service/internal/consumer`, `order-projector/internal/consumer`, consumer groups, commits, retries, DLQs, or lag behavior in this task.

---

## Task 4: Optional gRPC Admission Or MaxConcurrentStreams

**Files:**
- Prefer shared support in `go/pkg/grpcmetrics` or `go/pkg/admission`
- Candidate service config files:
  - `go/product-service/cmd/server/config.go`
  - `go/cart-service/cmd/server/config.go`
  - `go/payment-service/cmd/server/config.go`
  - defer `auth-service` unless profiling shows token checks are a hotspot
- Candidate service startup files:
  - `go/product-service/cmd/server/main.go`
  - `go/cart-service/cmd/server/main.go`
  - `go/payment-service/cmd/server/main.go`

- [ ] Start with a shared unary server interceptor test that returns `codes.ResourceExhausted` when saturated.
- [ ] Implement one reusable option:
  - `grpc.MaxConcurrentStreams(N)` for simple transport-level limits, or
  - an application-level unary interceptor backed by `go/pkg/admission` when metrics and method visibility matter.
- [ ] Preserve existing gRPC reliability requirements:
  - keep `grpc.StatsHandler(otelgrpc.NewServerHandler())`
  - keep mTLS opt-in through `TLS_CERT_DIR`
  - keep graceful shutdown registration unchanged
  - keep plaintext local/CI fallback.
- [ ] Add `GRPC_MAX_CONCURRENT_STREAMS` parsing as unset-by-default. Invalid positive integer values should fail startup clearly.
- [ ] If enabling the interceptor for a service, add metrics:
  - `grpc_server_in_flight{service,method}`
  - `grpc_server_admission_rejections_total{service,method}`
- [ ] Run targeted tests for `go/pkg` and each touched service.

**Recommendation:** Implement shared support and enable it first only for `payment-service` or `cart-service` if a concrete limit is chosen. Leave broad service rollout unset by default.

---

## Task 5: Saturation And Backlog Metrics

**Files:**
- AI metrics from Task 2
- Producer metrics from Task 3
- Payment outbox metrics in `go/payment-service/internal/outbox` and `go/payment-service/internal/metrics`
- Optional pgx pool collector helper under `go/pkg/dbmetrics`

- [ ] Add only cheap, low-cardinality metrics in this pass.
- [ ] For pgx pools, prefer a reusable collector/helper that reads `pool.Stat()` for acquired/idle/acquire pressure where service owners can register it explicitly.
- [ ] For payment outbox, expose unpublished backlog from repository queries if there is already a cheap indexed path; otherwise defer to a follow-up schema-aware task.
- [ ] Do not poll RabbitMQ queue depth app-side; rely on broker/exporter metrics.
- [ ] Do not duplicate Kafka consumer lag work where consumer packages already expose it or where the Kafka reliability branch is responsible.

---

## Task 6: Verification And Commit

- [ ] Run targeted package tests immediately after each slice.
- [ ] After code changes, run `go mod tidy` in `go/pkg` and any service that imports new shared packages.
- [ ] Run `make preflight-go` before commit.
- [ ] Commit only the back pressure/admission changes and this plan; do not include unrelated untracked RabbitMQ/Kafka spec docs unless Kyle asks.
- [ ] Because current branch is `main`, do not push autonomously.

## Suggested Implementation Order

1. `go/pkg/admission` tests and implementation.
2. AI chat config, limiter wiring, overload tests, and K8s defaults.
3. Kafka best-effort producer metrics and naming docs.
4. Shared gRPC admission support, enabled narrowly or left config-only.
5. Additional saturation/backlog metrics where cheap.
6. Targeted tests, `make preflight-go`, commit.
