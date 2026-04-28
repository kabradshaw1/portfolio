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
