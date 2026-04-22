# Payment Service + Saga Evolution Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a Stripe-integrated payment service to the ecommerce platform and extend the checkout saga with payment processing, webhook handling, and a transactional outbox.

**Architecture:** New `payment-service` (REST :8098, gRPC :9098) owns all Stripe interactions. Order-service's saga gains two new steps (PAYMENT_CREATED → PAYMENT_CONFIRMED) between stock validation and cart clearing. A transactional outbox in payment-service guarantees reliable RabbitMQ event delivery. Kafka events feed analytics.

**Tech Stack:** Go, Stripe Go SDK (`github.com/stripe/stripe-go/v82`), PostgreSQL (paymentdb), RabbitMQ (saga events), Kafka (analytics), gRPC + protobuf, `golang-migrate`

**Spec:** `docs/superpowers/specs/2026-04-22-payment-service-sql-optimization-design.md` (Track 1)

---

### Task 1: Proto Definition + Code Generation

**Files:**
- Create: `go/proto/payment/v1/payment.proto`
- Create: `go/buf.gen.payment.yaml`

- [ ] **Step 1: Create the proto file**

Create `go/proto/payment/v1/payment.proto`:

```protobuf
syntax = "proto3";

package payment.v1;

option go_package = "github.com/kabradshaw1/portfolio/go/payment-service/pb/payment/v1";

import "google/protobuf/timestamp.proto";

service PaymentService {
  rpc CreatePayment(CreatePaymentRequest) returns (CreatePaymentResponse);
  rpc GetPaymentStatus(GetPaymentStatusRequest) returns (GetPaymentStatusResponse);
  rpc RefundPayment(RefundPaymentRequest) returns (RefundPaymentResponse);
}

message CreatePaymentRequest {
  string order_id = 1;
  int32 amount_cents = 2;
  string currency = 3;
  string success_url = 4;
  string cancel_url = 5;
}

message CreatePaymentResponse {
  string payment_id = 1;
  string checkout_session_url = 2;
  string status = 3;
}

message GetPaymentStatusRequest {
  string order_id = 1;
}

message GetPaymentStatusResponse {
  string payment_id = 1;
  string order_id = 2;
  string status = 3;
  int32 amount_cents = 4;
  string currency = 5;
  google.protobuf.Timestamp created_at = 6;
}

message RefundPaymentRequest {
  string order_id = 1;
  string reason = 2;
}

message RefundPaymentResponse {
  string payment_id = 1;
  string status = 2;
  string stripe_refund_id = 3;
}
```

- [ ] **Step 2: Create buf generation config**

Create `go/buf.gen.payment.yaml`:

```yaml
version: v2
plugins:
  - remote: buf.build/protocolbuffers/go
    out: payment-service/pb
    opt: paths=source_relative
  - remote: buf.build/grpc/go
    out: payment-service/pb
    opt: paths=source_relative
```

- [ ] **Step 3: Lint and generate**

```bash
cd go && buf lint --path proto/payment
cd go && buf generate --path proto/payment --template buf.gen.payment.yaml
```

Expected: Generated files at `go/payment-service/pb/payment/v1/payment.pb.go` and `payment_grpc.pb.go`.

- [ ] **Step 4: Commit**

```bash
git add go/proto/payment/ go/buf.gen.payment.yaml go/payment-service/pb/
git commit -m "feat(payment): add payment-service proto definition and generated code"
```

---

### Task 2: Payment Service Scaffold — Module, Config, Dockerfile

**Files:**
- Create: `go/payment-service/go.mod`
- Create: `go/payment-service/cmd/server/config.go`
- Create: `go/payment-service/Dockerfile`

- [ ] **Step 1: Initialize go module**

```bash
cd go/payment-service && go mod init github.com/kabradshaw1/portfolio/go/payment-service
```

Then edit `go/payment-service/go.mod` to add the replace directive:

```
replace github.com/kabradshaw1/portfolio/go/pkg => ../pkg
```

- [ ] **Step 2: Create config.go**

Create `go/payment-service/cmd/server/config.go`:

```go
package main

import (
	"log"
	"os"
)

type Config struct {
	DatabaseURL        string // required — paymentdb connection
	Port               string // default "8098"
	GRPCPort           string // default "9098"
	StripeSecretKey    string // required — Stripe API secret key
	StripeWebhookSecret string // required — Stripe webhook signing secret
	RabbitmqURL        string // required — for saga event publishing
	KafkaBrokers       string // optional — for analytics events
	OTELEndpoint       string // optional — OTLP gRPC endpoint
	AllowedOrigins     string // default "http://localhost:3000"
	TLSCertDir         string // optional — mTLS cert directory
}

func loadConfig() Config {
	cfg := Config{
		DatabaseURL:        os.Getenv("DATABASE_URL"),
		Port:               os.Getenv("PORT"),
		GRPCPort:           os.Getenv("GRPC_PORT"),
		StripeSecretKey:    os.Getenv("STRIPE_SECRET_KEY"),
		StripeWebhookSecret: os.Getenv("STRIPE_WEBHOOK_SECRET"),
		RabbitmqURL:        os.Getenv("RABBITMQ_URL"),
		KafkaBrokers:       os.Getenv("KAFKA_BROKERS"),
		OTELEndpoint:       os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"),
		AllowedOrigins:     os.Getenv("ALLOWED_ORIGINS"),
		TLSCertDir:         os.Getenv("TLS_CERT_DIR"),
	}

	if cfg.DatabaseURL == "" {
		log.Fatal("DATABASE_URL is required")
	}
	if cfg.StripeSecretKey == "" {
		log.Fatal("STRIPE_SECRET_KEY is required")
	}
	if cfg.StripeWebhookSecret == "" {
		log.Fatal("STRIPE_WEBHOOK_SECRET is required")
	}
	if cfg.RabbitmqURL == "" {
		log.Fatal("RABBITMQ_URL is required")
	}
	if cfg.Port == "" {
		cfg.Port = "8098"
	}
	if cfg.GRPCPort == "" {
		cfg.GRPCPort = "9098"
	}
	if cfg.AllowedOrigins == "" {
		cfg.AllowedOrigins = "http://localhost:3000"
	}

	return cfg
}
```

- [ ] **Step 3: Create Dockerfile**

Create `go/payment-service/Dockerfile`:

```dockerfile
FROM migrate/migrate:v4.17.0 AS migrate

FROM golang:1.24-alpine AS builder
WORKDIR /app/payment-service
COPY pkg/ /app/pkg/
COPY payment-service/go.mod payment-service/go.sum ./
RUN go mod download
COPY payment-service/ .
RUN CGO_ENABLED=0 GOOS=linux go build -o /payment-service ./cmd/server

FROM alpine:3.19
RUN apk add --no-cache postgresql-client && adduser -D -u 1001 appuser
COPY --from=builder /payment-service /payment-service
COPY --from=migrate /usr/local/bin/migrate /usr/local/bin/migrate
COPY payment-service/migrations/ /migrations/
USER appuser
EXPOSE 8098 9098
ENTRYPOINT ["/payment-service"]
```

- [ ] **Step 4: Commit**

```bash
git add go/payment-service/go.mod go/payment-service/cmd/server/config.go go/payment-service/Dockerfile
git commit -m "feat(payment): scaffold payment-service module, config, and Dockerfile"
```

---

### Task 3: Database Migrations

**Files:**
- Create: `go/payment-service/migrations/001_create_payments.up.sql`
- Create: `go/payment-service/migrations/001_create_payments.down.sql`
- Create: `go/payment-service/migrations/002_create_processed_events.up.sql`
- Create: `go/payment-service/migrations/002_create_processed_events.down.sql`
- Create: `go/payment-service/migrations/003_create_outbox.up.sql`
- Create: `go/payment-service/migrations/003_create_outbox.down.sql`

- [ ] **Step 1: Create payments table migration**

`go/payment-service/migrations/001_create_payments.up.sql`:

```sql
CREATE TABLE payments (
    id                         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id                   UUID NOT NULL UNIQUE,
    stripe_payment_intent_id   TEXT UNIQUE,
    stripe_checkout_session_id TEXT,
    amount_cents               INTEGER NOT NULL CHECK (amount_cents > 0),
    currency                   TEXT NOT NULL DEFAULT 'usd',
    status                     TEXT NOT NULL DEFAULT 'pending',
    idempotency_key            TEXT NOT NULL UNIQUE,
    created_at                 TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at                 TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_payments_order_id ON payments(order_id);
CREATE INDEX idx_payments_status ON payments(status);
```

`go/payment-service/migrations/001_create_payments.down.sql`:

```sql
DROP TABLE IF EXISTS payments;
```

- [ ] **Step 2: Create processed_events table migration**

`go/payment-service/migrations/002_create_processed_events.up.sql`:

```sql
CREATE TABLE processed_events (
    stripe_event_id TEXT PRIMARY KEY,
    event_type      TEXT NOT NULL,
    processed_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

`go/payment-service/migrations/002_create_processed_events.down.sql`:

```sql
DROP TABLE IF EXISTS processed_events;
```

- [ ] **Step 3: Create outbox table migration**

`go/payment-service/migrations/003_create_outbox.up.sql`:

```sql
CREATE TABLE outbox (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    exchange    TEXT NOT NULL,
    routing_key TEXT NOT NULL,
    payload     JSONB NOT NULL,
    published   BOOLEAN NOT NULL DEFAULT false,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_outbox_unpublished ON outbox (created_at) WHERE published = false;
```

`go/payment-service/migrations/003_create_outbox.down.sql`:

```sql
DROP TABLE IF EXISTS outbox;
```

- [ ] **Step 4: Commit**

```bash
git add go/payment-service/migrations/
git commit -m "feat(payment): add database migrations for payments, processed_events, and outbox"
```

---

### Task 4: Payment Repository

**Files:**
- Create: `go/payment-service/internal/model/payment.go`
- Create: `go/payment-service/internal/repository/payment.go`
- Create: `go/payment-service/internal/repository/payment_test.go`

- [ ] **Step 1: Create the domain model**

Create `go/payment-service/internal/model/payment.go`:

```go
package model

import (
	"time"

	"github.com/google/uuid"
)

type PaymentStatus string

const (
	PaymentStatusPending   PaymentStatus = "pending"
	PaymentStatusSucceeded PaymentStatus = "succeeded"
	PaymentStatusFailed    PaymentStatus = "failed"
	PaymentStatusRefunded  PaymentStatus = "refunded"
)

type Payment struct {
	ID                       uuid.UUID     `json:"id"`
	OrderID                  uuid.UUID     `json:"orderId"`
	StripePaymentIntentID    string        `json:"stripePaymentIntentId,omitempty"`
	StripeCheckoutSessionID  string        `json:"stripeCheckoutSessionId,omitempty"`
	AmountCents              int           `json:"amountCents"`
	Currency                 string        `json:"currency"`
	Status                   PaymentStatus `json:"status"`
	IdempotencyKey           string        `json:"idempotencyKey"`
	CreatedAt                time.Time     `json:"createdAt"`
	UpdatedAt                time.Time     `json:"updatedAt"`
}
```

- [ ] **Step 2: Write failing tests for the repository**

Create `go/payment-service/internal/repository/payment_test.go`:

```go
package repository_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kabradshaw1/portfolio/go/payment-service/internal/model"
	"github.com/kabradshaw1/portfolio/go/payment-service/internal/repository"
	"github.com/kabradshaw1/portfolio/go/pkg/resilience"
)

