# Kafka Event Sourcing + CQRS Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add event sourcing to the order domain (publishing every saga state transition to Kafka) and build a separate CQRS projection service that consumes the event stream and serves read-optimized views.

**Architecture:** The order-service saga orchestrator publishes versioned domain events to a compacted `ecommerce.order-events` Kafka topic at each state transition. A new `order-projector` Go service consumes these events via its own consumer group and builds three PostgreSQL read models (timeline, summary, stats). The frontend renders an order event timeline, stats dashboard, and consistency indicators.

**Tech Stack:** Go 1.26, segmentio/kafka-go, pgxpool, Gin, Prometheus, OpenTelemetry, Next.js, shadcn/ui, Tailwind CSS

**Spec:** `docs/superpowers/specs/2026-04-23-kafka-event-sourcing-cqrs-design.md`

---

## File Structure

### Order-Service (modified)

- `go/order-service/internal/events/types.go` — Event type constants, data structs, version constants
- `go/order-service/internal/events/publisher.go` — Wraps kafka.Producer with event envelope construction
- `go/order-service/internal/saga/orchestrator.go` — Modified to call events.Publish at each saga step
- `go/order-service/internal/events/publisher_test.go` — Unit tests for event publisher

### Order-Projector (new service)

- `go/order-projector/cmd/server/main.go` — Entrypoint: tracing, Postgres, Kafka consumer, HTTP, shutdown
- `go/order-projector/cmd/server/config.go` — Config from env vars
- `go/order-projector/cmd/server/routes.go` — Gin router setup
- `go/order-projector/internal/consumer/consumer.go` — Kafka consumer with message loop
- `go/order-projector/internal/consumer/deserializer.go` — Version-aware JSON deserialization
- `go/order-projector/internal/consumer/deserializer_test.go` — Tests for version upgrades
- `go/order-projector/internal/projection/timeline.go` — Timeline projection logic
- `go/order-projector/internal/projection/summary.go` — Summary projection logic
- `go/order-projector/internal/projection/stats.go` — Stats aggregation logic
- `go/order-projector/internal/projection/projection_test.go` — Tests for all projections
- `go/order-projector/internal/handler/timeline.go` — GET /orders/:id/timeline
- `go/order-projector/internal/handler/summary.go` — GET /orders/:id, GET /orders
- `go/order-projector/internal/handler/stats.go` — GET /stats/orders
- `go/order-projector/internal/handler/health.go` — GET /health
- `go/order-projector/internal/handler/replay.go` — POST /admin/replay
- `go/order-projector/internal/replay/replayer.go` — Offset reset, truncate, rebuild logic
- `go/order-projector/internal/replay/replayer_test.go` — Tests for replay
- `go/order-projector/internal/repository/projector.go` — PostgreSQL upserts for read models
- `go/order-projector/internal/repository/projector_test.go` — Repository tests
- `go/order-projector/internal/metrics/metrics.go` — Prometheus metrics
- `go/order-projector/migrations/001_create_read_models.up.sql` — Read model tables
- `go/order-projector/migrations/001_create_read_models.down.sql` — Drop tables
- `go/order-projector/go.mod` — Module definition
- `go/order-projector/go.sum` — Dependencies checksum
- `go/order-projector/Dockerfile` — Multi-stage build
- `go/order-projector/.golangci.yml` — Linter config (copy from analytics-service)

### Kubernetes

- `go/k8s/deployments/order-projector.yml` — Deployment manifest
- `go/k8s/services/order-projector.yml` — Service manifest
- `go/k8s/configmaps/order-projector-config.yml` — ConfigMap
- `go/k8s/jobs/order-projector-migrate.yml` — Migration job
- `go/k8s/ingress.yml` — Modified: add `/go-projector` path

### Frontend

- `frontend/src/lib/go-projector-api.ts` — API client for projector service
- `frontend/src/app/go/ecommerce/orders/[id]/page.tsx` — Order detail page with timeline
- `frontend/src/app/go/ecommerce/orders/page.tsx` — Modified: add stats card, link to detail
- `frontend/src/components/go/OrderTimeline.tsx` — Timeline component
- `frontend/src/components/go/OrderStatsCard.tsx` — Stats dashboard card
- `frontend/src/components/go/ProjectionLagBanner.tsx` — Consistency/replay indicator

### CI/CD

- `.github/workflows/ci.yml` — Modified: add order-projector to lint/test/build/deploy matrices
- `Makefile` — Modified: add order-projector to preflight-go

---

## Task 1: Scaffold Order-Projector Service

**Files:**
- Create: `go/order-projector/cmd/server/main.go`
- Create: `go/order-projector/cmd/server/config.go`
- Create: `go/order-projector/cmd/server/routes.go`
- Create: `go/order-projector/go.mod`
- Create: `go/order-projector/Dockerfile`
- Create: `go/order-projector/.golangci.yml`

Use the `/scaffold-go-service` skill to bootstrap. The projector follows the analytics-service pattern (Kafka consumer, no gRPC, HTTP-only).

- [ ] **Step 1: Invoke scaffold-go-service skill**

Invoke `/scaffold-go-service` to create the service skeleton. Provide these details:
- Service name: `order-projector`
- Port: 8097
- Dependencies: Kafka (consumer), PostgreSQL, Prometheus
- No gRPC, no RabbitMQ, no Redis
- Template: analytics-service (Kafka consumer pattern)

- [ ] **Step 2: Create go.mod**

```
go/order-projector/go.mod
```

```go
module github.com/kabradshaw1/portfolio/go/order-projector

go 1.26

require (
	github.com/gin-gonic/gin v1.12.0
	github.com/google/uuid v1.6.0
	github.com/jackc/pgx/v5 v5.7.4
	github.com/kabradshaw1/portfolio/go/pkg v0.0.0
	github.com/prometheus/client_golang v1.22.0
	github.com/segmentio/kafka-go v0.4.50
	go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin v0.60.0
)

replace github.com/kabradshaw1/portfolio/go/pkg => ../pkg
```

Then run: `cd go/order-projector && go mod tidy`

- [ ] **Step 3: Create config.go**

```go
// go/order-projector/cmd/server/config.go
package main

import (
	"log"
	"os"
)

type Config struct {
	Port           string
	DatabaseURL    string
	KafkaBrokers   string
	AllowedOrigins string
	OTELEndpoint   string
}

func loadConfig() Config {
	kafkaBrokers := os.Getenv("KAFKA_BROKERS")
	if kafkaBrokers == "" {
		log.Fatal("KAFKA_BROKERS is required")
	}
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL is required")
	}
	return Config{
		Port:           getenv("PORT", "8097"),
		DatabaseURL:    dbURL,
		KafkaBrokers:   kafkaBrokers,
		AllowedOrigins: getenv("ALLOWED_ORIGINS", "http://localhost:3000"),
		OTELEndpoint:   os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"),
	}
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
```

- [ ] **Step 4: Create main.go**

```go
// go/order-projector/cmd/server/main.go
package main

import (
	"context"
	"log"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kabradshaw1/portfolio/go/pkg/buildinfo"
	"github.com/kabradshaw1/portfolio/go/pkg/shutdown"
	"github.com/kabradshaw1/portfolio/go/pkg/tracing"
	"github.com/kabradshaw1/portfolio/go/pkg/resilience"

	"github.com/kabradshaw1/portfolio/go/order-projector/internal/consumer"
	"github.com/kabradshaw1/portfolio/go/order-projector/internal/handler"
	"github.com/kabradshaw1/portfolio/go/order-projector/internal/repository"
	"github.com/kabradshaw1/portfolio/go/order-projector/internal/replay"
)

func main() {
	cfg := loadConfig()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	shutdownTracer, err := tracing.Init(ctx, "order-projector", cfg.OTELEndpoint)
	if err != nil {
		log.Fatalf("tracing init: %v", err)
	}
	slog.SetDefault(slog.New(
		tracing.NewLogHandler(slog.NewJSONHandler(os.Stdout, nil)),
	))
	buildinfo.Log()

	pool := connectPostgres(ctx, cfg.DatabaseURL)

	pgBreaker := resilience.NewBreaker(resilience.BreakerConfig{
		Name:          "projector-postgres",
		OnStateChange: resilience.ObserveStateChange,
	})
	repo := repository.New(pool, pgBreaker)

	brokers := strings.Split(cfg.KafkaBrokers, ",")
	cons := consumer.New(brokers, repo)
	go func() {
		if err := cons.Run(ctx); err != nil {
			slog.Error("kafka consumer failed", "error", err)
		}
	}()

	replayer := replay.New(repo, cons)

	timelineHandler := handler.NewTimelineHandler(repo)
	summaryHandler := handler.NewSummaryHandler(repo)
	statsHandler := handler.NewStatsHandler(repo)
	healthHandler := handler.NewHealthHandler(pool, cons)
	replayHandler := handler.NewReplayHandler(replayer)

	router := setupRouter(cfg, timelineHandler, summaryHandler, statsHandler, healthHandler, replayHandler)

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		slog.Info("order-projector starting", "port", cfg.Port, "brokers", cfg.KafkaBrokers)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server failed: %v", err)
		}
	}()

	sm := shutdown.New(15 * time.Second)
	sm.Register("cancel-ctx", 0, func(_ context.Context) error {
		cancel()
		return nil
	})
	sm.Register("drain-http", 0, shutdown.DrainHTTP("projector-http", srv))
	sm.Register("wait-kafka", 10, shutdown.WaitForInflight("kafka-consumer", cons.IsIdle, 100*time.Millisecond))
	sm.Register("kafka-close", 20, func(_ context.Context) error {
		return cons.Close()
	})
	sm.Register("postgres", 25, func(_ context.Context) error {
		pool.Close()
		return nil
	})
	sm.Register("otel", 30, func(ctx context.Context) error {
		return shutdownTracer(ctx)
	})
	sm.Wait()
}

func connectPostgres(ctx context.Context, databaseURL string) *pgxpool.Pool {
	pgCfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		log.Fatalf("parse db config: %v", err)
	}
	pgCfg.MaxConns = 10
	pgCfg.MinConns = 2
	pgCfg.MaxConnIdleTime = 5 * time.Minute
	pgCfg.MaxConnLifetime = 30 * time.Minute
	pgCfg.HealthCheckPeriod = 1 * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, pgCfg)
	if err != nil {
		log.Fatalf("postgres connect: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("postgres ping: %v", err)
	}
	slog.Info("connected to postgres")
	return pool
}
```

- [ ] **Step 5: Create routes.go**

```go
// go/order-projector/cmd/server/routes.go
package main

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"

	"github.com/kabradshaw1/portfolio/go/order-projector/internal/handler"
	"github.com/kabradshaw1/portfolio/go/pkg/apperror"
	pkgmw "github.com/kabradshaw1/portfolio/go/pkg/middleware"
)

func setupRouter(
	cfg Config,
	timeline *handler.TimelineHandler,
	summary *handler.SummaryHandler,
	stats *handler.StatsHandler,
	health *handler.HealthHandler,
	replay *handler.ReplayHandler,
) *gin.Engine {
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(pkgmw.SecurityHeaders())
	router.Use(otelgin.Middleware("order-projector"))
	router.Use(corsMiddleware(cfg.AllowedOrigins))
	router.Use(apperror.ErrorHandler())

	router.GET("/orders/:id/timeline", timeline.GetTimeline)
	router.GET("/orders/:id", summary.GetOrder)
	router.GET("/orders", summary.ListOrders)
	router.GET("/stats/orders", stats.GetOrderStats)
	router.GET("/health", health.Health)
	router.GET("/metrics", gin.WrapH(promhttp.Handler()))
	router.POST("/admin/replay", replay.TriggerReplay)

	return router
}

func corsMiddleware(allowedOrigins string) gin.HandlerFunc {
	originSet := make(map[string]bool)
	for _, o := range strings.Split(allowedOrigins, ",") {
		originSet[strings.TrimSpace(o)] = true
	}
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if originSet[origin] {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			c.Header("Access-Control-Allow-Headers", "Content-Type, X-Requested-With")
			c.Header("Access-Control-Allow-Credentials", "true")
		}
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	}
}
```

- [ ] **Step 6: Create Dockerfile**

```dockerfile
# go/order-projector/Dockerfile
FROM golang:1.26-alpine AS builder

WORKDIR /app/order-projector
COPY pkg/ /app/pkg/
COPY order-projector/go.mod order-projector/go.sum ./
RUN go mod download
COPY order-projector/ .
ARG BUILD_VERSION=dev
ARG BUILD_COMMIT=unknown
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-X github.com/kabradshaw1/portfolio/go/pkg/buildinfo.Version=${BUILD_VERSION} -X github.com/kabradshaw1/portfolio/go/pkg/buildinfo.GitSHA=${BUILD_COMMIT}" \
    -o /order-projector ./cmd/server

FROM alpine:3.19

RUN adduser -D -u 1001 appuser

COPY --from=builder /order-projector /order-projector

USER appuser

EXPOSE 8097
ENTRYPOINT ["/order-projector"]
```

