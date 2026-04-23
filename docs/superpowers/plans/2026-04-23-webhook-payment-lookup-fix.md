# Webhook Payment Lookup Fix Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix the Stripe webhook handler so it can find payment records when `payment_intent.succeeded` fires, by extracting `order_id` from PaymentIntent metadata instead of looking up by the never-stored `stripe_payment_intent_id`.

**Architecture:** Extend the `EventVerifier` to return metadata from PaymentIntent events. The webhook service uses `order_id` from metadata to look up payments via `FindByOrderID` (already exists), then backfills the `stripe_payment_intent_id` for future refund lookups. Also add a startup readiness wait to the outbox poller so it doesn't trip the circuit breaker when postgres isn't ready yet.

**Tech Stack:** Go, Stripe Go SDK v82, pgxpool, RabbitMQ, circuit breakers (gobreaker)

---

## File Map

| File | Action | Responsibility |
|------|--------|---------------|
| `go/payment-service/internal/stripe/verifier.go` | Modify | Add metadata extraction from PaymentIntent events |
| `go/payment-service/internal/stripe/verifier_test.go` | Modify | Test metadata extraction via `extractIntentID` refactor |
| `go/payment-service/internal/handler/webhook.go` | Modify | Update interfaces, pass metadata through |
| `go/payment-service/internal/handler/webhook_test.go` | Modify | Update mocks for new signatures |
| `go/payment-service/internal/service/webhook.go` | Modify | Use order_id from metadata, backfill intent ID |
| `go/payment-service/internal/service/webhook_test.go` | Modify | Test metadata-based lookup and backfill |
| `go/payment-service/internal/outbox/poller.go` | Modify | Add waitForDB startup check |
| `go/payment-service/internal/outbox/poller_test.go` | Modify | Test waitForDB behavior |
| `go/payment-service/internal/repository/outbox.go` | Modify | Add Ping method |

---

### Task 1: Extend verifier to return metadata

**Files:**
- Modify: `go/payment-service/internal/stripe/verifier.go`
- Modify: `go/payment-service/internal/stripe/verifier_test.go`

- [ ] **Step 1: Write test for extractIntentAndMetadata**

Add a test for the new extraction function that returns both intentID and metadata:

```go
// In verifier_test.go — add to existing file

func TestExtractIntentAndMetadata_PaymentIntentEvent(t *testing.T) {
	raw := []byte(`{"id":"pi_123","object":"payment_intent","metadata":{"order_id":"f5cd888c-c661-41ad-a2fd-e14fdeac800d"}}`)
	event := gostripe.Event{
		Type: "payment_intent.succeeded",
		Data: &gostripe.EventData{Raw: raw},
	}

	intentID, metadata, err := extractIntentAndMetadata(event)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if intentID != "pi_123" {
		t.Errorf("expected intentID pi_123, got %s", intentID)
	}
	if metadata["order_id"] != "f5cd888c-c661-41ad-a2fd-e14fdeac800d" {
		t.Errorf("expected order_id in metadata, got %v", metadata)
	}
}

func TestExtractIntentAndMetadata_ChargeRefundedEvent(t *testing.T) {
	raw := []byte(`{"id":"ch_123","object":"charge","payment_intent":"pi_456"}`)
	event := gostripe.Event{
		Type: "charge.refunded",
		Data: &gostripe.EventData{Raw: raw},
	}

	intentID, metadata, err := extractIntentAndMetadata(event)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if intentID != "pi_456" {
		t.Errorf("expected intentID pi_456, got %s", intentID)
	}
	if metadata != nil {
		t.Errorf("expected nil metadata for charge event, got %v", metadata)
	}
}

func TestExtractIntentAndMetadata_UnknownEvent(t *testing.T) {
	event := gostripe.Event{
		Type: "unknown.event",
		Data: &gostripe.EventData{Raw: []byte(`{}`)},
	}

	intentID, metadata, err := extractIntentAndMetadata(event)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if intentID != "" {
		t.Errorf("expected empty intentID, got %s", intentID)
	}
	if metadata != nil {
		t.Errorf("expected nil metadata, got %v", metadata)
	}
}
```

Add the import for `gostripe`:

```go
import (
	"testing"

	gostripe "github.com/stripe/stripe-go/v82"
)
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd go/payment-service && go test ./internal/stripe/ -run TestExtractIntentAndMetadata -v`
Expected: FAIL — `extractIntentAndMetadata` is not defined

