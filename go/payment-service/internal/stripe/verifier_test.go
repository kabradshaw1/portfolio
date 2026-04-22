package stripe

import (
	"testing"
)

func TestVerifier_InvalidSignature(t *testing.T) {
	v := NewVerifier("whsec_test_secret")

	payload := []byte(`{"id":"evt_123","type":"payment_intent.succeeded","api_version":"2024-06-20.basil"}`)
	sigHeader := "t=1234567890,v1=invalidsignature"

	_, _, _, err := v.VerifyAndParse(payload, sigHeader)
	if err == nil {
		t.Fatal("expected error for invalid signature, got nil")
	}
}