// testRepo creates a repository with a test breaker.
// Integration tests with real DB are in internal/integration/.
// These tests verify method signatures and error paths using nil pool (expect panic/error).

func TestPaymentRepository_Create_NilPool(t *testing.T) {
	breaker := resilience.NewBreaker(resilience.BreakerConfig{Name: "test"})
	repo := repository.NewPaymentRepository(nil, breaker)
	_, err := repo.Create(context.Background(), uuid.New(), 1000, "usd")
	assert.Error(t, err)
}

func TestPaymentRepository_FindByOrderID_NilPool(t *testing.T) {
	breaker := resilience.NewBreaker(resilience.BreakerConfig{Name: "test"})
	repo := repository.NewPaymentRepository(nil, breaker)
	_, err := repo.FindByOrderID(context.Background(), uuid.New())
	assert.Error(t, err)
}

func TestPaymentModel_StatusConstants(t *testing.T) {
	assert.Equal(t, model.PaymentStatus("pending"), model.PaymentStatusPending)
	assert.Equal(t, model.PaymentStatus("succeeded"), model.PaymentStatusSucceeded)
	assert.Equal(t, model.PaymentStatus("failed"), model.PaymentStatusFailed)
	assert.Equal(t, model.PaymentStatus("refunded"), model.PaymentStatusRefunded)
}

func TestIdempotencyKey(t *testing.T) {
	orderID := uuid.New()
	key := repository.IdempotencyKey(orderID)
	require.NotEmpty(t, key)
	assert.Equal(t, "payment:"+orderID.String(), key)
}
```

- [ ] **Step 3: Run tests to verify they fail**

```bash
cd go/payment-service && go test ./internal/repository/... -v -run TestPayment
```

Expected: Compilation failures — `repository` package doesn't exist yet.

- [ ] **Step 4: Implement the repository**

Create `go/payment-service/internal/repository/payment.go`:

```go
package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sony/gobreaker/v2"

	"github.com/kabradshaw1/portfolio/go/payment-service/internal/model"
	"github.com/kabradshaw1/portfolio/go/pkg/apperror"
	"github.com/kabradshaw1/portfolio/go/pkg/resilience"
)

// IdempotencyKey returns a deterministic idempotency key for a given order.
func IdempotencyKey(orderID uuid.UUID) string {
	return "payment:" + orderID.String()
}

type PaymentRepository struct {
	pool    *pgxpool.Pool
	breaker *gobreaker.CircuitBreaker[any]
}

func NewPaymentRepository(pool *pgxpool.Pool, breaker *gobreaker.CircuitBreaker[any]) *PaymentRepository {
	return &PaymentRepository{pool: pool, breaker: breaker}
}

func (r *PaymentRepository) Create(ctx context.Context, orderID uuid.UUID, amountCents int, currency string) (*model.Payment, error) {
	p := &model.Payment{
		ID:             uuid.New(),
		OrderID:        orderID,
		AmountCents:    amountCents,
		Currency:       currency,
		Status:         model.PaymentStatusPending,
		IdempotencyKey: IdempotencyKey(orderID),
	}

	_, err := resilience.Call(ctx, r.breaker, resilience.DefaultRetryConfig(), func() (any, error) {
		_, execErr := r.pool.Exec(ctx,
			`INSERT INTO payments (id, order_id, amount_cents, currency, status, idempotency_key)
			 VALUES ($1, $2, $3, $4, $5, $6)`,
			p.ID, p.OrderID, p.AmountCents, p.Currency, p.Status, p.IdempotencyKey,
		)
		return nil, execErr
	})
	if err != nil {
		return nil, fmt.Errorf("create payment: %w", err)
	}
	return p, nil
}

func (r *PaymentRepository) FindByOrderID(ctx context.Context, orderID uuid.UUID) (*model.Payment, error) {
	result, err := resilience.Call(ctx, r.breaker, resilience.DefaultRetryConfig(), func() (any, error) {
		row := r.pool.QueryRow(ctx,
			`SELECT id, order_id, stripe_payment_intent_id, stripe_checkout_session_id,
			        amount_cents, currency, status, idempotency_key, created_at, updated_at
			 FROM payments WHERE order_id = $1`, orderID,
		)
		var p model.Payment
		var intentID, sessionID *string
		scanErr := row.Scan(
			&p.ID, &p.OrderID, &intentID, &sessionID,
			&p.AmountCents, &p.Currency, &p.Status, &p.IdempotencyKey,
			&p.CreatedAt, &p.UpdatedAt,
		)
		if scanErr == pgx.ErrNoRows {
			return nil, apperror.NotFound("PAYMENT_NOT_FOUND", "payment not found for order")
		}
		if scanErr != nil {
			return nil, scanErr
		}
		if intentID != nil {
			p.StripePaymentIntentID = *intentID
		}
		if sessionID != nil {
			p.StripeCheckoutSessionID = *sessionID
		}
		return &p, nil
	})
	if err != nil {
		return nil, err
	}
	return result.(*model.Payment), nil
}

func (r *PaymentRepository) UpdateStatus(ctx context.Context, orderID uuid.UUID, status model.PaymentStatus) error {
	_, err := resilience.Call(ctx, r.breaker, resilience.DefaultRetryConfig(), func() (any, error) {
		_, execErr := r.pool.Exec(ctx,
			`UPDATE payments SET status = $1, updated_at = now() WHERE order_id = $2`,
			status, orderID,
		)
		return nil, execErr
	})
	return err
}

func (r *PaymentRepository) UpdateStripeIDs(ctx context.Context, orderID uuid.UUID, intentID, sessionID string) error {
	_, err := resilience.Call(ctx, r.breaker, resilience.DefaultRetryConfig(), func() (any, error) {
		_, execErr := r.pool.Exec(ctx,
			`UPDATE payments SET stripe_payment_intent_id = $1, stripe_checkout_session_id = $2, updated_at = now()
			 WHERE order_id = $3`,
			intentID, sessionID, orderID,
		)
		return nil, execErr
	})
	return err
}

func (r *PaymentRepository) FindByStripeIntentID(ctx context.Context, intentID string) (*model.Payment, error) {
	result, err := resilience.Call(ctx, r.breaker, resilience.DefaultRetryConfig(), func() (any, error) {
		row := r.pool.QueryRow(ctx,
			`SELECT id, order_id, stripe_payment_intent_id, stripe_checkout_session_id,
			        amount_cents, currency, status, idempotency_key, created_at, updated_at
			 FROM payments WHERE stripe_payment_intent_id = $1`, intentID,
		)
		var p model.Payment
		var sIntentID, sSessionID *string
		scanErr := row.Scan(
			&p.ID, &p.OrderID, &sIntentID, &sSessionID,
			&p.AmountCents, &p.Currency, &p.Status, &p.IdempotencyKey,
			&p.CreatedAt, &p.UpdatedAt,
		)
		if scanErr == pgx.ErrNoRows {
			return nil, apperror.NotFound("PAYMENT_NOT_FOUND", "payment not found")
		}
		if scanErr != nil {
			return nil, scanErr
		}
		if sIntentID != nil {
			p.StripePaymentIntentID = *sIntentID
		}
		if sSessionID != nil {
			p.StripeCheckoutSessionID = *sSessionID
		}
		return &p, nil
	})
	if err != nil {
		return nil, err
	}
	return result.(*model.Payment), nil
}
```

- [ ] **Step 5: Run tests to verify they pass**

```bash
cd go/payment-service && go mod tidy && go test ./internal/repository/... -v
```

Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add go/payment-service/internal/model/ go/payment-service/internal/repository/
git commit -m "feat(payment): add payment domain model and repository with circuit breaker"
```

---

### Task 5: Outbox Repository

**Files:**
- Create: `go/payment-service/internal/model/outbox.go`
- Create: `go/payment-service/internal/repository/outbox.go`
- Create: `go/payment-service/internal/repository/outbox_test.go`

- [ ] **Step 1: Create outbox model**

Create `go/payment-service/internal/model/outbox.go`:

```go
package model

import (
	"time"

	"github.com/google/uuid"
)

type OutboxMessage struct {
	ID         uuid.UUID `json:"id"`
	Exchange   string    `json:"exchange"`
	RoutingKey string    `json:"routingKey"`
	Payload    []byte    `json:"payload"`
	Published  bool      `json:"published"`
	CreatedAt  time.Time `json:"createdAt"`
}
```

- [ ] **Step 2: Write failing test**

Create `go/payment-service/internal/repository/outbox_test.go`:

```go
package repository_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/kabradshaw1/portfolio/go/payment-service/internal/repository"
	"github.com/kabradshaw1/portfolio/go/pkg/resilience"
)

func TestOutboxRepository_Insert_NilPool(t *testing.T) {
	breaker := resilience.NewBreaker(resilience.BreakerConfig{Name: "test"})
	repo := repository.NewOutboxRepository(nil, breaker)
	err := repo.Insert(context.Background(), nil, "exchange", "key", []byte(`{}`))
	assert.Error(t, err)
}
```

- [ ] **Step 3: Run test to verify it fails**

```bash
cd go/payment-service && go test ./internal/repository/... -v -run TestOutbox
```

Expected: FAIL — `OutboxRepository` not defined.

- [ ] **Step 4: Implement the outbox repository**

Create `go/payment-service/internal/repository/outbox.go`:

```go
package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sony/gobreaker/v2"

	"github.com/kabradshaw1/portfolio/go/payment-service/internal/model"
	"github.com/kabradshaw1/portfolio/go/pkg/resilience"
)

type OutboxRepository struct {
	pool    *pgxpool.Pool
	breaker *gobreaker.CircuitBreaker[any]
}

func NewOutboxRepository(pool *pgxpool.Pool, breaker *gobreaker.CircuitBreaker[any]) *OutboxRepository {
	return &OutboxRepository{pool: pool, breaker: breaker}
}

// Insert writes an outbox message within the given transaction, or the pool if tx is nil.
func (r *OutboxRepository) Insert(ctx context.Context, tx pgx.Tx, exchange, routingKey string, payload []byte) error {
	query := `INSERT INTO outbox (id, exchange, routing_key, payload) VALUES ($1, $2, $3, $4)`
	id := uuid.New()

	if tx != nil {
		_, err := tx.Exec(ctx, query, id, exchange, routingKey, payload)
		if err != nil {
			return fmt.Errorf("insert outbox (tx): %w", err)
		}
		return nil
	}

	_, err := resilience.Call(ctx, r.breaker, resilience.DefaultRetryConfig(), func() (any, error) {
		_, execErr := r.pool.Exec(ctx, query, id, exchange, routingKey, payload)
		return nil, execErr
	})
	if err != nil {
		return fmt.Errorf("insert outbox: %w", err)
	}
	return nil
}

// FetchUnpublished returns up to `limit` unpublished messages ordered by creation time.
func (r *OutboxRepository) FetchUnpublished(ctx context.Context, limit int) ([]model.OutboxMessage, error) {
	result, err := resilience.Call(ctx, r.breaker, resilience.DefaultRetryConfig(), func() (any, error) {
		rows, queryErr := r.pool.Query(ctx,
			`SELECT id, exchange, routing_key, payload, published, created_at
			 FROM outbox WHERE published = false ORDER BY created_at LIMIT $1`, limit,
		)
		if queryErr != nil {
			return nil, queryErr
		}
		defer rows.Close()

		var messages []model.OutboxMessage
		for rows.Next() {
			var m model.OutboxMessage
			if scanErr := rows.Scan(&m.ID, &m.Exchange, &m.RoutingKey, &m.Payload, &m.Published, &m.CreatedAt); scanErr != nil {
				return nil, scanErr
			}
			messages = append(messages, m)
		}
		return messages, rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return result.([]model.OutboxMessage), nil
}

// MarkPublished marks an outbox message as published.
func (r *OutboxRepository) MarkPublished(ctx context.Context, id uuid.UUID) error {
	_, err := resilience.Call(ctx, r.breaker, resilience.DefaultRetryConfig(), func() (any, error) {
		_, execErr := r.pool.Exec(ctx, `UPDATE outbox SET published = true WHERE id = $1`, id)
		return nil, execErr
	})
	return err
}
```

- [ ] **Step 5: Run tests to verify they pass**

```bash
cd go/payment-service && go mod tidy && go test ./internal/repository/... -v
```

Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add go/payment-service/internal/model/outbox.go go/payment-service/internal/repository/outbox.go go/payment-service/internal/repository/outbox_test.go
git commit -m "feat(payment): add outbox model and repository for transactional messaging"
```

---

### Task 6: Processed Events Repository

**Files:**
- Create: `go/payment-service/internal/repository/processed_event.go`
- Create: `go/payment-service/internal/repository/processed_event_test.go`

- [ ] **Step 1: Write failing test**

Create `go/payment-service/internal/repository/processed_event_test.go`:

```go
package repository_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/kabradshaw1/portfolio/go/payment-service/internal/repository"
	"github.com/kabradshaw1/portfolio/go/pkg/resilience"
)

func TestProcessedEventRepository_TryInsert_NilPool(t *testing.T) {
	breaker := resilience.NewBreaker(resilience.BreakerConfig{Name: "test"})
	repo := repository.NewProcessedEventRepository(nil, breaker)
	_, err := repo.TryInsert(context.Background(), nil, "evt_123", "payment_intent.succeeded")
	assert.Error(t, err)
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd go/payment-service && go test ./internal/repository/... -v -run TestProcessedEvent
```

Expected: FAIL

- [ ] **Step 3: Implement**

Create `go/payment-service/internal/repository/processed_event.go`:

```go
package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sony/gobreaker/v2"

	"github.com/kabradshaw1/portfolio/go/pkg/resilience"
)

type ProcessedEventRepository struct {
	pool    *pgxpool.Pool
	breaker *gobreaker.CircuitBreaker[any]
}

func NewProcessedEventRepository(pool *pgxpool.Pool, breaker *gobreaker.CircuitBreaker[any]) *ProcessedEventRepository {
	return &ProcessedEventRepository{pool: pool, breaker: breaker}
}

// TryInsert attempts to record a processed Stripe event. Returns (true, nil) if inserted,
// (false, nil) if the event was already processed (duplicate), or (false, err) on failure.
// If tx is non-nil, uses the transaction; otherwise uses the pool with circuit breaker.
func (r *ProcessedEventRepository) TryInsert(ctx context.Context, tx pgx.Tx, eventID, eventType string) (bool, error) {
	query := `INSERT INTO processed_events (stripe_event_id, event_type) VALUES ($1, $2) ON CONFLICT (stripe_event_id) DO NOTHING`

	if tx != nil {
		tag, err := tx.Exec(ctx, query, eventID, eventType)
		if err != nil {
			return false, fmt.Errorf("insert processed event (tx): %w", err)
		}
		return tag.RowsAffected() > 0, nil
	}

	result, err := resilience.Call(ctx, r.breaker, resilience.DefaultRetryConfig(), func() (any, error) {
		tag, execErr := r.pool.Exec(ctx, query, eventID, eventType)
		if execErr != nil {
			return nil, execErr
		}
		return tag, nil
	})
	if err != nil {
		return false, fmt.Errorf("insert processed event: %w", err)
	}
	return result.(pgconn.CommandTag).RowsAffected() > 0, nil
}
```

- [ ] **Step 4: Run tests**

```bash
cd go/payment-service && go mod tidy && go test ./internal/repository/... -v
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add go/payment-service/internal/repository/processed_event.go go/payment-service/internal/repository/processed_event_test.go
git commit -m "feat(payment): add processed events repository for webhook deduplication"
```

---

### Task 7: Stripe Service (Business Logic)

**Files:**
- Create: `go/payment-service/internal/service/stripe.go`
- Create: `go/payment-service/internal/service/stripe_test.go`

- [ ] **Step 1: Write failing tests**

Create `go/payment-service/internal/service/stripe_test.go`:

```go
package service_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kabradshaw1/portfolio/go/payment-service/internal/model"
	"github.com/kabradshaw1/portfolio/go/payment-service/internal/service"
)

type mockPaymentRepo struct {
	payment *model.Payment
	err     error
}

func (m *mockPaymentRepo) Create(_ context.Context, orderID uuid.UUID, amountCents int, currency string) (*model.Payment, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &model.Payment{
		ID:             uuid.New(),
		OrderID:        orderID,
		AmountCents:    amountCents,
		Currency:       currency,
		Status:         model.PaymentStatusPending,
		IdempotencyKey: "payment:" + orderID.String(),
	}, nil
}

func (m *mockPaymentRepo) FindByOrderID(_ context.Context, _ uuid.UUID) (*model.Payment, error) {
	return m.payment, m.err
}

func (m *mockPaymentRepo) FindByStripeIntentID(_ context.Context, _ string) (*model.Payment, error) {
	return m.payment, m.err
}

func (m *mockPaymentRepo) UpdateStatus(_ context.Context, _ uuid.UUID, _ model.PaymentStatus) error {
	return m.err
}

func (m *mockPaymentRepo) UpdateStripeIDs(_ context.Context, _ uuid.UUID, _, _ string) error {
	return m.err
}

type mockStripeClient struct {
	sessionURL string
	intentID   string
	sessionID  string
	refundID   string
	err        error
}

func (m *mockStripeClient) CreateCheckoutSession(_ context.Context, _ service.CheckoutParams) (*service.CheckoutResult, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &service.CheckoutResult{
		SessionURL:      m.sessionURL,
		PaymentIntentID: m.intentID,
		SessionID:       m.sessionID,
	}, nil
}

func (m *mockStripeClient) Refund(_ context.Context, intentID, _ string) (string, error) {
	return m.refundID, m.err
}

func TestPaymentService_CreatePayment(t *testing.T) {
	svc := service.NewPaymentService(
		&mockPaymentRepo{},
		&mockStripeClient{
			sessionURL: "https://checkout.stripe.com/test",
			intentID:   "pi_test123",
			sessionID:  "cs_test123",
		},
	)

	result, err := svc.CreatePayment(context.Background(), uuid.New(), 5000, "usd", "https://example.com/success", "https://example.com/cancel")
	require.NoError(t, err)
	assert.Equal(t, "https://checkout.stripe.com/test", result.CheckoutURL)
	assert.Equal(t, model.PaymentStatusPending, result.Payment.Status)
}

func TestPaymentService_CreatePayment_StripeError(t *testing.T) {
	svc := service.NewPaymentService(
		&mockPaymentRepo{},
		&mockStripeClient{err: assert.AnError},
	)

	_, err := svc.CreatePayment(context.Background(), uuid.New(), 5000, "usd", "", "")
	assert.Error(t, err)
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd go/payment-service && go test ./internal/service/... -v
```

Expected: FAIL — `service` package doesn't exist.

- [ ] **Step 3: Implement the Stripe service**

Create `go/payment-service/internal/service/stripe.go`:

```go
package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/kabradshaw1/portfolio/go/payment-service/internal/model"
)

// PaymentRepo abstracts payment persistence.
type PaymentRepo interface {
	Create(ctx context.Context, orderID uuid.UUID, amountCents int, currency string) (*model.Payment, error)
	FindByOrderID(ctx context.Context, orderID uuid.UUID) (*model.Payment, error)
	FindByStripeIntentID(ctx context.Context, intentID string) (*model.Payment, error)
	UpdateStatus(ctx context.Context, orderID uuid.UUID, status model.PaymentStatus) error
	UpdateStripeIDs(ctx context.Context, orderID uuid.UUID, intentID, sessionID string) error
}

// CheckoutParams holds the inputs for creating a Stripe Checkout Session.
type CheckoutParams struct {
	AmountCents    int
	Currency       string
	OrderID        string
	IdempotencyKey string
	SuccessURL     string
	CancelURL      string
}

// CheckoutResult is the output from Stripe Checkout Session creation.
type CheckoutResult struct {
	SessionURL      string
	PaymentIntentID string
	SessionID       string
}

// StripeClient abstracts Stripe API calls for testability.
type StripeClient interface {
	CreateCheckoutSession(ctx context.Context, params CheckoutParams) (*CheckoutResult, error)
	Refund(ctx context.Context, paymentIntentID, reason string) (string, error)
}

// CreatePaymentResult is the return value from CreatePayment.
type CreatePaymentResult struct {
	Payment     *model.Payment
	CheckoutURL string
}

// PaymentService orchestrates payment creation and refunds.
type PaymentService struct {
	repo   PaymentRepo
	stripe StripeClient
}

func NewPaymentService(repo PaymentRepo, stripe StripeClient) *PaymentService {
	return &PaymentService{repo: repo, stripe: stripe}
}

func (s *PaymentService) CreatePayment(ctx context.Context, orderID uuid.UUID, amountCents int, currency, successURL, cancelURL string) (*CreatePaymentResult, error) {
	payment, err := s.repo.Create(ctx, orderID, amountCents, currency)
	if err != nil {
		return nil, fmt.Errorf("create payment record: %w", err)
	}

	result, err := s.stripe.CreateCheckoutSession(ctx, CheckoutParams{
		AmountCents:    amountCents,
		Currency:       currency,
		OrderID:        orderID.String(),
		IdempotencyKey: payment.IdempotencyKey,
		SuccessURL:     successURL,
		CancelURL:      cancelURL,
	})
	if err != nil {
		// Mark payment as failed so it can be retried with the same idempotency key
		_ = s.repo.UpdateStatus(ctx, orderID, model.PaymentStatusFailed)
		return nil, fmt.Errorf("create checkout session: %w", err)
	}

	if err := s.repo.UpdateStripeIDs(ctx, orderID, result.PaymentIntentID, result.SessionID); err != nil {
		return nil, fmt.Errorf("update stripe IDs: %w", err)
	}

	return &CreatePaymentResult{
		Payment:     payment,
		CheckoutURL: result.SessionURL,
	}, nil
}