- [ ] **Step 3: Refactor verifier.go — rename and extend extraction function**

Replace the `extractIntentID` function and update `VerifyAndParse` to return metadata:

In `verifier.go`, change the `VerifyAndParse` method signature and replace `extractIntentID`:

```go
// VerifyAndParse validates the Stripe-Signature header and returns the event type,
// event ID, payment intent ID, and metadata extracted from the event payload.
// For "payment_intent.*" events, metadata contains the PaymentIntent's metadata map.
// For other event types, metadata is nil.
func (v *Verifier) VerifyAndParse(payload []byte, sigHeader string) (eventType, eventID, intentID string, metadata map[string]string, err error) {
	event, err := webhook.ConstructEventWithOptions(payload, sigHeader, v.webhookSecret, webhook.ConstructEventOptions{
		IgnoreAPIVersionMismatch: true,
	})
	if err != nil {
		return "", "", "", nil, fmt.Errorf("stripe signature verification: %w", err)
	}

	eventType = string(event.Type)
	eventID = event.ID

	intentID, metadata, err = extractIntentAndMetadata(event)
	if err != nil {
		return "", "", "", nil, fmt.Errorf("extract intent id from event %s: %w", eventID, err)
	}

	return eventType, eventID, intentID, metadata, nil
}

// extractIntentAndMetadata reads the PaymentIntent ID and metadata from the event's data object.
// For payment_intent.* events the object is a PaymentIntent; for charge.refunded
// the object is a Charge whose PaymentIntent field holds the intent ID.
// Metadata is only returned for payment_intent.* events.
func extractIntentAndMetadata(event gostripe.Event) (string, map[string]string, error) {
	switch {
	case strings.HasPrefix(string(event.Type), "payment_intent."):
		var pi gostripe.PaymentIntent
		if err := json.Unmarshal(event.Data.Raw, &pi); err != nil {
			return "", nil, fmt.Errorf("unmarshal payment intent: %w", err)
		}
		return pi.ID, pi.Metadata, nil

	case event.Type == "charge.refunded":
		var charge gostripe.Charge
		if err := json.Unmarshal(event.Data.Raw, &charge); err != nil {
			return "", nil, fmt.Errorf("unmarshal charge: %w", err)
		}
		if charge.PaymentIntent != nil {
			return charge.PaymentIntent.ID, nil, nil
		}
		return "", nil, fmt.Errorf("charge.refunded event has no payment intent")

	default:
		return "", nil, nil
	}
}
```

Delete the old `extractIntentID` function entirely.

- [ ] **Step 4: Update existing verifier test for new signature**

In `verifier_test.go`, update `TestVerifier_InvalidSignature` to match the new return signature:

```go
func TestVerifier_InvalidSignature(t *testing.T) {
	v := NewVerifier("whsec_test_secret")

	payload := []byte(`{"id":"evt_123","type":"payment_intent.succeeded","api_version":"2024-06-20.basil"}`)
	sigHeader := "t=1234567890,v1=invalidsignature"

	_, _, _, _, err := v.VerifyAndParse(payload, sigHeader)
	if err == nil {
		t.Fatal("expected error for invalid signature, got nil")
	}
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd go/payment-service && go test ./internal/stripe/ -v`
Expected: PASS — all tests pass

- [ ] **Step 6: Commit**

```bash
git add go/payment-service/internal/stripe/verifier.go go/payment-service/internal/stripe/verifier_test.go
git commit -m "fix(payment): extend verifier to return PaymentIntent metadata"
```

---

### Task 2: Update handler interfaces and pass metadata through

**Files:**
- Modify: `go/payment-service/internal/handler/webhook.go`
- Modify: `go/payment-service/internal/handler/webhook_test.go`

- [ ] **Step 1: Update mocks in webhook_test.go for new signatures**

Update both mock types and the existing tests to use the new signatures:

```go
// In webhook_test.go — replace the mock types and their methods

type mockWebhookService struct {
	succeededErr error
	failedErr    error
	refundErr    error
	calls        []string
}

func (m *mockWebhookService) HandlePaymentSucceeded(_ context.Context, _, _ string, _ map[string]string) error {
	m.calls = append(m.calls, "succeeded")
	return m.succeededErr
}

func (m *mockWebhookService) HandlePaymentFailed(_ context.Context, _, _ string, _ map[string]string) error {
	m.calls = append(m.calls, "failed")
	return m.failedErr
}

func (m *mockWebhookService) HandleRefund(_ context.Context, _, _ string) error {
	m.calls = append(m.calls, "refund")
	return m.refundErr
}

type mockEventVerifier struct {
	eventType string
	eventID   string
	intentID  string
	metadata  map[string]string
	err       error
}

func (m *mockEventVerifier) VerifyAndParse(_ []byte, _ string) (string, string, string, map[string]string, error) {
	return m.eventType, m.eventID, m.intentID, m.metadata, m.err
}
```

Update the test for `TestWebhookHandler_PaymentSucceeded` to include metadata in the verifier mock:

```go
func TestWebhookHandler_PaymentSucceeded(t *testing.T) {
	svc := &mockWebhookService{}
	verifier := &mockEventVerifier{
		eventType: "payment_intent.succeeded",
		eventID:   "evt_123",
		intentID:  "pi_123",
		metadata:  map[string]string{"order_id": "f5cd888c-c661-41ad-a2fd-e14fdeac800d"},
	}
	router := setupWebhookRouter(svc, verifier)

	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(`{}`))
	req.Header.Set("Stripe-Signature", "v1=abc")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if len(svc.calls) != 1 || svc.calls[0] != "succeeded" {
		t.Errorf("expected succeeded call, got %v", svc.calls)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd go/payment-service && go test ./internal/handler/ -v`
Expected: FAIL — `WebhookService` interface has wrong signature

- [ ] **Step 3: Update interfaces and handler in webhook.go**

Replace the interfaces and update the `HandleWebhook` method:

```go
// WebhookService processes validated Stripe webhook events.
type WebhookService interface {
	HandlePaymentSucceeded(ctx context.Context, eventID, intentID string, metadata map[string]string) error
	HandlePaymentFailed(ctx context.Context, eventID, intentID string, metadata map[string]string) error
	HandleRefund(ctx context.Context, eventID, intentID string) error
}

// EventVerifier validates the Stripe-Signature header and extracts event fields.
type EventVerifier interface {
	VerifyAndParse(payload []byte, sigHeader string) (eventType, eventID, intentID string, metadata map[string]string, err error)
}
```

Update `HandleWebhook`:

```go
func (h *WebhookHandler) HandleWebhook(c *gin.Context) {
	payload, err := io.ReadAll(c.Request.Body)
	if err != nil {
		_ = c.Error(apperror.BadRequest("WEBHOOK_READ_FAILED", "failed to read webhook body"))
		return
	}

	sigHeader := c.GetHeader("Stripe-Signature")
	eventType, eventID, intentID, metadata, err := h.verifier.VerifyAndParse(payload, sigHeader)
	if err != nil {
		_ = c.Error(apperror.BadRequest("WEBHOOK_INVALID_SIGNATURE", "webhook signature verification failed"))
		return
	}

	ctx := c.Request.Context()
	slog.InfoContext(ctx, "webhook event received", "eventType", eventType, "eventID", eventID, "intentID", intentID)

	switch eventType {
	case "payment_intent.succeeded":
		if err := h.svc.HandlePaymentSucceeded(ctx, eventID, intentID, metadata); err != nil {
			metrics.WebhookEvents.WithLabelValues(eventType, "error").Inc()
			_ = c.Error(err)
			return
		}
		metrics.WebhookEvents.WithLabelValues(eventType, "processed").Inc()
	case "payment_intent.payment_failed":
		if err := h.svc.HandlePaymentFailed(ctx, eventID, intentID, metadata); err != nil {
			metrics.WebhookEvents.WithLabelValues(eventType, "error").Inc()
			_ = c.Error(err)
			return
		}
		metrics.WebhookEvents.WithLabelValues(eventType, "processed").Inc()
	case "charge.refunded":
		if err := h.svc.HandleRefund(ctx, eventID, intentID); err != nil {
			metrics.WebhookEvents.WithLabelValues(eventType, "error").Inc()
			_ = c.Error(err)
			return
		}
		metrics.WebhookEvents.WithLabelValues(eventType, "processed").Inc()
	default:
		slog.InfoContext(ctx, "received unknown webhook event type, ignoring", "eventType", eventType, "eventID", eventID)
	}

	c.JSON(http.StatusOK, gin.H{"received": true})
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd go/payment-service && go test ./internal/handler/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add go/payment-service/internal/handler/webhook.go go/payment-service/internal/handler/webhook_test.go
git commit -m "fix(payment): update handler interfaces to pass metadata through"
```

