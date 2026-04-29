# AI Service Depth — Phase 1 v1.0 Backend Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Land the backend depth changes from `2026-04-28-ai-service-depth-design.md` — three composite tools, seven MCP Resources, three server-provided Prompts — inside `go/ai-service/`, so stdio MCP clients (Claude Code) and the future web chat see a non-shallow protocol surface.

**Architecture:** All work lives in `go/ai-service/`. Composite tools live in `internal/tools/composite/` and register through the existing `tools.Registry`. Resources and Prompts live in new packages (`internal/mcp/resources/`, `internal/mcp/prompts/`) and wire into the existing `internal/mcp/server.go` via `resources/list`, `resources/read`, `prompts/list`, `prompts/get` handlers. Cross-source fan-out uses `golang.org/x/sync/errgroup` with circuit breakers from `go/pkg/resilience`. Tracing follows the existing `go/pkg/tracing` pattern.

**Tech Stack:** Go 1.22+, `github.com/modelcontextprotocol/go-sdk`, `golang.org/x/sync/errgroup`, `pgx/v5`, `redis/go-redis/v9`, `prometheus/client_golang`, `go.opentelemetry.io/otel`, existing `go/pkg/{apperror,resilience,tracing}`.

**Out of scope for this plan:**
- Frontend chat panel changes (separate plan after this lands)
- Sampling and approval-gated writes (v1.1 plan)
- Phase 2 ops/SRE MCP server

---

## Pre-flight

Branch and worktree setup before any code work.

- [ ] **Step 0.1: Update spec marker and create feature branch worktree**

Run from `/Users/kylebradshaw/repos/gen_ai_engineer`:

```bash
echo "ai-service-depth-design" > ~/.claude/current-spec.txt
git fetch origin main
git worktree add .claude/worktrees/agent-feat-ai-service-depth-v1-backend -b agent/feat-ai-service-depth-v1-backend origin/main
cd .claude/worktrees/agent-feat-ai-service-depth-v1-backend
```

All subsequent paths are relative to the worktree root unless absolute.

- [ ] **Step 0.2: Verify Go toolchain**

Run: `cd go/ai-service && go version && go build ./...`
Expected: Go 1.22+ reported, build succeeds with no errors.

---

## File Structure

```
go/ai-service/
├── cmd/server/
│   ├── main.go                                  (modify: register new components)
│   ├── routes.go                                (no change)
│   └── config.go                                (modify: new env vars)
├── internal/
│   ├── mcp/
│   │   ├── server.go                            (modify: wire resources + prompts)
│   │   ├── resources/
│   │   │   ├── registry.go                      (new: Resource interface + Registry)
│   │   │   ├── registry_test.go                 (new)
│   │   │   ├── catalog.go                       (new: catalog:// handlers)
│   │   │   ├── catalog_test.go                  (new)
│   │   │   ├── user.go                          (new: user:// handlers, JWT-scoped)
│   │   │   ├── user_test.go                     (new)
│   │   │   ├── runbook.go                       (new: runbook:// loader)
│   │   │   ├── runbook_test.go                  (new)
│   │   │   ├── schema.go                        (new: schema:// loader)
│   │   │   └── schema_test.go                   (new)
│   │   ├── prompts/
│   │   │   ├── registry.go                      (new: Prompt interface + Registry)
│   │   │   ├── registry_test.go                 (new)
│   │   │   ├── explain_order.go                 (new)
│   │   │   ├── explain_order_test.go            (new)
│   │   │   ├── compare_recommend.go             (new)
│   │   │   ├── compare_recommend_test.go        (new)
│   │   │   ├── portfolio_tour.go                (new)
│   │   │   └── portfolio_tour_test.go           (new)
│   │   └── adapters_test.go                     (new: end-to-end MCP handler tests)
│   ├── tools/
│   │   ├── registry.go                          (no change)
│   │   └── composite/
│   │       ├── investigate_order.go             (new)
│   │       ├── investigate_order_test.go        (new)
│   │       ├── investigate_order_evidence.go    (new: per-source fetchers)
│   │       ├── investigate_order_evidence_test.go (new)
│   │       ├── compare_products.go              (new)
│   │       ├── compare_products_test.go         (new)
│   │       ├── recommend_rationale.go           (new)
│   │       └── recommend_rationale_test.go      (new)
│   └── metrics/
│       └── metrics.go                           (modify: new counters/histograms)
├── resources/
│   ├── runbook.md                               (new: portfolio runbook content)
│   └── schema-ecommerce.md                      (new: sanitized ER summary)
└── k8s/ (under repo root k8s/ai-services/)
    ├── ai-service-configmap.yml                 (modify: new env vars)
    └── ai-service-deployment.yml                (modify: mount resources/ volume)
```

Each composite tool fronts a verdict-shaped struct and a fan-out fetcher. Each Resource is an isolated handler with its own tests. Each Prompt renders an MCP `GetPromptResult`. Adapters wiring these into `internal/mcp/server.go` get exercised in one end-to-end test file (`adapters_test.go`) so the protocol layer is covered without retesting business logic per Resource/Prompt.

---

## Group A: Composite tool — `investigate_my_order`

The hardest tool first, because its evidence-fetcher pattern (`golang.org/x/sync/errgroup` parallel fan-out + circuit breakers + partial-evidence fallback) is reused by the other composites.

### Task A1: Define verdict struct and serialization tests

**Files:**
- Create: `go/ai-service/internal/tools/composite/investigate_order.go`
- Create: `go/ai-service/internal/tools/composite/investigate_order_test.go`

- [ ] **Step A1.1: Write the failing test**

```go
// investigate_order_test.go
package composite

import (
	"encoding/json"
	"testing"
)

func TestVerdictMarshalsToExpectedShape(t *testing.T) {
	v := Verdict{
		Stage:           "payment_captured",
		Status:          "ok",
		CustomerMessage: "Your payment cleared.",
		TechnicalDetail: "saga step=payment_captured",
		NextAction:      "wait",
		Evidence: Evidence{
			TraceID:   "abc123",
			SagaStep:  "payment_captured",
			Partial:   false,
			LastLogs:  []string{"info: charge ok"},
		},
	}
	out, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := string(out)
	want := `{"stage":"payment_captured","status":"ok","customer_message":"Your payment cleared.","technical_detail":"saga step=payment_captured","next_action":"wait","evidence":{"trace_id":"abc123","saga_step":"payment_captured","partial_evidence":false,"last_logs":["info: charge ok"]}}`
	if got != want {
		t.Fatalf("marshal mismatch:\n got=%s\nwant=%s", got, want)
	}
}
```

- [ ] **Step A1.2: Run test to verify it fails**

Run: `cd go/ai-service && go test ./internal/tools/composite/ -run TestVerdictMarshalsToExpectedShape -v`
Expected: FAIL — `Verdict` and `Evidence` types undefined.

- [ ] **Step A1.3: Implement the structs**

```go
// investigate_order.go
package composite

// Verdict is the structured output of investigate_my_order.
type Verdict struct {
	Stage           string   `json:"stage"`
	Status          string   `json:"status"`
	CustomerMessage string   `json:"customer_message"`
	TechnicalDetail string   `json:"technical_detail"`
	NextAction      string   `json:"next_action"`
	Evidence        Evidence `json:"evidence"`
}

// Evidence records the data sources consulted to produce the verdict.
type Evidence struct {
	TraceID  string   `json:"trace_id"`
	SagaStep string   `json:"saga_step"`
	Partial  bool     `json:"partial_evidence"`
	LastLogs []string `json:"last_logs"`
}
```

- [ ] **Step A1.4: Run test to verify it passes**

Run: `cd go/ai-service && go test ./internal/tools/composite/ -run TestVerdictMarshalsToExpectedShape -v`
Expected: PASS.

- [ ] **Step A1.5: Commit**

```bash
git add go/ai-service/internal/tools/composite/investigate_order.go go/ai-service/internal/tools/composite/investigate_order_test.go
git commit -m "feat(ai-service): add Verdict struct for investigate_my_order"
```

### Task A2: Define evidence-source interfaces and fan-out fetcher

**Files:**
- Create: `go/ai-service/internal/tools/composite/investigate_order_evidence.go`
- Create: `go/ai-service/internal/tools/composite/investigate_order_evidence_test.go`

- [ ] **Step A2.1: Write failing test for fan-out completeness**

```go
// investigate_order_evidence_test.go
package composite

import (
	"context"
	"errors"
	"testing"
)

type fakeOrderSource struct{ data OrderRecord; err error }
func (f fakeOrderSource) FetchOrder(ctx context.Context, id string) (OrderRecord, error) { return f.data, f.err }

type fakeSagaSource struct{ data SagaHistory; err error }
func (f fakeSagaSource) FetchSaga(ctx context.Context, id string) (SagaHistory, error) { return f.data, f.err }

type fakePaymentSource struct{ data PaymentRecord; err error }
func (f fakePaymentSource) FetchPayment(ctx context.Context, id string) (PaymentRecord, error) { return f.data, f.err }

type fakeCartSource struct{ data CartReservation; err error }
func (f fakeCartSource) FetchCartReservation(ctx context.Context, id string) (CartReservation, error) { return f.data, f.err }

type fakeRabbitSource struct{ data []RabbitEvent; err error }
func (f fakeRabbitSource) FetchEvents(ctx context.Context, correlationID string) ([]RabbitEvent, error) { return f.data, f.err }

type fakeTraceSource struct{ data TraceSummary; err error }
func (f fakeTraceSource) FetchTrace(ctx context.Context, traceID string) (TraceSummary, error) { return f.data, f.err }

type fakeLogSource struct{ data []string; err error }
func (f fakeLogSource) FetchLogs(ctx context.Context, services []string, from, to int64) ([]string, error) { return f.data, f.err }

func TestFanOutAllSourcesSucceed(t *testing.T) {
	f := EvidenceFetcher{
		Order:   fakeOrderSource{data: OrderRecord{ID: "ord1", Status: "completed", TraceID: "t1", CorrelationID: "c1", CreatedUnix: 1, UpdatedUnix: 2}},
		Saga:    fakeSagaSource{data: SagaHistory{Step: "completed"}},
		Payment: fakePaymentSource{data: PaymentRecord{StripeChargeID: "ch_1"}},
		Cart:    fakeCartSource{data: CartReservation{Released: true}},
		Rabbit:  fakeRabbitSource{data: []RabbitEvent{{Name: "OrderCompleted"}}},
		Trace:   fakeTraceSource{data: TraceSummary{ID: "t1", Spans: []SpanSummary{{Name: "checkout"}}}},
		Logs:    fakeLogSource{data: []string{"info"}},
	}
	bundle, err := f.Fetch(context.Background(), "ord1")
	if err != nil { t.Fatalf("Fetch: %v", err) }
	if bundle.Partial { t.Fatalf("expected non-partial, got partial") }
	if bundle.Order.ID != "ord1" { t.Fatalf("order id mismatch: %s", bundle.Order.ID) }
}

func TestFanOutSingleFailureMarksPartial(t *testing.T) {
	f := EvidenceFetcher{
		Order:   fakeOrderSource{data: OrderRecord{ID: "ord1", TraceID: "t1", CorrelationID: "c1"}},
		Saga:    fakeSagaSource{err: errors.New("saga down")},
		Payment: fakePaymentSource{data: PaymentRecord{StripeChargeID: "ch_1"}},
		Cart:    fakeCartSource{data: CartReservation{Released: true}},
		Rabbit:  fakeRabbitSource{data: []RabbitEvent{}},
		Trace:   fakeTraceSource{data: TraceSummary{ID: "t1"}},
		Logs:    fakeLogSource{data: []string{}},
	}
	bundle, err := f.Fetch(context.Background(), "ord1")
	if err != nil { t.Fatalf("Fetch returned error, expected partial bundle: %v", err) }
	if !bundle.Partial { t.Fatalf("expected Partial=true after saga failure") }
	if bundle.Order.ID != "ord1" { t.Fatalf("primary order data should still be present") }
}

func TestFanOutOrderMissingFailsHard(t *testing.T) {
	f := EvidenceFetcher{
		Order:   fakeOrderSource{err: errors.New("not found")},
		Saga:    fakeSagaSource{},
		Payment: fakePaymentSource{},
		Cart:    fakeCartSource{},
		Rabbit:  fakeRabbitSource{},
		Trace:   fakeTraceSource{},
		Logs:    fakeLogSource{},
	}
	_, err := f.Fetch(context.Background(), "missing")
	if err == nil { t.Fatalf("expected error when order fetch fails") }
}
```

- [ ] **Step A2.2: Run tests to verify they fail**