- [ ] **Step 7: Copy .golangci.yml from analytics-service**

```bash
cp go/analytics-service/.golangci.yml go/order-projector/.golangci.yml
```

- [ ] **Step 8: Verify module compiles**

Run: `cd go/order-projector && go mod tidy && go vet ./...`

Note: This will fail until the internal packages are created in subsequent tasks. That's expected — this step confirms the go.mod is valid.

- [ ] **Step 9: Commit scaffold**

```bash
git add go/order-projector/
git commit -m "feat(order-projector): scaffold new CQRS projection service

Kafka consumer service that will build read-optimized views from
order domain events. Follows analytics-service patterns.

Co-Authored-By: Claude Opus 4.6 (1M context) <noreply@anthropic.com>"
```

---

## Task 2: Event Types and Publisher (order-service)

**Files:**
- Create: `go/order-service/internal/events/types.go`
- Create: `go/order-service/internal/events/publisher.go`
- Create: `go/order-service/internal/events/publisher_test.go`

- [ ] **Step 1: Create event type constants and data structs**

```go
// go/order-service/internal/events/types.go
package events

const (
	TopicOrderEvents = "ecommerce.order-events"

	OrderCreated          = "order.created"
	OrderReserved         = "order.reserved"
	OrderPaymentInitiated = "order.payment_initiated"
	OrderPaymentCompleted = "order.payment_completed"
	OrderCompleted        = "order.completed"
	OrderFailed           = "order.failed"
	OrderCancelled        = "order.cancelled"

	// CurrentVersion is the event schema version.
	CurrentVersion = 1
)

// OrderCreatedData is the payload for order.created events.
type OrderCreatedData struct {
	UserID     string            `json:"userID"`
	TotalCents int               `json:"totalCents"`
	Currency   string            `json:"currency"`
	Items      []OrderItemData   `json:"items"`
}

type OrderItemData struct {
	ProductID  string `json:"productID"`
	Quantity   int    `json:"quantity"`
	PriceCents int    `json:"priceCents"`
}

// OrderReservedData is the payload for order.reserved events.
type OrderReservedData struct {
	ReservedItems []string `json:"reservedItems"` // product IDs
}

// OrderPaymentInitiatedData is the payload for order.payment_initiated events.
type OrderPaymentInitiatedData struct {
	CheckoutURL     string `json:"checkoutURL"`
	PaymentProvider string `json:"paymentProvider"`
}

// OrderPaymentCompletedData is the payload for order.payment_completed events.
type OrderPaymentCompletedData struct {
	PaymentID  string `json:"paymentID,omitempty"`
	AmountCents int   `json:"amountCents"`
}

// OrderCompletedData is the payload for order.completed events.
type OrderCompletedData struct {
	CompletedAt string `json:"completedAt"` // RFC3339
}

// OrderFailedData is the payload for order.failed events.
type OrderFailedData struct {
	FailureReason string `json:"failureReason"`
	FailedStep    string `json:"failedStep"`
}

// OrderCancelledData is the payload for order.cancelled events.
type OrderCancelledData struct {
	CancelReason string `json:"cancelReason"`
	RefundStatus string `json:"refundStatus"`
}
```

- [ ] **Step 2: Create event publisher**

```go
// go/order-service/internal/events/publisher.go
package events

import (
	"context"
	"log/slog"

	"github.com/kabradshaw1/portfolio/go/order-service/internal/kafka"
)

// Publisher publishes domain events to the order-events Kafka topic.
type Publisher struct {
	kafkaPub kafka.Producer
}

// NewPublisher creates an event publisher. If kafkaPub is nil, publishes are no-ops.
func NewPublisher(kafkaPub kafka.Producer) *Publisher {
	return &Publisher{kafkaPub: kafkaPub}
}

// Publish publishes a domain event. Fire-and-forget: errors are logged, not returned.
func (p *Publisher) Publish(ctx context.Context, orderID string, eventType string, data any) {
	if p.kafkaPub == nil {
		return
	}

	event := kafka.Event{
		Type:    eventType,
		Version: CurrentVersion,
		Data:    data,
	}

	kafka.SafePublish(ctx, p.kafkaPub, TopicOrderEvents, orderID, event)
	slog.DebugContext(ctx, "published order event", "type", eventType, "orderID", orderID)
}
```

- [ ] **Step 3: Add Version field to kafka.Event**

Modify `go/order-service/internal/kafka/producer.go` to add the `Version` field:

```go
// Event is the envelope for all Kafka analytics events.
type Event struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	Version   int       `json:"version,omitempty"`
	Source    string    `json:"source"`
	Timestamp time.Time `json:"timestamp"`
	TraceID   string    `json:"traceID"`
	Data      any       `json:"data"`
}
```

Add `Version` field between `Type` and `Source` in the Event struct at `go/order-service/internal/kafka/producer.go:17`.

- [ ] **Step 4: Write publisher test**

```go
// go/order-service/internal/events/publisher_test.go
package events

import (
	"context"
	"testing"

	"github.com/kabradshaw1/portfolio/go/order-service/internal/kafka"
)

type mockProducer struct {
	published []kafka.Event
	topic     string
	key       string
}

func (m *mockProducer) Publish(_ context.Context, topic string, key string, event kafka.Event) error {
	m.topic = topic
	m.key = key
	m.published = append(m.published, event)
	return nil
}

func (m *mockProducer) Close() error { return nil }

func TestPublisher_Publish(t *testing.T) {
	mock := &mockProducer{}
	pub := NewPublisher(mock)

	pub.Publish(context.Background(), "order-123", OrderCreated, OrderCreatedData{
		UserID:     "user-1",
		TotalCents: 4597,
		Currency:   "USD",
		Items:      []OrderItemData{{ProductID: "prod-1", Quantity: 2, PriceCents: 1999}},
	})

	if len(mock.published) != 1 {
		t.Fatalf("expected 1 event, got %d", len(mock.published))
	}
	if mock.topic != TopicOrderEvents {
		t.Errorf("expected topic %s, got %s", TopicOrderEvents, mock.topic)
	}
	if mock.key != "order-123" {
		t.Errorf("expected key order-123, got %s", mock.key)
	}
	if mock.published[0].Type != OrderCreated {
		t.Errorf("expected type %s, got %s", OrderCreated, mock.published[0].Type)
	}
	if mock.published[0].Version != CurrentVersion {
		t.Errorf("expected version %d, got %d", CurrentVersion, mock.published[0].Version)
	}
}

func TestPublisher_NilProducer(t *testing.T) {
	pub := NewPublisher(nil)
	// Should not panic
	pub.Publish(context.Background(), "order-123", OrderCreated, nil)
}
```

- [ ] **Step 5: Run tests**

Run: `cd go/order-service && go test ./internal/events/ -v -race`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add go/order-service/internal/events/ go/order-service/internal/kafka/producer.go
git commit -m "feat(order-service): add domain event types and publisher

Defines 7 order event types (created, reserved, payment_initiated,
payment_completed, completed, failed, cancelled) and a fire-and-forget
publisher for the ecommerce.order-events Kafka topic.

Co-Authored-By: Claude Opus 4.6 (1M context) <noreply@anthropic.com>"
```

---

## Task 3: Wire Event Publishing into Saga Orchestrator

**Files:**
- Modify: `go/order-service/internal/saga/orchestrator.go`

The orchestrator already has `kafkaPub kafka.Producer`. We inject an `events.Publisher` and call it at each saga step.

- [ ] **Step 1: Add EventPublisher to Orchestrator**

Modify `go/order-service/internal/saga/orchestrator.go`:

Add import:
```go
"github.com/kabradshaw1/portfolio/go/order-service/internal/events"
```

Add field to Orchestrator struct (line 39-46):
```go
type Orchestrator struct {
	repo        OrderRepository
	pub         SagaPublisher
	stock       StockChecker
	payment     PaymentCreator
	kafkaPub    kafka.Producer
	eventPub    *events.Publisher
	frontendURL string
}
```

Update NewOrchestrator (line 49-51):
```go
func NewOrchestrator(repo OrderRepository, pub SagaPublisher, stock StockChecker, payment PaymentCreator, kafkaPub kafka.Producer, frontendURL string) *Orchestrator {
	return &Orchestrator{
		repo: repo, pub: pub, stock: stock, payment: payment,
		kafkaPub: kafkaPub, eventPub: events.NewPublisher(kafkaPub),
		frontendURL: frontendURL,
	}
}
```

- [ ] **Step 2: Publish order.created in handleCreated**

At the end of `handleCreated` (line 95-112), before the return, add event publishing. The event must fire before the RabbitMQ command because it records the state transition:

```go
func (o *Orchestrator) handleCreated(ctx context.Context, order *model.Order) error {
	items := make([]CommandItem, len(order.Items))
	eventItems := make([]events.OrderItemData, len(order.Items))
	for i, item := range order.Items {
		items[i] = CommandItem{
			ProductID: item.ProductID.String(),
			Quantity:  item.Quantity,
		}
		eventItems[i] = events.OrderItemData{
			ProductID:  item.ProductID.String(),
			Quantity:   item.Quantity,
			PriceCents: item.PriceAtPurchase,
		}
	}

	o.eventPub.Publish(ctx, order.ID.String(), events.OrderCreated, events.OrderCreatedData{
		UserID:     order.UserID.String(),
		TotalCents: order.Total,
		Currency:   "USD",
		Items:      eventItems,
	})

	SagaStepsTotal.WithLabelValues(StepCreated, "success").Inc()

	return o.pub.PublishCommand(ctx, Command{
		Command: CmdReserveItems,
		OrderID: order.ID.String(),
		UserID:  order.UserID.String(),
		Items:   items,
	})
}
```

- [ ] **Step 3: Publish order.reserved in HandleEvent (EvtItemsReserved case)**

In `HandleEvent` (line 263-268), after updating the saga step:

```go
case EvtItemsReserved:
	if err := o.repo.UpdateSagaStep(ctx, orderID, StepItemsReserved); err != nil {
		return err
	}
	SagaStepsTotal.WithLabelValues(StepItemsReserved, "success").Inc()

	order, err := o.repo.FindByID(ctx, orderID)
	if err != nil {
		return fmt.Errorf("find order for reserved event: %w", err)
	}
	productIDs := make([]string, len(order.Items))
	for i, item := range order.Items {
		productIDs[i] = item.ProductID.String()
	}
	o.eventPub.Publish(ctx, orderID.String(), events.OrderReserved, events.OrderReservedData{
		ReservedItems: productIDs,
	})

	return o.Advance(ctx, orderID)
```

- [ ] **Step 4: Publish order.payment_initiated in handleStockValidated**

After setting `StepPaymentCreated` (line 156-160):

```go
if err := o.repo.UpdateSagaStep(ctx, order.ID, StepPaymentCreated); err != nil {
	return err
}
SagaStepsTotal.WithLabelValues(StepPaymentCreated, "success").Inc()

o.eventPub.Publish(ctx, order.ID.String(), events.OrderPaymentInitiated, events.OrderPaymentInitiatedData{
	CheckoutURL:     checkoutURL,
	PaymentProvider: "stripe",
})

return nil // Wait for webhook confirmation
```

- [ ] **Step 5: Publish order.payment_completed in HandleEvent (EvtPaymentConfirmed case)**

In `HandleEvent` (line 270-275):

```go
case EvtPaymentConfirmed:
	if err := o.repo.UpdateSagaStep(ctx, orderID, StepPaymentConfirmed); err != nil {
		return err
	}
	SagaStepsTotal.WithLabelValues(StepPaymentConfirmed, "success").Inc()

	order, err := o.repo.FindByID(ctx, orderID)
	if err != nil {
		return fmt.Errorf("find order for payment event: %w", err)
	}
	o.eventPub.Publish(ctx, orderID.String(), events.OrderPaymentCompleted, events.OrderPaymentCompletedData{
		AmountCents: order.Total,
	})

	return o.Advance(ctx, orderID)
```

- [ ] **Step 6: Publish order.completed in completeOrder**

In `completeOrder` (line 195), after the log statement, before the existing Kafka analytics event:

```go
slog.InfoContext(ctx, "saga completed", "orderID", order.ID)

o.eventPub.Publish(ctx, order.ID.String(), events.OrderCompleted, events.OrderCompletedData{
	CompletedAt: time.Now().UTC().Format(time.RFC3339),
})