func (s *PaymentService) GetPaymentStatus(ctx context.Context, orderID uuid.UUID) (*model.Payment, error) {
	return s.repo.FindByOrderID(ctx, orderID)
}

func (s *PaymentService) RefundPayment(ctx context.Context, orderID uuid.UUID, reason string) (*model.Payment, string, error) {
	payment, err := s.repo.FindByOrderID(ctx, orderID)
	if err != nil {
		return nil, "", err
	}

	refundID, err := s.stripe.Refund(ctx, payment.StripePaymentIntentID, reason)
	if err != nil {
		return nil, "", fmt.Errorf("stripe refund: %w", err)
	}

	if err := s.repo.UpdateStatus(ctx, orderID, model.PaymentStatusRefunded); err != nil {
		return nil, "", fmt.Errorf("update refund status: %w", err)
	}

	payment.Status = model.PaymentStatusRefunded
	return payment, refundID, nil
}
```

- [ ] **Step 4: Run tests**

```bash
cd go/payment-service && go mod tidy && go test ./internal/service/... -v
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add go/payment-service/internal/service/
git commit -m "feat(payment): add Stripe payment service with mock-friendly interface"
```

---

### Task 8: Stripe Client Implementation

**Files:**
- Create: `go/payment-service/internal/stripe/client.go`
- Create: `go/payment-service/internal/stripe/client_test.go`

- [ ] **Step 1: Write failing test**

Create `go/payment-service/internal/stripe/client_test.go`:

```go
package stripe_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	stripeClient "github.com/kabradshaw1/portfolio/go/payment-service/internal/stripe"
)

func TestNewClient(t *testing.T) {
	c := stripeClient.NewClient("sk_test_fake")
	assert.NotNil(t, c)
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd go/payment-service && go test ./internal/stripe/... -v
```

Expected: FAIL

- [ ] **Step 3: Implement the Stripe client**

Create `go/payment-service/internal/stripe/client.go`:

```go
package stripe

import (
	"context"
	"fmt"

	"github.com/stripe/stripe-go/v82"
	"github.com/stripe/stripe-go/v82/checkout/session"
	"github.com/stripe/stripe-go/v82/refund"

	"github.com/kabradshaw1/portfolio/go/payment-service/internal/service"
)

// Client wraps the Stripe SDK and implements service.StripeClient.
type Client struct {
	apiKey string
}

func NewClient(apiKey string) *Client {
	stripe.Key = apiKey
	return &Client{apiKey: apiKey}
}

func (c *Client) CreateCheckoutSession(_ context.Context, params service.CheckoutParams) (*service.CheckoutResult, error) {
	sessParams := &stripe.CheckoutSessionParams{
		Mode: stripe.String(string(stripe.CheckoutSessionModePayment)),
		LineItems: []*stripe.CheckoutSessionLineItemParams{
			{
				PriceData: &stripe.CheckoutSessionLineItemPriceDataParams{
					Currency:   stripe.String(params.Currency),
					UnitAmount: stripe.Int64(int64(params.AmountCents)),
					ProductData: &stripe.CheckoutSessionLineItemPriceDataProductDataParams{
						Name: stripe.String("Order " + params.OrderID),
					},
				},
				Quantity: stripe.Int64(1),
			},
		},
		PaymentIntentData: &stripe.CheckoutSessionPaymentIntentDataParams{
			Metadata: map[string]string{
				"order_id": params.OrderID,
			},
		},
		Metadata: map[string]string{
			"order_id": params.OrderID,
		},
		SuccessURL: stripe.String(params.SuccessURL),
		CancelURL:  stripe.String(params.CancelURL),
	}
	sessParams.IdempotencyKey = stripe.String(params.IdempotencyKey)

	sess, err := session.New(sessParams)
	if err != nil {
		return nil, fmt.Errorf("stripe checkout session: %w", err)
	}

	return &service.CheckoutResult{
		SessionURL:      sess.URL,
		PaymentIntentID: sess.PaymentIntent.ID,
		SessionID:       sess.ID,
	}, nil
}

func (c *Client) Refund(_ context.Context, paymentIntentID, reason string) (string, error) {
	params := &stripe.RefundParams{
		PaymentIntent: stripe.String(paymentIntentID),
	}
	if reason != "" {
		params.Reason = stripe.String(reason)
	}

	r, err := refund.New(params)
	if err != nil {
		return "", fmt.Errorf("stripe refund: %w", err)
	}
	return r.ID, nil
}
```

- [ ] **Step 4: Run tests**

```bash
cd go/payment-service && go get github.com/stripe/stripe-go/v82 && go mod tidy && go test ./internal/stripe/... -v
```

Expected: PASS (NewClient test doesn't call Stripe API)

- [ ] **Step 5: Commit**

```bash
git add go/payment-service/internal/stripe/
git commit -m "feat(payment): add Stripe SDK client wrapping checkout sessions and refunds"
```

---

### Task 9: Webhook Handler

**Files:**
- Create: `go/payment-service/internal/handler/webhook.go`
- Create: `go/payment-service/internal/handler/webhook_test.go`

- [ ] **Step 1: Write failing tests**

Create `go/payment-service/internal/handler/webhook_test.go`:

```go
package handler_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"github.com/kabradshaw1/portfolio/go/payment-service/internal/handler"
	"github.com/kabradshaw1/portfolio/go/payment-service/internal/model"
	"github.com/kabradshaw1/portfolio/go/pkg/apperror"
)

type mockWebhookService struct {
	err error
}

func (m *mockWebhookService) HandlePaymentSucceeded(ctx context.Context, eventID, intentID string) error {
	return m.err
}

func (m *mockWebhookService) HandlePaymentFailed(ctx context.Context, eventID, intentID string) error {
	return m.err
}

func (m *mockWebhookService) HandleRefund(ctx context.Context, eventID, intentID string) error {
	return m.err
}

type mockEventVerifier struct {
	eventType string
	intentID  string
	eventID   string
	err       error
}

func (m *mockEventVerifier) VerifyAndParse(payload []byte, sigHeader string) (eventType, eventID, intentID string, err error) {
	return m.eventType, m.eventID, m.intentID, m.err
}

func setupWebhookRouter(h *handler.WebhookHandler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(apperror.ErrorHandler())
	r.POST("/webhooks/stripe", h.HandleWebhook)
	return r
}

func TestWebhookHandler_InvalidSignature(t *testing.T) {
	h := handler.NewWebhookHandler(
		&mockWebhookService{},
		&mockEventVerifier{err: assert.AnError},
	)
	router := setupWebhookRouter(h)

	req := httptest.NewRequest(http.MethodPost, "/webhooks/stripe", strings.NewReader(`{}`))
	req.Header.Set("Stripe-Signature", "bad")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestWebhookHandler_PaymentSucceeded(t *testing.T) {
	h := handler.NewWebhookHandler(
		&mockWebhookService{},
		&mockEventVerifier{
			eventType: "payment_intent.succeeded",
			eventID:   "evt_" + uuid.New().String(),
			intentID:  "pi_test123",
		},
	)
	router := setupWebhookRouter(h)

	req := httptest.NewRequest(http.MethodPost, "/webhooks/stripe", strings.NewReader(`{}`))
	req.Header.Set("Stripe-Signature", "valid")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd go/payment-service && go test ./internal/handler/... -v
```

Expected: FAIL

- [ ] **Step 3: Implement webhook handler**

Create `go/payment-service/internal/handler/webhook.go`:

```go
package handler

import (
	"context"
	"io"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/kabradshaw1/portfolio/go/pkg/apperror"
)

// WebhookService processes verified Stripe events.
type WebhookService interface {
	HandlePaymentSucceeded(ctx context.Context, eventID, intentID string) error
	HandlePaymentFailed(ctx context.Context, eventID, intentID string) error
	HandleRefund(ctx context.Context, eventID, intentID string) error
}

// EventVerifier verifies Stripe webhook signatures and extracts event data.
type EventVerifier interface {
	VerifyAndParse(payload []byte, sigHeader string) (eventType, eventID, intentID string, err error)
}

type WebhookHandler struct {
	svc      WebhookService
	verifier EventVerifier
}

func NewWebhookHandler(svc WebhookService, verifier EventVerifier) *WebhookHandler {
	return &WebhookHandler{svc: svc, verifier: verifier}
}

func (h *WebhookHandler) HandleWebhook(c *gin.Context) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.Error(apperror.BadRequest("INVALID_BODY", "cannot read request body"))
		return
	}

	sigHeader := c.GetHeader("Stripe-Signature")
	eventType, eventID, intentID, err := h.verifier.VerifyAndParse(body, sigHeader)
	if err != nil {
		slog.WarnContext(c.Request.Context(), "webhook signature verification failed", "error", err)
		c.Error(apperror.BadRequest("INVALID_SIGNATURE", "webhook signature verification failed"))
		return
	}

	ctx := c.Request.Context()
	slog.InfoContext(ctx, "processing webhook", "eventType", eventType, "eventID", eventID)

	switch eventType {
	case "payment_intent.succeeded":
		if err := h.svc.HandlePaymentSucceeded(ctx, eventID, intentID); err != nil {
			slog.ErrorContext(ctx, "handle payment succeeded", "error", err)
			c.Error(apperror.Internal("WEBHOOK_PROCESSING_ERROR", "failed to process webhook"))
			return
		}
	case "payment_intent.payment_failed":
		if err := h.svc.HandlePaymentFailed(ctx, eventID, intentID); err != nil {
			slog.ErrorContext(ctx, "handle payment failed", "error", err)
			c.Error(apperror.Internal("WEBHOOK_PROCESSING_ERROR", "failed to process webhook"))
			return
		}
	case "charge.refunded":
		if err := h.svc.HandleRefund(ctx, eventID, intentID); err != nil {
			slog.ErrorContext(ctx, "handle refund", "error", err)
			c.Error(apperror.Internal("WEBHOOK_PROCESSING_ERROR", "failed to process webhook"))
			return
		}
	default:
		slog.InfoContext(ctx, "ignoring unhandled webhook event", "eventType", eventType)
	}

	c.JSON(http.StatusOK, gin.H{"received": true})
}
```

- [ ] **Step 4: Run tests**

```bash
cd go/payment-service && go mod tidy && go test ./internal/handler/... -v
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add go/payment-service/internal/handler/
git commit -m "feat(payment): add webhook handler with signature verification and event routing"
```

---

### Task 10: Webhook Service (Event Processing + Outbox)

**Files:**
- Create: `go/payment-service/internal/service/webhook.go`
- Create: `go/payment-service/internal/service/webhook_test.go`

- [ ] **Step 1: Write failing tests**

Create `go/payment-service/internal/service/webhook_test.go`:

```go
package service_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kabradshaw1/portfolio/go/payment-service/internal/model"
	"github.com/kabradshaw1/portfolio/go/payment-service/internal/service"
)