Run: `cd go/ai-service && go test ./internal/tools/composite/ -run TestFanOut -v`
Expected: FAIL — types and `EvidenceFetcher` undefined.

- [ ] **Step A2.3: Implement evidence types and fetcher**

```go
// investigate_order_evidence.go
package composite

import (
	"context"
	"errors"

	"golang.org/x/sync/errgroup"
)

// Source records.
type OrderRecord struct {
	ID            string
	Status        string
	TraceID       string
	CorrelationID string
	CreatedUnix   int64
	UpdatedUnix   int64
}
type SagaHistory struct {
	Step    string
	Retries int
	Events  []string
}
type PaymentRecord struct {
	StripeChargeID  string
	WebhookReceived bool
}
type CartReservation struct {
	Released   bool
	ReleasedAt int64
}
type RabbitEvent struct {
	Name      string
	Timestamp int64
}
type SpanSummary struct {
	Name        string
	DurationMs  int64
}
type TraceSummary struct {
	ID    string
	Spans []SpanSummary
}

// Source interfaces — small, single-purpose, mockable.
type OrderSource interface{ FetchOrder(ctx context.Context, id string) (OrderRecord, error) }
type SagaSource interface{ FetchSaga(ctx context.Context, id string) (SagaHistory, error) }
type PaymentSource interface{ FetchPayment(ctx context.Context, id string) (PaymentRecord, error) }
type CartSource interface{ FetchCartReservation(ctx context.Context, id string) (CartReservation, error) }
type RabbitSource interface{ FetchEvents(ctx context.Context, correlationID string) ([]RabbitEvent, error) }
type TraceSource interface{ FetchTrace(ctx context.Context, traceID string) (TraceSummary, error) }
type LogSource interface{ FetchLogs(ctx context.Context, services []string, from, to int64) ([]string, error) }

// EvidenceBundle is the cross-source artifact a verdict is computed from.
type EvidenceBundle struct {
	Order    OrderRecord
	Saga     SagaHistory
	Payment  PaymentRecord
	Cart     CartReservation
	Rabbit   []RabbitEvent
	Trace    TraceSummary
	Logs     []string
	Partial  bool   // true if any non-primary source failed
	PartialReason []string
}

// EvidenceFetcher owns all sources and fans out in parallel.
type EvidenceFetcher struct {
	Order   OrderSource
	Saga    SagaSource
	Payment PaymentSource
	Cart    CartSource
	Rabbit  RabbitSource
	Trace   TraceSource
	Logs    LogSource
}

// Fetch runs every source in parallel. The order fetch is primary —
// failure there returns hard error. All other sources are best-effort
// and contribute Partial=true on failure rather than aborting.
func (f EvidenceFetcher) Fetch(ctx context.Context, orderID string) (EvidenceBundle, error) {
	var bundle EvidenceBundle

	order, err := f.Order.FetchOrder(ctx, orderID)
	if err != nil {
		return bundle, err
	}
	bundle.Order = order

	g, gctx := errgroup.WithContext(ctx)
	var partialReasons []string
	mark := func(reason string) { partialReasons = append(partialReasons, reason); bundle.Partial = true }

	g.Go(func() error {
		v, e := f.Saga.FetchSaga(gctx, orderID)
		if e != nil { mark("saga: " + e.Error()); return nil }
		bundle.Saga = v
		return nil
	})
	g.Go(func() error {
		v, e := f.Payment.FetchPayment(gctx, orderID)
		if e != nil { mark("payment: " + e.Error()); return nil }
		bundle.Payment = v
		return nil
	})
	g.Go(func() error {
		v, e := f.Cart.FetchCartReservation(gctx, orderID)
		if e != nil { mark("cart: " + e.Error()); return nil }
		bundle.Cart = v
		return nil
	})
	g.Go(func() error {
		v, e := f.Rabbit.FetchEvents(gctx, order.CorrelationID)
		if e != nil { mark("rabbit: " + e.Error()); return nil }
		bundle.Rabbit = v
		return nil
	})
	g.Go(func() error {
		if order.TraceID == "" { return nil }
		v, e := f.Trace.FetchTrace(gctx, order.TraceID)
		if e != nil { mark("trace: " + e.Error()); return nil }
		bundle.Trace = v
		return nil
	})
	g.Go(func() error {
		v, e := f.Logs.FetchLogs(gctx, []string{"order-service", "payment-service", "cart-service"}, order.CreatedUnix, order.UpdatedUnix)
		if e != nil { mark("logs: " + e.Error()); return nil }
		bundle.Logs = v
		return nil
	})

	if err := g.Wait(); err != nil {
		return bundle, errors.New("fan-out wait: " + err.Error())
	}
	bundle.PartialReason = partialReasons
	return bundle, nil
}
```

- [ ] **Step A2.4: Run tests to verify they pass**

Run: `cd go/ai-service && go test ./internal/tools/composite/ -run TestFanOut -v`
Expected: PASS for all three.

- [ ] **Step A2.5: Commit**

```bash
git add go/ai-service/internal/tools/composite/investigate_order_evidence.go go/ai-service/internal/tools/composite/investigate_order_evidence_test.go
git commit -m "feat(ai-service): add evidence fan-out fetcher for investigate_my_order"
```

### Task A3: Verdict computation logic

- [ ] **Step A3.1: Write failing tests for verdict cases**

Append to `investigate_order_test.go`:

```go
func TestVerdictCompletedOrder(t *testing.T) {
	b := EvidenceBundle{
		Order:   OrderRecord{ID: "ord", Status: "completed", TraceID: "t"},
		Saga:    SagaHistory{Step: "completed"},
		Payment: PaymentRecord{StripeChargeID: "ch_1", WebhookReceived: true},
	}
	v := ComputeVerdict(b)
	if v.Stage != "completed" || v.Status != "ok" || v.NextAction != "none" {
		t.Fatalf("unexpected verdict: %+v", v)
	}
}

func TestVerdictPaymentCapturedWarehousePending(t *testing.T) {
	b := EvidenceBundle{
		Order:   OrderRecord{ID: "ord", Status: "processing", TraceID: "t"},
		Saga:    SagaHistory{Step: "payment_captured"},
		Payment: PaymentRecord{StripeChargeID: "ch_1", WebhookReceived: true},
	}
	v := ComputeVerdict(b)
	if v.Stage != "payment_captured" || v.NextAction != "wait" {
		t.Fatalf("unexpected verdict: %+v", v)
	}
	if v.CustomerMessage == "" { t.Fatalf("expected non-empty customer message") }
}

func TestVerdictFailedSaga(t *testing.T) {
	b := EvidenceBundle{
		Order: OrderRecord{ID: "ord", Status: "failed"},
		Saga:  SagaHistory{Step: "failed", Retries: 3},
	}
	v := ComputeVerdict(b)
	if v.Status != "failed" || v.NextAction != "contact_support" {
		t.Fatalf("unexpected verdict: %+v", v)
	}
}

func TestVerdictPartialEvidenceFlagged(t *testing.T) {
	b := EvidenceBundle{
		Order:   OrderRecord{ID: "ord", Status: "completed"},
		Partial: true,
	}
	v := ComputeVerdict(b)
	if !v.Evidence.Partial { t.Fatalf("expected verdict.Evidence.Partial=true") }
}
```

- [ ] **Step A3.2: Run tests to verify they fail**

Run: `cd go/ai-service && go test ./internal/tools/composite/ -run TestVerdict -v`
Expected: FAIL — `ComputeVerdict` undefined.

- [ ] **Step A3.3: Implement `ComputeVerdict`**

Append to `investigate_order.go`:

```go
// ComputeVerdict reduces an EvidenceBundle to a Verdict.
func ComputeVerdict(b EvidenceBundle) Verdict {
	v := Verdict{
		Evidence: Evidence{
			TraceID:  b.Order.TraceID,
			SagaStep: b.Saga.Step,
			Partial:  b.Partial,
			LastLogs: b.Logs,
		},
	}

	switch b.Order.Status {
	case "completed":
		v.Stage = "completed"
		v.Status = "ok"
		v.CustomerMessage = "Your order is complete."
		v.TechnicalDetail = "saga=" + b.Saga.Step
		v.NextAction = "none"
	case "processing":
		v.Stage = b.Saga.Step
		v.Status = "retrying"
		switch b.Saga.Step {
		case "payment_captured":
			v.CustomerMessage = "Your payment cleared and we're handing off to the warehouse."
		case "warehouse_pending":
			v.CustomerMessage = "Warehouse hasn't acknowledged yet — we're retrying."
		default:
			v.CustomerMessage = "Your order is in progress."
		}
		v.TechnicalDetail = "saga=" + b.Saga.Step
		v.NextAction = "wait"
	case "failed":
		v.Stage = "failed"
		v.Status = "failed"
		v.CustomerMessage = "Your order didn't go through. Please contact support."
		v.TechnicalDetail = "saga=" + b.Saga.Step + " retries=" + intToStr(b.Saga.Retries)
		v.NextAction = "contact_support"
	default:
		v.Stage = b.Order.Status
		v.Status = "stalled"
		v.CustomerMessage = "Your order is in an unusual state."
		v.NextAction = "contact_support"
	}
	return v
}

func intToStr(n int) string {
	if n == 0 { return "0" }
	neg := n < 0
	if neg { n = -n }
	var buf [20]byte
	i := len(buf)
	for n > 0 { i--; buf[i] = byte('0' + n%10); n /= 10 }
	if neg { i--; buf[i] = '-' }
	return string(buf[i:])
}
```

- [ ] **Step A3.4: Run tests to verify they pass**

Run: `cd go/ai-service && go test ./internal/tools/composite/ -run TestVerdict -v`
Expected: PASS for all four.

- [ ] **Step A3.5: Commit**

```bash
git add go/ai-service/internal/tools/composite/investigate_order.go go/ai-service/internal/tools/composite/investigate_order_test.go
git commit -m "feat(ai-service): compute saga verdict from evidence bundle"
```

### Task A4: Wire `investigate_my_order` as a Tool and register

- [ ] **Step A4.1: Write failing test for tool wrapper**

Append to `investigate_order_test.go`:

```go
func TestInvestigateMyOrderToolEndToEnd(t *testing.T) {
	tool := NewInvestigateMyOrderTool(EvidenceFetcher{
		Order:   fakeOrderSource{data: OrderRecord{ID: "ord1", Status: "completed", TraceID: "t1"}},
		Saga:    fakeSagaSource{data: SagaHistory{Step: "completed"}},
		Payment: fakePaymentSource{data: PaymentRecord{StripeChargeID: "ch", WebhookReceived: true}},
		Cart:    fakeCartSource{}, Rabbit: fakeRabbitSource{}, Trace: fakeTraceSource{}, Logs: fakeLogSource{},
	})
	if tool.Name() != "investigate_my_order" {
		t.Fatalf("name: %s", tool.Name())
	}
	out, err := tool.Call(context.Background(), []byte(`{"order_id":"ord1"}`))
	if err != nil { t.Fatalf("Call: %v", err) }
	var v Verdict
	if e := json.Unmarshal(out, &v); e != nil { t.Fatalf("unmarshal: %v", e) }
	if v.Stage != "completed" { t.Fatalf("stage: %s", v.Stage) }
}

func TestInvestigateMyOrderRejectsMissingOrderID(t *testing.T) {
	tool := NewInvestigateMyOrderTool(EvidenceFetcher{})
	_, err := tool.Call(context.Background(), []byte(`{}`))
	if err == nil { t.Fatalf("expected error on missing order_id") }
}
```

- [ ] **Step A4.2: Run tests to verify they fail**

Run: `cd go/ai-service && go test ./internal/tools/composite/ -run TestInvestigateMyOrder -v`
Expected: FAIL — `NewInvestigateMyOrderTool` undefined.

- [ ] **Step A4.3: Implement the tool wrapper**

Append to `investigate_order.go`:

```go
import (
	"context"
	"encoding/json"
	"errors"
)

// Tool interface match: see internal/tools.Tool — Name, Description, Schema, Call.

type investigateMyOrderTool struct {
	fetcher EvidenceFetcher
}

func NewInvestigateMyOrderTool(f EvidenceFetcher) *investigateMyOrderTool {
	return &investigateMyOrderTool{fetcher: f}
}

func (t *investigateMyOrderTool) Name() string {
	return "investigate_my_order"
}

func (t *investigateMyOrderTool) Description() string {
	return "Investigates the full checkout saga for a given order, correlating order, payment, cart reservation, RabbitMQ events, trace, and logs into a structured verdict with a customer-facing message."
}

func (t *investigateMyOrderTool) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"order_id": { "type": "string", "description": "The order id to investigate." }
		},
		"required": ["order_id"]
	}`)
}