---

### Task 3: Update webhook service to use order_id lookup + backfill

**Files:**
- Modify: `go/payment-service/internal/service/webhook.go`
- Modify: `go/payment-service/internal/service/webhook_test.go`

- [ ] **Step 1: Write test for metadata-based lookup in HandlePaymentSucceeded**

Add a test that verifies the new flow — lookup by order_id from metadata, backfill intent ID, write outbox:

```go
// In webhook_test.go — replace the mock and add new tests

type mockPaymentRepoForWebhook struct {
	findByOrderPayment *model.Payment
	findByOrderErr     error
	findByIntentErr    error
	updateStatusErr    error
	updateStripeIDsErr error
	findByOrderCalled  int
	findByIntentCalled int
	updateStripeCalled int
}

func (m *mockPaymentRepoForWebhook) Create(_ context.Context, _ uuid.UUID, _ int, _ string) (*model.Payment, error) {
	return nil, nil
}

func (m *mockPaymentRepoForWebhook) FindByOrderID(_ context.Context, _ uuid.UUID) (*model.Payment, error) {
	m.findByOrderCalled++
	return m.findByOrderPayment, m.findByOrderErr
}

func (m *mockPaymentRepoForWebhook) FindByStripeIntentID(_ context.Context, _ string) (*model.Payment, error) {
	m.findByIntentCalled++
	return nil, m.findByIntentErr
}

func (m *mockPaymentRepoForWebhook) UpdateStatus(_ context.Context, _ uuid.UUID, _ model.PaymentStatus) error {
	return m.updateStatusErr
}

func (m *mockPaymentRepoForWebhook) UpdateStripeIDs(_ context.Context, _ uuid.UUID, _, _ string) error {
	m.updateStripeCalled++
	return m.updateStripeIDsErr
}
```

Then the test:

```go
func TestWebhookService_HandlePaymentSucceeded_MetadataLookup(t *testing.T) {
	orderID := uuid.MustParse("f5cd888c-c661-41ad-a2fd-e14fdeac800d")
	eventRepo := &mockProcessedEventRepo{inserted: true, err: nil}
	outboxRepo := &mockOutboxRepo{}
	paymentRepo := &mockPaymentRepoForWebhook{
		findByOrderPayment: &model.Payment{
			OrderID:                 orderID,
			StripeCheckoutSessionID: "cs_test_abc",
		},
	}
	svc := NewWebhookService(paymentRepo, eventRepo, outboxRepo, nil)

	metadata := map[string]string{"order_id": orderID.String()}
	err := svc.HandlePaymentSucceeded(context.Background(), "evt_123", "pi_new_123", metadata)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if paymentRepo.findByOrderCalled != 1 {
		t.Errorf("expected FindByOrderID called once, got %d", paymentRepo.findByOrderCalled)
	}
	if paymentRepo.findByIntentCalled != 0 {
		t.Errorf("expected FindByStripeIntentID not called, got %d", paymentRepo.findByIntentCalled)
	}
	if paymentRepo.updateStripeCalled != 1 {
		t.Errorf("expected UpdateStripeIDs called once to backfill, got %d", paymentRepo.updateStripeCalled)
	}
	if outboxRepo.calls != 1 {
		t.Errorf("expected 1 outbox insert, got %d", outboxRepo.calls)
	}
}

func TestWebhookService_HandlePaymentSucceeded_DuplicateEvent(t *testing.T) {
	eventRepo := &mockProcessedEventRepo{inserted: false, err: nil}
	outboxRepo := &mockOutboxRepo{}
	paymentRepo := &mockPaymentRepoForWebhook{}

	svc := NewWebhookService(paymentRepo, eventRepo, outboxRepo, nil)

	err := svc.HandlePaymentSucceeded(context.Background(), "evt_dup", "pi_123", map[string]string{"order_id": "f5cd888c-c661-41ad-a2fd-e14fdeac800d"})
	if err != nil {
		t.Errorf("expected nil error for duplicate event, got %v", err)
	}
	if paymentRepo.findByOrderCalled != 0 {
		t.Errorf("expected no FindByOrderID call for duplicate, got %d", paymentRepo.findByOrderCalled)
	}
	if outboxRepo.calls != 0 {
		t.Errorf("expected no outbox insert for duplicate event, got %d", outboxRepo.calls)
	}
}

func TestWebhookService_HandlePaymentSucceeded_MissingOrderID(t *testing.T) {
	eventRepo := &mockProcessedEventRepo{inserted: true, err: nil}
	outboxRepo := &mockOutboxRepo{}
	paymentRepo := &mockPaymentRepoForWebhook{}

	svc := NewWebhookService(paymentRepo, eventRepo, outboxRepo, nil)

	err := svc.HandlePaymentSucceeded(context.Background(), "evt_123", "pi_123", map[string]string{})
	if err == nil {
		t.Fatal("expected error for missing order_id in metadata")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd go/payment-service && go test ./internal/service/ -run TestWebhookService -v`