// Publish Kafka analytics event (fire-and-forget) — existing code below...
```

- [ ] **Step 7: Publish order.failed and order.cancelled in compensate**

In `compensate` (line 226-251), after updating status to failed:

```go
func (o *Orchestrator) compensate(ctx context.Context, order *model.Order) error {
	if o.payment != nil && (order.SagaStep == StepPaymentConfirmed || order.SagaStep == StepPaymentCreated) {
		if err := o.payment.RefundPayment(ctx, order.ID, "saga compensation"); err != nil {
			slog.ErrorContext(ctx, "refund failed during compensation",
				"orderID", order.ID, "error", err)
			SagaStepsTotal.WithLabelValues("refund", "error").Inc()
		}
	}

	if err := o.repo.UpdateStatus(ctx, order.ID, model.OrderStatusFailed); err != nil {
		return err
	}
	if err := o.repo.UpdateSagaStep(ctx, order.ID, StepCompensating); err != nil {
		return err
	}

	o.eventPub.Publish(ctx, order.ID.String(), events.OrderFailed, events.OrderFailedData{
		FailureReason: "saga compensation",
		FailedStep:    order.SagaStep,
	})

	order.Status = model.OrderStatusFailed
	order.SagaStep = StepCompensating
	SagaStepsTotal.WithLabelValues(StepCompensating, "success").Inc()

	return o.pub.PublishCommand(ctx, Command{
		Command: CmdReleaseItems,
		OrderID: order.ID.String(),
		UserID:  order.UserID.String(),
	})
}
```

Also in `HandleEvent` for `EvtItemsReleased` (line 288-290), publish the cancellation event:

```go
case EvtItemsReleased:
	SagaStepsTotal.WithLabelValues(StepCompensationComplete, "success").Inc()
	o.eventPub.Publish(ctx, orderID.String(), events.OrderCancelled, events.OrderCancelledData{
		CancelReason: "saga compensation complete",
		RefundStatus: "items_released",
	})
	return o.repo.UpdateSagaStep(ctx, orderID, StepCompensationComplete)
```

- [ ] **Step 8: Run order-service tests**

Run: `cd go/order-service && go test ./... -v -race`
Expected: PASS (existing tests should still pass — event publishing is fire-and-forget via SafePublish)

- [ ] **Step 9: Run linter**

Run: `cd go/order-service && golangci-lint run ./...`
Expected: PASS

- [ ] **Step 10: Commit**

```bash
git add go/order-service/internal/saga/orchestrator.go
git commit -m "feat(order-service): publish domain events at each saga step

Publishes 7 event types (created, reserved, payment_initiated,
payment_completed, completed, failed, cancelled) to the
ecommerce.order-events Kafka topic. Fire-and-forget via SafePublish,
does not affect saga flow.

Co-Authored-By: Claude Opus 4.6 (1M context) <noreply@anthropic.com>"
```

---

## Task 4: Database Migrations (order-projector)

**Files:**
- Create: `go/order-projector/migrations/001_create_read_models.up.sql`
- Create: `go/order-projector/migrations/001_create_read_models.down.sql`

- [ ] **Step 1: Create up migration**

```sql
-- go/order-projector/migrations/001_create_read_models.up.sql

-- Full audit trail: one row per event
CREATE TABLE order_timeline (
    event_id      UUID PRIMARY KEY,
    order_id      UUID NOT NULL,
    event_type    TEXT NOT NULL,
    event_version INT NOT NULL DEFAULT 1,
    data_json     JSONB NOT NULL,
    timestamp     TIMESTAMPTZ NOT NULL,
    created_at    TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_timeline_order_id ON order_timeline(order_id, timestamp);

-- Latest state: one row per order, upserted on each event
CREATE TABLE order_summary (
    order_id       UUID PRIMARY KEY,
    user_id        UUID NOT NULL,
    status         TEXT NOT NULL DEFAULT 'created',
    total_cents    BIGINT NOT NULL DEFAULT 0,
    currency       TEXT NOT NULL DEFAULT 'USD',
    items_json     JSONB,
    created_at     TIMESTAMPTZ NOT NULL,
    updated_at     TIMESTAMPTZ NOT NULL,
    completed_at   TIMESTAMPTZ,
    failure_reason TEXT
);

CREATE INDEX idx_summary_user_id ON order_summary(user_id);
CREATE INDEX idx_summary_status ON order_summary(status);

-- Hourly aggregation
CREATE TABLE order_stats (
    hour_bucket           TIMESTAMPTZ PRIMARY KEY,
    orders_created        INT DEFAULT 0,
    orders_completed      INT DEFAULT 0,
    orders_failed         INT DEFAULT 0,
    avg_completion_seconds FLOAT DEFAULT 0,
    total_revenue_cents   BIGINT DEFAULT 0
);

-- Replay tracking
CREATE TABLE replay_status (
    id                SERIAL PRIMARY KEY,
    is_replaying      BOOLEAN DEFAULT FALSE,
    projection        TEXT NOT NULL DEFAULT 'all',
    started_at        TIMESTAMPTZ,
    completed_at      TIMESTAMPTZ,
    events_processed  BIGINT DEFAULT 0,
    total_events      BIGINT DEFAULT 0
);
```

- [ ] **Step 2: Create down migration**

```sql
-- go/order-projector/migrations/001_create_read_models.down.sql
DROP TABLE IF EXISTS replay_status;
DROP TABLE IF EXISTS order_stats;
DROP TABLE IF EXISTS order_summary;
DROP TABLE IF EXISTS order_timeline;
```

- [ ] **Step 3: Commit**

```bash
git add go/order-projector/migrations/
git commit -m "feat(order-projector): add read model database migrations

Creates four tables: order_timeline (audit trail), order_summary
(latest order state), order_stats (hourly aggregation), and
replay_status (replay tracking).

Co-Authored-By: Claude Opus 4.6 (1M context) <noreply@anthropic.com>"
```

---

## Task 5: Repository Layer (order-projector)

**Files:**
- Create: `go/order-projector/internal/repository/projector.go`
- Create: `go/order-projector/internal/repository/projector_test.go`

- [ ] **Step 1: Create repository**

```go
// go/order-projector/internal/repository/projector.go
package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sony/gobreaker/v2"

	"github.com/kabradshaw1/portfolio/go/pkg/resilience"
)

// Repository handles all read model persistence.
type Repository struct {
	pool    *pgxpool.Pool
	breaker *gobreaker.CircuitBreaker[any]
}

// New creates a repository with a circuit breaker.
func New(pool *pgxpool.Pool, breaker *gobreaker.CircuitBreaker[any]) *Repository {
	return &Repository{pool: pool, breaker: breaker}
}

// TimelineEvent represents a single event in an order's timeline.
type TimelineEvent struct {
	EventID      string          `json:"eventId"`
	OrderID      string          `json:"orderId"`
	EventType    string          `json:"eventType"`
	EventVersion int             `json:"eventVersion"`
	Data         json.RawMessage `json:"data"`
	Timestamp    time.Time       `json:"timestamp"`
}

// OrderSummary represents the current projected state of an order.
type OrderSummary struct {
	OrderID       string          `json:"orderId"`
	UserID        string          `json:"userId"`
	Status        string          `json:"status"`
	TotalCents    int64           `json:"totalCents"`
	Currency      string          `json:"currency"`
	Items         json.RawMessage `json:"items,omitempty"`
	CreatedAt     time.Time       `json:"createdAt"`
	UpdatedAt     time.Time       `json:"updatedAt"`
	CompletedAt   *time.Time      `json:"completedAt,omitempty"`
	FailureReason *string         `json:"failureReason,omitempty"`
}

// OrderStats represents hourly aggregated order statistics.
type OrderStats struct {
	HourBucket           time.Time `json:"hourBucket"`
	OrdersCreated        int       `json:"ordersCreated"`
	OrdersCompleted      int       `json:"ordersCompleted"`
	OrdersFailed         int       `json:"ordersFailed"`
	AvgCompletionSeconds float64   `json:"avgCompletionSeconds"`
	TotalRevenueCents    int64     `json:"totalRevenueCents"`
}

// ReplayStatus represents the current state of a replay operation.
type ReplayStatus struct {
	IsReplaying     bool       `json:"isReplaying"`
	Projection      string     `json:"projection"`
	StartedAt       *time.Time `json:"startedAt,omitempty"`
	CompletedAt     *time.Time `json:"completedAt,omitempty"`
	EventsProcessed int64      `json:"eventsProcessed"`
	TotalEvents     int64      `json:"totalEvents"`
}

// InsertTimelineEvent inserts an event into the timeline. Idempotent via ON CONFLICT DO NOTHING.
func (r *Repository) InsertTimelineEvent(ctx context.Context, e TimelineEvent) error {
	return resilience.Do(ctx, r.breaker, resilience.DefaultRetry(), func(ctx context.Context) error {
		_, err := r.pool.Exec(ctx,
			`INSERT INTO order_timeline (event_id, order_id, event_type, event_version, data_json, timestamp)
			 VALUES ($1, $2, $3, $4, $5, $6)
			 ON CONFLICT (event_id) DO NOTHING`,
			e.EventID, e.OrderID, e.EventType, e.EventVersion, e.Data, e.Timestamp,
		)
		return err
	})
}

// UpsertOrderSummary creates or updates an order summary.
func (r *Repository) UpsertOrderSummary(ctx context.Context, s OrderSummary) error {
	return resilience.Do(ctx, r.breaker, resilience.DefaultRetry(), func(ctx context.Context) error {
		_, err := r.pool.Exec(ctx,
			`INSERT INTO order_summary (order_id, user_id, status, total_cents, currency, items_json, created_at, updated_at, completed_at, failure_reason)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
			 ON CONFLICT (order_id) DO UPDATE SET
			   status = EXCLUDED.status,
			   total_cents = EXCLUDED.total_cents,
			   currency = EXCLUDED.currency,
			   items_json = COALESCE(EXCLUDED.items_json, order_summary.items_json),
			   updated_at = EXCLUDED.updated_at,
			   completed_at = COALESCE(EXCLUDED.completed_at, order_summary.completed_at),
			   failure_reason = COALESCE(EXCLUDED.failure_reason, order_summary.failure_reason)`,
			s.OrderID, s.UserID, s.Status, s.TotalCents, s.Currency, s.Items,
			s.CreatedAt, s.UpdatedAt, s.CompletedAt, s.FailureReason,
		)
		return err
	})
}

// UpsertOrderStats increments hourly stats counters.
func (r *Repository) UpsertOrderStats(ctx context.Context, bucket time.Time, created, completed, failed int, revenueCents int64) error {
	return resilience.Do(ctx, r.breaker, resilience.DefaultRetry(), func(ctx context.Context) error {
		_, err := r.pool.Exec(ctx,
			`INSERT INTO order_stats (hour_bucket, orders_created, orders_completed, orders_failed, total_revenue_cents)
			 VALUES ($1, $2, $3, $4, $5)
			 ON CONFLICT (hour_bucket) DO UPDATE SET
			   orders_created = order_stats.orders_created + EXCLUDED.orders_created,
			   orders_completed = order_stats.orders_completed + EXCLUDED.orders_completed,
			   orders_failed = order_stats.orders_failed + EXCLUDED.orders_failed,
			   total_revenue_cents = order_stats.total_revenue_cents + EXCLUDED.total_revenue_cents`,
			bucket, created, completed, failed, revenueCents,
		)
		return err
	})
}

// GetTimeline returns all events for an order, ordered by timestamp.
func (r *Repository) GetTimeline(ctx context.Context, orderID string) ([]TimelineEvent, error) {
	return resilience.Call(ctx, r.breaker, resilience.DefaultRetry(), func(ctx context.Context) ([]TimelineEvent, error) {
		rows, err := r.pool.Query(ctx,
			`SELECT event_id, order_id, event_type, event_version, data_json, timestamp
			 FROM order_timeline WHERE order_id = $1 ORDER BY timestamp ASC`, orderID)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		var events []TimelineEvent
		for rows.Next() {
			var e TimelineEvent
			if err := rows.Scan(&e.EventID, &e.OrderID, &e.EventType, &e.EventVersion, &e.Data, &e.Timestamp); err != nil {
				return nil, err
			}
			events = append(events, e)
		}
		return events, rows.Err()
	})
}

// GetOrderSummary returns the projected summary for a single order.
func (r *Repository) GetOrderSummary(ctx context.Context, orderID string) (*OrderSummary, error) {
	return resilience.Call(ctx, r.breaker, resilience.DefaultRetry(), func(ctx context.Context) (*OrderSummary, error) {
		var s OrderSummary
		err := r.pool.QueryRow(ctx,
			`SELECT order_id, user_id, status, total_cents, currency, items_json, created_at, updated_at, completed_at, failure_reason
			 FROM order_summary WHERE order_id = $1`, orderID,
		).Scan(&s.OrderID, &s.UserID, &s.Status, &s.TotalCents, &s.Currency, &s.Items,
			&s.CreatedAt, &s.UpdatedAt, &s.CompletedAt, &s.FailureReason)
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return &s, err
	})
}

