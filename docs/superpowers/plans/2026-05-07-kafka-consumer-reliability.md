# Kafka Consumer Reliability Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Harden the Go Kafka consumers with at-least-once processing, idempotent projector sinks, DLQ quarantine, retry/backoff, and reliability metrics.

**Architecture:** Add a small shared `go/pkg/kafkaconsumer` package for DLQ publishing and retry helpers. Keep service-specific processing in `order-projector` and `analytics-service`; make `order-projector` strict and idempotent while keeping `analytics-service` bounded best effort with explicit DLQ and observability.

**Tech Stack:** Go 1.26.1, `segmentio/kafka-go`, PostgreSQL/pgx, Prometheus client, existing `go/pkg/resilience`, `golang-migrate`, Kubernetes ConfigMaps.

---

## File Structure

- Create `go/pkg/kafkaconsumer/dlq.go`: shared DLQ envelope and publisher.
- Create `go/pkg/kafkaconsumer/dlq_test.go`: unit coverage for DLQ envelopes and writer behavior.
- Create `go/pkg/kafkaconsumer/retry.go`: shared bounded retry/backoff helper for consumer loops.
- Create `go/pkg/kafkaconsumer/retry_test.go`: retry behavior coverage.
- Create `go/order-projector/migrations/002_processed_projection_events.up.sql`: processed-event guard table.
- Create `go/order-projector/migrations/002_processed_projection_events.down.sql`: rollback for the guard table.
- Modify `go/order-projector/internal/repository/projector.go`: atomic stats processed-event guard, stale summary protection.
- Modify `go/order-projector/internal/projection/*.go`: expose pure conversion helpers where needed, keep projection writes thin.
- Modify `go/order-projector/internal/consumer/consumer.go`: inject config, DLQ publisher, retry policy, and repository-level event processing.
- Modify `go/order-projector/internal/metrics/metrics.go`: add retry, commit, duplicate, DLQ, and duration metrics.
- Modify `go/order-projector/cmd/server/config.go`: add group, DLQ topic, and retry config defaults.
- Modify `go/order-projector/cmd/server/main.go`: wire the new consumer config and DLQ publisher.
- Modify `go/k8s/configmaps/order-projector-config.yml`: expose the new config defaults.
- Modify `go/analytics-service/internal/consumer/consumer.go`: add DLQ handling for invalid messages, fetch backoff, and flush visibility.
- Modify `go/analytics-service/internal/metrics/prometheus.go`: add DLQ, retry, invalid-event, commit-error, and last-flush metrics.
- Modify `go/analytics-service/cmd/server/config.go`: add group, DLQ topic, and retry config defaults.
- Modify `go/analytics-service/cmd/server/main.go`: wire the new consumer config and DLQ publisher.
- Modify `go/k8s/configmaps/analytics-service-config.yml`: expose the new config defaults.

## Task 1: Shared Kafka DLQ And Retry Helpers

**Files:**
- Create: `go/pkg/kafkaconsumer/dlq.go`
- Create: `go/pkg/kafkaconsumer/dlq_test.go`
- Create: `go/pkg/kafkaconsumer/retry.go`
- Create: `go/pkg/kafkaconsumer/retry_test.go`

- [ ] **Step 1: Write failing DLQ tests**

Create `go/pkg/kafkaconsumer/dlq_test.go`:

```go
package kafkaconsumer

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/segmentio/kafka-go"
)

type recordingWriter struct {
	msgs []kafka.Message
	err  error
}

func (w *recordingWriter) WriteMessages(_ context.Context, msgs ...kafka.Message) error {
	if w.err != nil {
		return w.err
	}
	w.msgs = append(w.msgs, msgs...)
	return nil
}

func TestDLQPublisherPublishesOriginalRecordAndError(t *testing.T) {
	t.Parallel()

	writer := &recordingWriter{}
	pub := NewDLQPublisher(writer, "orders.dlq", "order-projector-group")
	msg := kafka.Message{
		Topic:     "ecommerce.order-events",
		Partition: 2,
		Offset:    42,
		Key:       []byte("order-1"),
		Value:     []byte(`{"bad":true}`),
		Headers:   []kafka.Header{{Key: "traceparent", Value: []byte("trace")}},
		Time:      time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC),
	}

	err := pub.Publish(context.Background(), msg, "decode", errors.New("invalid json"))
	if err != nil {
		t.Fatalf("Publish returned error: %v", err)
	}
	if len(writer.msgs) != 1 {
		t.Fatalf("messages written = %d, want 1", len(writer.msgs))
	}
	if writer.msgs[0].Topic != "orders.dlq" {
		t.Fatalf("topic = %q, want orders.dlq", writer.msgs[0].Topic)
	}

	var env DLQEnvelope
	if err := json.Unmarshal(writer.msgs[0].Value, &env); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	if env.ConsumerGroup != "order-projector-group" {
		t.Errorf("consumer group = %q", env.ConsumerGroup)
	}
	if env.Source.Topic != "ecommerce.order-events" || env.Source.Partition != 2 || env.Source.Offset != 42 {
		t.Errorf("source = %+v", env.Source)
	}
	if env.ErrorClass != "decode" || env.ErrorMessage != "invalid json" {
		t.Errorf("error fields = %q %q", env.ErrorClass, env.ErrorMessage)
	}
	if string(env.Source.Value) != `{"bad":true}` {
		t.Errorf("source value = %s", env.Source.Value)
	}
}

func TestDLQPublisherReturnsWriterError(t *testing.T) {
	t.Parallel()

	writer := &recordingWriter{err: errors.New("broker unavailable")}
	pub := NewDLQPublisher(writer, "orders.dlq", "order-projector-group")

	err := pub.Publish(context.Background(), kafka.Message{Topic: "source"}, "decode", errors.New("bad"))
	if err == nil {
		t.Fatal("expected writer error")
	}
}
```