type mockProcessedEventRepo struct {
	inserted bool
	err      error
}

func (m *mockProcessedEventRepo) TryInsert(_ context.Context, _ pgx.Tx, _, _ string) (bool, error) {
	return m.inserted, m.err
}

type mockOutboxRepo struct {
	err error
}

func (m *mockOutboxRepo) Insert(_ context.Context, _ pgx.Tx, _, _ string, _ []byte) error {
	return m.err
}

type mockTxBeginner struct{}

func (m *mockTxBeginner) Begin(_ context.Context) (pgx.Tx, error) {
	return nil, assert.AnError // Simplification for unit tests
}

func TestWebhookService_HandlePaymentSucceeded_DuplicateEvent(t *testing.T) {
	svc := service.NewWebhookService(
		&mockPaymentRepo{payment: &model.Payment{OrderID: uuid.New(), Status: model.PaymentStatusPending}},
		&mockProcessedEventRepo{inserted: false}, // duplicate
		&mockOutboxRepo{},
		nil, // txBeginner not used for duplicates
	)

	err := svc.HandlePaymentSucceeded(context.Background(), "evt_dup", "pi_test")
	require.NoError(t, err) // Duplicates are silently skipped
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd go/payment-service && go test ./internal/service/... -v -run TestWebhookService
```

Expected: FAIL

- [ ] **Step 3: Implement webhook service**

Create `go/payment-service/internal/service/webhook.go`:

```go
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"

	"github.com/kabradshaw1/portfolio/go/payment-service/internal/model"
)

// ProcessedEventRepo checks/stores webhook event deduplication records.
type ProcessedEventRepo interface {
	TryInsert(ctx context.Context, tx pgx.Tx, eventID, eventType string) (bool, error)
}

// OutboxRepo writes outbound messages transactionally.
type OutboxRepo interface {
	Insert(ctx context.Context, tx pgx.Tx, exchange, routingKey string, payload []byte) error
}

// TxBeginner starts a database transaction.
type TxBeginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

// WebhookService processes verified Stripe webhook events with deduplication
// and transactional outbox for reliable saga event delivery.
type WebhookService struct {
	paymentRepo PaymentRepo
	eventRepo   ProcessedEventRepo
	outboxRepo  OutboxRepo
	txBeginner  TxBeginner
}

func NewWebhookService(paymentRepo PaymentRepo, eventRepo ProcessedEventRepo, outboxRepo OutboxRepo, txBeginner TxBeginner) *WebhookService {
	return &WebhookService{
		paymentRepo: paymentRepo,
		eventRepo:   eventRepo,
		outboxRepo:  outboxRepo,
		txBeginner:  txBeginner,
	}
}

func (s *WebhookService) HandlePaymentSucceeded(ctx context.Context, eventID, intentID string) error {
	return s.processEvent(ctx, eventID, "payment_intent.succeeded", intentID, model.PaymentStatusSucceeded, "saga.order.events", map[string]any{
		"event": "payment.confirmed",
	})
}

func (s *WebhookService) HandlePaymentFailed(ctx context.Context, eventID, intentID string) error {
	return s.processEvent(ctx, eventID, "payment_intent.payment_failed", intentID, model.PaymentStatusFailed, "saga.order.events", map[string]any{
		"event": "payment.failed",
	})
}

func (s *WebhookService) HandleRefund(ctx context.Context, eventID, intentID string) error {
	// Refund events update status but don't publish to saga — they go to Kafka via the outbox poller.
	payment, err := s.paymentRepo.FindByStripeIntentID(ctx, intentID)
	if err != nil {
		return fmt.Errorf("find payment for refund: %w", err)
	}

	inserted, err := s.eventRepo.TryInsert(ctx, nil, eventID, "charge.refunded")
	if err != nil {
		return fmt.Errorf("dedup check: %w", err)
	}
	if !inserted {
		slog.InfoContext(ctx, "duplicate refund webhook, skipping", "eventID", eventID)
		return nil
	}

	return s.paymentRepo.UpdateStatus(ctx, payment.OrderID, model.PaymentStatusRefunded)
}

func (s *WebhookService) processEvent(ctx context.Context, eventID, eventType, intentID string, newStatus model.PaymentStatus, routingKey string, sagaPayload map[string]any) error {
	// Check dedup outside transaction for the fast path
	inserted, err := s.eventRepo.TryInsert(ctx, nil, eventID, eventType)
	if err != nil {
		return fmt.Errorf("dedup check: %w", err)
	}
	if !inserted {
		slog.InfoContext(ctx, "duplicate webhook, skipping", "eventID", eventID)
		return nil
	}

	payment, err := s.paymentRepo.FindByStripeIntentID(ctx, intentID)
	if err != nil {
		return fmt.Errorf("find payment: %w", err)
	}

	// Add order context to saga payload
	sagaPayload["order_id"] = payment.OrderID.String()

	payloadBytes, err := json.Marshal(sagaPayload)
	if err != nil {
		return fmt.Errorf("marshal saga event: %w", err)
	}

	// Update payment status (non-transactional — outbox poller handles delivery)
	if err := s.paymentRepo.UpdateStatus(ctx, payment.OrderID, newStatus); err != nil {
		return fmt.Errorf("update payment status: %w", err)
	}

	// Write to outbox for reliable delivery to RabbitMQ
	if err := s.outboxRepo.Insert(ctx, nil, "ecommerce.saga", routingKey, payloadBytes); err != nil {
		return fmt.Errorf("write outbox: %w", err)
	}

	slog.InfoContext(ctx, "webhook processed", "eventType", eventType, "orderID", payment.OrderID)
	return nil
}
```

- [ ] **Step 4: Run tests**

```bash
cd go/payment-service && go mod tidy && go test ./internal/service/... -v
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add go/payment-service/internal/service/webhook.go go/payment-service/internal/service/webhook_test.go
git commit -m "feat(payment): add webhook service with dedup and transactional outbox"
```

---

### Task 11: Outbox Poller

**Files:**
- Create: `go/payment-service/internal/outbox/poller.go`
- Create: `go/payment-service/internal/outbox/poller_test.go`

- [ ] **Step 1: Write failing test**

Create `go/payment-service/internal/outbox/poller_test.go`:

```go
package outbox_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/kabradshaw1/portfolio/go/payment-service/internal/outbox"
)

func TestNewPoller(t *testing.T) {
	p := outbox.NewPoller(nil, nil, 5*time.Second, 100)
	assert.NotNil(t, p)
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd go/payment-service && go test ./internal/outbox/... -v
```

Expected: FAIL

- [ ] **Step 3: Implement the outbox poller**

Create `go/payment-service/internal/outbox/poller.go`:

```go
package outbox

import (
	"context"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/kabradshaw1/portfolio/go/payment-service/internal/model"
)

// OutboxFetcher reads and marks outbox messages.
type OutboxFetcher interface {
	FetchUnpublished(ctx context.Context, limit int) ([]model.OutboxMessage, error)
	MarkPublished(ctx context.Context, id uuid.UUID) error
}

// Poller periodically reads the outbox table and publishes messages to RabbitMQ.
type Poller struct {
	fetcher  OutboxFetcher
	ch       *amqp.Channel
	interval time.Duration
	batch    int
	idle     atomic.Bool
}

func NewPoller(fetcher OutboxFetcher, ch *amqp.Channel, interval time.Duration, batch int) *Poller {
	p := &Poller{
		fetcher:  fetcher,
		ch:       ch,
		interval: interval,
		batch:    batch,
	}
	p.idle.Store(true)
	return p
}

// IsIdle returns true if the poller is not currently processing messages.
func (p *Poller) IsIdle() bool {
	return p.idle.Load()
}

// Run starts the polling loop. It blocks until the context is cancelled.
func (p *Poller) Run(ctx context.Context) {
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.poll(ctx)
		}
	}
}

func (p *Poller) poll(ctx context.Context) {
	p.idle.Store(false)
	defer p.idle.Store(true)

	messages, err := p.fetcher.FetchUnpublished(ctx, p.batch)
	if err != nil {
		slog.ErrorContext(ctx, "outbox fetch failed", "error", err)
		return
	}

	for _, msg := range messages {
		if err := p.publish(ctx, msg); err != nil {
			slog.ErrorContext(ctx, "outbox publish failed", "messageID", msg.ID, "error", err)
			continue // Try next message; this one will be retried on next poll
		}

		if err := p.fetcher.MarkPublished(ctx, msg.ID); err != nil {
			slog.ErrorContext(ctx, "outbox mark published failed", "messageID", msg.ID, "error", err)
		}
	}
}

func (p *Poller) publish(ctx context.Context, msg model.OutboxMessage) error {
	return p.ch.PublishWithContext(ctx,
		msg.Exchange,
		msg.RoutingKey,
		false, // mandatory
		false, // immediate
		amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent,
			Body:         msg.Payload,
			MessageId:    msg.ID.String(),
		},
	)
}
```

- [ ] **Step 4: Run tests**

```bash
cd go/payment-service && go mod tidy && go test ./internal/outbox/... -v
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add go/payment-service/internal/outbox/
git commit -m "feat(payment): add outbox poller for reliable RabbitMQ event delivery"
```

---

### Task 12: Event Verifier (Stripe Signature)

**Files:**
- Create: `go/payment-service/internal/stripe/verifier.go`
- Create: `go/payment-service/internal/stripe/verifier_test.go`

- [ ] **Step 1: Write failing test**

Create `go/payment-service/internal/stripe/verifier_test.go`:

```go
package stripe_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	stripeClient "github.com/kabradshaw1/portfolio/go/payment-service/internal/stripe"
)