// ListOrderSummaries returns paginated order summaries.
func (r *Repository) ListOrderSummaries(ctx context.Context, limit int, offset int) ([]OrderSummary, error) {
	return resilience.Call(ctx, r.breaker, resilience.DefaultRetry(), func(ctx context.Context) ([]OrderSummary, error) {
		rows, err := r.pool.Query(ctx,
			`SELECT order_id, user_id, status, total_cents, currency, items_json, created_at, updated_at, completed_at, failure_reason
			 FROM order_summary ORDER BY updated_at DESC LIMIT $1 OFFSET $2`, limit, offset)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		var summaries []OrderSummary
		for rows.Next() {
			var s OrderSummary
			if err := rows.Scan(&s.OrderID, &s.UserID, &s.Status, &s.TotalCents, &s.Currency, &s.Items,
				&s.CreatedAt, &s.UpdatedAt, &s.CompletedAt, &s.FailureReason); err != nil {
				return nil, err
			}
			summaries = append(summaries, s)
		}
		return summaries, rows.Err()
	})
}

// GetOrderStats returns hourly stats for the given time range.
func (r *Repository) GetOrderStats(ctx context.Context, since time.Time, limit int) ([]OrderStats, error) {
	return resilience.Call(ctx, r.breaker, resilience.DefaultRetry(), func(ctx context.Context) ([]OrderStats, error) {
		rows, err := r.pool.Query(ctx,
			`SELECT hour_bucket, orders_created, orders_completed, orders_failed, avg_completion_seconds, total_revenue_cents
			 FROM order_stats WHERE hour_bucket >= $1 ORDER BY hour_bucket DESC LIMIT $2`, since, limit)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		var stats []OrderStats
		for rows.Next() {
			var s OrderStats
			if err := rows.Scan(&s.HourBucket, &s.OrdersCreated, &s.OrdersCompleted, &s.OrdersFailed,
				&s.AvgCompletionSeconds, &s.TotalRevenueCents); err != nil {
				return nil, err
			}
			stats = append(stats, s)
		}
		return stats, rows.Err()
	})
}

// GetReplayStatus returns the most recent replay status.
func (r *Repository) GetReplayStatus(ctx context.Context) (*ReplayStatus, error) {
	return resilience.Call(ctx, r.breaker, resilience.DefaultRetry(), func(ctx context.Context) (*ReplayStatus, error) {
		var s ReplayStatus
		err := r.pool.QueryRow(ctx,
			`SELECT is_replaying, projection, started_at, completed_at, events_processed, total_events
			 FROM replay_status ORDER BY id DESC LIMIT 1`,
		).Scan(&s.IsReplaying, &s.Projection, &s.StartedAt, &s.CompletedAt, &s.EventsProcessed, &s.TotalEvents)
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return &s, err
	})
}

// StartReplay inserts a replay-started record.
func (r *Repository) StartReplay(ctx context.Context, projection string) error {
	return resilience.Do(ctx, r.breaker, resilience.DefaultRetry(), func(ctx context.Context) error {
		now := time.Now().UTC()
		_, err := r.pool.Exec(ctx,
			`INSERT INTO replay_status (is_replaying, projection, started_at, events_processed, total_events)
			 VALUES (true, $1, $2, 0, 0)`, projection, now)
		return err
	})
}

// CompleteReplay marks the most recent replay as done.
func (r *Repository) CompleteReplay(ctx context.Context) error {
	return resilience.Do(ctx, r.breaker, resilience.DefaultRetry(), func(ctx context.Context) error {
		now := time.Now().UTC()
		_, err := r.pool.Exec(ctx,
			`UPDATE replay_status SET is_replaying = false, completed_at = $1
			 WHERE id = (SELECT id FROM replay_status ORDER BY id DESC LIMIT 1)`, now)
		return err
	})
}

// IncrementReplayProgress updates the events_processed counter.
func (r *Repository) IncrementReplayProgress(ctx context.Context, count int64) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE replay_status SET events_processed = events_processed + $1
		 WHERE id = (SELECT id FROM replay_status ORDER BY id DESC LIMIT 1)`, count)
	return err
}

// TruncateProjection truncates the specified projection table(s).
func (r *Repository) TruncateProjection(ctx context.Context, projection string) error {
	var query string
	switch projection {
	case "timeline":
		query = "TRUNCATE order_timeline"
	case "summary":
		query = "TRUNCATE order_summary"
	case "stats":
		query = "TRUNCATE order_stats"
	case "all":
		query = "TRUNCATE order_timeline, order_summary, order_stats"
	default:
		return fmt.Errorf("unknown projection: %s", projection)
	}
	_, err := r.pool.Exec(ctx, query)
	return err
}

// LatestEventTimestamp returns the most recent event timestamp in the timeline.
func (r *Repository) LatestEventTimestamp(ctx context.Context) (*time.Time, error) {
	return resilience.Call(ctx, r.breaker, resilience.DefaultRetry(), func(ctx context.Context) (*time.Time, error) {
		var t time.Time
		err := r.pool.QueryRow(ctx,
			`SELECT COALESCE(MAX(timestamp), '1970-01-01'::timestamptz) FROM order_timeline`,
		).Scan(&t)
		if err != nil {
			return nil, err
		}
		if t.Year() == 1970 {
			return nil, nil
		}
		return &t, nil
	})
}
```

- [ ] **Step 2: Write repository tests (unit level — mock pool)**

```go
// go/order-projector/internal/repository/projector_test.go
package repository

import (
	"encoding/json"
	"testing"
	"time"
)

func TestTimelineEvent_JSON(t *testing.T) {
	e := TimelineEvent{
		EventID:      "evt-1",
		OrderID:      "order-1",
		EventType:    "order.created",
		EventVersion: 1,
		Data:         json.RawMessage(`{"userID":"u1"}`),
		Timestamp:    time.Now().UTC(),
	}
	b, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	var parsed TimelineEvent
	if err := json.Unmarshal(b, &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed.EventType != "order.created" {
		t.Errorf("expected order.created, got %s", parsed.EventType)
	}
}

func TestOrderSummary_JSON(t *testing.T) {
	s := OrderSummary{
		OrderID:    "order-1",
		UserID:     "user-1",
		Status:     "completed",
		TotalCents: 4597,
		Currency:   "USD",
		CreatedAt:  time.Now().UTC(),
		UpdatedAt:  time.Now().UTC(),
	}
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var parsed OrderSummary
	if err := json.Unmarshal(b, &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed.TotalCents != 4597 {
		t.Errorf("expected 4597, got %d", parsed.TotalCents)
	}
}
```

- [ ] **Step 3: Run tests**

Run: `cd go/order-projector && go test ./internal/repository/ -v -race`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add go/order-projector/internal/repository/
git commit -m "feat(order-projector): add repository layer for read models

PostgreSQL repository with circuit breaker + retry wrapping for all
CRUD operations on timeline, summary, stats, and replay_status tables.
Idempotent upserts for event processing.

Co-Authored-By: Claude Opus 4.6 (1M context) <noreply@anthropic.com>"
```

---

## Task 6: Deserializer with Version-Aware Upgrades

**Files:**
- Create: `go/order-projector/internal/consumer/deserializer.go`
- Create: `go/order-projector/internal/consumer/deserializer_test.go`

- [ ] **Step 1: Create deserializer**

```go
// go/order-projector/internal/consumer/deserializer.go
package consumer

import (
	"encoding/json"
	"fmt"
	"time"
)

// LatestVersion is the version all events are upgraded to before processing.
const LatestVersion = 2

// OrderEvent is the deserialized Kafka event envelope.
type OrderEvent struct {
	ID        string          `json:"id"`
	Type      string          `json:"type"`
	Version   int             `json:"version"`
	Source    string          `json:"source"`
	OrderID   string          `json:"order_id"`
	Timestamp time.Time       `json:"timestamp"`
	TraceID   string          `json:"traceID"`
	Data      json.RawMessage `json:"data"`
}

// upgrader transforms event data from one version to the next.
type upgrader func(data map[string]any) map[string]any

// upgradeRegistry maps (eventType, fromVersion) to an upgrader.
var upgradeRegistry = map[string]map[int]upgrader{
	"order.created": {
		1: upgradeOrderCreatedV1toV2,
	},
}

// upgradeOrderCreatedV1toV2 adds the currency field that was missing in v1.
func upgradeOrderCreatedV1toV2(data map[string]any) map[string]any {
	if _, ok := data["currency"]; !ok {
		data["currency"] = "USD"
	}
	return data
}

// Deserialize parses a Kafka message value into an OrderEvent,
// upgrading the data to the latest version.
func Deserialize(value []byte) (*OrderEvent, error) {
	var evt OrderEvent
	if err := json.Unmarshal(value, &evt); err != nil {
		return nil, fmt.Errorf("unmarshal event: %w", err)
	}

	if evt.Version == 0 {
		evt.Version = 1 // treat unversioned events as v1
	}

	// Upgrade data if needed.
	if evt.Version < LatestVersion {
		upgraded, err := upgradeData(evt.Type, evt.Version, evt.Data)
		if err != nil {
			return nil, fmt.Errorf("upgrade event data: %w", err)
		}
		evt.Data = upgraded
		evt.Version = LatestVersion
	}

	return &evt, nil
}

// upgradeData chains version upgraders from current version to LatestVersion.
func upgradeData(eventType string, fromVersion int, raw json.RawMessage) (json.RawMessage, error) {
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		return raw, fmt.Errorf("unmarshal data for upgrade: %w", err)
	}

	typeUpgraders, ok := upgradeRegistry[eventType]
	if !ok {
		// No upgraders registered for this event type — return as-is.
		return raw, nil
	}

	for v := fromVersion; v < LatestVersion; v++ {
		fn, ok := typeUpgraders[v]
		if !ok {
			continue // no upgrader for this version step — skip
		}
		data = fn(data)
	}

	return json.Marshal(data)
}
```

- [ ] **Step 2: Write deserializer tests**

```go
// go/order-projector/internal/consumer/deserializer_test.go
package consumer

import (
	"encoding/json"
	"testing"
)