- [ ] **Step 2: Run DLQ test and verify it fails**

Run:

```bash
cd go/pkg && go test ./kafkaconsumer -run 'TestDLQPublisher' -count=1
```

Expected: fail because package `kafkaconsumer` does not exist.

- [ ] **Step 3: Implement DLQ publisher**

Create `go/pkg/kafkaconsumer/dlq.go`:

```go
package kafkaconsumer

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/segmentio/kafka-go"
)

type Writer interface {
	WriteMessages(ctx context.Context, msgs ...kafka.Message) error
}

type DLQSource struct {
	Topic     string         `json:"topic"`
	Partition int            `json:"partition"`
	Offset    int64          `json:"offset"`
	Key       []byte         `json:"key,omitempty"`
	Value     []byte         `json:"value"`
	Headers   []kafka.Header `json:"headers,omitempty"`
	Time      time.Time      `json:"time"`
}

type DLQEnvelope struct {
	Source        DLQSource `json:"source"`
	ConsumerGroup string    `json:"consumerGroup"`
	ErrorClass    string    `json:"errorClass"`
	ErrorMessage  string    `json:"errorMessage"`
	FailedAt      time.Time `json:"failedAt"`
}

type DLQPublisher struct {
	writer        Writer
	topic         string
	consumerGroup string
	now           func() time.Time
}

func NewDLQPublisher(writer Writer, topic string, consumerGroup string) *DLQPublisher {
	return &DLQPublisher{
		writer:        writer,
		topic:         topic,
		consumerGroup: consumerGroup,
		now:           time.Now,
	}
}

func (p *DLQPublisher) Publish(ctx context.Context, msg kafka.Message, errorClass string, cause error) error {
	env := DLQEnvelope{
		Source: DLQSource{
			Topic:     msg.Topic,
			Partition: msg.Partition,
			Offset:    msg.Offset,
			Key:       msg.Key,
			Value:     msg.Value,
			Headers:   msg.Headers,
			Time:      msg.Time,
		},
		ConsumerGroup: p.consumerGroup,
		ErrorClass:    errorClass,
		ErrorMessage:  cause.Error(),
		FailedAt:      p.now().UTC(),
	}
	value, err := json.Marshal(env)
	if err != nil {
		return fmt.Errorf("marshal dlq envelope: %w", err)
	}
	if err := p.writer.WriteMessages(ctx, kafka.Message{
		Topic: p.topic,
		Key:   msg.Key,
		Value: value,
	}); err != nil {
		return fmt.Errorf("publish dlq message: %w", err)
	}
	return nil
}
```

- [ ] **Step 4: Write failing retry tests**

Create `go/pkg/kafkaconsumer/retry_test.go`:

```go
package kafkaconsumer

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRetryEventuallySucceeds(t *testing.T) {
	t.Parallel()

	attempts := 0
	cfg := RetryConfig{MaxAttempts: 3, BaseDelay: time.Nanosecond, MaxDelay: time.Nanosecond}
	err := Retry(context.Background(), cfg, func(context.Context) error {
		attempts++
		if attempts < 2 {
			return errors.New("temporary")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Retry returned error: %v", err)
	}
	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2", attempts)
	}
}

func TestRetryStopsOnNonRetryableError(t *testing.T) {
	t.Parallel()

	attempts := 0
	errBadRecord := errors.New("bad record")
	cfg := RetryConfig{
		MaxAttempts: 3,
		BaseDelay:   time.Nanosecond,
		MaxDelay:    time.Nanosecond,
		IsRetryable: func(err error) bool { return !errors.Is(err, errBadRecord) },
	}
	err := Retry(context.Background(), cfg, func(context.Context) error {
		attempts++
		return errBadRecord
	})
	if !errors.Is(err, errBadRecord) {
		t.Fatalf("error = %v, want bad record", err)
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1", attempts)
	}
}
```

- [ ] **Step 5: Implement retry helper**

Create `go/pkg/kafkaconsumer/retry.go`:

```go
package kafkaconsumer

import (
	"context"
	"time"

	"github.com/kabradshaw1/portfolio/go/pkg/resilience"
)

type RetryConfig = resilience.RetryConfig

func DefaultRetryConfig() RetryConfig {
	return resilience.DefaultRetryConfig()
}

func Retry(ctx context.Context, cfg RetryConfig, fn func(context.Context) error) error {
	_, err := resilience.Retry(ctx, cfg, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, fn(ctx)
	})
	return err
}

func SleepWithContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
```

- [ ] **Step 6: Run shared package tests**

Run:

```bash
cd go/pkg && go test ./kafkaconsumer -count=1
```