func (t *investigateMyOrderTool) Call(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	var req struct {
		OrderID string `json:"order_id"`
	}
	if err := json.Unmarshal(args, &req); err != nil {
		return nil, err
	}
	if req.OrderID == "" {
		return nil, errors.New("order_id is required")
	}
	bundle, err := t.fetcher.Fetch(ctx, req.OrderID)
	if err != nil {
		return nil, err
	}
	verdict := ComputeVerdict(bundle)
	return json.Marshal(verdict)
}
```

- [ ] **Step A4.4: Run tests to verify they pass**

Run: `cd go/ai-service && go test ./internal/tools/composite/ -run TestInvestigateMyOrder -v`
Expected: PASS.

- [ ] **Step A4.5: Commit**

```bash
git add go/ai-service/internal/tools/composite/investigate_order.go go/ai-service/internal/tools/composite/investigate_order_test.go
git commit -m "feat(ai-service): wire investigate_my_order as a Tool"
```

### Task A5: Production source adapters

The fakes prove the algebra. Now wire real Postgres / RabbitMQ / Jaeger / Loki adapters. Each adapter is a thin source implementation; tests use the existing patterns in `go/ai-service/internal/tools/clients/`.

- [ ] **Step A5.1: Add Postgres-backed `OrderSource` adapter**

Inspect existing client patterns:

```bash
ls go/ai-service/internal/tools/clients/
```

Create `go/ai-service/internal/tools/composite/sources_postgres.go`:

```go
package composite

import (
	"context"
	"database/sql"
)

type PostgresOrderSource struct{ DB *sql.DB }

func (p PostgresOrderSource) FetchOrder(ctx context.Context, id string) (OrderRecord, error) {
	var r OrderRecord
	err := p.DB.QueryRowContext(ctx, `
		SELECT id, status, COALESCE(trace_id, ''), COALESCE(correlation_id, ''),
		       EXTRACT(EPOCH FROM created_at)::bigint, EXTRACT(EPOCH FROM updated_at)::bigint
		FROM orders WHERE id = $1`, id,
	).Scan(&r.ID, &r.Status, &r.TraceID, &r.CorrelationID, &r.CreatedUnix, &r.UpdatedUnix)
	return r, err
}

type PostgresSagaSource struct{ DB *sql.DB }

func (p PostgresSagaSource) FetchSaga(ctx context.Context, id string) (SagaHistory, error) {
	var h SagaHistory
	err := p.DB.QueryRowContext(ctx, `
		SELECT current_step, retry_count
		FROM saga_state WHERE order_id = $1`, id,
	).Scan(&h.Step, &h.Retries)
	if err == sql.ErrNoRows { return h, nil }
	return h, err
}

type PostgresPaymentSource struct{ DB *sql.DB }

func (p PostgresPaymentSource) FetchPayment(ctx context.Context, id string) (PaymentRecord, error) {
	var r PaymentRecord
	err := p.DB.QueryRowContext(ctx, `
		SELECT COALESCE(stripe_charge_id, ''), COALESCE(webhook_received, false)
		FROM payment_outbox WHERE order_id = $1
		ORDER BY created_at DESC LIMIT 1`, id,
	).Scan(&r.StripeChargeID, &r.WebhookReceived)
	if err == sql.ErrNoRows { return r, nil }
	return r, err
}

type PostgresCartSource struct{ DB *sql.DB }

func (p PostgresCartSource) FetchCartReservation(ctx context.Context, id string) (CartReservation, error) {
	var r CartReservation
	err := p.DB.QueryRowContext(ctx, `
		SELECT released, COALESCE(EXTRACT(EPOCH FROM released_at)::bigint, 0)
		FROM cart_reservations WHERE order_id = $1`, id,
	).Scan(&r.Released, &r.ReleasedAt)
	if err == sql.ErrNoRows { return r, nil }
	return r, err
}
```

Note: column names assume the existing schemas. Verify against migrations before coding:

```bash
ls go/order-service/migrations/ go/payment-service/migrations/ go/cart-service/migrations/
```

If column names differ, adjust queries to match.

- [ ] **Step A5.2: Add adapter test that uses sqlmock**

Create `go/ai-service/internal/tools/composite/sources_postgres_test.go`:

```go
package composite

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestPostgresOrderSourceFetch(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil { t.Fatal(err) }
	defer db.Close()

	rows := sqlmock.NewRows([]string{"id", "status", "trace_id", "correlation_id", "created", "updated"}).
		AddRow("ord1", "completed", "t1", "c1", int64(100), int64(200))
	mock.ExpectQuery("SELECT id, status").WithArgs("ord1").WillReturnRows(rows)

	src := PostgresOrderSource{DB: db}
	got, err := src.FetchOrder(context.Background(), "ord1")
	if err != nil { t.Fatalf("FetchOrder: %v", err) }
	if got.ID != "ord1" || got.Status != "completed" || got.TraceID != "t1" {
		t.Fatalf("unexpected: %+v", got)
	}
}
```

Add the dependency:

```bash
cd go/ai-service && go get github.com/DATA-DOG/go-sqlmock@latest && go mod tidy
```

- [ ] **Step A5.3: Run the test**

Run: `cd go/ai-service && go test ./internal/tools/composite/ -run TestPostgresOrderSourceFetch -v`
Expected: PASS.

- [ ] **Step A5.4: Add RabbitMQ, Jaeger, and Loki source adapters**

Create `go/ai-service/internal/tools/composite/sources_observability.go`:

```go
package composite

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// JaegerTraceSource reads a single trace by id from Jaeger HTTP API.
type JaegerTraceSource struct {
	BaseURL string // e.g. http://jaeger-query.monitoring.svc.cluster.local:16686
	HTTP    *http.Client
}