func TestDeserialize_V1Event(t *testing.T) {
	raw := `{
		"id": "evt-1",
		"type": "order.created",
		"version": 1,
		"source": "order-service",
		"order_id": "order-1",
		"timestamp": "2026-04-23T14:00:00Z",
		"data": {"userID": "u1", "totalCents": 4597, "items": []}
	}`

	evt, err := Deserialize([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}

	if evt.Version != LatestVersion {
		t.Errorf("expected version %d after upgrade, got %d", LatestVersion, evt.Version)
	}

	// Verify currency was backfilled.
	var data map[string]any
	if err := json.Unmarshal(evt.Data, &data); err != nil {
		t.Fatal(err)
	}
	currency, ok := data["currency"]
	if !ok {
		t.Fatal("expected currency field to be backfilled")
	}
	if currency != "USD" {
		t.Errorf("expected USD, got %v", currency)
	}
}

func TestDeserialize_V2Event(t *testing.T) {
	raw := `{
		"id": "evt-2",
		"type": "order.created",
		"version": 2,
		"source": "order-service",
		"order_id": "order-2",
		"timestamp": "2026-04-23T14:00:00Z",
		"data": {"userID": "u1", "totalCents": 1000, "currency": "EUR", "items": []}
	}`

	evt, err := Deserialize([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}

	if evt.Version != 2 {
		t.Errorf("expected version 2 (no upgrade needed), got %d", evt.Version)
	}

	var data map[string]any
	if err := json.Unmarshal(evt.Data, &data); err != nil {
		t.Fatal(err)
	}
	if data["currency"] != "EUR" {
		t.Errorf("expected EUR (should not be overwritten), got %v", data["currency"])
	}
}

func TestDeserialize_UnversionedEvent(t *testing.T) {
	raw := `{
		"id": "evt-3",
		"type": "order.completed",
		"source": "order-service",
		"order_id": "order-3",
		"timestamp": "2026-04-23T14:00:00Z",
		"data": {"completedAt": "2026-04-23T14:01:00Z"}
	}`

	evt, err := Deserialize([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}

	// Unversioned events default to v1, then upgrade to latest.
	// No upgrader registered for order.completed, so data stays the same.
	if evt.Version != LatestVersion {
		t.Errorf("expected version %d, got %d", LatestVersion, evt.Version)
	}
}

func TestDeserialize_NonCreatedEventNoUpgrader(t *testing.T) {
	raw := `{
		"id": "evt-4",
		"type": "order.reserved",
		"version": 1,
		"source": "order-service",
		"order_id": "order-4",
		"timestamp": "2026-04-23T14:00:00Z",
		"data": {"reservedItems": ["p1", "p2"]}
	}`

	evt, err := Deserialize([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}

	if evt.Version != LatestVersion {
		t.Errorf("expected version %d, got %d", LatestVersion, evt.Version)
	}

	var data map[string]any
	if err := json.Unmarshal(evt.Data, &data); err != nil {
		t.Fatal(err)
	}
	items, ok := data["reservedItems"].([]any)
	if !ok || len(items) != 2 {
		t.Errorf("expected 2 reserved items, got %v", data["reservedItems"])
	}
}

func TestUpgradeOrderCreatedV1toV2(t *testing.T) {
	data := map[string]any{"userID": "u1", "totalCents": float64(100)}
	result := upgradeOrderCreatedV1toV2(data)
	if result["currency"] != "USD" {
		t.Errorf("expected USD, got %v", result["currency"])
	}

	// Should not overwrite existing currency.
	data2 := map[string]any{"userID": "u2", "totalCents": float64(200), "currency": "GBP"}
	result2 := upgradeOrderCreatedV1toV2(data2)
	if result2["currency"] != "GBP" {
		t.Errorf("expected GBP, got %v", result2["currency"])
	}
}
```

- [ ] **Step 3: Run tests**

Run: `cd go/order-projector && go test ./internal/consumer/ -v -race`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add go/order-projector/internal/consumer/deserializer.go go/order-projector/internal/consumer/deserializer_test.go
git commit -m "feat(order-projector): add version-aware event deserializer

Deserializes Kafka events and chains version upgraders (v1→v2→...→latest).
Demonstrates schema evolution with order.created v1→v2 (adds currency field).

Co-Authored-By: Claude Opus 4.6 (1M context) <noreply@anthropic.com>"
```

---

## Task 7: Projection Logic

**Files:**
- Create: `go/order-projector/internal/projection/timeline.go`
- Create: `go/order-projector/internal/projection/summary.go`
- Create: `go/order-projector/internal/projection/stats.go`
- Create: `go/order-projector/internal/projection/projection_test.go`

- [ ] **Step 1: Create timeline projection**

```go
// go/order-projector/internal/projection/timeline.go
package projection

import (
	"context"

	"github.com/kabradshaw1/portfolio/go/order-projector/internal/consumer"
	"github.com/kabradshaw1/portfolio/go/order-projector/internal/repository"
)

// Timeline projects events into the order_timeline table.
type Timeline struct {
	repo *repository.Repository
}

func NewTimeline(repo *repository.Repository) *Timeline {
	return &Timeline{repo: repo}
}

func (t *Timeline) Apply(ctx context.Context, evt *consumer.OrderEvent) error {
	return t.repo.InsertTimelineEvent(ctx, repository.TimelineEvent{
		EventID:      evt.ID,
		OrderID:      evt.OrderID,
		EventType:    evt.Type,
		EventVersion: evt.Version,
		Data:         evt.Data,
		Timestamp:    evt.Timestamp,
	})
}
```

- [ ] **Step 2: Create summary projection**

```go
// go/order-projector/internal/projection/summary.go
package projection

import (
	"context"
	"encoding/json"
	"time"

	"github.com/kabradshaw1/portfolio/go/order-projector/internal/consumer"
	"github.com/kabradshaw1/portfolio/go/order-projector/internal/repository"
)

// Summary projects events into the order_summary table.
type Summary struct {
	repo *repository.Repository
}

func NewSummary(repo *repository.Repository) *Summary {
	return &Summary{repo: repo}
}

func (s *Summary) Apply(ctx context.Context, evt *consumer.OrderEvent) error {
	now := evt.Timestamp

	switch evt.Type {
	case "order.created":
		var data struct {
			UserID     string          `json:"userID"`
			TotalCents int64           `json:"totalCents"`
			Currency   string          `json:"currency"`
			Items      json.RawMessage `json:"items"`
		}
		if err := json.Unmarshal(evt.Data, &data); err != nil {
			return err
		}
		return s.repo.UpsertOrderSummary(ctx, repository.OrderSummary{
			OrderID:    evt.OrderID,
			UserID:     data.UserID,
			Status:     "created",
			TotalCents: data.TotalCents,
			Currency:   data.Currency,
			Items:      data.Items,
			CreatedAt:  now,
			UpdatedAt:  now,
		})

	case "order.reserved":
		return s.repo.UpsertOrderSummary(ctx, repository.OrderSummary{
			OrderID:   evt.OrderID,
			Status:    "reserved",
			CreatedAt: now,
			UpdatedAt: now,
		})

	case "order.payment_initiated":
		return s.repo.UpsertOrderSummary(ctx, repository.OrderSummary{
			OrderID:   evt.OrderID,
			Status:    "payment_initiated",
			CreatedAt: now,
			UpdatedAt: now,
		})

	case "order.payment_completed":
		return s.repo.UpsertOrderSummary(ctx, repository.OrderSummary{
			OrderID:   evt.OrderID,
			Status:    "payment_completed",
			CreatedAt: now,
			UpdatedAt: now,
		})

	case "order.completed":
		completedAt := now
		return s.repo.UpsertOrderSummary(ctx, repository.OrderSummary{
			OrderID:     evt.OrderID,
			Status:      "completed",
			CreatedAt:   now,
			UpdatedAt:   now,
			CompletedAt: &completedAt,
		})

	case "order.failed":
		var data struct {
			FailureReason string `json:"failureReason"`
		}
		if err := json.Unmarshal(evt.Data, &data); err != nil {
			return err
		}
		reason := data.FailureReason
		return s.repo.UpsertOrderSummary(ctx, repository.OrderSummary{
			OrderID:       evt.OrderID,
			Status:        "failed",
			CreatedAt:     now,
			UpdatedAt:     now,
			FailureReason: &reason,
		})

	case "order.cancelled":
		return s.repo.UpsertOrderSummary(ctx, repository.OrderSummary{
			OrderID:   evt.OrderID,
			Status:    "cancelled",
			CreatedAt: now,
			UpdatedAt: now,
		})
	}

	return nil
}

// statusFromEventType maps event types to order statuses (for testing).
func statusFromEventType(eventType string) string {
	switch eventType {
	case "order.created":
		return "created"
	case "order.reserved":
		return "reserved"
	case "order.payment_initiated":
		return "payment_initiated"
	case "order.payment_completed":
		return "payment_completed"
	case "order.completed":
		return "completed"
	case "order.failed":
		return "failed"
	case "order.cancelled":
		return "cancelled"
	default:
		return "unknown"
	}
}

// EventTimestampOrNow returns the event timestamp if non-zero, otherwise now.
func EventTimestampOrNow(t time.Time) time.Time {
	if t.IsZero() {
		return time.Now().UTC()
	}
	return t
}
```

- [ ] **Step 3: Create stats projection**

```go
// go/order-projector/internal/projection/stats.go
package projection

import (
	"context"
	"encoding/json"
	"time"

	"github.com/kabradshaw1/portfolio/go/order-projector/internal/consumer"
	"github.com/kabradshaw1/portfolio/go/order-projector/internal/repository"
)

// Stats projects events into hourly aggregation buckets.
type Stats struct {
	repo *repository.Repository
}

func NewStats(repo *repository.Repository) *Stats {
	return &Stats{repo: repo}
}

func (s *Stats) Apply(ctx context.Context, evt *consumer.OrderEvent) error {
	bucket := evt.Timestamp.Truncate(time.Hour)

	switch evt.Type {
	case "order.created":
		return s.repo.UpsertOrderStats(ctx, bucket, 1, 0, 0, 0)

	case "order.completed":
		var data struct {
			TotalCents int64 `json:"totalCents"`
		}
		// Try to get totalCents from the event data — if not available, use 0.
		_ = json.Unmarshal(evt.Data, &data)
		return s.repo.UpsertOrderStats(ctx, bucket, 0, 1, 0, data.TotalCents)

	case "order.failed":
		return s.repo.UpsertOrderStats(ctx, bucket, 0, 0, 1, 0)
	}

	return nil
}
```

- [ ] **Step 4: Write projection tests**

```go
// go/order-projector/internal/projection/projection_test.go
package projection

import (
	"testing"
)

func TestStatusFromEventType(t *testing.T) {
	tests := []struct {
		eventType string
		expected  string
	}{
		{"order.created", "created"},
		{"order.reserved", "reserved"},
		{"order.payment_initiated", "payment_initiated"},
		{"order.payment_completed", "payment_completed"},
		{"order.completed", "completed"},
		{"order.failed", "failed"},
		{"order.cancelled", "cancelled"},
		{"order.unknown", "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.eventType, func(t *testing.T) {
			got := statusFromEventType(tt.eventType)
			if got != tt.expected {
				t.Errorf("statusFromEventType(%q) = %q, want %q", tt.eventType, got, tt.expected)
			}
		})
	}
}
```

- [ ] **Step 5: Run tests**

Run: `cd go/order-projector && go test ./internal/projection/ -v -race`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add go/order-projector/internal/projection/
git commit -m "feat(order-projector): add timeline, summary, and stats projections

Three projections consuming order events: timeline (full audit trail),
summary (latest order state via upserts), stats (hourly aggregation).

Co-Authored-By: Claude Opus 4.6 (1M context) <noreply@anthropic.com>"
```

---

## Task 8: Kafka Consumer (order-projector)

**Files:**
- Create: `go/order-projector/internal/consumer/consumer.go`
- Create: `go/order-projector/internal/metrics/metrics.go`

- [ ] **Step 1: Create metrics**

```go
// go/order-projector/internal/metrics/metrics.go
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	EventsConsumed = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "projector_events_consumed_total",
		Help: "Total order events consumed and projected",
	}, []string{"event_type"})

	ProjectionLag = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "projector_projection_lag_seconds",
		Help: "Seconds between latest event timestamp and now",
	})

	ReplayInProgress = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "projector_replay_in_progress",
		Help: "1 if replay is in progress, 0 otherwise",
	})

	ConsumerErrors = promauto.NewCounter(prometheus.CounterOpts{
		Name: "projector_consumer_errors_total",
		Help: "Total consumer errors",
	})

	ProjectionErrors = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "projector_projection_errors_total",
		Help: "Errors applying projections",
	}, []string{"projection", "event_type"})
)
```

- [ ] **Step 2: Create consumer**

```go
// go/order-projector/internal/consumer/consumer.go
package consumer

import (
	"context"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/segmentio/kafka-go"

	"github.com/kabradshaw1/portfolio/go/order-projector/internal/metrics"
	"github.com/kabradshaw1/portfolio/go/order-projector/internal/projection"
	"github.com/kabradshaw1/portfolio/go/order-projector/internal/repository"
	"github.com/kabradshaw1/portfolio/go/pkg/tracing"
)

const (
	TopicOrderEvents = "ecommerce.order-events"
	GroupID          = "order-projector-group"
)

// Consumer reads order events from Kafka and applies projections.
type Consumer struct {
	reader     *kafka.Reader
	timeline   *projection.Timeline
	summary    *projection.Summary
	stats      *projection.Stats
	connected  atomic.Bool
	processing atomic.Bool
	latestTS   atomic.Value // stores time.Time
}

// New creates a Kafka consumer for order events.
func New(brokers []string, repo *repository.Repository) *Consumer {
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:     brokers,
		GroupID:     GroupID,
		Topic:       TopicOrderEvents,
		MinBytes:    1,
		MaxBytes:    10e6,
	})

	return &Consumer{
		reader:   reader,
		timeline: projection.NewTimeline(repo),
		summary:  projection.NewSummary(repo),
		stats:    projection.NewStats(repo),
	}
}

// Connected returns whether the consumer has connected to Kafka.
func (c *Consumer) Connected() bool {
	return c.connected.Load()
}

// IsIdle returns true when not processing a message.
func (c *Consumer) IsIdle() bool {
	return !c.processing.Load()
}

// LatestEventTime returns the timestamp of the most recently consumed event.
func (c *Consumer) LatestEventTime() time.Time {
	v := c.latestTS.Load()
	if v == nil {
		return time.Time{}
	}
	return v.(time.Time)
}

// Run reads and projects events until ctx is cancelled.
func (c *Consumer) Run(ctx context.Context) error {
	slog.Info("order-projector consumer starting", "topic", TopicOrderEvents, "group", GroupID)

	for {
		msg, err := c.reader.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			slog.Error("kafka fetch error", "error", err)
			metrics.ConsumerErrors.Inc()
			continue
		}

		c.connected.Store(true)
		c.processing.Store(true)

		// Extract trace context.
		msgCtx := tracing.ExtractKafka(ctx, msg.Headers)

		evt, err := Deserialize(msg.Value)
		if err != nil {
			slog.Error("deserialize event", "error", err, "offset", msg.Offset)
			metrics.ConsumerErrors.Inc()
			c.processing.Store(false)
			// Commit to avoid reprocessing bad messages.
			_ = c.reader.CommitMessages(ctx, msg)
			continue
		}

		c.applyProjections(msgCtx, evt)

		c.latestTS.Store(evt.Timestamp)
		lagSeconds := time.Since(evt.Timestamp).Seconds()
		metrics.ProjectionLag.Set(lagSeconds)
		metrics.EventsConsumed.WithLabelValues(evt.Type).Inc()

		if err := c.reader.CommitMessages(ctx, msg); err != nil {
			slog.Error("kafka commit error", "error", err)
		}

		c.processing.Store(false)
	}
}