Expected: pass.

- [ ] **Step 7: Commit shared helper package**

Run:

```bash
git add go/pkg/kafkaconsumer go/pkg/go.mod go/pkg/go.sum
git commit -m "feat: add kafka consumer reliability helpers"
```

## Task 2: Order Projector Idempotent Persistence

**Files:**
- Create: `go/order-projector/migrations/002_processed_projection_events.up.sql`
- Create: `go/order-projector/migrations/002_processed_projection_events.down.sql`
- Modify: `go/order-projector/internal/repository/projector.go`
- Modify: `go/order-projector/internal/repository/projector_test.go`

- [ ] **Step 1: Add migration files**

Create `go/order-projector/migrations/002_processed_projection_events.up.sql`:

```sql
CREATE TABLE processed_projection_events (
    projection_name TEXT NOT NULL,
    event_id        UUID NOT NULL,
    processed_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (projection_name, event_id)
);

CREATE INDEX idx_processed_projection_events_processed_at
    ON processed_projection_events(processed_at);
```

Create `go/order-projector/migrations/002_processed_projection_events.down.sql`:

```sql
DROP TABLE IF EXISTS processed_projection_events;
```

- [ ] **Step 2: Write repository SQL tests**

Append to `go/order-projector/internal/repository/projector_test.go`:

```go
func TestRepositoryQueriesIncludeProcessedProjectionGuard(t *testing.T) {
	t.Parallel()

	if !strings.Contains(upsertOrderStatsOnceSQL, "processed_projection_events") {
		t.Fatal("stats upsert guard SQL does not reference processed_projection_events")
	}
	if !strings.Contains(upsertOrderStatsOnceSQL, "WITH inserted AS") {
		t.Fatal("stats upsert guard SQL must atomically guard and update")
	}
}

func TestOrderSummaryUpsertRejectsOlderUpdates(t *testing.T) {
	t.Parallel()

	if !strings.Contains(upsertOrderSummarySQL, "WHERE order_summary.updated_at <= EXCLUDED.updated_at") {
		t.Fatal("summary upsert must not overwrite newer rows with stale events")
	}
}
```

- [ ] **Step 3: Run repository tests and verify failure**

Run:

```bash
cd go/order-projector && go test ./internal/repository -run 'TestRepositoryQueriesIncludeProcessedProjectionGuard|TestOrderSummaryUpsertRejectsOlderUpdates' -count=1
```

Expected: fail because `upsertOrderStatsOnceSQL` is undefined and summary SQL has no stale-event guard.

- [ ] **Step 4: Add atomic stats guard SQL and stale summary guard**

Modify `go/order-projector/internal/repository/projector.go` constants:

```go
	upsertOrderStatsOnceSQL = `WITH inserted AS (
			   INSERT INTO processed_projection_events (projection_name, event_id)
			   VALUES ('stats', $1)
			   ON CONFLICT (projection_name, event_id) DO NOTHING
			   RETURNING 1
			 )
			 INSERT INTO order_stats (hour_bucket, orders_created, orders_completed, orders_failed, total_revenue_cents)
			 SELECT $2, $3, $4, $5, $6 FROM inserted
			 ON CONFLICT (hour_bucket) DO UPDATE SET
			   orders_created      = order_stats.orders_created + EXCLUDED.orders_created,
			   orders_completed    = order_stats.orders_completed + EXCLUDED.orders_completed,
			   orders_failed       = order_stats.orders_failed + EXCLUDED.orders_failed,
			   total_revenue_cents = order_stats.total_revenue_cents + EXCLUDED.total_revenue_cents`
```

Change the `upsertOrderSummarySQL` conflict clause to:

```sql
			 ON CONFLICT (order_id) DO UPDATE SET
			   user_id        = COALESCE(EXCLUDED.user_id, order_summary.user_id),
			   status         = EXCLUDED.status,
			   total_cents    = COALESCE(EXCLUDED.total_cents, order_summary.total_cents),
			   currency       = COALESCE(EXCLUDED.currency, order_summary.currency),
			   items_json     = COALESCE(EXCLUDED.items_json, order_summary.items_json),
			   updated_at     = EXCLUDED.updated_at,
			   completed_at   = COALESCE(EXCLUDED.completed_at, order_summary.completed_at),
			   failure_reason = COALESCE(EXCLUDED.failure_reason, order_summary.failure_reason)
			 WHERE order_summary.updated_at <= EXCLUDED.updated_at`