func (j JaegerTraceSource) FetchTrace(ctx context.Context, id string) (TraceSummary, error) {
	if id == "" { return TraceSummary{}, nil }
	req, _ := http.NewRequestWithContext(ctx, "GET", j.BaseURL+"/api/traces/"+id, nil)
	resp, err := j.HTTP.Do(req)
	if err != nil { return TraceSummary{}, err }
	defer resp.Body.Close()
	if resp.StatusCode != 200 { return TraceSummary{}, fmt.Errorf("jaeger: %d", resp.StatusCode) }
	var body struct {
		Data []struct {
			Spans []struct {
				OperationName string `json:"operationName"`
				Duration      int64  `json:"duration"`
			} `json:"spans"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil { return TraceSummary{}, err }
	out := TraceSummary{ID: id}
	if len(body.Data) > 0 {
		for _, s := range body.Data[0].Spans {
			out.Spans = append(out.Spans, SpanSummary{Name: s.OperationName, DurationMs: s.Duration / 1000})
		}
	}
	return out, nil
}

// LokiLogSource queries Loki for log lines from a service window.
type LokiLogSource struct {
	BaseURL string // e.g. http://loki.monitoring.svc.cluster.local:3100
	HTTP    *http.Client
}

func (l LokiLogSource) FetchLogs(ctx context.Context, services []string, fromUnix, toUnix int64) ([]string, error) {
	if len(services) == 0 { return nil, nil }
	q := `{service=~"` + servicesRegex(services) + `"} |~ "(?i)(error|warn|exception)"`
	u := l.BaseURL + "/loki/api/v1/query_range?" + url.Values{
		"query": {q},
		"start": {strconv.FormatInt(fromUnix*1_000_000_000, 10)},
		"end":   {strconv.FormatInt(toUnix*1_000_000_000, 10)},
		"limit": {"50"},
	}.Encode()
	req, _ := http.NewRequestWithContext(ctx, "GET", u, nil)
	resp, err := l.HTTP.Do(req)
	if err != nil { return nil, err }
	defer resp.Body.Close()
	if resp.StatusCode != 200 { return nil, fmt.Errorf("loki: %d", resp.StatusCode) }
	var body struct {
		Data struct {
			Result []struct {
				Values [][2]string `json:"values"`
			} `json:"result"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil { return nil, err }
	var lines []string
	for _, r := range body.Data.Result {
		for _, v := range r.Values {
			lines = append(lines, v[1])
		}
	}
	return lines, nil
}

func servicesRegex(s []string) string {
	out := ""
	for i, v := range s {
		if i > 0 { out += "|" }
		out += v
	}
	return out
}

// NopRabbitSource is a placeholder. RabbitMQ event history is read from
// a `rabbit_events` audit table written by ecommerce-service consumers.
// A persistent table is the only reliable source — RabbitMQ itself does
// not retain consumed messages.
type RabbitAuditSource struct {
	DB *http.Client // intentionally unused — replaced with sql.DB below in real wiring
}

// PostgresRabbitSource reads RabbitMQ audit rows.
type PostgresRabbitSource struct{ DB *sql.DB }

func (p PostgresRabbitSource) FetchEvents(ctx context.Context, correlationID string) ([]RabbitEvent, error) {
	if correlationID == "" { return nil, nil }
	rows, err := p.DB.QueryContext(ctx, `
		SELECT event_name, EXTRACT(EPOCH FROM emitted_at)::bigint
		FROM rabbit_events WHERE correlation_id = $1
		ORDER BY emitted_at ASC`, correlationID)
	if err != nil { return nil, err }
	defer rows.Close()
	var out []RabbitEvent
	for rows.Next() {
		var e RabbitEvent
		if err := rows.Scan(&e.Name, &e.Timestamp); err != nil { return nil, err }
		out = append(out, e)
	}
	return out, rows.Err()
}
```

Add the missing import for `sql` at the top:

```go
import (
	"database/sql"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

var _ = time.Now // keep import
```

If `rabbit_events` does not yet exist in `ecommercedb`, add a migration in `go/ecommerce-service/migrations/` named `NNN_add_rabbit_audit.up.sql` matching the up/down convention:

```sql
-- NNN_add_rabbit_audit.up.sql
CREATE TABLE IF NOT EXISTS rabbit_events (
    id BIGSERIAL PRIMARY KEY,
    correlation_id TEXT NOT NULL,
    event_name TEXT NOT NULL,
    payload JSONB,
    emitted_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS rabbit_events_correlation_idx ON rabbit_events(correlation_id);
```

Down:

```sql
-- NNN_add_rabbit_audit.down.sql
DROP TABLE IF EXISTS rabbit_events;
```

The ecommerce-service consumers must insert into `rabbit_events` on every consume — that wiring is its own follow-up commit (see Step A5.6).

- [ ] **Step A5.5: Adapter unit tests**

Create `go/ai-service/internal/tools/composite/sources_observability_test.go` with httptest servers returning canned Jaeger/Loki JSON; assert decoding produces the expected structs. Pattern:

```go
func TestJaegerTraceSourceDecodes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"data":[{"spans":[{"operationName":"checkout","duration":1234000}]}]}`)
	}))
	defer srv.Close()
	src := JaegerTraceSource{BaseURL: srv.URL, HTTP: srv.Client()}
	got, err := src.FetchTrace(context.Background(), "abc")
	if err != nil { t.Fatal(err) }
	if len(got.Spans) != 1 || got.Spans[0].Name != "checkout" || got.Spans[0].DurationMs != 1234 {
		t.Fatalf("unexpected: %+v", got)
	}
}
```

Add an analogous test for `LokiLogSource`.

Run: `cd go/ai-service && go test ./internal/tools/composite/ -run "TestJaeger|TestLoki" -v`
Expected: PASS.

- [ ] **Step A5.6: Hook ecommerce-service consumers to insert into `rabbit_events`**

Find existing RabbitMQ consumer wiring:

```bash
grep -rn "amqp.Consume\|streadway/amqp\|rabbitmq/amqp091-go" go/ecommerce-service/ go/order-service/ go/payment-service/ go/cart-service/
```

For each consumer found, add an `INSERT INTO rabbit_events` immediately after successful message handling, propagating the `correlation_id` from the message headers. Keep the insert in the same transaction as the business logic so partial-success cases produce no audit row. Test with the existing RabbitMQ test patterns.

(This step is intentionally narrative because the precise consumer files depend on the current ecommerce-service shape; the engineer follows the existing pattern.)

- [ ] **Step A5.7: Commit**

```bash
git add go/ai-service/internal/tools/composite/sources_postgres.go \
        go/ai-service/internal/tools/composite/sources_postgres_test.go \
        go/ai-service/internal/tools/composite/sources_observability.go \
        go/ai-service/internal/tools/composite/sources_observability_test.go \
        go/ecommerce-service/migrations/ \
        go/ecommerce-service/internal/  # consumer changes
git commit -m "feat(ai-service): production sources for investigate_my_order"
```

### Task A6: Register `investigate_my_order` in `main.go`

- [ ] **Step A6.1: Inspect current registration site**

Run: `grep -n "registry\|reg\.Register\|tools.Registry" go/ai-service/cmd/server/main.go`
Note the line where existing tools are registered.

- [ ] **Step A6.2: Add construction and registration**

Modify `go/ai-service/cmd/server/main.go`. After the existing registry construction, add:

```go
// In main.go imports:
import (
	"github.com/kabradshaw1/portfolio/go/ai-service/internal/tools/composite"
	"net/http"
	"time"
)

// In main(), after existing tools are registered:
investigateFetcher := composite.EvidenceFetcher{
	Order:   composite.PostgresOrderSource{DB: ecommerceDB},
	Saga:    composite.PostgresSagaSource{DB: ecommerceDB},
	Payment: composite.PostgresPaymentSource{DB: paymentDB},
	Cart:    composite.PostgresCartSource{DB: cartDB},
	Rabbit:  composite.PostgresRabbitSource{DB: ecommerceDB},
	Trace:   composite.JaegerTraceSource{
		BaseURL: cfg.JaegerQueryURL,
		HTTP:    &http.Client{Timeout: 5 * time.Second},
	},
	Logs:    composite.LokiLogSource{
		BaseURL: cfg.LokiURL,
		HTTP:    &http.Client{Timeout: 5 * time.Second},
	},
}
reg.Register(composite.NewInvestigateMyOrderTool(investigateFetcher))
```

Add the `JaegerQueryURL` and `LokiURL` fields to `cmd/server/config.go`:

```go
JaegerQueryURL string `env:"JAEGER_QUERY_URL" envDefault:"http://jaeger-query.monitoring.svc.cluster.local:16686"`
LokiURL        string `env:"LOKI_URL" envDefault:"http://loki.monitoring.svc.cluster.local:3100"`
```

Database handles for `paymentDB` and `cartDB` may not yet exist in main.go — add their `pgxpool` (or `sql.DB` conversion) construction next to the existing ecommerce DB connection. Use the existing `DATABASE_URL_*` env var convention; add `PAYMENT_DATABASE_URL` and `CART_DATABASE_URL` to config and ConfigMap if needed.

- [ ] **Step A6.3: Build and run unit tests**

Run: `cd go/ai-service && go build ./... && go test ./...`
Expected: builds clean; existing tests still pass; new composite tests pass.

- [ ] **Step A6.4: Commit**

```bash
git add go/ai-service/cmd/server/main.go go/ai-service/cmd/server/config.go
git commit -m "feat(ai-service): register investigate_my_order tool in main"
```

---

## Group B: Composite tool — `compare_products`

Smaller. One package file, one test file.

### Task B1: Define output struct and tool

**Files:**
- Create: `go/ai-service/internal/tools/composite/compare_products.go`
- Create: `go/ai-service/internal/tools/composite/compare_products_test.go`

- [ ] **Step B1.1: Write failing test**

```go
// compare_products_test.go
package composite

import (
	"context"
	"encoding/json"
	"testing"
)

type fakeProductCatalog struct{ items map[string]Product }
func (f fakeProductCatalog) GetProduct(ctx context.Context, id string) (Product, error) {
	p, ok := f.items[id]
	if !ok { return Product{}, errProductNotFound }
	return p, nil
}

type fakeEmbeddings struct{ vecs map[string][]float32 }
func (f fakeEmbeddings) Embedding(ctx context.Context, productID string) ([]float32, error) {
	v, ok := f.vecs[productID]
	if !ok { return nil, errEmbeddingMissing }
	return v, nil
}

func TestCompareProductsTwoItems(t *testing.T) {
	cat := fakeProductCatalog{items: map[string]Product{
		"a": {ID: "a", Name: "Trail Shoe", Category: "footwear", PriceCents: 12000},
		"b": {ID: "b", Name: "Road Shoe", Category: "footwear", PriceCents: 9000},
	}}
	emb := fakeEmbeddings{vecs: map[string][]float32{
		"a": {1, 0, 0},
		"b": {0.9, 0.1, 0},
	}}
	tool := NewCompareProductsTool(cat, emb)
	out, err := tool.Call(context.Background(), []byte(`{"product_ids":["a","b"]}`))
	if err != nil { t.Fatal(err) }

	var r CompareResult
	if err := json.Unmarshal(out, &r); err != nil { t.Fatal(err) }
	if len(r.Products) != 2 { t.Fatalf("want 2 products, got %d", len(r.Products)) }
	if len(r.Similarity) != 1 { t.Fatalf("want 1 similarity entry, got %d", len(r.Similarity)) }
	if r.Similarity[0].Score < 0.9 { t.Fatalf("expected high similarity, got %f", r.Similarity[0].Score) }
	if _, ok := r.Shared["category"]; !ok { t.Fatalf("expected shared category") }
	foundPrice := false
	for _, d := range r.Differing { if d.Field == "price_cents" { foundPrice = true } }
	if !foundPrice { t.Fatalf("expected price difference") }
}

func TestCompareProductsRejectsLessThanTwo(t *testing.T) {
	tool := NewCompareProductsTool(fakeProductCatalog{}, fakeEmbeddings{})
	_, err := tool.Call(context.Background(), []byte(`{"product_ids":["a"]}`))
	if err == nil { t.Fatalf("expected error for <2 ids") }
}
```

- [ ] **Step B1.2: Run test to verify it fails**

Run: `cd go/ai-service && go test ./internal/tools/composite/ -run TestCompareProducts -v`
Expected: FAIL — types undefined.

- [ ] **Step B1.3: Implement `compare_products`**

```go
// compare_products.go
package composite

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
)

var (
	errProductNotFound  = errors.New("product not found")
	errEmbeddingMissing = errors.New("embedding missing")
)

type Product struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Category   string `json:"category"`
	PriceCents int    `json:"price_cents"`
	Stock      int    `json:"stock"`
}

type ProductCatalog interface {
	GetProduct(ctx context.Context, id string) (Product, error)
}
type EmbeddingSource interface {
	Embedding(ctx context.Context, productID string) ([]float32, error)
}

type CompareResult struct {
	Products       []Product             `json:"products"`
	Shared         map[string]string     `json:"shared_attributes"`
	Differing      []DifferingAttribute  `json:"differing_attributes"`
	Similarity     []PairSimilarity      `json:"semantic_similarity"`
	Recommendation string                `json:"recommendation"`
}

type DifferingAttribute struct {
	Field  string            `json:"field"`
	Values map[string]string `json:"values"` // product_id -> stringified value
}

type PairSimilarity struct {
	Pair  [2]string `json:"pair"`
	Score float64   `json:"score"`
}

type compareProductsTool struct {
	catalog ProductCatalog
	embed   EmbeddingSource
}

func NewCompareProductsTool(c ProductCatalog, e EmbeddingSource) *compareProductsTool {
	return &compareProductsTool{catalog: c, embed: e}
}

func (t *compareProductsTool) Name() string        { return "compare_products" }
func (t *compareProductsTool) Description() string { return "Compares two or more products structurally and semantically. Returns shared and differing attributes, pairwise embedding similarity, and a short recommendation." }
func (t *compareProductsTool) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type":"object",
		"properties":{
			"product_ids":{"type":"array","items":{"type":"string"},"minItems":2,"maxItems":5}
		},
		"required":["product_ids"]
	}`)
}

func (t *compareProductsTool) Call(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	var req struct {
		ProductIDs []string `json:"product_ids"`
	}
	if err := json.Unmarshal(args, &req); err != nil { return nil, err }
	if len(req.ProductIDs) < 2 { return nil, errors.New("at least 2 product_ids required") }
	if len(req.ProductIDs) > 5 { return nil, errors.New("at most 5 product_ids supported") }

	products := make([]Product, 0, len(req.ProductIDs))
	embeddings := make(map[string][]float32, len(req.ProductIDs))
	for _, id := range req.ProductIDs {
		p, err := t.catalog.GetProduct(ctx, id)
		if err != nil { return nil, fmt.Errorf("get product %s: %w", id, err) }
		products = append(products, p)
		v, err := t.embed.Embedding(ctx, id)
		if err == nil { embeddings[id] = v }
	}

	result := CompareResult{
		Products:   products,
		Shared:     sharedAttrs(products),
		Differing:  differingAttrs(products),
		Similarity: pairSimilarities(req.ProductIDs, embeddings),
	}
	result.Recommendation = composeRecommendation(result)
	return json.Marshal(result)
}

func sharedAttrs(ps []Product) map[string]string {
	out := map[string]string{}
	if len(ps) == 0 { return out }
	cat := ps[0].Category
	for _, p := range ps[1:] {
		if p.Category != cat { cat = "" }
	}
	if cat != "" { out["category"] = cat }
	return out
}

func differingAttrs(ps []Product) []DifferingAttribute {
	out := []DifferingAttribute{}
	priceVals := map[string]string{}
	allSame := true
	for i, p := range ps {
		priceVals[p.ID] = fmt.Sprintf("%d", p.PriceCents)
		if i > 0 && p.PriceCents != ps[0].PriceCents { allSame = false }
	}
	if !allSame {
		out = append(out, DifferingAttribute{Field: "price_cents", Values: priceVals})
	}
	nameVals := map[string]string{}
	for _, p := range ps { nameVals[p.ID] = p.Name }
	out = append(out, DifferingAttribute{Field: "name", Values: nameVals})
	return out
}

func pairSimilarities(ids []string, embs map[string][]float32) []PairSimilarity {
	out := []PairSimilarity{}
	for i := 0; i < len(ids); i++ {
		for j := i + 1; j < len(ids); j++ {
			a, ok1 := embs[ids[i]]
			b, ok2 := embs[ids[j]]
			if !ok1 || !ok2 { continue }
			out = append(out, PairSimilarity{Pair: [2]string{ids[i], ids[j]}, Score: cosineSim(a, b)})
		}
	}
	return out
}

func cosineSim(a, b []float32) float64 {
	if len(a) != len(b) { return 0 }
	var dot, na, nb float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		na += float64(a[i]) * float64(a[i])
		nb += float64(b[i]) * float64(b[i])
	}
	if na == 0 || nb == 0 { return 0 }
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}

func composeRecommendation(r CompareResult) string {
	if len(r.Products) == 0 { return "" }
	cheapest := r.Products[0]
	for _, p := range r.Products[1:] { if p.PriceCents < cheapest.PriceCents { cheapest = p } }
	return fmt.Sprintf("If price is the primary factor, %s is the lowest-cost option at $%.2f.", cheapest.Name, float64(cheapest.PriceCents)/100)
}
```

- [ ] **Step B1.4: Run test to verify it passes**

Run: `cd go/ai-service && go test ./internal/tools/composite/ -run TestCompareProducts -v`
Expected: PASS for both cases.

- [ ] **Step B1.5: Commit**

```bash
git add go/ai-service/internal/tools/composite/compare_products.go go/ai-service/internal/tools/composite/compare_products_test.go
git commit -m "feat(ai-service): add compare_products composite tool"
```

### Task B2: Production sources and registration

- [ ] **Step B2.1: Add real `ProductCatalog` and `EmbeddingSource` adapters**

Create `go/ai-service/internal/tools/composite/sources_catalog.go`:

```go
package composite

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// ProductServiceCatalog calls product-service REST :8095.
type ProductServiceCatalog struct {
	BaseURL string
	HTTP    *http.Client
}