func (c *Consumer) applyProjections(ctx context.Context, evt *consumer.OrderEvent) {
	if err := c.timeline.Apply(ctx, evt); err != nil {
		slog.Error("timeline projection failed", "error", err, "eventID", evt.ID)
		metrics.ProjectionErrors.WithLabelValues("timeline", evt.Type).Inc()
	}
	if err := c.summary.Apply(ctx, evt); err != nil {
		slog.Error("summary projection failed", "error", err, "eventID", evt.ID)
		metrics.ProjectionErrors.WithLabelValues("summary", evt.Type).Inc()
	}
	if err := c.stats.Apply(ctx, evt); err != nil {
		slog.Error("stats projection failed", "error", err, "eventID", evt.ID)
		metrics.ProjectionErrors.WithLabelValues("stats", evt.Type).Inc()
	}
}

// ResetOffset resets the consumer group to the earliest offset for replay.
func (c *Consumer) ResetOffset() error {
	return c.reader.SetOffset(kafka.FirstOffset)
}

// Close shuts down the Kafka reader.
func (c *Consumer) Close() error {
	return c.reader.Close()
}
```

**Important fix:** The `applyProjections` method has a self-referential type issue. Since this is inside the `consumer` package and `OrderEvent` is also in the `consumer` package, the parameter type should just be `*OrderEvent`, not `*consumer.OrderEvent`. Fix the signature:

```go
func (c *Consumer) applyProjections(ctx context.Context, evt *OrderEvent) {
```

- [ ] **Step 3: Run linter**

Run: `cd go/order-projector && golangci-lint run ./...`
Expected: PASS (or expected failures for missing handler packages — those come in the next task)

- [ ] **Step 4: Commit**

```bash
git add go/order-projector/internal/consumer/consumer.go go/order-projector/internal/metrics/
git commit -m "feat(order-projector): add Kafka consumer with projection pipeline

Consumer reads from ecommerce.order-events topic, deserializes with
version upgrades, and applies timeline/summary/stats projections.
Prometheus metrics for lag, errors, and events consumed.

Co-Authored-By: Claude Opus 4.6 (1M context) <noreply@anthropic.com>"
```

---

## Task 9: HTTP Handlers

**Files:**
- Create: `go/order-projector/internal/handler/timeline.go`
- Create: `go/order-projector/internal/handler/summary.go`
- Create: `go/order-projector/internal/handler/stats.go`
- Create: `go/order-projector/internal/handler/health.go`
- Create: `go/order-projector/internal/handler/replay.go`

- [ ] **Step 1: Create timeline handler**

```go
// go/order-projector/internal/handler/timeline.go
package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/kabradshaw1/portfolio/go/order-projector/internal/repository"
	"github.com/kabradshaw1/portfolio/go/pkg/apperror"
)

type TimelineHandler struct {
	repo *repository.Repository
}

func NewTimelineHandler(repo *repository.Repository) *TimelineHandler {
	return &TimelineHandler{repo: repo}
}

func (h *TimelineHandler) GetTimeline(c *gin.Context) {
	orderID := c.Param("id")
	if orderID == "" {
		_ = c.Error(apperror.BadRequest("MISSING_ORDER_ID", "order ID is required"))
		return
	}

	events, err := h.repo.GetTimeline(c.Request.Context(), orderID)
	if err != nil {
		_ = c.Error(err)
		return
	}

	if events == nil {
		events = []repository.TimelineEvent{}
	}

	c.JSON(http.StatusOK, gin.H{"events": events})
}
```

- [ ] **Step 2: Create summary handler**

```go
// go/order-projector/internal/handler/summary.go
package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/kabradshaw1/portfolio/go/order-projector/internal/repository"
	"github.com/kabradshaw1/portfolio/go/pkg/apperror"
)

type SummaryHandler struct {
	repo *repository.Repository
}

func NewSummaryHandler(repo *repository.Repository) *SummaryHandler {
	return &SummaryHandler{repo: repo}
}

func (h *SummaryHandler) GetOrder(c *gin.Context) {
	orderID := c.Param("id")
	if orderID == "" {
		_ = c.Error(apperror.BadRequest("MISSING_ORDER_ID", "order ID is required"))
		return
	}

	summary, err := h.repo.GetOrderSummary(c.Request.Context(), orderID)
	if err != nil {
		_ = c.Error(err)
		return
	}
	if summary == nil {
		_ = c.Error(apperror.NotFound("ORDER_NOT_FOUND", "order not found in projections"))
		return
	}

	c.JSON(http.StatusOK, summary)
}

const defaultLimit = 20
const maxLimit = 100

func (h *SummaryHandler) ListOrders(c *gin.Context) {
	limit := defaultLimit
	if l := c.Query("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= maxLimit {
			limit = parsed
		}
	}

	offset := 0
	if o := c.Query("offset"); o != "" {
		if parsed, err := strconv.Atoi(o); err == nil && parsed >= 0 {
			offset = parsed
		}
	}

	summaries, err := h.repo.ListOrderSummaries(c.Request.Context(), limit, offset)
	if err != nil {
		_ = c.Error(err)
		return
	}

	if summaries == nil {
		summaries = []repository.OrderSummary{}
	}

	c.JSON(http.StatusOK, gin.H{"orders": summaries, "limit": limit, "offset": offset})
}
```

- [ ] **Step 3: Create stats handler**

```go
// go/order-projector/internal/handler/stats.go
package handler

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/kabradshaw1/portfolio/go/order-projector/internal/repository"
)

type StatsHandler struct {
	repo *repository.Repository
}

func NewStatsHandler(repo *repository.Repository) *StatsHandler {
	return &StatsHandler{repo: repo}
}

const defaultHours = 24

func (h *StatsHandler) GetOrderStats(c *gin.Context) {
	hours := defaultHours
	if h := c.Query("hours"); h != "" {
		if parsed, err := strconv.Atoi(h); err == nil && parsed > 0 {
			hours = parsed
		}
	}

	since := time.Now().UTC().Add(-time.Duration(hours) * time.Hour)
	stats, err := h.repo.GetOrderStats(c.Request.Context(), since, hours)
	if err != nil {
		_ = c.Error(err)
		return
	}

	if stats == nil {
		stats = []repository.OrderStats{}
	}

	c.JSON(http.StatusOK, gin.H{"stats": stats, "hours": hours})
}
```

- [ ] **Step 4: Create health handler**

```go
// go/order-projector/internal/handler/health.go
package handler

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kabradshaw1/portfolio/go/order-projector/internal/consumer"
)

type HealthHandler struct {
	pool     *pgxpool.Pool
	consumer *consumer.Consumer
}

func NewHealthHandler(pool *pgxpool.Pool, consumer *consumer.Consumer) *HealthHandler {
	return &HealthHandler{pool: pool, consumer: consumer}
}

func (h *HealthHandler) Health(c *gin.Context) {
	ctx := c.Request.Context()

	status := "ok"
	dbOK := true
	kafkaOK := h.consumer.Connected()

	if err := h.pool.Ping(ctx); err != nil {
		status = "degraded"
		dbOK = false
	}
	if !kafkaOK {
		status = "degraded"
	}

	// Calculate projection lag.
	var lagSeconds float64
	latestEvent := h.consumer.LatestEventTime()
	if !latestEvent.IsZero() {
		lagSeconds = time.Since(latestEvent).Seconds()
	}

	// Set projection lag header for frontend use.
	c.Header("X-Projection-Lag", time.Duration(lagSeconds*float64(time.Second)).String())

	httpStatus := http.StatusOK
	if status != "ok" {
		httpStatus = http.StatusServiceUnavailable
	}

	c.JSON(httpStatus, gin.H{
		"status":     status,
		"database":   dbOK,
		"kafka":      kafkaOK,
		"lagSeconds": lagSeconds,
	})
}
```

- [ ] **Step 5: Create replay handler**

```go
// go/order-projector/internal/handler/replay.go
package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/kabradshaw1/portfolio/go/order-projector/internal/replay"
	"github.com/kabradshaw1/portfolio/go/pkg/apperror"
)

type ReplayHandler struct {
	replayer *replay.Replayer
}

func NewReplayHandler(replayer *replay.Replayer) *ReplayHandler {
	return &ReplayHandler{replayer: replayer}
}

func (h *ReplayHandler) TriggerReplay(c *gin.Context) {
	projection := c.DefaultQuery("projection", "all")

	if err := h.replayer.Start(c.Request.Context(), projection); err != nil {
		_ = c.Error(apperror.BadRequest("REPLAY_FAILED", err.Error()))
		return
	}

	c.JSON(http.StatusAccepted, gin.H{
		"message":    "replay started",
		"projection": projection,
	})
}
```

- [ ] **Step 6: Commit**

```bash
git add go/order-projector/internal/handler/
git commit -m "feat(order-projector): add HTTP handlers for read endpoints

Timeline, summary, stats, health, and replay endpoints.
Health includes X-Projection-Lag header for frontend consistency indicators.

Co-Authored-By: Claude Opus 4.6 (1M context) <noreply@anthropic.com>"
```

---

## Task 10: Replay Coordinator

**Files:**
- Create: `go/order-projector/internal/replay/replayer.go`
- Create: `go/order-projector/internal/replay/replayer_test.go`

- [ ] **Step 1: Create replayer**

```go
// go/order-projector/internal/replay/replayer.go
package replay

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/kabradshaw1/portfolio/go/order-projector/internal/consumer"
	"github.com/kabradshaw1/portfolio/go/order-projector/internal/metrics"
	"github.com/kabradshaw1/portfolio/go/order-projector/internal/repository"
)

// Replayer coordinates event replay: truncate projections, reset offsets, rebuild.
type Replayer struct {
	repo     *repository.Repository
	consumer *consumer.Consumer
}

// New creates a replay coordinator.
func New(repo *repository.Repository, consumer *consumer.Consumer) *Replayer {
	return &Replayer{repo: repo, consumer: consumer}
}

// Start initiates a replay for the specified projection.
func (r *Replayer) Start(ctx context.Context, projection string) error {
	// Validate projection name.
	switch projection {
	case "timeline", "summary", "stats", "all":
		// valid
	default:
		return fmt.Errorf("unknown projection: %s", projection)
	}

	slog.Info("starting replay", "projection", projection)
	metrics.ReplayInProgress.Set(1)

	// Record replay start.
	if err := r.repo.StartReplay(ctx, projection); err != nil {
		metrics.ReplayInProgress.Set(0)
		return fmt.Errorf("record replay start: %w", err)
	}

	// Truncate target tables.
	if err := r.repo.TruncateProjection(ctx, projection); err != nil {
		metrics.ReplayInProgress.Set(0)
		return fmt.Errorf("truncate projection: %w", err)
	}

	// Reset consumer offset to earliest.
	if err := r.consumer.ResetOffset(); err != nil {
		slog.Error("failed to reset kafka offset", "error", err)
		// Non-fatal: consumer will pick up from current offset.
	}

	slog.Info("replay initiated — consumer will rebuild from earliest offset", "projection", projection)

	// The consumer's Run loop will process all events from the beginning.
	// Replay completion is tracked via the metrics and replay_status table.
	// A background goroutine monitors completion is optional for v1.

	return nil
}
```

- [ ] **Step 2: Write test**

```go
// go/order-projector/internal/replay/replayer_test.go
package replay

import (
	"testing"
)

func TestReplayerValidation(t *testing.T) {
	r := &Replayer{} // nil repo/consumer — just testing validation

	tests := []struct {
		projection string
		wantErr    bool
	}{
		{"timeline", false},
		{"summary", false},
		{"stats", false},
		{"all", false},
		{"invalid", true},
	}

	for _, tt := range tests {
		t.Run(tt.projection, func(t *testing.T) {
			err := r.Start(t.Context(), tt.projection)
			// All will fail (nil repo) but "invalid" should fail with validation error.
			if tt.wantErr {
				if err == nil {
					t.Error("expected error for invalid projection")
				}
			}
			// For valid projections, we expect a nil-pointer panic or repo error,
			// not a validation error. Skip those — integration test covers them.
		})
	}
}
```

Note: This test will panic on valid projections due to nil repo. The proper validation test should use `recover`. Better to test just the invalid case:

```go
// go/order-projector/internal/replay/replayer_test.go
package replay