```

- [ ] **Step 5: Add atomic stats method**

Add to `go/order-projector/internal/repository/projector.go`:

```go
func (r *Repository) UpsertOrderStatsOnce(ctx context.Context, eventID string, bucket time.Time, created, completed, failed int, revenueCents int64) (bool, error) {
	return resilience.Call(ctx, r.breaker, r.retryCfg, func(ctx context.Context) (bool, error) {
		tag, err := r.pool.Exec(ctx, upsertOrderStatsOnceSQL, eventID, bucket, created, completed, failed, revenueCents)
		if err != nil {
			return false, fmt.Errorf("upsert order stats once: %w", err)
		}
		return tag.RowsAffected() == 1, nil
	})
}
```

- [ ] **Step 6: Run repository tests**

Run:

```bash
cd go/order-projector && go test ./internal/repository -count=1
```

Expected: pass.

- [ ] **Step 7: Run migration lint**

Run:

```bash
make preflight-go-migrations
```

Expected: pass. If Docker or Colima is unavailable, capture the exact blocker and continue with Go unit tests.

- [ ] **Step 8: Commit projector persistence changes**

Run:

```bash
git add go/order-projector/migrations go/order-projector/internal/repository
git commit -m "feat: add projector processed-event guard"
```

## Task 3: Strict Order Projector Consumer Flow

**Files:**
- Modify: `go/order-projector/internal/consumer/consumer.go`
- Modify: `go/order-projector/internal/metrics/metrics.go`
- Modify: `go/order-projector/internal/consumer/consumer_test.go`
- Modify: `go/order-projector/cmd/server/config.go`
- Modify: `go/order-projector/cmd/server/main.go`

- [ ] **Step 1: Write consumer unit tests with fake reader and DLQ**

Create `go/order-projector/internal/consumer/consumer_test.go` with fake reader and repository interfaces:

```go
package consumer

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/segmentio/kafka-go"
)

type fakeReader struct {
	msg        kafka.Message
	fetchErr   error
	committed  bool
	commitErr  error
	fetchCalled bool
}

func (r *fakeReader) FetchMessage(context.Context) (kafka.Message, error) {
	r.fetchCalled = true
	return r.msg, r.fetchErr
}

func (r *fakeReader) CommitMessages(context.Context, ...kafka.Message) error {
	if r.commitErr != nil {
		return r.commitErr
	}
	r.committed = true
	return nil
}

func (r *fakeReader) Close() error { return nil }

type fakeProcessor struct {
	err       error
	received bool
}

func (p *fakeProcessor) ProcessOrderEvent(context.Context, *OrderEventMessage) error {
	p.received = true
	return p.err
}

type fakeDLQ struct {
	err       error
	published bool
}

func (d *fakeDLQ) Publish(context.Context, kafka.Message, string, error) error {
	if d.err != nil {
		return d.err
	}
	d.published = true
	return nil
}

func TestProcessOneDoesNotCommitOnProcessingError(t *testing.T) {
	reader := &fakeReader{msg: kafka.Message{Value: validOrderEventJSON(t)}}
	processor := &fakeProcessor{err: errors.New("postgres unavailable")}
	cons := NewWithDependencies(reader, processor, nil, Config{
		GroupID:     "order-projector-group",
		RetryConfig: RetryConfig{MaxAttempts: 1, BaseDelay: time.Nanosecond, MaxDelay: time.Nanosecond},
	})

	err := cons.processOne(context.Background())
	if err == nil {
		t.Fatal("expected processing error")
	}
	if reader.committed {
		t.Fatal("source offset was committed after processing failure")
	}
}

func TestProcessOneCommitsAfterPoisonRecordDLQ(t *testing.T) {
	reader := &fakeReader{msg: kafka.Message{Value: []byte(`{bad json`)}}
	dlq := &fakeDLQ{}
	cons := NewWithDependencies(reader, &fakeProcessor{}, dlq, Config{
		GroupID:     "order-projector-group",
		RetryConfig: RetryConfig{MaxAttempts: 1, BaseDelay: time.Nanosecond, MaxDelay: time.Nanosecond},
	})

	err := cons.processOne(context.Background())
	if err != nil {
		t.Fatalf("processOne returned error: %v", err)
	}
	if !dlq.published {
		t.Fatal("poison record was not published to DLQ")
	}
	if !reader.committed {
		t.Fatal("source offset was not committed after DLQ publish")
	}
}
```

Add helper `validOrderEventJSON` in the same test file:

```go
func validOrderEventJSON(t *testing.T) []byte {
	t.Helper()
	return []byte(`{
		"id":"00000000-0000-0000-0000-000000000001",
		"type":"order.created",
		"version":2,
		"source":"order-service",
		"order_id":"00000000-0000-0000-0000-000000000002",
		"timestamp":"2026-05-07T12:00:00Z",
		"traceID":"trace-1",
		"data":{"userID":"00000000-0000-0000-0000-000000000003","totalCents":1200,"currency":"USD","items":[]}
	}`)
}
```

- [ ] **Step 2: Run consumer tests and verify failure**

Run:

```bash
cd go/order-projector && go test ./internal/consumer -run 'TestProcessOne' -count=1
```

Expected: fail because fakeable dependencies and `processOne` do not exist.

- [ ] **Step 3: Refactor consumer dependencies**

Modify `go/order-projector/internal/consumer/consumer.go` to define these interfaces and config:

```go
type Reader interface {
	FetchMessage(ctx context.Context) (kafka.Message, error)
	CommitMessages(ctx context.Context, msgs ...kafka.Message) error
	Close() error
}

type DLQPublisher interface {
	Publish(ctx context.Context, msg kafka.Message, errorClass string, cause error) error
}

type EventProcessor interface {
	ProcessOrderEvent(ctx context.Context, msg *OrderEventMessage) error
}

type RetryConfig = kafkaconsumer.RetryConfig

type Config struct {
	GroupID     string
	RetryConfig RetryConfig
	FetchBackoff time.Duration
}