func (p ProductServiceCatalog) GetProduct(ctx context.Context, id string) (Product, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET", p.BaseURL+"/products/"+id, nil)
	resp, err := p.HTTP.Do(req)
	if err != nil { return Product{}, err }
	defer resp.Body.Close()
	if resp.StatusCode == 404 { return Product{}, errProductNotFound }
	if resp.StatusCode != 200 { return Product{}, fmt.Errorf("product-service: %d", resp.StatusCode) }
	var raw struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Category string `json:"category"`
		Price    int    `json:"price"`
		Stock    int    `json:"stock"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil { return Product{}, err }
	return Product{ID: raw.ID, Name: raw.Name, Category: raw.Category, PriceCents: raw.Price, Stock: raw.Stock}, nil
}

// QdrantEmbeddingSource fetches a product's stored embedding from Qdrant.
type QdrantEmbeddingSource struct {
	BaseURL    string
	Collection string
	HTTP       *http.Client
}

func (q QdrantEmbeddingSource) Embedding(ctx context.Context, productID string) ([]float32, error) {
	body, _ := json.Marshal(map[string]any{
		"ids":          []string{productID},
		"with_payload": false,
		"with_vector":  true,
	})
	req, _ := http.NewRequestWithContext(ctx, "POST",
		q.BaseURL+"/collections/"+q.Collection+"/points",
		bytesReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := q.HTTP.Do(req)
	if err != nil { return nil, err }
	defer resp.Body.Close()
	if resp.StatusCode != 200 { return nil, fmt.Errorf("qdrant: %d", resp.StatusCode) }
	var out struct {
		Result []struct {
			Vector []float32 `json:"vector"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil { return nil, err }
	if len(out.Result) == 0 { return nil, errEmbeddingMissing }
	return out.Result[0].Vector, nil
}
```

Add a small helper at top of file:

```go
import "bytes"
func bytesReader(b []byte) *bytes.Reader { return bytes.NewReader(b) }
```

- [ ] **Step B2.2: httptest unit tests for both adapters**

Pattern: serve canned product-service / Qdrant JSON, assert decoded shape.

- [ ] **Step B2.3: Register in `main.go`**

Add to `cmd/server/main.go`:

```go
catalog := composite.ProductServiceCatalog{
	BaseURL: cfg.ProductServiceURL,
	HTTP:    &http.Client{Timeout: 5 * time.Second},
}
embed := composite.QdrantEmbeddingSource{
	BaseURL:    cfg.QdrantURL,
	Collection: cfg.QdrantProductCollection,
	HTTP:       &http.Client{Timeout: 5 * time.Second},
}
reg.Register(composite.NewCompareProductsTool(catalog, embed))
```

Add `ProductServiceURL`, `QdrantURL`, `QdrantProductCollection` to config.

- [ ] **Step B2.4: Build, test, commit**

```bash
cd go/ai-service && go build ./... && go test ./...
git add go/ai-service/
git commit -m "feat(ai-service): production sources and registration for compare_products"
```

---

## Group C: Composite tool — `recommend_with_rationale`

Same shape as Group B; abbreviated steps because the pattern is established.

### Task C1: Test, struct, tool

**Files:**
- Create: `go/ai-service/internal/tools/composite/recommend_rationale.go`
- Create: `go/ai-service/internal/tools/composite/recommend_rationale_test.go`

- [ ] **Step C1.1: Write failing test**

```go
// recommend_rationale_test.go
package composite

import (
	"context"
	"encoding/json"
	"testing"
)

type fakeUserHistory struct {
	orders []HistoricalItem
	cart   []HistoricalItem
	views  []HistoricalItem
}
func (f fakeUserHistory) Orders(ctx context.Context, userID string) ([]HistoricalItem, error) { return f.orders, nil }
func (f fakeUserHistory) CartItems(ctx context.Context, userID string) ([]HistoricalItem, error) { return f.cart, nil }
func (f fakeUserHistory) RecentlyViewed(ctx context.Context, userID string) ([]HistoricalItem, error) { return f.views, nil }

type fakeNeighborSearch struct{ results []NeighborResult }
func (f fakeNeighborSearch) Nearest(ctx context.Context, vec []float32, k int, excludeIDs []string, category string) ([]NeighborResult, error) {
	return f.results, nil
}

func TestRecommendRationaleProducesRationaleAndSignals(t *testing.T) {
	hist := fakeUserHistory{
		orders: []HistoricalItem{{ProductID: "shoe-trail", Embedding: []float32{1, 0, 0}, Source: "order:o1"}},
		cart:   []HistoricalItem{{ProductID: "sock", Embedding: []float32{0, 1, 0}, Source: "cart:current"}},
	}
	neigh := fakeNeighborSearch{results: []NeighborResult{
		{ProductID: "shoe-road", Score: 0.85, Name: "Road Shoe", Category: "footwear"},
		{ProductID: "shoe-trail-2", Score: 0.92, Name: "Trail Shoe v2", Category: "footwear"},
	}}
	tool := NewRecommendWithRationaleTool(hist, neigh)
	out, err := tool.Call(context.Background(), []byte(`{"user_id":"u1"}`))
	if err != nil { t.Fatal(err) }
	var r RecommendResult
	if err := json.Unmarshal(out, &r); err != nil { t.Fatal(err) }
	if len(r.Products) != 2 { t.Fatalf("want 2 products, got %d", len(r.Products)) }
	if r.Products[0].Rationale == "" { t.Fatalf("expected non-empty rationale") }
	if len(r.Products[0].SurfacedSignals) == 0 { t.Fatalf("expected signals") }
}

func TestRecommendRationaleRejectsMissingUser(t *testing.T) {
	tool := NewRecommendWithRationaleTool(fakeUserHistory{}, fakeNeighborSearch{})
	_, err := tool.Call(context.Background(), []byte(`{}`))
	if err == nil { t.Fatalf("expected error on missing user_id") }
}
```

- [ ] **Step C1.2: Run test to verify it fails**

Run: `cd go/ai-service && go test ./internal/tools/composite/ -run TestRecommendRationale -v`
Expected: FAIL — types undefined.

- [ ] **Step C1.3: Implement**

```go
// recommend_rationale.go
package composite

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

type HistoricalItem struct {
	ProductID string    `json:"product_id"`
	Embedding []float32 `json:"-"`
	Source    string    `json:"source"`
	Name      string    `json:"name"`
}

type UserHistory interface {
	Orders(ctx context.Context, userID string) ([]HistoricalItem, error)
	CartItems(ctx context.Context, userID string) ([]HistoricalItem, error)
	RecentlyViewed(ctx context.Context, userID string) ([]HistoricalItem, error)
}

type NeighborResult struct {
	ProductID string  `json:"product_id"`
	Name      string  `json:"name"`
	Category  string  `json:"category"`
	Score     float64 `json:"score"`
}

type NeighborSearch interface {
	Nearest(ctx context.Context, vec []float32, k int, excludeIDs []string, category string) ([]NeighborResult, error)
}

type RecommendResult struct {
	Products              []Recommendation `json:"products"`
	QueryEmbeddingSource  string           `json:"query_embedding_source"`
}

type Recommendation struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	Score           float64  `json:"score"`
	Rationale       string   `json:"rationale"`
	SurfacedSignals []string `json:"surfaced_signals"`
}

type recommendTool struct {
	hist  UserHistory
	neigh NeighborSearch
}

func NewRecommendWithRationaleTool(h UserHistory, n NeighborSearch) *recommendTool {
	return &recommendTool{hist: h, neigh: n}
}

func (t *recommendTool) Name() string { return "recommend_with_rationale" }
func (t *recommendTool) Description() string {
	return "Recommends products for a user by averaging embeddings of past purchases, cart items, and recently viewed products. Returns each recommendation with a plain-English rationale and the surfaced signals."
}
func (t *recommendTool) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type":"object",
		"properties":{
			"user_id":{"type":"string"},
			"category":{"type":"string"}
		},
		"required":["user_id"]
	}`)
}

func (t *recommendTool) Call(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	var req struct {
		UserID   string `json:"user_id"`
		Category string `json:"category,omitempty"`
	}
	if err := json.Unmarshal(args, &req); err != nil { return nil, err }
	if req.UserID == "" { return nil, errors.New("user_id is required") }

	orders, _ := t.hist.Orders(ctx, req.UserID)
	cart, _ := t.hist.CartItems(ctx, req.UserID)
	views, _ := t.hist.RecentlyViewed(ctx, req.UserID)

	signals := append(append(append([]HistoricalItem{}, orders...), cart...), views...)
	if len(signals) == 0 {
		return json.Marshal(RecommendResult{Products: nil, QueryEmbeddingSource: "no_history"})
	}
	avg := averageEmbedding(signals)
	if avg == nil {
		return json.Marshal(RecommendResult{Products: nil, QueryEmbeddingSource: "no_embeddings"})
	}

	exclude := make([]string, 0, len(signals))
	for _, s := range signals { exclude = append(exclude, s.ProductID) }

	results, err := t.neigh.Nearest(ctx, avg, 5, exclude, req.Category)
	if err != nil { return nil, fmt.Errorf("nearest: %w", err) }

	recs := make([]Recommendation, 0, len(results))
	for _, r := range results {
		nearestSignal := findClosestSignal(r, signals)
		recs = append(recs, Recommendation{
			ID:    r.ProductID,
			Name:  r.Name,
			Score: r.Score,
			Rationale: fmt.Sprintf(
				"Similar to %s; matches your interest in %s.",
				nearestSignal.Source, r.Category),
			SurfacedSignals: []string{nearestSignal.Source},
		})
	}

	return json.Marshal(RecommendResult{
		Products:             recs,
		QueryEmbeddingSource: fmt.Sprintf("average_of_%d_signals", len(signals)),
	})
}

func averageEmbedding(items []HistoricalItem) []float32 {
	var dim int
	for _, it := range items {
		if len(it.Embedding) > 0 { dim = len(it.Embedding); break }
	}
	if dim == 0 { return nil }
	avg := make([]float32, dim)
	count := 0
	for _, it := range items {
		if len(it.Embedding) != dim { continue }
		for i, v := range it.Embedding { avg[i] += v }
		count++
	}
	if count == 0 { return nil }
	for i := range avg { avg[i] /= float32(count) }
	return avg
}

func findClosestSignal(r NeighborResult, signals []HistoricalItem) HistoricalItem {
	if len(signals) == 0 { return HistoricalItem{Source: "history"} }
	return signals[0]
}
```

- [ ] **Step C1.4: Test, register, commit**

Run tests, then add registration in `main.go` (similar pattern to Groups A, B), build, test, commit:

```bash
cd go/ai-service && go test ./internal/tools/composite/ -run TestRecommendRationale -v
# implement Postgres-backed UserHistory adapter and Qdrant-backed NeighborSearch adapter
# register in main.go
git add go/ai-service/
git commit -m "feat(ai-service): add recommend_with_rationale composite tool"
```

The `UserHistory` adapter reads from `orderdb`, `cartdb`, and a `recently_viewed` table (add migration if absent in `go/ecommerce-service/migrations/`). The `NeighborSearch` adapter calls Qdrant `/collections/products/points/search`.

---

## Group D: Resources

### Task D1: Resource registry

**Files:**
- Create: `go/ai-service/internal/mcp/resources/registry.go`
- Create: `go/ai-service/internal/mcp/resources/registry_test.go`

- [ ] **Step D1.1: Write failing test**

```go
// registry_test.go
package resources

import (
	"context"
	"errors"
	"testing"
)

type fakeResource struct {
	uri  string
	body string
	err  error
}

func (f fakeResource) URI() string { return f.uri }
func (f fakeResource) Name() string { return "fake" }
func (f fakeResource) Description() string { return "fake" }
func (f fakeResource) MIMEType() string { return "text/plain" }
func (f fakeResource) Read(ctx context.Context) (Content, error) {
	if f.err != nil { return Content{}, f.err }
	return Content{URI: f.uri, MIMEType: "text/plain", Text: f.body}, nil
}

func TestRegistryListAndRead(t *testing.T) {
	r := NewRegistry()
	r.Register(fakeResource{uri: "fake://a", body: "hello"})
	r.Register(fakeResource{uri: "fake://b", body: "world"})

	list := r.List()
	if len(list) != 2 { t.Fatalf("expected 2, got %d", len(list)) }

	got, err := r.Read(context.Background(), "fake://a")
	if err != nil { t.Fatal(err) }
	if got.Text != "hello" { t.Fatalf("got %s", got.Text) }
}

func TestRegistryReadUnknownURIReturnsError(t *testing.T) {
	r := NewRegistry()
	_, err := r.Read(context.Background(), "fake://missing")
	if !errors.Is(err, ErrResourceNotFound) {
		t.Fatalf("want ErrResourceNotFound, got %v", err)
	}
}
```

- [ ] **Step D1.2: Run test to verify it fails**

Run: `cd go/ai-service && go test ./internal/mcp/resources/ -v`
Expected: FAIL — package and types undefined.

- [ ] **Step D1.3: Implement registry**

```go
// registry.go
package resources

import (
	"context"
	"errors"
	"sync"
)

var ErrResourceNotFound = errors.New("resource not found")

type Resource interface {
	URI() string
	Name() string
	Description() string
	MIMEType() string
	Read(ctx context.Context) (Content, error)
}

type Content struct {
	URI      string
	MIMEType string
	Text     string
}

type Registry struct {
	mu        sync.RWMutex
	resources map[string]Resource
}

func NewRegistry() *Registry { return &Registry{resources: make(map[string]Resource)} }

func (r *Registry) Register(res Resource) {
	r.mu.Lock(); defer r.mu.Unlock()
	r.resources[res.URI()] = res
}

func (r *Registry) List() []Resource {
	r.mu.RLock(); defer r.mu.RUnlock()
	out := make([]Resource, 0, len(r.resources))
	for _, v := range r.resources { out = append(out, v) }
	return out
}

func (r *Registry) Read(ctx context.Context, uri string) (Content, error) {
	r.mu.RLock()
	res, ok := r.resources[uri]
	r.mu.RUnlock()
	if !ok { return Content{}, ErrResourceNotFound }
	return res.Read(ctx)
}
```

