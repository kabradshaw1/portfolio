package stripe

import (
	"encoding/json"
	"fmt"
	"strings"

	gostripe "github.com/stripe/stripe-go/v82"
	"github.com/stripe/stripe-go/v82/webhook"
)

// Verifier validates Stripe webhook signatures and extracts event fields.
type Verifier struct {
	webhookSecret string
}

// NewVerifier creates a Verifier with the given Stripe webhook signing secret.
func NewVerifier(webhookSecret string) *Verifier {
	return &Verifier{webhookSecret: webhookSecret}
}

// VerifyAndParse validates the Stripe-Signature header and returns the event type,
// event ID, payment intent ID, and metadata extracted from the event payload.
// For "payment_intent.*" events, intentID is the PaymentIntent ID and metadata
// contains the PaymentIntent's metadata map (e.g., order_id).
// For "charge.refunded" events, intentID is the PaymentIntent ID from the Charge.
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