func TestVerifier_InvalidSignature(t *testing.T) {
	v := stripeClient.NewVerifier("whsec_test_secret")
	_, _, _, err := v.VerifyAndParse([]byte(`{}`), "bad_sig")
	assert.Error(t, err)
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd go/payment-service && go test ./internal/stripe/... -v -run TestVerifier
```

Expected: FAIL

- [ ] **Step 3: Implement verifier**

Create `go/payment-service/internal/stripe/verifier.go`:

```go
package stripe

import (
	"encoding/json"
	"fmt"

	"github.com/stripe/stripe-go/v82"
	"github.com/stripe/stripe-go/v82/webhook"
)

// Verifier verifies Stripe webhook signatures and extracts event data.
type Verifier struct {
	webhookSecret string
}

func NewVerifier(webhookSecret string) *Verifier {
	return &Verifier{webhookSecret: webhookSecret}
}

// VerifyAndParse verifies the webhook signature and extracts event type, event ID,
// and payment intent ID from the event payload.
func (v *Verifier) VerifyAndParse(payload []byte, sigHeader string) (eventType, eventID, intentID string, err error) {
	event, err := webhook.ConstructEvent(payload, sigHeader, v.webhookSecret)
	if err != nil {
		return "", "", "", fmt.Errorf("verify webhook signature: %w", err)
	}

	intentID, err = extractPaymentIntentID(event)
	if err != nil {
		return "", "", "", err
	}

	return string(event.Type), event.ID, intentID, nil
}

func extractPaymentIntentID(event stripe.Event) (string, error) {
	switch event.Type {
	case "payment_intent.succeeded", "payment_intent.payment_failed":
		var pi stripe.PaymentIntent
		if err := json.Unmarshal(event.Data.Raw, &pi); err != nil {
			return "", fmt.Errorf("unmarshal payment intent: %w", err)
		}
		return pi.ID, nil
	case "charge.refunded":
		var ch stripe.Charge
		if err := json.Unmarshal(event.Data.Raw, &ch); err != nil {
			return "", fmt.Errorf("unmarshal charge: %w", err)
		}
		return ch.PaymentIntent.ID, nil
	default:
		return "", nil
	}
}
```

- [ ] **Step 4: Run tests**

```bash
cd go/payment-service && go mod tidy && go test ./internal/stripe/... -v
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add go/payment-service/internal/stripe/verifier.go go/payment-service/internal/stripe/verifier_test.go
git commit -m "feat(payment): add Stripe webhook signature verifier"
```

---

### Task 13: gRPC Server

**Files:**
- Create: `go/payment-service/internal/grpcserver/server.go`
- Create: `go/payment-service/internal/grpcserver/server_test.go`

- [ ] **Step 1: Write failing test**

Create `go/payment-service/internal/grpcserver/server_test.go`:

```go
package grpcserver_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kabradshaw1/portfolio/go/payment-service/internal/grpcserver"
	"github.com/kabradshaw1/portfolio/go/payment-service/internal/model"
	"github.com/kabradshaw1/portfolio/go/payment-service/internal/service"
	pb "github.com/kabradshaw1/portfolio/go/payment-service/pb/payment/v1"
)

type mockPaymentService struct {
	createResult *service.CreatePaymentResult
	payment      *model.Payment
	refundID     string
	err          error
}

func (m *mockPaymentService) CreatePayment(_ context.Context, _ uuid.UUID, _ int, _, _, _ string) (*service.CreatePaymentResult, error) {
	return m.createResult, m.err
}

func (m *mockPaymentService) GetPaymentStatus(_ context.Context, _ uuid.UUID) (*model.Payment, error) {
	return m.payment, m.err
}

func (m *mockPaymentService) RefundPayment(_ context.Context, _ uuid.UUID, _ string) (*model.Payment, string, error) {
	return m.payment, m.refundID, m.err
}

func TestGRPCServer_CreatePayment(t *testing.T) {
	paymentID := uuid.New()
	srv := grpcserver.NewServer(&mockPaymentService{
		createResult: &service.CreatePaymentResult{
			Payment:     &model.Payment{ID: paymentID, Status: model.PaymentStatusPending},
			CheckoutURL: "https://checkout.stripe.com/test",
		},
	})

	resp, err := srv.CreatePayment(context.Background(), &pb.CreatePaymentRequest{
		OrderId:     uuid.New().String(),
		AmountCents: 5000,
		Currency:    "usd",
		SuccessUrl:  "https://example.com/success",
		CancelUrl:   "https://example.com/cancel",
	})
	require.NoError(t, err)
	assert.Equal(t, paymentID.String(), resp.PaymentId)
	assert.Equal(t, "https://checkout.stripe.com/test", resp.CheckoutSessionUrl)
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd go/payment-service && go test ./internal/grpcserver/... -v
```

Expected: FAIL

- [ ] **Step 3: Implement gRPC server**

Create `go/payment-service/internal/grpcserver/server.go`:

```go
package grpcserver

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/kabradshaw1/portfolio/go/payment-service/internal/model"
	"github.com/kabradshaw1/portfolio/go/payment-service/internal/service"
	pb "github.com/kabradshaw1/portfolio/go/payment-service/pb/payment/v1"
)

// PaymentServiceAPI is the interface the gRPC server depends on.
type PaymentServiceAPI interface {
	CreatePayment(ctx context.Context, orderID uuid.UUID, amountCents int, currency, successURL, cancelURL string) (*service.CreatePaymentResult, error)
	GetPaymentStatus(ctx context.Context, orderID uuid.UUID) (*model.Payment, error)
	RefundPayment(ctx context.Context, orderID uuid.UUID, reason string) (*model.Payment, string, error)
}

type Server struct {
	pb.UnimplementedPaymentServiceServer
	svc PaymentServiceAPI
}

func NewServer(svc PaymentServiceAPI) *Server {
	return &Server{svc: svc}
}

func (s *Server) CreatePayment(ctx context.Context, req *pb.CreatePaymentRequest) (*pb.CreatePaymentResponse, error) {
	orderID, err := uuid.Parse(req.OrderId)
	if err != nil {
		return nil, fmt.Errorf("invalid order_id: %w", err)
	}

	result, err := s.svc.CreatePayment(ctx, orderID, int(req.AmountCents), req.Currency, req.SuccessUrl, req.CancelUrl)
	if err != nil {
		return nil, err
	}

	return &pb.CreatePaymentResponse{
		PaymentId:          result.Payment.ID.String(),
		CheckoutSessionUrl: result.CheckoutURL,
		Status:             string(result.Payment.Status),
	}, nil
}

func (s *Server) GetPaymentStatus(ctx context.Context, req *pb.GetPaymentStatusRequest) (*pb.GetPaymentStatusResponse, error) {
	orderID, err := uuid.Parse(req.OrderId)
	if err != nil {
		return nil, fmt.Errorf("invalid order_id: %w", err)
	}

	payment, err := s.svc.GetPaymentStatus(ctx, orderID)
	if err != nil {
		return nil, err
	}

	return &pb.GetPaymentStatusResponse{
		PaymentId:   payment.ID.String(),
		OrderId:     payment.OrderID.String(),
		Status:      string(payment.Status),
		AmountCents: int32(payment.AmountCents),
		Currency:    payment.Currency,
		CreatedAt:   timestamppb.New(payment.CreatedAt),
	}, nil
}

func (s *Server) RefundPayment(ctx context.Context, req *pb.RefundPaymentRequest) (*pb.RefundPaymentResponse, error) {
	orderID, err := uuid.Parse(req.OrderId)
	if err != nil {
		return nil, fmt.Errorf("invalid order_id: %w", err)
	}

	payment, refundID, err := s.svc.RefundPayment(ctx, orderID, req.Reason)
	if err != nil {
		return nil, err
	}

	return &pb.RefundPaymentResponse{
		PaymentId:      payment.ID.String(),
		Status:         string(payment.Status),
		StripeRefundId: refundID,
	}, nil
}
```

- [ ] **Step 4: Run tests**

```bash
cd go/payment-service && go mod tidy && go test ./internal/grpcserver/... -v
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add go/payment-service/internal/grpcserver/
git commit -m "feat(payment): add gRPC server implementing PaymentService proto"
```

---

### Task 14: Health Handler + REST Routes

**Files:**
- Create: `go/payment-service/internal/handler/health.go`
- Create: `go/payment-service/cmd/server/routes.go`

- [ ] **Step 1: Create health handler**

Create `go/payment-service/internal/handler/health.go`:

```go
package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

type HealthHandler struct {
	pool *pgxpool.Pool
}

func NewHealthHandler(pool *pgxpool.Pool) *HealthHandler {
	return &HealthHandler{pool: pool}
}

func (h *HealthHandler) Health(c *gin.Context) {
	if err := h.pool.Ping(c.Request.Context()); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "unhealthy", "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "healthy"})
}
```

- [ ] **Step 2: Create routes.go**

Create `go/payment-service/cmd/server/routes.go`:

```go
package main

import (
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"

	"github.com/kabradshaw1/portfolio/go/payment-service/internal/handler"
	"github.com/kabradshaw1/portfolio/go/pkg/apperror"
	pkgmw "github.com/kabradshaw1/portfolio/go/pkg/middleware"
)

func setupRouter(
	cfg Config,
	webhookHandler *handler.WebhookHandler,
	healthHandler *handler.HealthHandler,
) *gin.Engine {
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(pkgmw.SecurityHeaders())
	router.Use(otelgin.Middleware("payment-service"))
	router.Use(apperror.ErrorHandler())

	// Public routes
	router.GET("/health", healthHandler.Health)
	router.GET("/metrics", gin.WrapH(promhttp.Handler()))

	// Stripe webhook — no auth middleware (Stripe verifies via signature)
	router.POST("/webhooks/stripe", webhookHandler.HandleWebhook)

	return router
}
```

- [ ] **Step 3: Commit**

```bash
git add go/payment-service/internal/handler/health.go go/payment-service/cmd/server/routes.go
git commit -m "feat(payment): add health handler and REST route setup"
```

---

### Task 15: Main Server Bootstrap

**Files:**
- Create: `go/payment-service/cmd/server/deps.go`
- Create: `go/payment-service/cmd/server/main.go`

- [ ] **Step 1: Create deps.go**

Create `go/payment-service/cmd/server/deps.go`:

```go
package main

import (
	"context"
	"log"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/segmentio/kafka-go"
)

func connectPostgres(ctx context.Context, databaseURL string) *pgxpool.Pool {
	poolConfig, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		log.Fatalf("parse database URL: %v", err)
	}

	poolConfig.MaxConns = 15
	poolConfig.MinConns = 3
	poolConfig.MaxConnIdleTime = 5 * time.Minute
	poolConfig.MaxConnLifetime = 30 * time.Minute
	poolConfig.HealthCheckPeriod = 30 * time.Second

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		log.Fatalf("connect postgres: %v", err)
	}

	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("ping postgres: %v", err)
	}

	slog.Info("connected to PostgreSQL")
	return pool
}

func connectRabbitMQ(url string) (*amqp.Connection, *amqp.Channel) {
	conn, err := amqp.Dial(url)
	if err != nil {
		log.Fatalf("rabbitmq dial: %v", err)
	}
	ch, err := conn.Channel()
	if err != nil {
		log.Fatalf("rabbitmq channel: %v", err)
	}
	slog.Info("connected to RabbitMQ")
	return conn, ch
}

func connectKafka(brokers string) *kafka.Writer {
	if brokers == "" {
		return nil
	}
	w := &kafka.Writer{
		Addr:         kafka.TCP(brokers),
		Balancer:     &kafka.LeastBytes{},
		Async:        true,
		BatchSize:    100,
		BatchTimeout: 1 * time.Second,
		RequiredAcks: kafka.RequireOne,
	}
	slog.Info("connected to Kafka", "brokers", brokers)
	return w
}
```

- [ ] **Step 2: Create main.go**

Create `go/payment-service/cmd/server/main.go`:

```go
package main