import (
	"context"
	"strings"
	"testing"
)

func TestReplayerRejectsInvalidProjection(t *testing.T) {
	r := &Replayer{}
	err := r.Start(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for invalid projection")
	}
	if !strings.Contains(err.Error(), "unknown projection") {
		t.Errorf("expected 'unknown projection' error, got: %v", err)
	}
}
```

- [ ] **Step 3: Run tests**

Run: `cd go/order-projector && go test ./internal/replay/ -v -race`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add go/order-projector/internal/replay/
git commit -m "feat(order-projector): add replay coordinator

Truncates target projection tables, resets Kafka consumer offset to
earliest, and lets the consumer rebuild from the full event log.

Co-Authored-By: Claude Opus 4.6 (1M context) <noreply@anthropic.com>"
```

---

## Task 11: Wire Everything Together and Verify Compilation

**Files:**
- Modify: `go/order-projector/cmd/server/main.go` (finalize imports)
- Modify: `go/order-projector/go.mod` (tidy)

- [ ] **Step 1: Run go mod tidy**

Run: `cd go/order-projector && go mod tidy`

- [ ] **Step 2: Run full linter**

Run: `cd go/order-projector && golangci-lint run ./...`
Fix any lint issues.

- [ ] **Step 3: Run all tests**

Run: `cd go/order-projector && go test ./... -v -race`
Expected: PASS

- [ ] **Step 4: Verify Docker build**

Run: `cd go && docker build -f order-projector/Dockerfile -t order-projector:test .`
Expected: Build succeeds (requires Colima running: `colima start`)

- [ ] **Step 5: Commit**

```bash
git add go/order-projector/
git commit -m "feat(order-projector): wire all components and verify build

All packages compile, tests pass, linter clean, Docker image builds.

Co-Authored-By: Claude Opus 4.6 (1M context) <noreply@anthropic.com>"
```

---

## Task 12: Kubernetes Manifests

**Files:**
- Create: `go/k8s/deployments/order-projector.yml`
- Create: `go/k8s/services/order-projector.yml`
- Create: `go/k8s/configmaps/order-projector-config.yml`
- Create: `go/k8s/jobs/order-projector-migrate.yml`
- Modify: `go/k8s/ingress.yml`

- [ ] **Step 1: Create deployment**

```yaml
# go/k8s/deployments/order-projector.yml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: go-order-projector
  namespace: go-ecommerce
spec:
  replicas: 1
  selector:
    matchLabels:
      app: go-order-projector
  template:
    metadata:
      labels:
        app: go-order-projector
      annotations:
        prometheus.io/scrape: "true"
        prometheus.io/port: "8097"
        prometheus.io/path: "/metrics"
    spec:
      terminationGracePeriodSeconds: 20
      imagePullSecrets:
        - name: ghcr-secret
      containers:
        - name: go-order-projector
          image: ghcr.io/kabradshaw1/portfolio/go-order-projector:latest
          imagePullPolicy: Always
          ports:
            - containerPort: 8097
          envFrom:
            - configMapRef:
                name: order-projector-config
          securityContext:
            runAsNonRoot: true
            runAsUser: 1001
            readOnlyRootFilesystem: true
            allowPrivilegeEscalation: false
            capabilities:
              drop:
                - ALL
          resources:
            requests:
              memory: "64Mi"
              cpu: "100m"
            limits:
              memory: "256Mi"
              cpu: "500m"
          readinessProbe:
            httpGet:
              path: /health
              port: 8097
            initialDelaySeconds: 5
            periodSeconds: 10
          livenessProbe:
            httpGet:
              path: /health
              port: 8097
            initialDelaySeconds: 15
            periodSeconds: 30
```

- [ ] **Step 2: Create service**

```yaml
# go/k8s/services/order-projector.yml
apiVersion: v1
kind: Service
metadata:
  name: go-order-projector
  namespace: go-ecommerce
spec:
  selector:
    app: go-order-projector
  ports:
    - port: 8097
      targetPort: 8097
```

- [ ] **Step 3: Create configmap**

```yaml
# go/k8s/configmaps/order-projector-config.yml
apiVersion: v1
kind: ConfigMap
metadata:
  name: order-projector-config
  namespace: go-ecommerce
data:
  DATABASE_URL: "postgres://postgres:postgres@postgres.go-ecommerce.svc.cluster.local:5432/projectordb?sslmode=disable"
  KAFKA_BROKERS: "kafka.go-ecommerce.svc.cluster.local:9092"
  PORT: "8097"
  ALLOWED_ORIGINS: "http://localhost:3000,https://kylebradshaw.dev"
  OTEL_EXPORTER_OTLP_ENDPOINT: "jaeger.monitoring.svc.cluster.local:4317"
```

- [ ] **Step 4: Create migration job**

```yaml
# go/k8s/jobs/order-projector-migrate.yml
apiVersion: batch/v1
kind: Job
metadata:
  name: go-projector-migrate
  namespace: go-ecommerce
spec:
  ttlSecondsAfterFinished: 600
  template:
    spec:
      restartPolicy: Never
      containers:
        - name: migrate
          image: migrate/migrate:v4.18.1
          command:
            - migrate
            - -path
            - /migrations
            - -database
            - postgres://postgres:postgres@postgres.go-ecommerce.svc.cluster.local:5432/projectordb?sslmode=disable&x-migrations-table=projector_schema_migrations
            - up
          volumeMounts:
            - name: migrations
              mountPath: /migrations
      volumes:
        - name: migrations
          configMap:
            name: order-projector-migrations
```

Note: The migration ConfigMap approach depends on how other services handle it. If migrations are baked into the Docker image instead, adjust accordingly. Check the existing migration job pattern.

- [ ] **Step 5: Add ingress path**

Add to `go/k8s/ingress.yml` after the `/go-payments` entry:

```yaml
          - path: /go-projector(/|$)(.*)
            pathType: ImplementationSpecific
            backend:
              service:
                name: go-order-projector
                port:
                  number: 8097
```

- [ ] **Step 6: Commit**

```bash
git add go/k8s/deployments/order-projector.yml go/k8s/services/order-projector.yml go/k8s/configmaps/order-projector-config.yml go/k8s/jobs/order-projector-migrate.yml go/k8s/ingress.yml
git commit -m "feat(order-projector): add Kubernetes deployment manifests

Deployment, service, configmap, migration job, and ingress route
for the order-projector service in go-ecommerce namespace.

Co-Authored-By: Claude Opus 4.6 (1M context) <noreply@anthropic.com>"
```

---

## Task 13: CI/CD Integration

**Files:**
- Modify: `.github/workflows/ci.yml`
- Modify: `Makefile`

- [ ] **Step 1: Add order-projector to CI lint/test matrices**

In `.github/workflows/ci.yml`, add `order-projector` to the Go service matrices:

Line 175 (go-lint matrix):
```yaml
service: [auth-service, order-service, ai-service, analytics-service, product-service, cart-service, order-projector]
```

Line 193 (go-tests matrix):
```yaml
service: [auth-service, order-service, ai-service, analytics-service, product-service, cart-service, order-projector]
```

- [ ] **Step 2: Add order-projector to build matrix**

After the `go-payment-service` entry (line 783), add:

```yaml
          - service: go-order-projector
            context: go
            file: go/order-projector/Dockerfile
            image: go-order-projector
            paths: go/order-projector go/pkg go/go.work
```

- [ ] **Step 3: Add order-projector to deploy sections**

In the QA deploy section, add migration and deployment steps for order-projector (after the payment-service block). Follow the same pattern: apply migration job → wait → apply deployment.

In the prod deploy section, add the same.

- [ ] **Step 4: Add order-projector to Makefile preflight-go**

Add to `Makefile` after the analytics-service entries:

```makefile
	cd go/order-projector && golangci-lint run ./...
```

and:

```makefile
	cd go/order-projector && go test ./... -v -race
```

- [ ] **Step 5: Add to migration pipeline test (if applicable)**

Check lines 65+ in Makefile for the migration pipeline test. Add order-projector migrations to the test.

- [ ] **Step 6: Commit**

```bash
git add .github/workflows/ci.yml Makefile
git commit -m "ci: add order-projector to lint, test, build, and deploy pipelines

Co-Authored-By: Claude Opus 4.6 (1M context) <noreply@anthropic.com>"
```

---

## Task 14: Frontend — API Client and Projector Types

**Files:**
- Create: `frontend/src/lib/go-projector-api.ts`

- [ ] **Step 1: Create projector API client**

```typescript
// frontend/src/lib/go-projector-api.ts
import { refreshGoAccessToken } from "@/lib/go-auth";

export const GO_PROJECTOR_URL =
  process.env.NEXT_PUBLIC_GO_PROJECTOR_URL || "http://localhost:8097";

export async function goProjectorFetch(
  path: string,
  options: RequestInit = {},
): Promise<Response> {
  const headers = new Headers(options.headers);
  if (!headers.has("Content-Type") && options.body) {
    headers.set("Content-Type", "application/json");
  }

  const res = await fetch(`${GO_PROJECTOR_URL}${path}`, {
    ...options,
    headers,
    credentials: "include",
  });

  if (res.status === 401 || res.status === 403) {
    const success = await refreshGoAccessToken();
    if (success) {
      return fetch(`${GO_PROJECTOR_URL}${path}`, {
        ...options,
        headers,
        credentials: "include",
      });
    }
  }

  return res;
}

// Types for projector API responses.
export interface TimelineEvent {
  eventId: string;
  orderId: string;
  eventType: string;
  eventVersion: number;
  data: Record<string, unknown>;
  timestamp: string;
}

export interface OrderSummary {
  orderId: string;
  userId: string;
  status: string;
  totalCents: number;
  currency: string;
  items?: Record<string, unknown>[];
  createdAt: string;
  updatedAt: string;
  completedAt?: string;
  failureReason?: string;
}

export interface OrderStats {
  hourBucket: string;
  ordersCreated: number;
  ordersCompleted: number;
  ordersFailed: number;
  avgCompletionSeconds: number;
  totalRevenueCents: number;
}
```

- [ ] **Step 2: Commit**

```bash
git add frontend/src/lib/go-projector-api.ts
git commit -m "feat(frontend): add projector API client and types

Co-Authored-By: Claude Opus 4.6 (1M context) <noreply@anthropic.com>"
```

---

## Task 15: Frontend — Order Timeline Component

**Files:**
- Create: `frontend/src/components/go/OrderTimeline.tsx`

- [ ] **Step 1: Read Next.js docs for current patterns**

Before writing frontend code, check the AGENTS.md instruction:
Run: Read `frontend/node_modules/next/dist/docs/` for any relevant guides on client components.

- [ ] **Step 2: Create OrderTimeline component**

```tsx
// frontend/src/components/go/OrderTimeline.tsx
"use client";

import { CheckCircle, Circle, XCircle, Clock, CreditCard, Package, ShoppingCart } from "lucide-react";
import type { TimelineEvent } from "@/lib/go-projector-api";

interface OrderTimelineProps {
  events: TimelineEvent[];
}

function eventIcon(type: string) {
  switch (type) {
    case "order.created":
      return <ShoppingCart className="size-4" />;
    case "order.reserved":
      return <Package className="size-4" />;
    case "order.payment_initiated":
      return <CreditCard className="size-4" />;
    case "order.payment_completed":
      return <CreditCard className="size-4" />;
    case "order.completed":
      return <CheckCircle className="size-4" />;
    case "order.failed":
      return <XCircle className="size-4" />;
    case "order.cancelled":
      return <XCircle className="size-4" />;
    default:
      return <Circle className="size-4" />;
  }
}

function eventColor(type: string): string {
  if (type === "order.completed") return "text-green-500";
  if (type === "order.failed" || type === "order.cancelled") return "text-red-500";
  if (type.includes("payment")) return "text-blue-500";
  return "text-yellow-500";
}

function eventLabel(type: string): string {
  switch (type) {
    case "order.created": return "Order Created";
    case "order.reserved": return "Stock Reserved";
    case "order.payment_initiated": return "Payment Initiated";
    case "order.payment_completed": return "Payment Completed";
    case "order.completed": return "Order Completed";
    case "order.failed": return "Order Failed";
    case "order.cancelled": return "Order Cancelled";
    default: return type;
  }
}

function eventDescription(event: TimelineEvent): string {
  const data = event.data;
  switch (event.eventType) {
    case "order.created": {
      const items = (data.items as unknown[]) || [];
      const total = data.totalCents as number;
      return `${items.length} item${items.length !== 1 ? "s" : ""}, $${(total / 100).toFixed(2)}`;
    }
    case "order.reserved":
      return "All items available";
    case "order.payment_initiated":
      return `${data.paymentProvider || "Stripe"} checkout created`;
    case "order.payment_completed": {
      const amount = data.amountCents as number;
      return `Payment confirmed, $${(amount / 100).toFixed(2)}`;
    }
    case "order.completed":
      return "Fulfilled";
    case "order.failed":
      return `${data.failureReason || "Unknown failure"}`;
    case "order.cancelled":
      return `${data.cancelReason || "Cancelled"}`;
    default:
      return "";
  }
}

export default function OrderTimeline({ events }: OrderTimelineProps) {
  if (events.length === 0) {
    return <p className="text-muted-foreground text-sm">No events recorded.</p>;
  }

  return (
    <div className="relative space-y-6 pl-6">
      {/* Vertical line */}
      <div className="absolute left-[11px] top-2 bottom-2 w-px bg-foreground/10" />

      {events.map((event) => (
        <div key={event.eventId} className="relative flex items-start gap-3">
          <div className={`relative z-10 rounded-full bg-background p-0.5 ${eventColor(event.eventType)}`}>
            {eventIcon(event.eventType)}
          </div>
          <div className="flex-1 min-w-0">
            <div className="flex items-center justify-between gap-2">
              <p className="text-sm font-medium">{eventLabel(event.eventType)}</p>
              <time className="text-xs text-muted-foreground whitespace-nowrap">
                {new Date(event.timestamp).toLocaleString()}
              </time>
            </div>
            <p className="text-xs text-muted-foreground mt-0.5">
              {eventDescription(event)}
            </p>
          </div>
        </div>
      ))}
    </div>
  );
}
```

