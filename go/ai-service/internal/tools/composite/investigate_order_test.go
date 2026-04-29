package composite

import (
	"context"
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
			TraceID:  "abc123",
			SagaStep: "payment_captured",
			Partial:  false,
			LastLogs: []string{"info: charge ok"},
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
	if v.CustomerMessage == "" {
		t.Fatalf("expected non-empty customer message")
	}
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
	if !v.Evidence.Partial {
		t.Fatalf("expected verdict.Evidence.Partial=true")
	}
}

func TestInvestigateMyOrderToolEndToEnd(t *testing.T) {
	tool := NewInvestigateMyOrderTool(EvidenceFetcher{
		Order:   fakeOrderSource{data: OrderRecord{ID: "ord1", Status: "completed", TraceID: "t1"}},
		Saga:    fakeSagaSource{data: SagaHistory{Step: "completed"}},
		Payment: fakePaymentSource{data: PaymentRecord{StripeChargeID: "ch", WebhookReceived: true}},
		Cart:    fakeCartSource{},
		Rabbit:  fakeRabbitSource{},
		Trace:   fakeTraceSource{},
		Logs:    fakeLogSource{},
	})
	if tool.Name() != "investigate_my_order" {
		t.Fatalf("name: %s", tool.Name())
	}
	result, err := tool.Call(context.Background(), []byte(`{"order_id":"ord1"}`), "")
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	out, err := json.Marshal(result.Content)
	if err != nil {
		t.Fatalf("marshal content: %v", err)
	}
	var v Verdict
	if e := json.Unmarshal(out, &v); e != nil {
		t.Fatalf("unmarshal: %v", e)
	}
	if v.Stage != "completed" {
		t.Fatalf("stage: %s", v.Stage)
	}
}

func TestInvestigateMyOrderRejectsMissingOrderID(t *testing.T) {
	tool := NewInvestigateMyOrderTool(EvidenceFetcher{})
	_, err := tool.Call(context.Background(), []byte(`{}`), "")
	if err == nil {
		t.Fatalf("expected error on missing order_id")
	}
}
