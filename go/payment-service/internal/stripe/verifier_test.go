package stripe

import (
	"testing"

	gostripe "github.com/stripe/stripe-go/v82"
)

func TestVerifier_InvalidSignature(t *testing.T) {
	v := NewVerifier("whsec_test_secret")

	payload := []byte(`{"id":"evt_123","type":"payment_intent.succeeded","api_version":"2024-06-20.basil"}`)
	sigHeader := "t=1234567890,v1=invalidsignature"

	_, _, _, _, err := v.VerifyAndParse(payload, sigHeader)
	if err == nil {
		t.Fatal("expected error for invalid signature, got nil")
	}
}

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
