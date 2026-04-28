package composite

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type fakeOrderSource struct {
	data OrderRecord
	err  error
}

func (f fakeOrderSource) FetchOrder(ctx context.Context, id string) (OrderRecord, error) {
	return f.data, f.err
}

type fakeSagaSource struct {
	data SagaHistory
	err  error
}

func (f fakeSagaSource) FetchSaga(ctx context.Context, id string) (SagaHistory, error) {
	return f.data, f.err
}

type fakePaymentSource struct {
	data PaymentRecord
	err  error
}

func (f fakePaymentSource) FetchPayment(ctx context.Context, id string) (PaymentRecord, error) {
	return f.data, f.err
}

type fakeCartSource struct {
	data CartReservation
	err  error
}

func (f fakeCartSource) FetchCartReservation(ctx context.Context, id string) (CartReservation, error) {
	return f.data, f.err
}

type fakeRabbitSource struct {
	data []RabbitEvent
	err  error
}

func (f fakeRabbitSource) FetchEvents(ctx context.Context, correlationID string) ([]RabbitEvent, error) {
	return f.data, f.err
}

type fakeTraceSource struct {
	data TraceSummary
	err  error
}

func (f fakeTraceSource) FetchTrace(ctx context.Context, traceID string) (TraceSummary, error) {
	return f.data, f.err
}

type fakeLogSource struct {
	data []string
	err  error
}

func (f fakeLogSource) FetchLogs(ctx context.Context, services []string, from, to int64) ([]string, error) {
	return f.data, f.err
}

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
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if bundle.Partial {
		t.Fatalf("expected non-partial, got partial")
	}
	if bundle.Order.ID != "ord1" {
		t.Fatalf("order id mismatch: %s", bundle.Order.ID)
	}
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
	if err != nil {
		t.Fatalf("Fetch returned error, expected partial bundle: %v", err)
	}
	if !bundle.Partial {
		t.Fatalf("expected Partial=true after saga failure")
	}
	if bundle.Order.ID != "ord1" {
		t.Fatalf("primary order data should still be present")
	}
	if len(bundle.PartialReason) != 1 {
		t.Fatalf("expected exactly 1 partial reason, got %d: %v", len(bundle.PartialReason), bundle.PartialReason)
	}
	if !strings.HasPrefix(bundle.PartialReason[0], "saga:") {
		t.Fatalf("expected partial reason to be prefixed 'saga:', got %q", bundle.PartialReason[0])
	}
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
	if err == nil {
		t.Fatalf("expected error when order fetch fails")
	}
}