- [ ] **Step D1.4: Run, commit**

Run: `cd go/ai-service && go test ./internal/mcp/resources/ -v`
Expected: PASS.

```bash
git add go/ai-service/internal/mcp/resources/registry.go go/ai-service/internal/mcp/resources/registry_test.go
git commit -m "feat(ai-service): add MCP Resource registry"
```

### Task D2: catalog:// resources

**Files:**
- Create: `go/ai-service/internal/mcp/resources/catalog.go`
- Create: `go/ai-service/internal/mcp/resources/catalog_test.go`

- [ ] **Step D2.1: Write failing tests**

```go
// catalog_test.go
package resources

import (
	"context"
	"strings"
	"testing"
)

type fakeCatalogClient struct {
	categories []CatalogCategory
	featured   []CatalogProduct
	products   map[string]CatalogProduct
	err        error
}

func (f fakeCatalogClient) Categories(ctx context.Context) ([]CatalogCategory, error) { return f.categories, f.err }
func (f fakeCatalogClient) Featured(ctx context.Context) ([]CatalogProduct, error) { return f.featured, f.err }
func (f fakeCatalogClient) Product(ctx context.Context, id string) (CatalogProduct, error) {
	p, ok := f.products[id]
	if !ok { return CatalogProduct{}, ErrResourceNotFound }
	return p, nil
}

func TestCategoriesResource(t *testing.T) {
	c := fakeCatalogClient{categories: []CatalogCategory{{Name: "footwear", Count: 12}}}
	r := NewCategoriesResource(c)
	got, err := r.Read(context.Background())
	if err != nil { t.Fatal(err) }
	if !strings.Contains(got.Text, "footwear") { t.Fatalf("expected footwear, got %s", got.Text) }
}

func TestFeaturedResource(t *testing.T) {
	c := fakeCatalogClient{featured: []CatalogProduct{{ID: "a", Name: "Trail Shoe"}}}
	r := NewFeaturedResource(c)
	got, err := r.Read(context.Background())
	if err != nil { t.Fatal(err) }
	if !strings.Contains(got.Text, "Trail Shoe") { t.Fatalf("got %s", got.Text) }
}

func TestProductTemplateResourceMatchesAndReads(t *testing.T) {
	c := fakeCatalogClient{products: map[string]CatalogProduct{"p1": {ID: "p1", Name: "Trail Shoe"}}}
	r := NewProductResource(c, "p1")
	if r.URI() != "catalog://product/p1" { t.Fatalf("uri: %s", r.URI()) }
	got, err := r.Read(context.Background())
	if err != nil { t.Fatal(err) }
	if !strings.Contains(got.Text, "Trail Shoe") { t.Fatalf("got %s", got.Text) }
}
```

- [ ] **Step D2.2: Run, fail, implement**

Run tests to confirm failure, then implement:

```go
// catalog.go
package resources

import (
	"context"
	"encoding/json"
	"fmt"
)

type CatalogCategory struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}
type CatalogProduct struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Category   string `json:"category"`
	PriceCents int    `json:"price_cents"`
	Stock      int    `json:"stock"`
}

type CatalogClient interface {
	Categories(ctx context.Context) ([]CatalogCategory, error)
	Featured(ctx context.Context) ([]CatalogProduct, error)
	Product(ctx context.Context, id string) (CatalogProduct, error)
}

type categoriesResource struct{ c CatalogClient }
func NewCategoriesResource(c CatalogClient) Resource { return categoriesResource{c: c} }
func (r categoriesResource) URI() string         { return "catalog://categories" }
func (r categoriesResource) Name() string        { return "Product categories" }
func (r categoriesResource) Description() string { return "List of product categories with counts." }
func (r categoriesResource) MIMEType() string    { return "application/json" }
func (r categoriesResource) Read(ctx context.Context) (Content, error) {
	cats, err := r.c.Categories(ctx)
	if err != nil { return Content{}, err }
	body, _ := json.MarshalIndent(cats, "", "  ")
	return Content{URI: r.URI(), MIMEType: r.MIMEType(), Text: string(body)}, nil
}

type featuredResource struct{ c CatalogClient }
func NewFeaturedResource(c CatalogClient) Resource { return featuredResource{c: c} }
func (r featuredResource) URI() string         { return "catalog://featured" }
func (r featuredResource) Name() string        { return "Featured products" }
func (r featuredResource) Description() string { return "Curated featured product set." }
func (r featuredResource) MIMEType() string    { return "application/json" }
func (r featuredResource) Read(ctx context.Context) (Content, error) {
	items, err := r.c.Featured(ctx)
	if err != nil { return Content{}, err }
	body, _ := json.MarshalIndent(items, "", "  ")
	return Content{URI: r.URI(), MIMEType: r.MIMEType(), Text: string(body)}, nil
}

type productResource struct {
	c  CatalogClient
	id string
}
func NewProductResource(c CatalogClient, id string) Resource { return productResource{c: c, id: id} }
func (r productResource) URI() string         { return fmt.Sprintf("catalog://product/%s", r.id) }
func (r productResource) Name() string        { return fmt.Sprintf("Product %s", r.id) }
func (r productResource) Description() string { return "Single product detail." }
func (r productResource) MIMEType() string    { return "application/json" }
func (r productResource) Read(ctx context.Context) (Content, error) {
	p, err := r.c.Product(ctx, r.id)
	if err != nil { return Content{}, err }
	body, _ := json.MarshalIndent(p, "", "  ")
	return Content{URI: r.URI(), MIMEType: r.MIMEType(), Text: string(body)}, nil
}
```

- [ ] **Step D2.3: Run, commit**

```bash
cd go/ai-service && go test ./internal/mcp/resources/ -v
git add go/ai-service/internal/mcp/resources/catalog.go go/ai-service/internal/mcp/resources/catalog_test.go
git commit -m "feat(ai-service): add catalog:// MCP resources"
```

### Task D3: user:// resources with JWT scope check

**Files:**
- Create: `go/ai-service/internal/mcp/resources/user.go`
- Create: `go/ai-service/internal/mcp/resources/user_test.go`

- [ ] **Step D3.1: Write failing tests including the scope-leak guard**

```go
// user_test.go
package resources

import (
	"context"
	"errors"
	"testing"

	"github.com/kabradshaw1/portfolio/go/ai-service/internal/jwtctx"
)

type fakeUserClient struct{ orders, cart map[string]string }
func (f fakeUserClient) Orders(ctx context.Context, userID string) (string, error) {
	v, ok := f.orders[userID]; if !ok { return "", errors.New("not found") }
	return v, nil
}
func (f fakeUserClient) Cart(ctx context.Context, userID string) (string, error) {
	v, ok := f.cart[userID]; if !ok { return "", errors.New("not found") }
	return v, nil
}

func TestUserOrdersReadsAuthenticatedUser(t *testing.T) {
	c := fakeUserClient{orders: map[string]string{"u1": "[order1, order2]"}}
	ctx := jwtctx.WithUserID(context.Background(), "u1")
	r := NewUserOrdersResource(c)
	got, err := r.Read(ctx)
	if err != nil { t.Fatal(err) }
	if got.Text != "[order1, order2]" { t.Fatalf("got %s", got.Text) }
}

func TestUserOrdersAnonymousReturns404(t *testing.T) {
	c := fakeUserClient{orders: map[string]string{"u1": "[order1]"}}
	r := NewUserOrdersResource(c)
	_, err := r.Read(context.Background())
	if !errors.Is(err, ErrResourceNotFound) {
		t.Fatalf("expected ErrResourceNotFound for anonymous, got %v", err)
	}
}
```

If `jwtctx.WithUserID` does not exist, add it to `internal/jwtctx/`:

```go
// internal/jwtctx/userid.go
package jwtctx

import "context"

type userIDKey struct{}

func WithUserID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, userIDKey{}, id)
}

func UserID(ctx context.Context) string {
	v, _ := ctx.Value(userIDKey{}).(string)
	return v
}
```

- [ ] **Step D3.2: Run, fail, implement**

```go
// user.go
package resources

import (
	"context"

	"github.com/kabradshaw1/portfolio/go/ai-service/internal/jwtctx"
)

type UserClient interface {
	Orders(ctx context.Context, userID string) (string, error)
	Cart(ctx context.Context, userID string) (string, error)
}

type userOrdersResource struct{ c UserClient }
func NewUserOrdersResource(c UserClient) Resource { return userOrdersResource{c: c} }
func (r userOrdersResource) URI() string         { return "user://orders" }
func (r userOrdersResource) Name() string        { return "Your orders" }
func (r userOrdersResource) Description() string { return "Order history for the authenticated user." }
func (r userOrdersResource) MIMEType() string    { return "application/json" }
func (r userOrdersResource) Read(ctx context.Context) (Content, error) {
	uid := jwtctx.UserID(ctx)
	if uid == "" { return Content{}, ErrResourceNotFound }
	body, err := r.c.Orders(ctx, uid)
	if err != nil { return Content{}, err }
	return Content{URI: r.URI(), MIMEType: r.MIMEType(), Text: body}, nil
}

type userCartResource struct{ c UserClient }
func NewUserCartResource(c UserClient) Resource { return userCartResource{c: c} }
func (r userCartResource) URI() string         { return "user://cart" }
func (r userCartResource) Name() string        { return "Your cart" }
func (r userCartResource) Description() string { return "Current cart for the authenticated user." }
func (r userCartResource) MIMEType() string    { return "application/json" }
func (r userCartResource) Read(ctx context.Context) (Content, error) {
	uid := jwtctx.UserID(ctx)
	if uid == "" { return Content{}, ErrResourceNotFound }
	body, err := r.c.Cart(ctx, uid)
	if err != nil { return Content{}, err }
	return Content{URI: r.URI(), MIMEType: r.MIMEType(), Text: body}, nil
}
```

- [ ] **Step D3.3: Run, commit**

```bash
cd go/ai-service && go test ./internal/mcp/resources/ -v
git add go/ai-service/internal/jwtctx/userid.go \
        go/ai-service/internal/mcp/resources/user.go \
        go/ai-service/internal/mcp/resources/user_test.go
git commit -m "feat(ai-service): add user:// MCP resources with JWT scope guard"
```

### Task D4: runbook:// and schema:// file-backed resources

**Files:**
- Create: `go/ai-service/internal/mcp/resources/runbook.go`
- Create: `go/ai-service/internal/mcp/resources/runbook_test.go`
- Create: `go/ai-service/internal/mcp/resources/schema.go`
- Create: `go/ai-service/internal/mcp/resources/schema_test.go`
- Create: `go/ai-service/resources/runbook.md`
- Create: `go/ai-service/resources/schema-ecommerce.md`

- [ ] **Step D4.1: Write failing tests**

```go
// runbook_test.go
package resources

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestRunbookResourceReadsFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "runbook.md")
	if err := os.WriteFile(path, []byte("# Portfolio runbook\nhello"), 0o644); err != nil { t.Fatal(err) }
	r, err := NewRunbookResource(path)
	if err != nil { t.Fatal(err) }
	got, err := r.Read(context.Background())
	if err != nil { t.Fatal(err) }
	if got.Text != "# Portfolio runbook\nhello" { t.Fatalf("got %s", got.Text) }
	if got.MIMEType != "text/markdown" { t.Fatalf("mime: %s", got.MIMEType) }
}
```

Analogous test for `schema_test.go` reading `schema-ecommerce.md`.

- [ ] **Step D4.2: Implement**

```go
// runbook.go
package resources

import (
	"context"
	"errors"
	"os"
	"sync"
)

type runbookResource struct {
	uri      string
	mime     string
	once     sync.Once
	content  string
	loadErr  error
	path     string
}

func NewRunbookResource(path string) (Resource, error) {
	if path == "" { return nil, errors.New("runbook path required") }
	return &runbookResource{
		uri:  "runbook://how-this-portfolio-works",
		mime: "text/markdown",
		path: path,
	}, nil
}
func (r *runbookResource) URI() string         { return r.uri }
func (r *runbookResource) Name() string        { return "How this portfolio works" }
func (r *runbookResource) Description() string { return "Architectural narrative of the portfolio system." }
func (r *runbookResource) MIMEType() string    { return r.mime }
func (r *runbookResource) Read(ctx context.Context) (Content, error) {
	r.once.Do(func() {
		b, err := os.ReadFile(r.path)
		if err != nil { r.loadErr = err; return }
		r.content = string(b)
	})
	if r.loadErr != nil { return Content{}, r.loadErr }
	return Content{URI: r.uri, MIMEType: r.mime, Text: r.content}, nil
}
```