Expected: FAIL — `HandlePaymentSucceeded` has wrong signature

- [ ] **Step 3: Update HandlePaymentSucceeded and HandlePaymentFailed**

Replace both methods in `webhook.go`:

```go
// HandlePaymentSucceeded deduplicates the event, looks up the payment by order_id
// from the PaymentIntent metadata, backfills the stripe_payment_intent_id,
// updates payment status to succeeded, and writes a saga confirmation event to the outbox.
func (s *WebhookService) HandlePaymentSucceeded(ctx context.Context, eventID, intentID string, metadata map[string]string) error {
	inserted, err := s.eventRepo.TryInsert(ctx, nil, eventID, "payment_intent.succeeded")
	if err != nil {
		return fmt.Errorf("dedup payment succeeded: %w", err)
	}
	if !inserted {
		metrics.WebhookEvents.WithLabelValues("payment_intent.succeeded", "duplicate").Inc()
		slog.InfoContext(ctx, "duplicate payment_intent.succeeded event, skipping", "eventID", eventID)
		return nil
	}

	orderID, err := orderIDFromMetadata(metadata)
	if err != nil {
		return fmt.Errorf("payment succeeded: %w", err)
	}

	payment, err := s.paymentRepo.FindByOrderID(ctx, orderID)
	if err != nil {
		return fmt.Errorf("find payment for succeeded event: %w", err)
	}

	if intentID != "" {
		if backfillErr := s.paymentRepo.UpdateStripeIDs(ctx, orderID, intentID, payment.StripeCheckoutSessionID); backfillErr != nil {
			slog.ErrorContext(ctx, "failed to backfill stripe intent ID", "orderID", orderID, "intentID", intentID, "error", backfillErr)
		}
	}

	if err := s.paymentRepo.UpdateStatus(ctx, payment.OrderID, model.PaymentStatusSucceeded); err != nil {
		slog.ErrorContext(ctx, "failed to update payment status", "orderID", payment.OrderID, "status", "succeeded", "error", err)
		return fmt.Errorf("update payment status to succeeded: %w", err)
	}

	payload, err := json.Marshal(map[string]string{
		"event":    "payment.confirmed",
		"order_id": payment.OrderID.String(),
	})
	if err != nil {
		return fmt.Errorf("marshal payment confirmed outbox payload: %w", err)
	}

	if err := s.outboxRepo.Insert(ctx, nil, "ecommerce.saga", "saga.order.events", payload); err != nil {
		return fmt.Errorf("insert payment confirmed outbox message: %w", err)
	}

	slog.InfoContext(ctx, "payment succeeded", "orderID", payment.OrderID, "intentID", intentID)
	return nil
}

// HandlePaymentFailed deduplicates the event, looks up the payment by order_id
// from the PaymentIntent metadata, updates payment status to failed,
// and writes a saga failure event to the outbox.
func (s *WebhookService) HandlePaymentFailed(ctx context.Context, eventID, intentID string, metadata map[string]string) error {
	inserted, err := s.eventRepo.TryInsert(ctx, nil, eventID, "payment_intent.payment_failed")
	if err != nil {
		return fmt.Errorf("dedup payment failed: %w", err)
	}
	if !inserted {
		metrics.WebhookEvents.WithLabelValues("payment_intent.payment_failed", "duplicate").Inc()
		slog.InfoContext(ctx, "duplicate payment_intent.payment_failed event, skipping", "eventID", eventID)
		return nil
	}

	orderID, err := orderIDFromMetadata(metadata)
	if err != nil {
		return fmt.Errorf("payment failed: %w", err)
	}

	payment, err := s.paymentRepo.FindByOrderID(ctx, orderID)
	if err != nil {
		return fmt.Errorf("find payment for failed event: %w", err)
	}

	if intentID != "" {
		if backfillErr := s.paymentRepo.UpdateStripeIDs(ctx, orderID, intentID, payment.StripeCheckoutSessionID); backfillErr != nil {
			slog.ErrorContext(ctx, "failed to backfill stripe intent ID", "orderID", orderID, "intentID", intentID, "error", backfillErr)
		}
	}

	if err := s.paymentRepo.UpdateStatus(ctx, payment.OrderID, model.PaymentStatusFailed); err != nil {
		slog.ErrorContext(ctx, "failed to update payment status", "orderID", payment.OrderID, "status", "failed", "error", err)
		return fmt.Errorf("update payment status to failed: %w", err)
	}

	payload, err := json.Marshal(map[string]string{
		"event":    "payment.failed",
		"order_id": payment.OrderID.String(),
	})
	if err != nil {
		return fmt.Errorf("marshal payment failed outbox payload: %w", err)
	}

	if err := s.outboxRepo.Insert(ctx, nil, "ecommerce.saga", "saga.order.events", payload); err != nil {
		return fmt.Errorf("insert payment failed outbox message: %w", err)
	}

	slog.InfoContext(ctx, "payment failed", "orderID", payment.OrderID, "intentID", intentID)
	return nil
}
```

