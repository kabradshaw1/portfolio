package composite

import (
	"context"
	"sync/atomic"
	"testing"
)

type recordingUserHistory struct {
	seen []string
}

func (f *recordingUserHistory) Orders(ctx context.Context, userID string) ([]HistoricalItem, error) {
	f.seen = append(f.seen, userID)
	return nil, nil
}

func (f *recordingUserHistory) CartItems(ctx context.Context, userID string) ([]HistoricalItem, error) {
	f.seen = append(f.seen, userID)
	return nil, nil
}

func (f *recordingUserHistory) RecentlyViewed(ctx context.Context, userID string) ([]HistoricalItem, error) {
	f.seen = append(f.seen, userID)
	return nil, nil
}

func TestRecommendWithRationaleRejectsMissingAuthenticatedUser(t *testing.T) {
	tool := NewRecommendWithRationaleTool(&recordingUserHistory{}, fakeNeighborSearch{})

	_, err := tool.Call(context.Background(), []byte(`{"user_id":"customer-1"}`), "")
	if err == nil {
		t.Fatalf("expected missing authenticated user to be rejected")
	}
}

func TestRecommendWithRationaleRejectsCrossUserIDArgument(t *testing.T) {
	history := &recordingUserHistory{}
	tool := NewRecommendWithRationaleTool(history, fakeNeighborSearch{})

	_, err := tool.Call(context.Background(), []byte(`{"user_id":"other-customer"}`), "customer-1")
	if err == nil {
		t.Fatalf("expected mismatched user_id argument to be rejected")
	}
	if len(history.seen) > 0 {
		t.Fatalf("history lookup should not run with user_id argument, saw %v", history.seen)
	}
}

func TestRecommendWithRationaleUsesAuthenticatedUserForHistory(t *testing.T) {
	history := &recordingUserHistory{}
	tool := NewRecommendWithRationaleTool(history, fakeNeighborSearch{})

	_, err := tool.Call(context.Background(), []byte(`{"user_id":"customer-1"}`), "customer-1")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"customer-1", "customer-1", "customer-1"}
	if len(history.seen) != len(want) {
		t.Fatalf("history lookups = %v, want %v", history.seen, want)
	}
	for i := range want {
		if history.seen[i] != want[i] {
			t.Fatalf("history lookups = %v, want %v", history.seen, want)
		}
	}
}

func TestInvestigateMyOrderRejectsMissingAuthenticatedUser(t *testing.T) {
	tool := NewInvestigateMyOrderTool(authBoundaryFetcher(OrderRecord{
		ID:     "ord-1",
		Status: "completed",
		UserID: "customer-1",
	}))

	_, err := tool.Call(context.Background(), []byte(`{"order_id":"ord-1"}`), "")
	if err == nil {
		t.Fatalf("expected missing authenticated user to be rejected")
	}
}

func TestInvestigateMyOrderRejectsOwnerMismatch(t *testing.T) {
	tool := NewInvestigateMyOrderTool(authBoundaryFetcher(OrderRecord{
		ID:     "ord-1",
		Status: "completed",
		UserID: "other-customer",
	}))

	_, err := tool.Call(context.Background(), []byte(`{"order_id":"ord-1"}`), "customer-1")
	if err == nil {
		t.Fatalf("expected order owned by another user to be rejected")
	}
}

func TestInvestigateMyOrderRejectsMissingOwnershipEvidence(t *testing.T) {
	tool := NewInvestigateMyOrderTool(authBoundaryFetcher(OrderRecord{
		ID:     "ord-1",
		Status: "completed",
		UserID: "",
	}))

	_, err := tool.Call(context.Background(), []byte(`{"order_id":"ord-1"}`), "customer-1")
	if err == nil {
		t.Fatalf("expected order without ownership evidence to be rejected")
	}
}

func TestInvestigateMyOrderRejectsOwnerMismatchBeforeSecondaryEvidence(t *testing.T) {
	secondary := &recordingSecondaryEvidence{}
	tool := NewInvestigateMyOrderTool(EvidenceFetcher{
		Order:   fakeOrderSource{data: OrderRecord{ID: "ord-1", Status: "completed", UserID: "other-customer", TraceID: "trace-1", CorrelationID: "corr-1"}},
		Saga:    secondary,
		Payment: secondary,
		Cart:    secondary,
		Rabbit:  secondary,
		Trace:   secondary,
		Logs:    secondary,
	})

	_, err := tool.Call(context.Background(), []byte(`{"order_id":"ord-1"}`), "customer-1")
	if err == nil {
		t.Fatalf("expected order owned by another user to be rejected")
	}
	if got := secondary.calls.Load(); got != 0 {
		t.Fatalf("secondary evidence should not be fetched before owner check, got %d calls", got)
	}
}

func authBoundaryFetcher(order OrderRecord) EvidenceFetcher {
	return EvidenceFetcher{
		Order:   fakeOrderSource{data: order},
		Saga:    fakeSagaSource{data: SagaHistory{Step: "completed"}},
		Payment: fakePaymentSource{data: PaymentRecord{StripeChargeID: "ch_1", WebhookReceived: true}},
		Cart:    fakeCartSource{},
		Rabbit:  fakeRabbitSource{},
		Trace:   fakeTraceSource{},
		Logs:    fakeLogSource{},
	}
}

type recordingSecondaryEvidence struct {
	calls atomic.Int32
}

func (r *recordingSecondaryEvidence) FetchSaga(ctx context.Context, id string) (SagaHistory, error) {
	r.calls.Add(1)
	return SagaHistory{}, nil
}

func (r *recordingSecondaryEvidence) FetchPayment(ctx context.Context, id string) (PaymentRecord, error) {
	r.calls.Add(1)
	return PaymentRecord{}, nil
}

func (r *recordingSecondaryEvidence) FetchCartReservation(ctx context.Context, id string) (CartReservation, error) {
	r.calls.Add(1)
	return CartReservation{}, nil
}

func (r *recordingSecondaryEvidence) FetchEvents(ctx context.Context, correlationID string) ([]RabbitEvent, error) {
	r.calls.Add(1)
	return nil, nil
}

func (r *recordingSecondaryEvidence) FetchTrace(ctx context.Context, traceID string) (TraceSummary, error) {
	r.calls.Add(1)
	return TraceSummary{}, nil
}

func (r *recordingSecondaryEvidence) FetchLogs(ctx context.Context, services []string, from, to int64) ([]string, error) {
	r.calls.Add(1)
	return nil, nil
}