import (
	"context"
	"log"
	"log/slog"
	"net"
	"net/http"
	"os"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"

	"github.com/kabradshaw1/portfolio/go/payment-service/internal/grpcserver"
	"github.com/kabradshaw1/portfolio/go/payment-service/internal/handler"
	"github.com/kabradshaw1/portfolio/go/payment-service/internal/outbox"
	"github.com/kabradshaw1/portfolio/go/payment-service/internal/repository"
	"github.com/kabradshaw1/portfolio/go/payment-service/internal/service"
	stripeClient "github.com/kabradshaw1/portfolio/go/payment-service/internal/stripe"
	pb "github.com/kabradshaw1/portfolio/go/payment-service/pb/payment/v1"
	"github.com/kabradshaw1/portfolio/go/pkg/resilience"
	"github.com/kabradshaw1/portfolio/go/pkg/shutdown"
	"github.com/kabradshaw1/portfolio/go/pkg/tracing"
)

func main() {
	cfg := loadConfig()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	shutdownTracer, err := tracing.Init(ctx, "payment-service", cfg.OTELEndpoint)
	if err != nil {
		log.Fatalf("tracing init: %v", err)
	}
	slog.SetDefault(slog.New(
		tracing.NewLogHandler(slog.NewJSONHandler(os.Stdout, nil)),
	))

	pool := connectPostgres(ctx, cfg.DatabaseURL)

	conn, ch := connectRabbitMQ(cfg.RabbitmqURL)
	defer conn.Close()
	defer ch.Close()

	kafkaWriter := connectKafka(cfg.KafkaBrokers)

	pgBreaker := resilience.NewBreaker(resilience.BreakerConfig{
		Name:          "payment-postgres",
		OnStateChange: resilience.ObserveStateChange,
	})

	paymentRepo := repository.NewPaymentRepository(pool, pgBreaker)
	outboxRepo := repository.NewOutboxRepository(pool, pgBreaker)
	eventRepo := repository.NewProcessedEventRepository(pool, pgBreaker)

	sc := stripeClient.NewClient(cfg.StripeSecretKey)
	verifier := stripeClient.NewVerifier(cfg.StripeWebhookSecret)

	paymentSvc := service.NewPaymentService(paymentRepo, sc)
	webhookSvc := service.NewWebhookService(paymentRepo, eventRepo, outboxRepo, pool)

	// Start outbox poller — polls every 5 seconds, processes up to 100 messages per batch.
	outboxPoller := outbox.NewPoller(outboxRepo, ch, 5*time.Second, 100)
	go outboxPoller.Run(ctx)

	// REST server
	webhookHandler := handler.NewWebhookHandler(webhookSvc, verifier)
	healthHandler := handler.NewHealthHandler(pool)
	router := setupRouter(cfg, webhookHandler, healthHandler)

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		slog.Info("REST server starting", "port", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("REST server: %v", err)
		}
	}()

	// gRPC server
	grpcSrv := grpc.NewServer(
		grpc.StatsHandler(otelgrpc.NewServerHandler()),
	)
	pb.RegisterPaymentServiceServer(grpcSrv, grpcserver.NewServer(paymentSvc))
	healthSrv := health.NewServer()
	healthpb.RegisterHealthServer(grpcSrv, healthSrv)
	reflection.Register(grpcSrv)

	lis, err := net.Listen("tcp", ":"+cfg.GRPCPort)
	if err != nil {
		log.Fatalf("gRPC listen: %v", err)
	}

	go func() {
		slog.Info("gRPC server starting", "port", cfg.GRPCPort)
		if err := grpcSrv.Serve(lis); err != nil {
			log.Fatalf("gRPC server: %v", err)
		}
	}()

	// Graceful shutdown
	sm := shutdown.New(15 * time.Second)
	sm.Register("cancel-ctx", 0, func(_ context.Context) error {
		cancel()
		return nil
	})
	sm.Register("drain-http", 0, shutdown.DrainHTTP("payment-http", srv))
	sm.Register("drain-grpc", 0, func(_ context.Context) error {
		grpcSrv.GracefulStop()
		return nil
	})
	sm.Register("wait-outbox", 10, shutdown.WaitForInflight("outbox-poller", outboxPoller.IsIdle, 100*time.Millisecond))
	sm.Register("postgres", 20, func(_ context.Context) error {
		pool.Close()
		return nil
	})
	sm.Register("rabbitmq", 20, func(_ context.Context) error {
		_ = ch.Close()
		return conn.Close()
	})
	if kafkaWriter != nil {
		sm.Register("kafka", 20, func(_ context.Context) error {
			return kafkaWriter.Close()
		})
	}
	sm.Register("otel", 30, func(ctx context.Context) error {
		return shutdownTracer(ctx)
	})
	sm.Wait()
}
```

- [ ] **Step 3: Tidy and verify compilation**

```bash
cd go/payment-service && go mod tidy && go build ./cmd/server/
```

Expected: Successful compilation.

- [ ] **Step 4: Run all unit tests**

```bash
cd go/payment-service && go test ./... -v -race
```

Expected: All tests pass.

- [ ] **Step 5: Commit**

```bash
git add go/payment-service/cmd/server/deps.go go/payment-service/cmd/server/main.go go/payment-service/go.mod go/payment-service/go.sum
git commit -m "feat(payment): add main server bootstrap with REST + gRPC + outbox poller + graceful shutdown"
```

---

### Task 16: Order-Service Saga Evolution — New Steps + Payment Client

**Files:**
- Modify: `go/order-service/internal/saga/types.go`
- Create: `go/order-service/internal/paymentclient/client.go`
- Modify: `go/order-service/internal/saga/orchestrator.go`
- Modify: `go/order-service/internal/saga/consumer.go`
- Modify: `go/order-service/cmd/server/config.go`
- Modify: `go/order-service/cmd/server/main.go`

- [ ] **Step 1: Add payment-service proto to order-service go.mod**

Add replace directive to `go/order-service/go.mod`:

```
replace github.com/kabradshaw1/portfolio/go/payment-service => ../payment-service
```

Then: `cd go/order-service && go mod tidy`

- [ ] **Step 2: Add new saga step constants**

Modify `go/order-service/internal/saga/types.go` — add after `StepStockValidated`:

```go
StepPaymentCreated  = "PAYMENT_CREATED"
StepPaymentConfirmed = "PAYMENT_CONFIRMED"
```

Add new command and event constants:

```go
// Commands
CmdCreatePayment = "create.payment"

// Events
EvtPaymentConfirmed = "payment.confirmed"
EvtPaymentFailed    = "payment.failed"
```

- [ ] **Step 3: Create payment gRPC client**

Create `go/order-service/internal/paymentclient/client.go`:

```go
package paymentclient

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	pb "github.com/kabradshaw1/portfolio/go/payment-service/pb/payment/v1"
)

type GRPCClient struct {
	conn   *grpc.ClientConn
	client pb.PaymentServiceClient
}

func New(addr string, creds credentials.TransportCredentials) (*GRPCClient, error) {
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(creds))
	if err != nil {
		return nil, fmt.Errorf("payment grpc dial: %w", err)
	}
	return &GRPCClient{
		conn:   conn,
		client: pb.NewPaymentServiceClient(conn),
	}, nil
}

func (c *GRPCClient) Close() error {
	return c.conn.Close()
}

func (c *GRPCClient) CreatePayment(ctx context.Context, orderID uuid.UUID, amountCents int, currency, successURL, cancelURL string) (string, error) {
	resp, err := c.client.CreatePayment(ctx, &pb.CreatePaymentRequest{
		OrderId:     orderID.String(),
		AmountCents: int32(amountCents),
		Currency:    currency,
		SuccessUrl:  successURL,
		CancelUrl:   cancelURL,
	})
	if err != nil {
		return "", fmt.Errorf("create payment: %w", err)
	}
	return resp.CheckoutSessionUrl, nil
}

func (c *GRPCClient) RefundPayment(ctx context.Context, orderID uuid.UUID, reason string) error {
	_, err := c.client.RefundPayment(ctx, &pb.RefundPaymentRequest{
		OrderId: orderID.String(),
		Reason:  reason,
	})
	return err
}
```

- [ ] **Step 4: Update orchestrator — add PaymentCreator interface and new handlers**

Modify `go/order-service/internal/saga/orchestrator.go`:

Add new interface:

```go
// PaymentCreator abstracts payment-service gRPC calls.
type PaymentCreator interface {
	CreatePayment(ctx context.Context, orderID uuid.UUID, amountCents int, currency, successURL, cancelURL string) (string, error)
	RefundPayment(ctx context.Context, orderID uuid.UUID, reason string) error
}
```

Add `payment` field to `Orchestrator` struct:

```go
type Orchestrator struct {
	repo     OrderRepository
	pub      SagaPublisher
	stock    StockChecker
	payment  PaymentCreator
	kafkaPub kafka.Producer
}
```

Update `NewOrchestrator`:

```go
func NewOrchestrator(repo OrderRepository, pub SagaPublisher, stock StockChecker, payment PaymentCreator, kafkaPub kafka.Producer) *Orchestrator {
	return &Orchestrator{repo: repo, pub: pub, stock: stock, payment: payment, kafkaPub: kafkaPub}
}
```

Update `Advance()` switch to add `StepPaymentCreated` and `StepPaymentConfirmed`:

```go
case StepPaymentCreated:
	return nil // Waiting for webhook event via outbox poller
case StepPaymentConfirmed:
	return o.handlePaymentConfirmed(ctx, order)
```

Update `handleStockValidated` to create payment instead of going directly to completion:

```go
func (o *Orchestrator) handleStockValidated(ctx context.Context, order *model.Order) error {
	if o.payment != nil {
		// Create payment via payment-service gRPC
		_, err := o.payment.CreatePayment(ctx, order.ID, order.Total, "usd",
			"https://kylebradshaw.dev/go/ecommerce/checkout/success?order="+order.ID.String(),
			"https://kylebradshaw.dev/go/ecommerce/checkout/cancel?order="+order.ID.String(),
		)
		if err != nil {
			slog.ErrorContext(ctx, "create payment failed, compensating", "orderID", order.ID, "error", err)
			return o.compensate(ctx, order)
		}

		if err := o.repo.UpdateSagaStep(ctx, order.ID, StepPaymentCreated); err != nil {
			return err
		}
		SagaStepsTotal.WithLabelValues(StepPaymentCreated, "success").Inc()
		return nil // Wait for webhook confirmation
	}

	// No payment service configured — skip payment step (development mode)
	return o.handlePaymentConfirmed(ctx, order)
}
```

Add new handler:

```go
func (o *Orchestrator) handlePaymentConfirmed(ctx context.Context, order *model.Order) error {
	if err := o.repo.UpdateSagaStep(ctx, order.ID, StepPaymentConfirmed); err != nil {
		return err
	}
	SagaStepsTotal.WithLabelValues(StepPaymentConfirmed, "success").Inc()

	// Clear cart and complete
	return o.pub.PublishCommand(ctx, Command{
		Command: CmdClearCart,
		OrderID: order.ID.String(),
		UserID:  order.UserID.String(),
	})
}
```

Update the existing cart-cleared handler (`handleStockValidated` completion logic) to move into the `EvtCartCleared` handling, updating `HandleEvent`:

```go
case EvtCartCleared:
	return o.completeOrder(ctx, orderID)