Add the `orderIDFromMetadata` helper at the bottom of `webhook.go`, above the closing of the file:

```go
// orderIDFromMetadata extracts and parses the order_id UUID from PaymentIntent metadata.
func orderIDFromMetadata(metadata map[string]string) (uuid.UUID, error) {
	raw, ok := metadata["order_id"]
	if !ok || raw == "" {
		return uuid.Nil, fmt.Errorf("order_id not found in payment intent metadata")
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, fmt.Errorf("invalid order_id in metadata: %w", err)
	}
	return id, nil
}
```

Add `"github.com/google/uuid"` to the imports in `webhook.go`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd go/payment-service && go test ./internal/service/ -run TestWebhookService -v`
Expected: PASS — all three tests pass

- [ ] **Step 5: Commit**

```bash
git add go/payment-service/internal/service/webhook.go go/payment-service/internal/service/webhook_test.go
git commit -m "fix(payment): use order_id from metadata for webhook payment lookup"
```

---

### Task 4: Add outbox poller startup resilience

**Files:**
- Modify: `go/payment-service/internal/outbox/poller.go`
- Modify: `go/payment-service/internal/outbox/poller_test.go`
- Modify: `go/payment-service/internal/repository/outbox.go`

- [ ] **Step 1: Write test for waitForDB**

```go
// In poller_test.go — replace entire file

package outbox

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/kabradshaw1/portfolio/go/payment-service/internal/model"
)

type mockFetcher struct {
	pingErr   error
	pingCalls int
}

func (m *mockFetcher) FetchUnpublished(_ context.Context, _ int) ([]model.OutboxMessage, error) {
	return nil, nil
}

func (m *mockFetcher) MarkPublished(_ context.Context, _ uuid.UUID) error {
	return nil
}

func (m *mockFetcher) Ping(ctx context.Context) error {
	m.pingCalls++
	return m.pingErr
}

func TestNewPoller(t *testing.T) {
	p := NewPoller(nil, (*amqp.Channel)(nil), time.Second, 10)
	if p == nil {
		t.Fatal("expected non-nil Poller")
	}
}

func TestWaitForDB_SuccessFirstAttempt(t *testing.T) {
	f := &mockFetcher{}
	p := NewPoller(f, (*amqp.Channel)(nil), time.Second, 10)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	p.waitForDB(ctx)

	if f.pingCalls != 1 {
		t.Errorf("expected 1 ping call, got %d", f.pingCalls)
	}
}

