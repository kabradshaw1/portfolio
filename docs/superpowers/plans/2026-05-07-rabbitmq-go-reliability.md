# RabbitMQ Go Reliability Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Harden the Go ecommerce RabbitMQ saga path so command, reply, and outbox messages use explicit at-least-once reliability controls.

**Architecture:** Add focused RabbitMQ reliability helpers under `go/pkg/rabbitmq` and wire them into `order-service`, `cart-service`, and `payment-service`. Consumers will use explicit prefetch, typed retry/permanent failure classification, bounded retry headers, confirmed retry republishing, DLQ routing, and metrics. Publishers will use persistent delivery, mandatory publishing, returned-message handling, and publisher confirms before treating a workflow-critical publish as successful.

**Tech Stack:** Go, `github.com/rabbitmq/amqp091-go`, Prometheus client, existing `go/pkg/tracing`, existing saga topology, Testcontainers RabbitMQ integration tests.

---

## File Structure

- Create `go/pkg/rabbitmq/reliability.go`: shared constants, retry header helpers, typed `PermanentError` / `RetryableError`, and failure classification helpers.
- Create `go/pkg/rabbitmq/publisher.go`: confirm publisher wrapper with mandatory publish, returned-message detection, persistent defaults, and context-aware confirm wait.
- Create `go/pkg/rabbitmq/publisher_test.go`: unit tests for persistent publish options, returned-message behavior, nack behavior, and context timeout behavior using a fake publish/confirm adapter.
- Modify `go/order-service/internal/saga/topology.go`: add retry/dead-letter arguments needed by consumers and keep existing queue names stable.
- Modify `go/order-service/internal/saga/publisher.go`: use the shared confirm publisher for saga commands.
- Modify `go/order-service/internal/saga/consumer.go`: set QoS, classify failures, increment retry headers, and DLQ permanent or exhausted messages.
- Modify `go/order-service/internal/saga/metrics.go`: add consumer outcome metrics.
- Modify `go/cart-service/internal/worker/saga_handler.go`: set QoS, classify malformed commands as permanent, make reply events persistent and confirmed, and prevent hot requeue loops.
- Modify `go/payment-service/internal/outbox/poller.go`: publish through confirm wrapper and mark outbox rows published only after broker confirm.
- Modify `go/order-service/cmd/server/deps.go`, `go/order-service/cmd/server/main.go`, and `go/cart-service/cmd/server/main.go`: reconnect AMQP connections/channels after broker or channel failure.
- Add or modify focused tests in `go/order-service/internal/saga`, `go/cart-service/internal/worker`, and `go/payment-service/internal/outbox`.
- Extend `go/order-service/internal/integration/saga_test.go`: add broker-backed tests for malformed/permanent message DLQ behavior and duplicate delivery safety.

## Task 1: Shared Failure Classification And Retry Headers

**Files:**
- Create: `go/pkg/rabbitmq/reliability.go`
- Test: `go/pkg/rabbitmq/reliability_test.go`

- [ ] **Step 1: Write failing tests for retry header parsing and typed classification**

```go
func TestRetryCountHandlesMissingAndAMQPIntegerTypes(t *testing.T) {
	headers := amqp.Table{}
	if got := rabbitmq.RetryCount(headers); got != 0 {
		t.Fatalf("missing retry count = %d, want 0", got)
	}
	headers[rabbitmq.RetryCountHeader] = int32(2)
	if got := rabbitmq.RetryCount(headers); got != 2 {
		t.Fatalf("int32 retry count = %d, want 2", got)
	}
	headers[rabbitmq.RetryCountHeader] = int64(3)
	if got := rabbitmq.RetryCount(headers); got != 3 {
		t.Fatalf("int64 retry count = %d, want 3", got)
	}
}

func TestFailureClassification(t *testing.T) {
	if !rabbitmq.IsPermanent(rabbitmq.PermanentErrorf("bad payload")) {
		t.Fatal("PermanentErrorf should classify as permanent")
	}
	if rabbitmq.IsPermanent(rabbitmq.RetryableErrorf("db unavailable")) {
		t.Fatal("RetryableErrorf should not classify as permanent")
	}
}
```

- [ ] **Step 2: Run tests and verify they fail**

Run: `cd go/pkg && go test ./rabbitmq`

Expected: fail because `go/pkg/rabbitmq` does not exist.

- [ ] **Step 3: Implement retry helpers and typed errors**

Create `go/pkg/rabbitmq/reliability.go` with:

```go
package rabbitmq

import (
	"errors"
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	RetryCountHeader = "x-retry-count"
	DefaultMaxRetries = 3
)

type PermanentError struct{ err error }

func PermanentErrorf(format string, args ...any) error {
	return PermanentError{err: fmt.Errorf(format, args...)}
}

func (e PermanentError) Error() string { return e.err.Error() }
func (e PermanentError) Unwrap() error { return e.err }

type RetryableError struct{ err error }

func RetryableErrorf(format string, args ...any) error {
	return RetryableError{err: fmt.Errorf(format, args...)}
}

func (e RetryableError) Error() string { return e.err.Error() }
func (e RetryableError) Unwrap() error { return e.err }

func IsPermanent(err error) bool {
	var permanent PermanentError
	return errors.As(err, &permanent)
}

func RetryCount(headers amqp.Table) int {
	if headers == nil {
		return 0
	}
	switch v := headers[RetryCountHeader].(type) {
	case int:
		return v
	case int32:
		return int(v)
	case int64:
		return int(v)
	default:
		return 0
	}
}

func IncrementRetry(headers amqp.Table) amqp.Table {
	if headers == nil {
		headers = amqp.Table{}
	}
	headers[RetryCountHeader] = int32(RetryCount(headers) + 1)
	return headers
}
```

- [ ] **Step 4: Run tests and tidy shared module**

Run: `cd go/pkg && go test ./rabbitmq && go mod tidy`

Expected: pass.

- [ ] **Step 5: Commit**

```bash
git add go/pkg/rabbitmq go/pkg/go.mod go/pkg/go.sum
git commit -m "feat: add go rabbitmq reliability helpers"
```

## Task 2: Confirm Publisher Wrapper

**Files:**
- Create: `go/pkg/rabbitmq/publisher.go`
- Test: `go/pkg/rabbitmq/publisher_test.go`

- [ ] **Step 1: Write failing tests for mandatory confirmed publishing**

Write tests using a small fake adapter interface. Cover confirmed ack, confirmed nack, returned message, and context timeout.

```go
func TestPublisherReturnsErrorOnReturnedMessage(t *testing.T) {
	pub := rabbitmq.NewPublisher(fakeChannel{
		returned: amqp.Return{ReplyText: "NO_ROUTE", RoutingKey: "missing"},
		confirm: amqp.Confirmation{Ack: true},
	})
	err := pub.Publish(context.Background(), "exchange", "missing", amqp.Publishing{Body: []byte("body")})
	if err == nil || !strings.Contains(err.Error(), "returned") {
		t.Fatalf("expected returned-message error, got %v", err)
	}
}
```

- [ ] **Step 2: Run tests and verify they fail**

Run: `cd go/pkg && go test ./rabbitmq`

Expected: fail because `NewPublisher` is missing.

- [ ] **Step 3: Implement publisher wrapper**

Add a wrapper that calls `Confirm(false)`, subscribes to `NotifyPublish` and `NotifyReturn`, uses `mandatory=true`, sets `DeliveryMode=amqp.Persistent` when unset, and waits for ack/nack or context cancellation.

- [ ] **Step 4: Run tests**

Run: `cd go/pkg && go test ./rabbitmq`

Expected: pass.

- [ ] **Step 5: Commit**

```bash
git add go/pkg/rabbitmq
git commit -m "feat: add confirmed rabbitmq publisher"
```

## Task 3: Harden Go Saga Consumers

**Files:**
- Modify: `go/order-service/internal/saga/consumer.go`
- Modify: `go/order-service/internal/saga/topology.go`
- Modify: `go/order-service/internal/saga/metrics.go`
- Test: `go/order-service/internal/saga/consumer_test.go`

- [ ] **Step 1: Write failing tests for permanent, retryable, and exhausted failures**

Add tests that call a new decision helper:

```go
func TestAckDecisionPermanentErrorDeadLetters(t *testing.T) {
	headers := amqp.Table{}
	decision := saga.DecideFailure(headers, rabbitmq.PermanentErrorf("bad payload"), 3)
	if decision.Action != saga.FailureDeadLetter {
		t.Fatalf("action = %v, want dead-letter", decision.Action)
	}
	if decision.RetryCount != 0 {
		t.Fatalf("retry count = %d, want 0", decision.RetryCount)
	}
}

func TestAckDecisionRetryableRepublishesWithIncrementedHeader(t *testing.T) {
	headers := amqp.Table{rabbitmq.RetryCountHeader: int32(1)}
	decision := saga.DecideFailure(headers, rabbitmq.RetryableErrorf("db down"), 3)
	if decision.Action != saga.FailureRetryRepublish {
		t.Fatalf("action = %v, want retry republish", decision.Action)
	}
	if got := rabbitmq.RetryCount(decision.Headers); got != 2 {
		t.Fatalf("retry count = %d, want 2", got)
	}
}

func TestAckDecisionRetryableExhaustedDeadLetters(t *testing.T) {
	headers := amqp.Table{rabbitmq.RetryCountHeader: int32(3)}
	decision := saga.DecideFailure(headers, rabbitmq.RetryableErrorf("db down"), 3)
	if decision.Action != saga.FailureDeadLetter {
		t.Fatalf("action = %v, want dead-letter", decision.Action)
	}
}
```