type OrderEventMessage struct {
	Event   *event.OrderEvent
	Message kafka.Message
}
```

Change `Consumer.reader` to the `Reader` interface and add `processor`, `dlq`, and `cfg` fields.

- [ ] **Step 4: Add constructors**

Add to `go/order-projector/internal/consumer/consumer.go`:

```go
func New(brokers []string, repo *repository.Repository, cfg Config, dlq DLQPublisher) *Consumer {
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:  brokers,
		GroupID:  cfg.GroupID,
		Topic:   "ecommerce.order-events",
		MinBytes: 1,
		MaxBytes: 10e6,
	})
	return NewWithDependencies(reader, NewRepositoryProcessor(repo), dlq, cfg)
}

func NewWithDependencies(reader Reader, processor EventProcessor, dlq DLQPublisher, cfg Config) *Consumer {
	if cfg.GroupID == "" {
		cfg.GroupID = "order-projector-group"
	}
	if cfg.RetryConfig.MaxAttempts == 0 {
		cfg.RetryConfig = kafkaconsumer.DefaultRetryConfig()
	}
	if cfg.FetchBackoff == 0 {
		cfg.FetchBackoff = 250 * time.Millisecond
	}
	return &Consumer{reader: reader, processor: processor, dlq: dlq, cfg: cfg}
}
```

- [ ] **Step 5: Add `processOne` behavior**

Add to `go/order-projector/internal/consumer/consumer.go`:

```go
func (c *Consumer) processOne(ctx context.Context) error {
	msg, err := c.reader.FetchMessage(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		metrics.ConsumerErrors.Inc()
		metrics.FetchErrors.WithLabelValues(c.cfg.GroupID, "ecommerce.order-events").Inc()
		return err
	}

	c.connected.Store(true)
	c.processing.Store(true)
	defer c.processing.Store(false)

	msgCtx := tracing.ExtractKafka(ctx, msg.Headers)
	evt, err := Deserialize(msg.Value)
	if err != nil {
		metrics.ConsumerErrors.Inc()
		if c.dlq == nil {
			return err
		}
		if dlqErr := c.dlq.Publish(ctx, msg, "decode", err); dlqErr != nil {
			metrics.DLQPublishErrors.WithLabelValues(c.cfg.GroupID, msg.Topic).Inc()
			return dlqErr
		}
		metrics.DLQPublished.WithLabelValues(c.cfg.GroupID, msg.Topic).Inc()
		return c.commit(ctx, msg)
	}

	err = kafkaconsumer.Retry(msgCtx, c.cfg.RetryConfig, func(ctx context.Context) error {
		return c.processor.ProcessOrderEvent(ctx, &OrderEventMessage{Event: evt, Message: msg})
	})
	if err != nil {
		return err
	}

	c.latestTS.Store(evt.Timestamp)
	metrics.ProjectionLag.Set(time.Since(evt.Timestamp).Seconds())
	metrics.EventsConsumed.WithLabelValues(evt.Type).Inc()
	return c.commit(ctx, msg)
}

func (c *Consumer) commit(ctx context.Context, msg kafka.Message) error {
	if err := c.reader.CommitMessages(ctx, msg); err != nil {
		metrics.CommitErrors.WithLabelValues(c.cfg.GroupID, msg.Topic).Inc()
		return err
	}
	return nil
}
```

- [ ] **Step 6: Update Run loop with fetch backoff**

Replace the body of `Run` with:

```go
func (c *Consumer) Run(ctx context.Context) error {
	slog.Info("kafka consumer starting", "topic", "ecommerce.order-events", "group", c.cfg.GroupID)
	for {
		err := c.processOne(ctx)
		if err == nil {
			continue
		}
		if ctx.Err() != nil {
			return nil
		}
		slog.Error("kafka consumer iteration failed", "error", err)
		_ = kafkaconsumer.SleepWithContext(ctx, c.cfg.FetchBackoff)
	}
}
```

- [ ] **Step 7: Add metrics**

Add to `go/order-projector/internal/metrics/metrics.go`:

```go
FetchErrors = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "projector_consumer_fetch_errors_total",
	Help: "Kafka fetch errors by group and topic",
}, []string{"group", "topic"})

CommitErrors = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "projector_consumer_commit_errors_total",
	Help: "Kafka commit errors by group and topic",
}, []string{"group", "topic"})

DLQPublished = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "projector_consumer_dlq_published_total",
	Help: "Kafka records published to DLQ by group and source topic",
}, []string{"group", "topic"})

DLQPublishErrors = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "projector_consumer_dlq_publish_errors_total",
	Help: "Kafka DLQ publish errors by group and source topic",
}, []string{"group", "topic"})

DuplicateEvents = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "projector_duplicate_events_total",
	Help: "Duplicate projector events skipped by projection",
}, []string{"projection"})

ProjectionDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
	Name:    "projector_projection_duration_seconds",
	Help:    "Projection processing duration by projection and event type",
	Buckets: prometheus.DefBuckets,
}, []string{"projection", "event_type"})
```

- [ ] **Step 8: Wire config and DLQ in main**

Add fields to `go/order-projector/cmd/server/config.go`:

```go
KafkaGroupID       string
KafkaDLQTopic      string
KafkaRetryAttempts int
KafkaRetryBaseDelay time.Duration
KafkaRetryMaxDelay  time.Duration
KafkaFetchBackoff   time.Duration
```

Add parsing helpers for int and duration, matching the existing analytics duration parser style. Use defaults:

```go
KafkaGroupID:         getenv("KAFKA_GROUP_ID", "order-projector-group"),
KafkaDLQTopic:        getenv("KAFKA_DLQ_TOPIC", "ecommerce.order-events.dlq"),
KafkaRetryAttempts:   getenvInt("KAFKA_RETRY_ATTEMPTS", 3),
KafkaRetryBaseDelay:  getenvDuration("KAFKA_RETRY_BASE_DELAY", 100*time.Millisecond),
KafkaRetryMaxDelay:   getenvDuration("KAFKA_RETRY_MAX_DELAY", 2*time.Second),
KafkaFetchBackoff:    getenvDuration("KAFKA_FETCH_BACKOFF", 250*time.Millisecond),
```

In `go/order-projector/cmd/server/main.go`, create a DLQ writer and pass config:

```go
dlqWriter := &kafka.Writer{
	Addr:     kafka.TCP(brokers...),
	Topic:    cfg.KafkaDLQTopic,
	Balancer: &kafka.LeastBytes{},
}
defer dlqWriter.Close()
dlqPublisher := kafkaconsumer.NewDLQPublisher(dlqWriter, cfg.KafkaDLQTopic, cfg.KafkaGroupID)
cons := consumer.New(brokers, repo, consumer.Config{
	GroupID: cfg.KafkaGroupID,
	RetryConfig: kafkaconsumer.RetryConfig{
		MaxAttempts: cfg.KafkaRetryAttempts,
		BaseDelay:   cfg.KafkaRetryBaseDelay,
		MaxDelay:    cfg.KafkaRetryMaxDelay,
	},
	FetchBackoff: cfg.KafkaFetchBackoff,
}, dlqPublisher)
```

- [ ] **Step 9: Run tests**

Run:

```bash
cd go/order-projector && go test ./internal/consumer ./internal/repository ./internal/projection ./cmd/server -count=1
```

Expected: pass.

- [ ] **Step 10: Commit strict projector consumer flow**

Run:

```bash
git add go/order-projector
git commit -m "feat: harden order projector kafka consumer"
```

## Task 4: Repository Processor For Idempotent Projection Application

**Files:**
- Modify: `go/order-projector/internal/consumer/consumer.go`
- Modify: `go/order-projector/internal/repository/projector.go`
- Modify: `go/order-projector/internal/projection/timeline.go`
- Modify: `go/order-projector/internal/projection/summary.go`
- Modify: `go/order-projector/internal/projection/stats.go`
- Modify: `go/order-projector/internal/projection/projection_test.go`

- [ ] **Step 1: Write duplicate stats behavior test**

Append to `go/order-projector/internal/projection/projection_test.go`:

```go
func TestProjectionNamesAreStable(t *testing.T) {
	t.Parallel()

	if ProjectionTimeline != "timeline" {
		t.Fatalf("ProjectionTimeline = %q", ProjectionTimeline)
	}
	if ProjectionSummary != "summary" {
		t.Fatalf("ProjectionSummary = %q", ProjectionSummary)
	}
	if ProjectionStats != "stats" {
		t.Fatalf("ProjectionStats = %q", ProjectionStats)
	}
}
```

- [ ] **Step 2: Add projection names**

Add to a new or existing projection file:

```go
const (
	ProjectionTimeline = "timeline"
	ProjectionSummary  = "summary"
	ProjectionStats    = "stats"
)
```

- [ ] **Step 3: Add processor implementation**

Add to `go/order-projector/internal/consumer/consumer.go`:

```go
type RepositoryProcessor struct {
	repo     *repository.Repository
	timeline *projection.Timeline
	summary  *projection.Summary
	stats    *projection.Stats
}

func NewRepositoryProcessor(repo *repository.Repository) *RepositoryProcessor {
	return &RepositoryProcessor{
		repo:     repo,
		timeline: projection.NewTimeline(repo),
		summary:  projection.NewSummary(repo),
		stats:    projection.NewStats(repo),
	}
}