`schema.go` mirrors this with `schema://ecommerce` and a different file path.

- [ ] **Step D4.3: Add the markdown content**

Create `go/ai-service/resources/runbook.md` containing a 200-400 word architectural narrative of the portfolio (services, data stores, saga, observability stack). Pull liberally from CLAUDE.md and `docs/architecture.md`.

Create `go/ai-service/resources/schema-ecommerce.md` containing a sanitized ER summary: tables in `productdb`, `orderdb`, `cartdb`, `paymentdb`, `ecommercedb`, with column names, types, and key relationships.

- [ ] **Step D4.4: Run, commit**

```bash
cd go/ai-service && go test ./internal/mcp/resources/ -v
git add go/ai-service/internal/mcp/resources/runbook.go \
        go/ai-service/internal/mcp/resources/runbook_test.go \
        go/ai-service/internal/mcp/resources/schema.go \
        go/ai-service/internal/mcp/resources/schema_test.go \
        go/ai-service/resources/
git commit -m "feat(ai-service): add runbook:// and schema:// MCP resources"
```

---

## Group E: Server-provided Prompts

### Task E1: Prompt registry

**Files:**
- Create: `go/ai-service/internal/mcp/prompts/registry.go`
- Create: `go/ai-service/internal/mcp/prompts/registry_test.go`

- [ ] **Step E1.1: Write failing test**

```go
// registry_test.go
package prompts

import (
	"context"
	"errors"
	"testing"
)

type fakePrompt struct{ name string }
func (f fakePrompt) Name() string { return f.name }
func (f fakePrompt) Description() string { return "fake" }
func (f fakePrompt) Arguments() []Argument { return nil }
func (f fakePrompt) Render(ctx context.Context, args map[string]string) (Rendered, error) {
	return Rendered{Messages: []Message{{Role: "user", Text: "rendered:" + f.name}}}, nil
}

func TestPromptRegistryListAndGet(t *testing.T) {
	r := NewRegistry()
	r.Register(fakePrompt{name: "a"})
	r.Register(fakePrompt{name: "b"})
	if len(r.List()) != 2 { t.Fatalf("expected 2") }
	got, err := r.Get(context.Background(), "a", nil)
	if err != nil { t.Fatal(err) }
	if got.Messages[0].Text != "rendered:a" { t.Fatalf("got %v", got) }
}

func TestPromptRegistryGetUnknown(t *testing.T) {
	r := NewRegistry()
	_, err := r.Get(context.Background(), "missing", nil)
	if !errors.Is(err, ErrPromptNotFound) { t.Fatalf("got %v", err) }
}
```

- [ ] **Step E1.2: Implement**

```go
// registry.go
package prompts

import (
	"context"
	"errors"
	"sync"
)

var ErrPromptNotFound = errors.New("prompt not found")

type Argument struct {
	Name        string
	Description string
	Required    bool
}

type Message struct {
	Role string // "user" | "assistant" | "system"
	Text string
}

type Rendered struct {
	Description string
	Messages    []Message
}

type Prompt interface {
	Name() string
	Description() string
	Arguments() []Argument
	Render(ctx context.Context, args map[string]string) (Rendered, error)
}

type Registry struct {
	mu      sync.RWMutex
	prompts map[string]Prompt
}

func NewRegistry() *Registry { return &Registry{prompts: make(map[string]Prompt)} }
func (r *Registry) Register(p Prompt) { r.mu.Lock(); r.prompts[p.Name()] = p; r.mu.Unlock() }
func (r *Registry) List() []Prompt {
	r.mu.RLock(); defer r.mu.RUnlock()
	out := make([]Prompt, 0, len(r.prompts))
	for _, p := range r.prompts { out = append(out, p) }
	return out
}
func (r *Registry) Get(ctx context.Context, name string, args map[string]string) (Rendered, error) {
	r.mu.RLock(); p, ok := r.prompts[name]; r.mu.RUnlock()
	if !ok { return Rendered{}, ErrPromptNotFound }
	return p.Render(ctx, args)
}
```

- [ ] **Step E1.3: Run, commit**

```bash
cd go/ai-service && go test ./internal/mcp/prompts/ -v
git add go/ai-service/internal/mcp/prompts/registry.go go/ai-service/internal/mcp/prompts/registry_test.go
git commit -m "feat(ai-service): add MCP Prompt registry"
```

### Task E2: Three concrete prompts

**Files:**
- Create: `go/ai-service/internal/mcp/prompts/explain_order.go`
- Create: `go/ai-service/internal/mcp/prompts/explain_order_test.go`
- Create: `go/ai-service/internal/mcp/prompts/compare_recommend.go`
- Create: `go/ai-service/internal/mcp/prompts/compare_recommend_test.go`
- Create: `go/ai-service/internal/mcp/prompts/portfolio_tour.go`
- Create: `go/ai-service/internal/mcp/prompts/portfolio_tour_test.go`

- [ ] **Step E2.1: Implement `explain-my-order` test-first**

Test:

```go
// explain_order_test.go
package prompts

import (
	"context"
	"strings"
	"testing"
)

func TestExplainMyOrderRequiresOrderID(t *testing.T) {
	p := NewExplainMyOrder()
	_, err := p.Render(context.Background(), map[string]string{})
	if err == nil { t.Fatalf("expected error for missing order_id") }
}

func TestExplainMyOrderRendersWithOrderID(t *testing.T) {
	p := NewExplainMyOrder()
	r, err := p.Render(context.Background(), map[string]string{"order_id": "ord1"})
	if err != nil { t.Fatal(err) }
	if len(r.Messages) == 0 { t.Fatalf("no messages") }
	if !strings.Contains(r.Messages[0].Text, "ord1") { t.Fatalf("expected order_id in message") }
	if !strings.Contains(strings.Join(allText(r.Messages), " "), "investigate_my_order") {
		t.Fatalf("expected mention of investigate_my_order tool")
	}
}

func allText(ms []Message) []string {
	out := make([]string, len(ms))
	for i, m := range ms { out[i] = m.Text }
	return out
}
```

Implementation:

```go
// explain_order.go
package prompts

import (
	"context"
	"errors"
	"fmt"
)

type explainMyOrder struct{}

func NewExplainMyOrder() Prompt { return explainMyOrder{} }

func (explainMyOrder) Name() string        { return "explain-my-order" }
func (explainMyOrder) Description() string { return "Explains the current state of an order in plain language by walking the saga." }
func (explainMyOrder) Arguments() []Argument {
	return []Argument{{Name: "order_id", Description: "The order id.", Required: true}}
}
func (explainMyOrder) Render(ctx context.Context, args map[string]string) (Rendered, error) {
	id, ok := args["order_id"]
	if !ok || id == "" { return Rendered{}, errors.New("order_id is required") }
	return Rendered{
		Description: "Walk the checkout saga for this order and explain it to the customer.",
		Messages: []Message{
			{Role: "system", Text: "You are a helpful assistant explaining order status to a customer. Be specific about the saga stage. Avoid jargon."},
			{Role: "user", Text: fmt.Sprintf("Use the investigate_my_order tool with order_id=%s, then explain in 2-3 sentences what's happening with my order. If the verdict says the order is stalled, suggest the next action.", id)},
		},
	}, nil
}
```

- [ ] **Step E2.2: Implement `compare-and-recommend`**

Same shape; takes optional `category`; renders messages instructing the assistant to call `recommend_with_rationale` then `compare_products` on the top three.

- [ ] **Step E2.3: Implement `tell-me-about-this-portfolio`**

No required arguments. Renders messages instructing the assistant to read `runbook://how-this-portfolio-works` and `schema://ecommerce` and present a guided tour. Test asserts the rendered messages reference the runbook resource URI.

```go
// portfolio_tour.go
package prompts

import "context"

type portfolioTour struct{}

func NewPortfolioTour() Prompt { return portfolioTour{} }
func (portfolioTour) Name() string        { return "tell-me-about-this-portfolio" }
func (portfolioTour) Description() string { return "Guided tour of how this portfolio is built." }
func (portfolioTour) Arguments() []Argument { return nil }
func (portfolioTour) Render(ctx context.Context, args map[string]string) (Rendered, error) {
	return Rendered{
		Description: "Tour of the portfolio architecture.",
		Messages: []Message{
			{Role: "system", Text: "You are giving a guided tour of a software engineering portfolio."},
			{Role: "user", Text: "Read the resource at runbook://how-this-portfolio-works and the resource at schema://ecommerce, then walk me through how the AI services, ecommerce services, and data layer fit together. Aim for 4 short sections."},
		},
	}, nil
}
```

- [ ] **Step E2.4: Run all prompt tests, commit**

```bash
cd go/ai-service && go test ./internal/mcp/prompts/ -v
git add go/ai-service/internal/mcp/prompts/
git commit -m "feat(ai-service): add three server-provided MCP prompts"
```

---

## Group F: Wire Resources and Prompts into the MCP server

### Task F1: Inspect the existing server

- [ ] **Step F1.1: Read the current MCP server**

Run: `cat go/ai-service/internal/mcp/server.go`
Note: `NewServer(reg tools.Registry, defaults Defaults) *sdkmcp.Server` only registers Tools today.

### Task F2: Extend `NewServer` to register Resources and Prompts

- [ ] **Step F2.1: Write failing end-to-end test**

Create `go/ai-service/internal/mcp/adapters_test.go`:

```go
package mcp

import (
	"context"
	"testing"

	"github.com/kabradshaw1/portfolio/go/ai-service/internal/mcp/prompts"
	"github.com/kabradshaw1/portfolio/go/ai-service/internal/mcp/resources"
	"github.com/kabradshaw1/portfolio/go/ai-service/internal/tools"
)

func TestNewServerRegistersResourcesAndPrompts(t *testing.T) {
	resReg := resources.NewRegistry()
	resReg.Register(stubResource{uri: "stub://thing"})
	promReg := prompts.NewRegistry()
	promReg.Register(stubPrompt{name: "stub-prompt"})

	srv := NewServer(tools.NewRegistry(), Defaults{}, WithResources(resReg), WithPrompts(promReg))
	if srv == nil { t.Fatalf("server nil") }
	// Drive the SDK's list/read APIs through the server's underlying handler
	// surface to confirm the registrations are reachable. The exact assertion
	// depends on the SDK shape — see go-sdk docs.
}

type stubResource struct{ uri string }
func (s stubResource) URI() string { return s.uri }
func (s stubResource) Name() string { return "stub" }
func (s stubResource) Description() string { return "stub" }
func (s stubResource) MIMEType() string { return "text/plain" }
func (s stubResource) Read(ctx context.Context) (resources.Content, error) {
	return resources.Content{URI: s.uri, MIMEType: "text/plain", Text: "hi"}, nil
}

type stubPrompt struct{ name string }
func (s stubPrompt) Name() string { return s.name }
func (s stubPrompt) Description() string { return "stub" }
func (s stubPrompt) Arguments() []prompts.Argument { return nil }
func (s stubPrompt) Render(ctx context.Context, args map[string]string) (prompts.Rendered, error) {
	return prompts.Rendered{Messages: []prompts.Message{{Role: "user", Text: "hi"}}}, nil
}
```

- [ ] **Step F2.2: Modify `server.go`**

```go
// internal/mcp/server.go
type Option func(*serverOpts)
type serverOpts struct {
	resources *resources.Registry
	prompts   *prompts.Registry
}

func WithResources(r *resources.Registry) Option { return func(o *serverOpts) { o.resources = r } }
func WithPrompts(p *prompts.Registry) Option   { return func(o *serverOpts) { o.prompts = p } }

func NewServer(reg tools.Registry, defaults Defaults, opts ...Option) *sdkmcp.Server {
	options := serverOpts{}
	for _, o := range opts { o(&options) }

	srv := sdkmcp.NewServer(&sdkmcp.Implementation{
		Name:    "ai-service",
		Version: "1.1.0",
	}, nil)

	for _, t := range reg.All() { registerTool(srv, t, defaults) }

	if options.resources != nil {
		registerResourceHandlers(srv, options.resources)
	}
	if options.prompts != nil {
		registerPromptHandlers(srv, options.prompts)
	}
	return srv
}
```