func TestWaitForDB_ContextCancelled(t *testing.T) {
	f := &mockFetcher{pingErr: errors.New("connection refused")}
	p := NewPoller(f, (*amqp.Channel)(nil), time.Second, 10)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	p.waitForDB(ctx)

	if f.pingCalls < 1 {
		t.Errorf("expected at least 1 ping call, got %d", f.pingCalls)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd go/payment-service && go test ./internal/outbox/ -run TestWaitForDB -v`
Expected: FAIL — `Ping` method doesn't exist on `OutboxFetcher`, `waitForDB` not defined

- [ ] **Step 3: Add Ping method to OutboxFetcher interface and repository**

Update the interface in `poller.go`:

```go
// OutboxFetcher reads unpublished outbox messages and marks them published.
type OutboxFetcher interface {
	FetchUnpublished(ctx context.Context, limit int) ([]model.OutboxMessage, error)
	MarkPublished(ctx context.Context, id uuid.UUID) error
	Ping(ctx context.Context) error
}
```

Add the `Ping` method to `repository/outbox.go`:

```go
// Ping checks whether the database connection is alive.
func (r *OutboxRepository) Ping(ctx context.Context) error {
	return r.pool.Ping(ctx)
}
```

- [ ] **Step 4: Add waitForDB to poller.go**

Add `"math"` to the imports in `poller.go` and add the method:

```go
const (
	waitForDBInitialBackoff = 1 * time.Second
	waitForDBMaxBackoff     = 30 * time.Second
)

// waitForDB blocks until the database is reachable or the context is cancelled.
// Uses exponential backoff starting at 1s, capped at 30s.
func (p *Poller) waitForDB(ctx context.Context) {
	backoff := waitForDBInitialBackoff
	for {
		if err := p.fetcher.Ping(ctx); err == nil {
			slog.InfoContext(ctx, "outbox poller: database ready")
			return
		}

		slog.WarnContext(ctx, "outbox poller: database not ready, retrying", "backoff", backoff)

		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}

		backoff = time.Duration(math.Min(float64(backoff*2), float64(waitForDBMaxBackoff)))
	}
}
```

Update the `Run` method to call `waitForDB` first:

```go
// Run starts the polling loop. It waits for the database to be ready, then
// ticks at the configured interval and calls poll on each tick.
// It stops when the context is cancelled.
func (p *Poller) Run(ctx context.Context) {
	p.waitForDB(ctx)

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
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd go/payment-service && go test ./internal/outbox/ -v`
Expected: PASS — all tests pass

- [ ] **Step 6: Commit**

```bash
git add go/payment-service/internal/outbox/poller.go go/payment-service/internal/outbox/poller_test.go go/payment-service/internal/repository/outbox.go
git commit -m "fix(payment): add startup readiness wait to outbox poller"
```

---

### Task 5: Run full preflight and fix any issues

**Files:**
- All modified files from Tasks 1-4

- [ ] **Step 1: Run go vet and lint**

Run: `cd go && golangci-lint run ./payment-service/...`
Expected: PASS — no lint errors

- [ ] **Step 2: Run all payment-service tests**

Run: `cd go/payment-service && go test -race ./...`
Expected: PASS

- [ ] **Step 3: Run make preflight-go**

Run: `make preflight-go`
Expected: PASS — all Go services pass lint + tests

- [ ] **Step 4: Commit any lint fixes if needed**

```bash
git add -A go/payment-service/
git commit -m "fix(payment): lint fixes for webhook metadata changes"
```

---

### Task 6: Manual DB cleanup for stuck orders (post-deploy)

This task is run manually after the fix is deployed to production.

- [ ] **Step 1: Clean up stuck PAYMENT_CREATED orders**

```bash
ssh debian "kubectl exec -n java-tasks deploy/postgres -- psql -U taskuser -d ecommercedb -c \"UPDATE orders SET status = 'failed', saga_step = 'COMPENSATION_COMPLETE' WHERE status = 'pending' AND saga_step = 'PAYMENT_CREATED';\""
```

Expected: `UPDATE 2` (orders f5cd888c and db98de34)

- [ ] **Step 2: Clean up stuck COMPENSATING orders**

```bash
ssh debian "kubectl exec -n java-tasks deploy/postgres -- psql -U taskuser -d ecommercedb -c \"UPDATE orders SET status = 'failed', saga_step = 'COMPENSATION_COMPLETE' WHERE status = 'failed' AND saga_step = 'COMPENSATING';\""
```

Expected: `UPDATE 3` (the 3 remaining COMPENSATING orders)

- [ ] **Step 3: Verify cleanup**

```bash
ssh debian "kubectl exec -n java-tasks deploy/postgres -- psql -U taskuser -d ecommercedb -c \"SELECT id, status, saga_step FROM orders ORDER BY created_at DESC LIMIT 10;\""
```

Expected: All orders show either `completed`/`COMPLETED` or `failed`/`COMPENSATION_COMPLETE`. No `pending`/`PAYMENT_CREATED` or `failed`/`COMPENSATING` rows remain.