func (p *RepositoryProcessor) ProcessOrderEvent(ctx context.Context, msg *OrderEventMessage) error {
	evt := msg.Event
	start := time.Now()
	if err := p.timeline.Apply(ctx, evt); err != nil {
		return err
	}
	metrics.ProjectionDuration.WithLabelValues(projection.ProjectionTimeline, evt.Type).Observe(time.Since(start).Seconds())

	start = time.Now()
	if err := p.summary.Apply(ctx, evt); err != nil {
		return err
	}
	metrics.ProjectionDuration.WithLabelValues(projection.ProjectionSummary, evt.Type).Observe(time.Since(start).Seconds())

	start = time.Now()
	if err := p.stats.Apply(ctx, evt); err != nil {
		return err
	}
	metrics.ProjectionDuration.WithLabelValues(projection.ProjectionStats, evt.Type).Observe(time.Since(start).Seconds())
	return nil
}
```

- [ ] **Step 4: Make stats use the atomic processed-event guard**

Modify `go/order-projector/internal/projection/stats.go` so every relevant event calls `UpsertOrderStatsOnce` with `evt.ID`:

```go
func (s *Stats) Apply(ctx context.Context, evt *event.OrderEvent) error {
	bucket := evt.Timestamp.Truncate(time.Hour)

	switch evt.Type {
	case "order.created":
		inserted, err := s.repo.UpsertOrderStatsOnce(ctx, evt.ID, bucket, 1, 0, 0, 0)
		if err == nil && !inserted {
			metrics.DuplicateEvents.WithLabelValues(ProjectionStats).Inc()
		}
		return err

	case "order.completed":
		var d completedData
		_ = json.Unmarshal(evt.Data, &d)
		inserted, err := s.repo.UpsertOrderStatsOnce(ctx, evt.ID, bucket, 0, 1, 0, d.TotalCents)
		if err == nil && !inserted {
			metrics.DuplicateEvents.WithLabelValues(ProjectionStats).Inc()
		}
		return err

	case "order.failed":
		inserted, err := s.repo.UpsertOrderStatsOnce(ctx, evt.ID, bucket, 0, 0, 1, 0)
		if err == nil && !inserted {
			metrics.DuplicateEvents.WithLabelValues(ProjectionStats).Inc()
		}
		return err

	default:
		return nil
	}
}
```

This keeps the guard and counter update in one SQL statement. If the statement fails, the Kafka offset is not committed and the event is retried.

- [ ] **Step 5: Run projector tests**

Run:

```bash
cd go/order-projector && go test ./internal/consumer ./internal/projection ./internal/repository -count=1
```

Expected: pass.

- [ ] **Step 6: Commit idempotent processor**

Run:

```bash
git add go/order-projector/internal
git commit -m "feat: make projector projections idempotent"
```

## Task 5: Analytics DLQ And Flush Observability

**Files:**
- Modify: `go/analytics-service/internal/consumer/consumer.go`
- Modify: `go/analytics-service/internal/consumer/consumer_test.go`
- Modify: `go/analytics-service/internal/metrics/prometheus.go`
- Modify: `go/analytics-service/cmd/server/config.go`
- Modify: `go/analytics-service/cmd/server/main.go`

- [ ] **Step 1: Write analytics invalid-message test**

Append to `go/analytics-service/internal/consumer/consumer_test.go`:

```go
type testDLQPublisher struct {
	published bool
	err       error
}

func (p *testDLQPublisher) Publish(_ context.Context, _ kafka.Message, _ string, _ error) error {
	if p.err != nil {
		return p.err
	}
	p.published = true
	return nil
}

func TestRouteInvalidEnvelopePublishesDLQ(t *testing.T) {
	c, _, _ := testConsumer(t)
	dlq := &testDLQPublisher{}
	c.dlq = dlq

	ok := c.route(context.Background(), kafka.Message{Topic: TopicOrders, Value: []byte(`{bad json`)})
	if ok {
		t.Fatal("invalid envelope should not be treated as processed analytics event")
	}
	if !dlq.published {
		t.Fatal("invalid envelope was not published to DLQ")
	}
}
```

- [ ] **Step 2: Run analytics consumer test and verify failure**

Run:

```bash
cd go/analytics-service && go test ./internal/consumer -run TestRouteInvalidEnvelopePublishesDLQ -count=1
```

Expected: fail because `Consumer.dlq` and the new `route` signature do not exist.

- [ ] **Step 3: Add analytics consumer config and DLQ field**

Modify `go/analytics-service/internal/consumer/consumer.go`:

```go
type DLQPublisher interface {
	Publish(ctx context.Context, msg kafka.Message, errorClass string, cause error) error
}

type Config struct {
	GroupID      string
	RetryConfig  kafkaconsumer.RetryConfig
	FetchBackoff time.Duration
}
```

Add `dlq DLQPublisher` and `cfg Config` fields to `Consumer`.

- [ ] **Step 4: Change route to return processing status**

Change `route` signature:

```go
func (c *Consumer) route(ctx context.Context, msg kafka.Message) bool
```

For invalid envelope:

```go
if err := json.Unmarshal(msg.Value, &env); err != nil {
	slog.Error("unmarshal event", "topic", msg.Topic, "error", err)
	metrics.InvalidEvents.WithLabelValues(msg.Topic).Inc()
	if c.dlq != nil {
		if dlqErr := c.dlq.Publish(ctx, msg, "decode", err); dlqErr != nil {
			metrics.DLQPublishErrors.WithLabelValues(c.cfg.GroupID, msg.Topic).Inc()
			return false
		}
		metrics.DLQPublished.WithLabelValues(c.cfg.GroupID, msg.Topic).Inc()
	}
	return false
}
```

Return `true` after valid event routing reaches the metrics increment.

- [ ] **Step 5: Add analytics metrics**

Add to `go/analytics-service/internal/metrics/prometheus.go`:

```go
CommitErrors = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "analytics_kafka_commit_errors_total",
	Help: "Kafka commit errors by group and topic.",
}, []string{"group", "topic"})

InvalidEvents = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "analytics_invalid_events_total",
	Help: "Invalid Kafka events by source topic.",
}, []string{"topic"})

DLQPublished = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "analytics_kafka_dlq_published_total",
	Help: "Kafka records published to DLQ by group and source topic.",
}, []string{"group", "topic"})

