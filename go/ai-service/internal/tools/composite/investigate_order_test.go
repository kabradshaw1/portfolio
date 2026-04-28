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