- [ ] **Step 2: Run tests and verify they fail**

Run: `cd go/order-service && go test ./internal/saga`

Expected: fail because `DecideFailure` is missing.

- [ ] **Step 3: Implement QoS and failure decisions**

Add `ch.Qos(1, 0, false)` before `Consume`. Replace string matching with typed error classification. Permanent or exhausted failures call `msg.Nack(false, false)`. Retryable failures below `DefaultMaxRetries` must be republished through the confirmed publisher with `x-retry-count` incremented and the original delivery acked only after the retry publish succeeds. If retry republish fails, nack the original with `requeue=true` to avoid message loss.

- [ ] **Step 4: Convert parse errors to permanent errors**

In `handleMessage`, wrap JSON and UUID/event validation failures with `rabbitmq.PermanentErrorf`. Leave repository/downstream failures retryable unless they are already typed permanent errors.

- [ ] **Step 5: Run saga unit tests**

Run: `cd go/order-service && go test ./internal/saga`

Expected: pass.

- [ ] **Step 6: Commit**

```bash
git add go/order-service/internal/saga go/order-service/go.mod go/order-service/go.sum
git commit -m "feat: harden order saga consumer retries"
```

## Task 4: Harden Cart Saga Handler

**Files:**
- Modify: `go/cart-service/internal/worker/saga_handler.go`
- Test: `go/cart-service/internal/worker/saga_handler_test.go`

- [ ] **Step 1: Write failing tests for malformed command classification**

```go
func TestHandleMessageInvalidJSONIsPermanent(t *testing.T) {
	h := worker.NewSagaHandler(fakeCartService{}, nil)
	err := h.HandleMessageForTest(context.Background(), amqp.Delivery{Body: []byte("{")})
	if !rabbitmq.IsPermanent(err) {
		t.Fatalf("invalid json should be permanent, got %v", err)
	}
}
```

- [ ] **Step 2: Run tests and verify they fail**

Run: `cd go/cart-service && go test ./internal/worker`

Expected: fail because test helper/export or classification is missing.

- [ ] **Step 3: Implement classification, QoS, bounded retry republish behavior, and persistent replies**

Set `Qos(1, 0, false)` before consuming. Wrap JSON parse, UUID parse, and unknown command errors with `rabbitmq.PermanentErrorf`. Use the same failure decision helper pattern as order-service. Retryable failures below the max retry count are republished with incremented headers and confirmed before the original delivery is acked. Set `DeliveryMode: amqp.Persistent` for reply events.

- [ ] **Step 4: Run worker tests**

Run: `cd go/cart-service && go test ./internal/worker`

Expected: pass.

- [ ] **Step 5: Commit**

```bash
git add go/cart-service/internal/worker go/cart-service/go.mod go/cart-service/go.sum
git commit -m "feat: harden cart saga consumer retries"
```

## Task 5: Wire Publisher Confirms Into Saga And Outbox Publishers

**Files:**
- Modify: `go/order-service/internal/saga/publisher.go`
- Modify: `go/cart-service/internal/worker/saga_handler.go`
- Modify: `go/payment-service/internal/outbox/poller.go`
- Test: `go/order-service/internal/saga/publisher_test.go`
- Test: `go/payment-service/internal/outbox/poller_test.go`

- [ ] **Step 1: Write failing tests for persistent command publish and outbox confirm**

Assert saga commands and payment outbox messages use persistent delivery and that `MarkPublished` is not called when publish confirmation fails.

- [ ] **Step 2: Run tests and verify they fail**

Run:

```bash
cd go/order-service && go test ./internal/saga
cd ../payment-service && go test ./internal/outbox
```

Expected: fail before confirm publisher is wired.

- [ ] **Step 3: Update constructors to accept publisher interface**

Introduce narrow interfaces so unit tests can fake publishing without a broker:

```go
type ConfirmPublisher interface {
	Publish(ctx context.Context, exchange, routingKey string, msg amqp.Publishing) error
}
```

Keep compatibility constructors that wrap existing `*amqp.Channel` values.

- [ ] **Step 4: Publish via confirm wrapper**

Use mandatory confirmed publishing for saga commands, cart saga replies, and payment outbox messages. Leave `MarkPublished` after successful `Publish` only.

- [ ] **Step 5: Run tests**

Run:

```bash
cd go/order-service && go test ./internal/saga
cd ../cart-service && go test ./internal/worker
cd ../payment-service && go test ./internal/outbox
```

Expected: pass.

- [ ] **Step 6: Commit**

```bash
git add go/order-service/internal/saga go/cart-service/internal/worker go/payment-service/internal/outbox
git commit -m "feat: confirm rabbitmq saga and outbox publishes"
```

## Task 6: Broker Integration Tests For DLQ And Duplicate Safety

**Files:**
- Modify: `go/order-service/internal/integration/saga_test.go`
- Add optional test helpers under: `go/order-service/internal/integration/testutil/`

- [ ] **Step 1: Add malformed message DLQ integration test**

Publish malformed JSON to `saga.order.events`, start the consumer, and assert the message arrives in `ecommerce.saga.dlq` without remaining in the hot queue.

- [ ] **Step 2: Add duplicate event safety integration test**

Publish the same terminal saga reply twice and assert the order remains terminal with no backward transition.

- [ ] **Step 3: Run integration tests**

Run: `cd go/order-service && go test -tags=integration ./internal/integration -run 'TestSaga_.*DLQ|TestSaga_.*Duplicate'`

Expected: pass when Docker/Colima is available.

- [ ] **Step 4: Commit**

```bash
git add go/order-service/internal/integration
git commit -m "test: cover go rabbitmq dlq and duplicate delivery"
```

## Task 7: AMQP Reconnect And Channel Recovery

**Files:**
- Modify: `go/order-service/cmd/server/deps.go`
- Modify: `go/order-service/cmd/server/main.go`
- Modify: `go/cart-service/cmd/server/main.go`
- Modify: `go/payment-service/cmd/server/deps.go`
- Modify: `go/payment-service/cmd/server/main.go`
- Test: focused unit tests for retry/backoff helpers if extracted into a package-level helper

- [ ] **Step 1: Extract connection retry helper**

Create a helper that dials RabbitMQ with exponential backoff until context cancellation. Use `NotifyClose` on both connection and channel to trigger rebuilding topology, QoS, confirm mode, and consumers.

- [ ] **Step 2: Update order-service startup**

Replace single-shot `connectRabbitMQ` consumer startup with a supervised goroutine that reconnects, redeclares saga topology, rebuilds `saga.Publisher`, and restarts `saga.Consumer`. Shutdown should cancel the supervisor before closing the current connection.

- [ ] **Step 3: Update cart-service startup**

Replace single-shot AMQP setup with a supervised goroutine that reconnects, rebuilds the confirmed publisher, sets QoS, and restarts `worker.SagaHandler`.

- [ ] **Step 4: Update payment-service publisher channel setup**

Ensure the outbox poller receives a publisher abstraction that can reconnect after channel or connection close. During reconnect, publish attempts should fail fast or block on context according to the helper contract, but outbox rows must remain unpublished.

- [ ] **Step 5: Run service tests**

Run:

```bash
cd go/order-service && go test ./cmd/server ./internal/saga
cd ../cart-service && go test ./cmd/server ./internal/worker
cd ../payment-service && go test ./cmd/server ./internal/outbox
```

Expected: pass.

- [ ] **Step 6: Commit**

```bash
git add go/order-service/cmd/server go/cart-service/cmd/server go/payment-service/cmd/server
git commit -m "feat: recover go rabbitmq connections"
```

## Task 8: Go Preflight

**Files:**
- Verify all Go changes.

- [ ] **Step 1: Run targeted tests**

Run:

```bash
cd go/pkg && go test ./...
cd ../order-service && go test ./...
cd ../cart-service && go test ./...
cd ../payment-service && go test ./...
```

Expected: pass.

- [ ] **Step 2: Run project Go preflight**

Run: `make preflight-go`

Expected: pass. If blocked by Docker, toolchain, or platform limits, record the blocker and exact failing command.

- [ ] **Step 3: Commit any remaining fixes**

```bash
git status --short
git add <changed go files>
git commit -m "fix: complete go rabbitmq reliability preflight"
```
