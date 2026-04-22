package stripe_test

import (
	"testing"

	stripeclient "github.com/kabradshaw1/portfolio/go/payment-service/internal/stripe"
)

// TestNewClient verifies that NewClient returns a non-nil client without
// making any real calls to the Stripe API.
func TestNewClient(t *testing.T) {
	c := stripeclient.NewClient("sk_test_placeholder")
	if c == nil {
		t.Fatal("expected non-nil client")
	}
}