DLQPublishErrors = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "analytics_kafka_dlq_publish_errors_total",
	Help: "Kafka DLQ publish errors by group and source topic.",
}, []string{"group", "topic"})

LastSuccessfulFlushTimestamp = promauto.NewGaugeVec(prometheus.GaugeOpts{
	Name: "analytics_last_successful_flush_timestamp",
	Help: "Unix timestamp of the last successful window flush by aggregator.",
}, []string{"aggregator"})
```

In `flushAll`, set the timestamp after each successful flush:

```go
metrics.LastSuccessfulFlushTimestamp.WithLabelValues("revenue").Set(float64(time.Now().Unix()))
```

Repeat with labels `trending` and `abandonment`.

- [ ] **Step 6: Wire analytics config and DLQ in main**

Add fields to `go/analytics-service/cmd/server/config.go`:

```go
KafkaGroupID       string
KafkaDLQTopic      string
KafkaRetryAttempts int
KafkaRetryBaseDelay time.Duration
KafkaRetryMaxDelay  time.Duration
KafkaFetchBackoff   time.Duration
```

Use defaults:

```go
KafkaGroupID:         getenv("KAFKA_GROUP_ID", "analytics-group"),
KafkaDLQTopic:        getenv("KAFKA_DLQ_TOPIC", "ecommerce.analytics.dlq"),
KafkaRetryAttempts:   getenvInt("KAFKA_RETRY_ATTEMPTS", 3),
KafkaRetryBaseDelay:  getenvDuration("KAFKA_RETRY_BASE_DELAY", 100*time.Millisecond),
KafkaRetryMaxDelay:   getenvDuration("KAFKA_RETRY_MAX_DELAY", 2*time.Second),
KafkaFetchBackoff:    getenvDuration("KAFKA_FETCH_BACKOFF", 250*time.Millisecond),
```

In `go/analytics-service/cmd/server/main.go`, create a DLQ writer and pass it to the consumer constructor using the same `kafkaconsumer.NewDLQPublisher` pattern as Task 3.

- [ ] **Step 7: Run analytics tests**

Run:

```bash
cd go/analytics-service && go test ./internal/consumer ./internal/metrics ./cmd/server -count=1
```

Expected: pass.

- [ ] **Step 8: Commit analytics hardening**

Run:

```bash
git add go/analytics-service
git commit -m "feat: add analytics kafka dlq observability"
```

## Task 6: Kubernetes Config And Final Verification

**Files:**
- Modify: `go/k8s/configmaps/order-projector-config.yml`
- Modify: `go/k8s/configmaps/analytics-service-config.yml`
- Modify: `go/pkg/go.mod`
- Modify: `go/order-projector/go.mod`
- Modify: `go/analytics-service/go.mod`

- [ ] **Step 1: Add config map values**

Add to `go/k8s/configmaps/order-projector-config.yml`:

```yaml
  KAFKA_GROUP_ID: "order-projector-group"
  KAFKA_DLQ_TOPIC: "ecommerce.order-events.dlq"
  KAFKA_RETRY_ATTEMPTS: "3"
  KAFKA_RETRY_BASE_DELAY: "100ms"
  KAFKA_RETRY_MAX_DELAY: "2s"
  KAFKA_FETCH_BACKOFF: "250ms"
```

Add to `go/k8s/configmaps/analytics-service-config.yml`:

```yaml
  KAFKA_GROUP_ID: "analytics-group"
  KAFKA_DLQ_TOPIC: "ecommerce.analytics.dlq"
  KAFKA_RETRY_ATTEMPTS: "3"
  KAFKA_RETRY_BASE_DELAY: "100ms"
  KAFKA_RETRY_MAX_DELAY: "2s"
  KAFKA_FETCH_BACKOFF: "250ms"
```

- [ ] **Step 2: Tidy changed Go modules**

Run:

```bash
cd go/pkg && go mod tidy
cd ../order-projector && go mod tidy
cd ../analytics-service && go mod tidy
```

Expected: commands complete without errors.

- [ ] **Step 3: Run focused Go tests**

Run:

```bash
cd go/pkg && go test ./...
cd ../order-projector && go test ./...
cd ../analytics-service && go test ./...
```

Expected: all package tests pass.

- [ ] **Step 4: Run required preflights**

Run:

```bash
make preflight-go
make preflight-go-migrations
```

Expected: both pass. If `preflight-go-migrations` is blocked by Docker or Colima, record the exact blocker and keep the passing unit-test evidence.

- [ ] **Step 5: Commit config and verification fixes**

Run:

```bash
git add go/k8s/configmaps go/pkg/go.mod go/pkg/go.sum go/order-projector/go.mod go/order-projector/go.sum go/analytics-service/go.mod go/analytics-service/go.sum
git commit -m "chore: configure kafka consumer reliability defaults"
```

## Self-Review Checklist

- Spec coverage: projector strict at-least-once, idempotent stats, stale summary guard, DLQ quarantine, analytics bounded best effort, metrics, config, and tests are covered.
- Java scope: no Java files are modified because Java has RabbitMQ listeners, not Kafka consumers.
- Red-flag scan: this plan contains no deferred work markers or undefined future tasks.
- Type consistency: shared `kafkaconsumer` types are introduced before service consumers depend on them.