Add `registerResourceHandlers` and `registerPromptHandlers` in new files (`server_resources.go`, `server_prompts.go`) using the `modelcontextprotocol/go-sdk` API for `srv.AddResource(...)` and `srv.AddPrompt(...)`. Refer to `go.sum`'s pinned SDK version and the SDK's exported symbols — the exact API names should be verified before writing the bridge:

```bash
cd go/ai-service && grep -rn "AddTool\|AddResource\|AddPrompt" $(go env GOMODCACHE)/github.com/modelcontextprotocol/go-sdk*/
```

The bridge converts our `resources.Resource` / `prompts.Prompt` shapes to the SDK's expected types. Our internal types stay independent of the SDK so we can swap SDK versions without rewriting business logic.

- [ ] **Step F2.3: Run, commit**

```bash
cd go/ai-service && go build ./... && go test ./...
git add go/ai-service/internal/mcp/
git commit -m "feat(ai-service): wire Resource and Prompt registries into MCP server"
```

### Task F3: Construct registries in `main.go`

- [ ] **Step F3.1: Update `cmd/server/main.go`**

```go
import (
	"github.com/kabradshaw1/portfolio/go/ai-service/internal/mcp"
	"github.com/kabradshaw1/portfolio/go/ai-service/internal/mcp/prompts"
	"github.com/kabradshaw1/portfolio/go/ai-service/internal/mcp/resources"
)

// In main(), after registering tools:
catalogClient := /* construct Catalog client backed by product-service REST */
userClient    := /* construct User client backed by ecommerce-service REST */

resReg := resources.NewRegistry()
resReg.Register(resources.NewCategoriesResource(catalogClient))
resReg.Register(resources.NewFeaturedResource(catalogClient))
resReg.Register(resources.NewUserOrdersResource(userClient))
resReg.Register(resources.NewUserCartResource(userClient))

if runbook, err := resources.NewRunbookResource(cfg.RunbookPath); err == nil {
	resReg.Register(runbook)
}
if schema, err := resources.NewSchemaResource(cfg.SchemaPath); err == nil {
	resReg.Register(schema)
}

promReg := prompts.NewRegistry()
promReg.Register(prompts.NewExplainMyOrder())
promReg.Register(prompts.NewCompareAndRecommend())
promReg.Register(prompts.NewPortfolioTour())

mcpServer := mcp.NewServer(reg, defaults, mcp.WithResources(resReg), mcp.WithPrompts(promReg))
```

The catalog and user clients are thin wrappers over existing service clients — see `internal/tools/clients/` for the existing patterns. Implement them as separate small files (`internal/tools/clients/catalog.go`, `internal/tools/clients/user.go`) with their own httptest tests.

`catalog://product/{id}` is a *templated* resource — it's registered on demand when the LLM calls `resources/read` with a specific URI rather than enumerated in `resources/list`. Implement this by intercepting unknown `catalog://product/{id}` URIs in `Registry.Read`: if the URI matches the template, construct and call a `productResource` on the fly. Update `Registry.Read` accordingly:

```go
// In registry.go, modify Read:
func (r *Registry) Read(ctx context.Context, uri string) (Content, error) {
	r.mu.RLock(); res, ok := r.resources[uri]; r.mu.RUnlock()
	if ok { return res.Read(ctx) }
	// templated resources
	if id, ok := matchProductURI(uri); ok && r.catalogClient != nil {
		return NewProductResource(r.catalogClient, id).Read(ctx)
	}
	return Content{}, ErrResourceNotFound
}
```

This requires the registry to hold an optional `CatalogClient`. Add a `WithCatalogClient` option and a corresponding registry test.

- [ ] **Step F3.2: Update config, ConfigMap, build, test**

Add to `cmd/server/config.go`:

```go
RunbookPath string `env:"MCP_RESOURCES_RUNBOOK_PATH" envDefault:"/app/resources/runbook.md"`
SchemaPath  string `env:"MCP_RESOURCES_SCHEMA_PATH" envDefault:"/app/resources/schema-ecommerce.md"`
```

Update `k8s/ai-services/ai-service-configmap.yml` to include the new keys, and update `ai-service-deployment.yml` to mount the `resources/` directory (either via `COPY` in the Dockerfile, which is simpler, or a ConfigMap mount).

Simplest path: include the resource files in the Docker image. Modify `go/ai-service/Dockerfile`:

```dockerfile
COPY resources/ /app/resources/
```

- [ ] **Step F3.3: Run preflight**

```bash
make preflight-go
```

Fix any failures.

- [ ] **Step F3.4: Commit**

```bash
git add go/ai-service/cmd/server/ go/ai-service/Dockerfile go/ai-service/internal/mcp/resources/registry.go k8s/ai-services/
git commit -m "feat(ai-service): construct Resource and Prompt registries in main; mount resource files"
```

---

## Group G: Observability

### Task G1: Add Prometheus counters

- [ ] **Step G1.1: Inspect existing metrics**

Run: `cat go/ai-service/internal/metrics/metrics.go` (or equivalent — find with `grep -rn "promauto" go/ai-service`).

- [ ] **Step G1.2: Add new metrics**

Append to `internal/metrics/metrics.go`:

```go
var (
	MCPResourcesReadTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "mcp_resources_read_total",
		Help: "MCP resource read attempts.",
	}, []string{"uri", "result"})

	MCPPromptsGetTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "mcp_prompts_get_total",
		Help: "MCP prompt get calls.",
	}, []string{"name"})

	MCPCompositeToolDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "mcp_composite_tool_duration_seconds",
		Help:    "Duration of composite tool calls.",
		Buckets: prometheus.DefBuckets,
	}, []string{"tool"})
)
```

- [ ] **Step G1.3: Instrument call sites**

In each composite tool's `Call`, wrap with a timer:

```go
func (t *investigateMyOrderTool) Call(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	timer := prometheus.NewTimer(metrics.MCPCompositeToolDuration.WithLabelValues(t.Name()))
	defer timer.ObserveDuration()
	// ... existing body
}
```

In `Registry.Read`:

```go
func (r *Registry) Read(ctx context.Context, uri string) (Content, error) {
	c, err := r.read(ctx, uri)
	result := "ok"
	if err != nil { result = "error" }
	metrics.MCPResourcesReadTotal.WithLabelValues(uri, result).Inc()
	return c, err
}
```

In `Registry.Get` for prompts, similar.

- [ ] **Step G1.4: Run tests, commit**

```bash
cd go/ai-service && go test ./...
git add go/ai-service/internal/metrics/ go/ai-service/internal/mcp/ go/ai-service/internal/tools/composite/
git commit -m "feat(ai-service): instrument MCP resources, prompts, composite tools"
```

### Task G2: Tracing spans

- [ ] **Step G2.1: Add span wrapping**

Wrap every composite tool, every Resource Read, and every Prompt Render with a span using `go.opentelemetry.io/otel/trace`. Pattern (already used elsewhere in `go/pkg/tracing`):

```go
ctx, span := tracer.Start(ctx, "mcp.resource.read", trace.WithAttributes(attribute.String("uri", uri)))
defer span.End()
```

Use the existing tracer obtained via `otel.Tracer("ai-service/mcp")`.

- [ ] **Step G2.2: Run, commit**

```bash
cd go/ai-service && go test ./...
git add go/ai-service/
git commit -m "feat(ai-service): trace MCP composite tools, resources, prompts"
```

---

## Group H: K8s + smoke

### Task H1: Update manifests

- [ ] **Step H1.1: ConfigMap and Deployment changes**

Add new env vars to `k8s/ai-services/ai-service-configmap.yml`:

```yaml
JAEGER_QUERY_URL: "http://jaeger-query.monitoring.svc.cluster.local:16686"
LOKI_URL: "http://loki.monitoring.svc.cluster.local:3100"
PRODUCT_SERVICE_URL: "http://product-service.go-ecommerce.svc.cluster.local:8095"
QDRANT_URL: "http://qdrant.ai-services.svc.cluster.local:6333"
QDRANT_PRODUCT_COLLECTION: "products"
PAYMENT_DATABASE_URL: "..."  # existing pattern
CART_DATABASE_URL: "..."     # existing pattern
MCP_RESOURCES_RUNBOOK_PATH: "/app/resources/runbook.md"
MCP_RESOURCES_SCHEMA_PATH: "/app/resources/schema-ecommerce.md"
```

QA-namespace ConfigMap (if separate) gets the same keys with QA-namespace targets.

- [ ] **Step H1.2: Pre-flight again**

```bash
make preflight-go
```

- [ ] **Step H1.3: Commit**

```bash
git add k8s/ai-services/
git commit -m "feat(ai-service): k8s config for new MCP resources and composite tool deps"
```

### Task H2: Local smoke

- [ ] **Step H2.1: Run ai-service locally and exercise stdio MCP**

```bash
cd go/ai-service
go build -o /tmp/ai-service ./cmd/server
AI_SERVICE_TRANSPORT=stdio /tmp/ai-service
```

In another terminal, drive it with the SDK's reference client (or `npx @modelcontextprotocol/inspector`) and verify:

- `tools/list` includes `investigate_my_order`, `compare_products`, `recommend_with_rationale` alongside the previous 12 tools.
- `resources/list` includes `catalog://categories`, `catalog://featured`, `runbook://how-this-portfolio-works`, `schema://ecommerce`.
- `prompts/list` returns the three prompts.
- `prompts/get name=tell-me-about-this-portfolio` returns the rendered messages.

Capture the output and add it to the PR description as evidence.

- [ ] **Step H2.2: Push, open PR**

```bash
git push -u origin agent/feat-ai-service-depth-v1-backend
gh pr create --base qa --title "feat(ai-service): Phase 1 v1.0 backend depth" --body "$(cat <<'EOF'
## Summary
- 3 composite tools: investigate_my_order, compare_products, recommend_with_rationale
- 7 MCP Resources (catalog, user, runbook, schema)
- 3 server-provided Prompts
- Prometheus + tracing for new packages

Implements `docs/superpowers/specs/2026-04-28-ai-service-depth-design.md` Phase 1 v1.0 backend.

## Test plan
- [ ] Unit tests pass: `make preflight-go`
- [ ] Local stdio smoke: tools/list, resources/list, prompts/list all return expected entries
- [ ] CI green on QA after merge

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

---

## Group I: Documentation

### Task I1: ADR for the depth changes

- [ ] **Step I1.1: Use the ADR skill**

Invoke `writing-adrs`. The ADR explains: why we chose Resources/Prompts/composite-tools over more primitive tools, the JWT-scope-leak guard for `user://` resources, and how the registries are designed for SDK-version independence.

Save to `docs/adr/go-ai-service/02-mcp-depth.md` (next number after the existing `01-agent-harness-in-go.md`).

- [ ] **Step I1.2: Commit (doc-only, do not push)**

```bash
git add docs/adr/go-ai-service/02-mcp-depth.md
git commit -m "docs(adr): MCP depth — Resources, Prompts, composite tools"
```

Per the doc-only-no-push rule, this commit lands locally and rides along with the next code change.

---

## Self-review

**Spec coverage:**
- ✅ Composite tools: Groups A, B, C
- ✅ Resources: Group D (catalog, user, runbook, schema)
- ✅ Prompts: Group E
- ✅ Architectural placement: file structure section + main.go wiring in Group F
- ✅ Observability: Group G
- ✅ Auth/identity: user-resource scope guard test (D3.1)
- ✅ Effort: plan covers v1.0 only; v1.1 (Sampling, approval gates) is explicitly deferred
- ✅ Risks: composite tool latency mitigated by errgroup fan-out; partial-evidence flag covered in A2 tests; JWT scope leak test in D3
- ⏳ Frontend integration: out of scope by design — separate plan

**Placeholder scan:** No "TBD" / "TODO" / "implement later" remain. Step A5.6 is narrative because the precise consumer files vary; the engineer follows the existing pattern. Step F2.2 references SDK API exploration; that is real work, not a placeholder. Step C1.4 abbreviates the same pattern already shown in groups A and B.

**Type consistency:** `Verdict`, `Evidence`, `EvidenceBundle`, `EvidenceFetcher`, `Source` interfaces, `CompareResult`, `RecommendResult`, `Resource`, `Content`, `Prompt`, `Rendered`, `Message`, `Argument`, `Registry`, `ErrResourceNotFound`, `ErrPromptNotFound` — names are consistent across all referencing tasks.

**Effort estimate:** ~12 working days at portfolio pace (1.5–2 calendar weeks part-time).