```

Extract completion logic into `completeOrder`:

```go
func (o *Orchestrator) completeOrder(ctx context.Context, orderID uuid.UUID) error {
	order, err := o.repo.FindByID(ctx, orderID)
	if err != nil {
		return fmt.Errorf("find order for completion: %w", err)
	}

	if err := o.repo.UpdateStatus(ctx, order.ID, model.OrderStatusCompleted); err != nil {
		return err
	}
	if err := o.repo.UpdateSagaStep(ctx, order.ID, StepCompleted); err != nil {
		return err
	}

	SagaStepsTotal.WithLabelValues(StepCompleted, "success").Inc()
	SagaDuration.Observe(time.Since(order.CreatedAt).Seconds())
	slog.InfoContext(ctx, "saga completed", "orderID", order.ID)

	// Publish Kafka analytics event (fire-and-forget).
	if o.kafkaPub != nil {
		type itemData struct {
			ProductID  string `json:"productID"`
			Quantity   int    `json:"quantity"`
			PriceCents int    `json:"priceCents"`
		}
		items := make([]itemData, len(order.Items))
		for i, oi := range order.Items {
			items[i] = itemData{
				ProductID:  oi.ProductID.String(),
				Quantity:   oi.Quantity,
				PriceCents: oi.PriceAtPurchase,
			}
		}
		kafka.SafePublish(ctx, o.kafkaPub, "ecommerce.orders", order.ID.String(), kafka.Event{
			Type: "order.completed",
			Data: map[string]any{
				"orderID":    order.ID.String(),
				"userID":     order.UserID.String(),
				"totalCents": order.Total,
				"items":      items,
			},
		})
	}

	return nil
}
```

Update `compensate` to refund payment if the payment was already created:

```go
func (o *Orchestrator) compensate(ctx context.Context, order *model.Order) error {
	// Refund if payment was already processed
	if o.payment != nil && (order.SagaStep == StepPaymentConfirmed || order.SagaStep == StepPaymentCreated) {
		if err := o.payment.RefundPayment(ctx, order.ID, "saga compensation"); err != nil {
			slog.ErrorContext(ctx, "refund failed during compensation", "orderID", order.ID, "error", err)
		}
	}

	if err := o.repo.UpdateStatus(ctx, order.ID, model.OrderStatusFailed); err != nil {
		return err
	}
	if err := o.repo.UpdateSagaStep(ctx, order.ID, StepCompensating); err != nil {
		return err
	}
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

- [ ] **Step 5: Update HandleEvent to handle payment events**

In `go/order-service/internal/saga/consumer.go`, add to the `HandleEvent` switch:

```go
case EvtPaymentConfirmed:
	if err := o.repo.UpdateSagaStep(ctx, orderID, StepPaymentConfirmed); err != nil {
		return err
	}
	SagaStepsTotal.WithLabelValues(StepPaymentConfirmed, "success").Inc()
	return o.Advance(ctx, orderID)

case EvtPaymentFailed:
	order, err := o.repo.FindByID(ctx, orderID)
	if err != nil {
		return fmt.Errorf("find order for payment failure: %w", err)
	}
	return o.compensate(ctx, order)
```

- [ ] **Step 6: Update config and main.go**

Add to `go/order-service/cmd/server/config.go`:

```go
PaymentGRPCAddr string // optional, address of payment-service gRPC
```

And in `loadConfig()`:

```go
PaymentGRPCAddr: os.Getenv("PAYMENT_GRPC_ADDR"),
```

Update `go/order-service/cmd/server/main.go` to create payment client and pass to orchestrator:

```go
var payClient *paymentclient.GRPCClient
if cfg.PaymentGRPCAddr != "" {
	var err error
	payClient, err = paymentclient.New(cfg.PaymentGRPCAddr, grpcCreds)
	if err != nil {
		log.Fatalf("payment gRPC client: %v", err)
	}
	defer payClient.Close()
	slog.Info("connected to payment-service gRPC", "addr", cfg.PaymentGRPCAddr)
}
```

Update `NewOrchestrator` call to include `payClient`.

- [ ] **Step 7: Update order-service Dockerfile to COPY payment-service**

Add to `go/order-service/Dockerfile` (after `COPY pkg/`):

```dockerfile
COPY payment-service/ /app/payment-service/
```

- [ ] **Step 8: Run order-service tests**

```bash
cd go/order-service && go mod tidy && go test ./... -v -race
```

Expected: Tests pass (existing tests use nil payment creator which triggers the "skip payment" path).

- [ ] **Step 9: Commit**

```bash
git add go/order-service/
git commit -m "feat(order): extend saga with payment steps — PAYMENT_CREATED and PAYMENT_CONFIRMED"
```

---

### Task 17: Kafka Analytics — Payment Events

> **Note:** Payment-service also needs to publish Kafka events for analytics. In Task 15 (main.go), `connectKafka` is already wired up. The webhook service should publish `payment.succeeded`, `payment.failed`, and `payment.refunded` events to `ecommerce.payments` using the same `kafka.SafePublish` pattern from order-service. Wire the Kafka writer through to `WebhookService` and publish after updating payment status. This is fire-and-forget analytics, not saga-critical.

**Files:**
- Modify: `go/analytics-service/internal/consumer/consumer.go`

- [ ] **Step 1: Add payment topic constant**

Add to consumer.go constants:

```go
TopicPayments = "ecommerce.payments"
```

Add to the `GroupTopics` slice: `TopicPayments`

- [ ] **Step 2: Add routing case**

Add to the `route()` switch:

```go
case TopicPayments:
	c.handlePayment(env)
```

- [ ] **Step 3: Implement handlePayment**

```go
func (c *Consumer) handlePayment(env event) {
	switch env.Type {
	case "payment.succeeded":
		// Track successful payment metrics
		slog.Info("payment succeeded", "data", env.Data)
	case "payment.failed":
		slog.Info("payment failed", "data", env.Data)
	case "payment.refunded":
		slog.Info("payment refunded", "data", env.Data)
	}
}
```

- [ ] **Step 4: Run analytics-service tests**

```bash
cd go/analytics-service && go test ./... -v -race
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add go/analytics-service/
git commit -m "feat(analytics): add Kafka consumer for ecommerce.payments topic"
```

---

### Task 18: K8s Manifests + CI + Makefile

**Files:**
- Create: `go/k8s/deployments/payment-service.yml`
- Create: `go/k8s/configmaps/payment-service-config.yml`
- Create: `go/k8s/jobs/payment-service-migrate.yml`
- Create: `go/k8s/pdb/payment-service-pdb.yml`
- Create: `go/k8s/services/payment-service-svc.yml`
- Modify: `go/k8s/ingress.yml`
- Modify: `go/k8s/kustomization.yaml`
- Modify: `k8s/overlays/qa-go/kustomization.yaml`
- Modify: `.github/workflows/ci.yml`
- Modify: `Makefile`
- Modify: `k8s/deploy.sh`

- [ ] **Step 1: Create K8s deployment**

Create `go/k8s/deployments/payment-service.yml` following the product-service template pattern. Use ports 8098 (http) and 9098 (grpc). Add `envFrom: configMapRef: payment-service-config`. Include security context, probes, resource limits (64Mi/256Mi memory, 100m/500m CPU), and TLS volume mount.

- [ ] **Step 2: Create K8s ConfigMap**

Create `go/k8s/configmaps/payment-service-config.yml`:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: payment-service-config
  namespace: go-ecommerce
data:
  DATABASE_URL: "postgres://taskuser:taskpass@postgres.java-tasks.svc.cluster.local:5432/paymentdb?sslmode=disable"
  PORT: "8098"
  GRPC_PORT: "9098"
  RABBITMQ_URL: "amqp://guest:guest@rabbitmq.java-tasks.svc.cluster.local:5672"
  KAFKA_BROKERS: "kafka.go-ecommerce.svc.cluster.local:9092"
  ALLOWED_ORIGINS: "https://kylebradshaw.dev,http://localhost:3000"
  OTEL_EXPORTER_OTLP_ENDPOINT: "jaeger-collector.monitoring.svc.cluster.local:4317"
  TLS_CERT_DIR: "/etc/tls"
```

Note: `STRIPE_SECRET_KEY` and `STRIPE_WEBHOOK_SECRET` are stored as K8s Secrets, not in the ConfigMap.

- [ ] **Step 3: Create remaining K8s resources**

Create service, migration job, PDB, and Stripe secrets manifests following existing patterns.

- [ ] **Step 4: Update ingress**

Add to `go/k8s/ingress.yml`:

```yaml
- path: /go-payments(/|$)(.*)
  pathType: ImplementationSpecific
  backend:
    service:
      name: go-payment-service
      port:
        number: 8098
```

- [ ] **Step 5: Update kustomization.yaml**

Add new resources to `go/k8s/kustomization.yaml`.

- [ ] **Step 6: Update QA Kustomize overlay**

Add payment-service ConfigMap patch to `k8s/overlays/qa-go/kustomization.yaml` with `paymentdb_qa` DATABASE_URL and QA CORS origins.

- [ ] **Step 7: Update CI matrices**

Add `payment-service` to the `go-lint`, `go-tests`, `build-images`, and `security-hadolint` matrices in `.github/workflows/ci.yml`.

Add migration job delete+apply+wait for payment-service in both QA deploy and prod deploy steps.

- [ ] **Step 8: Update Makefile**

Add to `preflight-go`:

```makefile
cd go/payment-service && golangci-lint run ./...
```

and:

```makefile
cd go/payment-service && go test ./... -v -race
```

- [ ] **Step 9: Update deploy.sh**

Add `kubectl wait` for payment-service deployment in both QA and prod sections.

- [ ] **Step 10: Run preflight**

```bash
make preflight-go
```

Expected: PASS

- [ ] **Step 11: Commit**

```bash
git add go/k8s/ k8s/overlays/ .github/workflows/ci.yml Makefile k8s/deploy.sh
git commit -m "feat(payment): add K8s manifests, CI matrices, Makefile, and deploy script entries"
```

---

### Task 19: Order-Service ConfigMap Update

**Files:**
- Modify: `go/k8s/configmaps/order-service-config.yml`
- Modify: `k8s/overlays/qa-go/kustomization.yaml`

- [ ] **Step 1: Add PAYMENT_GRPC_ADDR to order-service configmap**

Add to `go/k8s/configmaps/order-service-config.yml`:

```yaml
PAYMENT_GRPC_ADDR: "go-payment-service.go-ecommerce.svc.cluster.local:9098"
```

- [ ] **Step 2: Add PAYMENT_GRPC_ADDR to QA overlay**

Add to the order-service patch in `k8s/overlays/qa-go/kustomization.yaml`:

```yaml
PAYMENT_GRPC_ADDR: "go-payment-service.go-ecommerce-qa.svc.cluster.local:9098"
```

- [ ] **Step 3: Commit**

```bash
git add go/k8s/configmaps/order-service-config.yml k8s/overlays/qa-go/kustomization.yaml
git commit -m "feat(order): add PAYMENT_GRPC_ADDR config for payment-service integration"
```