- [ ] **Step 3: Commit**

```bash
git add frontend/src/components/go/OrderTimeline.tsx
git commit -m "feat(frontend): add OrderTimeline component

Vertical timeline with color-coded event icons showing each saga step.

Co-Authored-By: Claude Opus 4.6 (1M context) <noreply@anthropic.com>"
```

---

## Task 16: Frontend — Order Detail Page

**Files:**
- Create: `frontend/src/app/go/ecommerce/orders/[id]/page.tsx`
- Modify: `frontend/src/app/go/ecommerce/orders/page.tsx`

- [ ] **Step 1: Create order detail page**

```tsx
// frontend/src/app/go/ecommerce/orders/[id]/page.tsx
"use client";

import { useEffect, useState } from "react";
import { useParams, useRouter } from "next/navigation";
import Link from "next/link";
import { ArrowLeft } from "lucide-react";
import { goProjectorFetch } from "@/lib/go-projector-api";
import type { TimelineEvent, OrderSummary } from "@/lib/go-projector-api";
import OrderTimeline from "@/components/go/OrderTimeline";

export default function OrderDetailPage() {
  const params = useParams();
  const router = useRouter();
  const orderId = params.id as string;

  const [summary, setSummary] = useState<OrderSummary | null>(null);
  const [events, setEvents] = useState<TimelineEvent[]>([]);
  const [loading, setLoading] = useState(true);
  const [projectionLag, setProjectionLag] = useState<string | null>(null);

  useEffect(() => {
    if (!orderId) return;

    Promise.all([
      goProjectorFetch(`/orders/${orderId}`).then((r) => {
        if (r.status === 401 || r.status === 403) {
          router.replace("/go/login?next=/go/ecommerce/orders");
          return null;
        }
        return r.ok ? r.json() : null;
      }),
      goProjectorFetch(`/orders/${orderId}/timeline`).then((r) => {
        if (r.ok) {
          const lag = r.headers.get("X-Projection-Lag");
          if (lag) setProjectionLag(lag);
          return r.json();
        }
        return null;
      }),
    ])
      .then(([summaryData, timelineData]) => {
        if (summaryData) setSummary(summaryData);
        if (timelineData?.events) setEvents(timelineData.events);
      })
      .catch(() => {})
      .finally(() => setLoading(false));
  }, [orderId, router]);

  if (loading) {
    return (
      <div className="mx-auto max-w-3xl px-6 py-12">
        <p className="text-muted-foreground">Loading order...</p>
      </div>
    );
  }

  return (
    <div className="mx-auto max-w-3xl px-6 py-12">
      <Link
        href="/go/ecommerce/orders"
        className="inline-flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground transition-colors"
      >
        <ArrowLeft className="size-4" />
        Back to orders
      </Link>

      {projectionLag && (
        <div className="mt-4 rounded-lg border border-yellow-500/20 bg-yellow-500/5 px-4 py-2 text-sm text-yellow-600">
          Projection lag: {projectionLag}
        </div>
      )}

      {summary && (
        <div className="mt-6">
          <h1 className="text-2xl font-bold">Order {summary.orderId.slice(0, 8)}...</h1>
          <div className="mt-4 grid grid-cols-2 gap-4">
            <div>
              <p className="text-sm text-muted-foreground">Status</p>
              <p className="font-medium">{summary.status}</p>
            </div>
            <div>
              <p className="text-sm text-muted-foreground">Total</p>
              <p className="font-medium">${(summary.totalCents / 100).toFixed(2)} {summary.currency}</p>
            </div>
            <div>
              <p className="text-sm text-muted-foreground">Created</p>
              <p className="font-medium">{new Date(summary.createdAt).toLocaleString()}</p>
            </div>
            {summary.completedAt && (
              <div>
                <p className="text-sm text-muted-foreground">Completed</p>
                <p className="font-medium">{new Date(summary.completedAt).toLocaleString()}</p>
              </div>
            )}
          </div>
        </div>
      )}

      <div className="mt-8">
        <h2 className="text-lg font-semibold mb-4">Event Timeline</h2>
        <OrderTimeline events={events} />
      </div>
    </div>
  );
}
```

- [ ] **Step 2: Make orders list items clickable**

Modify `frontend/src/app/go/ecommerce/orders/page.tsx` to wrap each order in a Link:

Add import:
```tsx
import Link from "next/link";
```

Change the order card `div` (around line 81-99) to be a Link:

Replace:
```tsx
<div
  key={order.id}
  className="flex items-center justify-between rounded-lg border border-foreground/10 p-4"
>
```

With:
```tsx
<Link
  key={order.id}
  href={`/go/ecommerce/orders/${order.id}`}
  className="flex items-center justify-between rounded-lg border border-foreground/10 p-4 hover:border-foreground/20 transition-colors"
>
```

And change the closing `</div>` to `</Link>`.

Note: `Link` is already imported in the file (line 3) for the back link, so just change the element.

- [ ] **Step 3: Run frontend checks**

Run: `cd frontend && npx tsc --noEmit && npx next lint`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add frontend/src/app/go/ecommerce/orders/
git commit -m "feat(frontend): add order detail page with event timeline

Clicking an order navigates to a detail page showing the projected
summary and a full event timeline from the CQRS projector.

Co-Authored-By: Claude Opus 4.6 (1M context) <noreply@anthropic.com>"
```

---

## Task 17: Frontend — Stats Card and Consistency Banner

**Files:**
- Create: `frontend/src/components/go/OrderStatsCard.tsx`
- Create: `frontend/src/components/go/ProjectionLagBanner.tsx`
- Modify: `frontend/src/app/go/ecommerce/orders/page.tsx`

- [ ] **Step 1: Create OrderStatsCard**

```tsx
// frontend/src/components/go/OrderStatsCard.tsx
"use client";

import { useEffect, useState } from "react";
import { goProjectorFetch } from "@/lib/go-projector-api";
import type { OrderStats } from "@/lib/go-projector-api";

export default function OrderStatsCard() {
  const [stats, setStats] = useState<OrderStats[]>([]);

  useEffect(() => {
    goProjectorFetch("/stats/orders?hours=24")
      .then((r) => (r.ok ? r.json() : null))
      .then((data) => {
        if (data?.stats) setStats(data.stats);
      })
      .catch(() => {});
  }, []);

  const totals = stats.reduce(
    (acc, s) => ({
      created: acc.created + s.ordersCreated,
      completed: acc.completed + s.ordersCompleted,
      failed: acc.failed + s.ordersFailed,
      revenue: acc.revenue + s.totalRevenueCents,
    }),
    { created: 0, completed: 0, failed: 0, revenue: 0 },
  );

  const completionRate =
    totals.created > 0
      ? Math.round((totals.completed / totals.created) * 100)
      : 0;

  return (
    <div className="grid grid-cols-4 gap-4 rounded-lg border border-foreground/10 p-4">
      <div>
        <p className="text-xs text-muted-foreground">Orders (24h)</p>
        <p className="text-lg font-semibold">{totals.created}</p>
      </div>
      <div>
        <p className="text-xs text-muted-foreground">Completed</p>
        <p className="text-lg font-semibold text-green-500">{totals.completed}</p>
      </div>
      <div>
        <p className="text-xs text-muted-foreground">Completion Rate</p>
        <p className="text-lg font-semibold">{completionRate}%</p>
      </div>
      <div>
        <p className="text-xs text-muted-foreground">Revenue (24h)</p>
        <p className="text-lg font-semibold">${(totals.revenue / 100).toFixed(2)}</p>
      </div>
    </div>
  );
}
```

- [ ] **Step 2: Create ProjectionLagBanner**

```tsx
// frontend/src/components/go/ProjectionLagBanner.tsx
"use client";

import { useEffect, useState } from "react";
import { goProjectorFetch } from "@/lib/go-projector-api";

export default function ProjectionLagBanner() {
  const [status, setStatus] = useState<{
    lagSeconds: number;
    isReplaying?: boolean;
  } | null>(null);

  useEffect(() => {
    goProjectorFetch("/health")
      .then((r) => (r.ok ? r.json() : null))
      .then((data) => {
        if (data) setStatus(data);
      })
      .catch(() => {});
  }, []);

  if (!status) return null;

  if (status.isReplaying) {
    return (
      <div className="rounded-lg border border-blue-500/20 bg-blue-500/5 px-4 py-2 text-sm text-blue-600">
        Read models rebuilding — data may be incomplete
      </div>
    );
  }

  const lagThreshold = 5; // seconds
  if (status.lagSeconds > lagThreshold) {
    return (
      <div className="rounded-lg border border-yellow-500/20 bg-yellow-500/5 px-4 py-2 text-sm text-yellow-600">
        Data may be slightly behind — projections updating ({Math.round(status.lagSeconds)}s lag)
      </div>
    );
  }

  return null;
}
```

- [ ] **Step 3: Add stats card and banner to orders page**

Modify `frontend/src/app/go/ecommerce/orders/page.tsx`:

Add imports:
```tsx
import OrderStatsCard from "@/components/go/OrderStatsCard";
import ProjectionLagBanner from "@/components/go/ProjectionLagBanner";
```

Add between the `<h1>` and the orders list:
```tsx
<h1 className="mt-6 text-2xl font-bold">Orders</h1>

<div className="mt-4 space-y-4">
  <ProjectionLagBanner />
  <OrderStatsCard />
</div>
```

- [ ] **Step 4: Run frontend checks**

Run: `cd frontend && npx tsc --noEmit && npx next lint`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/go/OrderStatsCard.tsx frontend/src/components/go/ProjectionLagBanner.tsx frontend/src/app/go/ecommerce/orders/page.tsx
git commit -m "feat(frontend): add order stats dashboard and consistency indicators

Stats card shows 24h order metrics. Projection lag banner warns when
data may be stale or replay is in progress.

Co-Authored-By: Claude Opus 4.6 (1M context) <noreply@anthropic.com>"
```

---

## Task 18: Preflight Verification

- [ ] **Step 1: Run Go preflight**

Run: `make preflight-go`
Expected: All services pass lint + tests including order-projector

- [ ] **Step 2: Run frontend preflight**

Run: `make preflight-frontend`
Expected: tsc + lint + build pass

- [ ] **Step 3: Fix any failures**

Address lint errors, type errors, or test failures before proceeding.

- [ ] **Step 4: Final commit if needed**

```bash
git add -A
git commit -m "fix: address preflight issues

Co-Authored-By: Claude Opus 4.6 (1M context) <noreply@anthropic.com>"
```

---

## Verification Plan

1. `make preflight-go` — all Go services pass lint + tests
2. `make preflight-frontend` — TypeScript compiles, lint passes
3. Docker build: `cd go && docker build -f order-projector/Dockerfile -t test .`
4. E2E (after deploy to QA):
   - Create an order through checkout
   - Verify events in `ecommerce.order-events` topic
   - `GET /go-projector/orders/:id/timeline` returns saga steps
   - `GET /go-projector/orders/:id` returns projected summary
   - `GET /go-projector/stats/orders` returns hourly stats
   - `POST /go-projector/admin/replay` triggers rebuild
   - Frontend timeline renders on order detail page
   - Stats card visible on orders list
5. Schema evolution: After deploy, update `CurrentVersion` to 2 in order-service events, verify projector handles both v1 and v2
