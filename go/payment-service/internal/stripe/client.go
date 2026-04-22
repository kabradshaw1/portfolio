package stripe

import (
	"context"
	"fmt"

	gostripe "github.com/stripe/stripe-go/v82"
	"github.com/stripe/stripe-go/v82/checkout/session"
	"github.com/stripe/stripe-go/v82/refund"

	"github.com/kabradshaw1/portfolio/go/payment-service/internal/service"
)

// Client wraps the Stripe SDK and satisfies service.StripeClient.
type Client struct {
	apiKey string
}

// NewClient sets the global Stripe API key and returns a new Client.
func NewClient(apiKey string) *Client {
	gostripe.Key = apiKey
	return &Client{apiKey: apiKey}
}

// CreateCheckoutSession creates a one-time Stripe Checkout Session and returns
// the hosted session URL together with the underlying PaymentIntent and Session IDs.
func (c *Client) CreateCheckoutSession(ctx context.Context, params service.CheckoutParams) (*service.CheckoutResult, error) {
	p := &gostripe.CheckoutSessionParams{
		Mode: gostripe.String(string(gostripe.CheckoutSessionModePayment)),
		LineItems: []*gostripe.CheckoutSessionLineItemParams{
			{
				PriceData: &gostripe.CheckoutSessionLineItemPriceDataParams{
					Currency:   gostripe.String(params.Currency),
					UnitAmount: gostripe.Int64(int64(params.AmountCents)),
					ProductData: &gostripe.CheckoutSessionLineItemPriceDataProductDataParams{
						Name: gostripe.String(fmt.Sprintf("Order %s", params.OrderID)),
					},
				},
				Quantity: gostripe.Int64(1),
			},
		},
		PaymentIntentData: &gostripe.CheckoutSessionPaymentIntentDataParams{
			Metadata: map[string]string{
				"order_id": params.OrderID,
			},
		},
		Metadata: map[string]string{
			"order_id": params.OrderID,
		},
		SuccessURL: gostripe.String(params.SuccessURL),
		CancelURL:  gostripe.String(params.CancelURL),
	}

	if params.IdempotencyKey != "" {
		p.SetIdempotencyKey(params.IdempotencyKey)
	}

	sess, err := session.New(p)
	if err != nil {
		return nil, fmt.Errorf("stripe checkout session: %w", err)
	}

	var intentID string
	if sess.PaymentIntent != nil {
		intentID = sess.PaymentIntent.ID
	}

	return &service.CheckoutResult{
		SessionURL:      sess.URL,
		PaymentIntentID: intentID,
		SessionID:       sess.ID,
	}, nil
}

// Refund issues a Stripe refund against the given PaymentIntent and returns the refund ID.
// reason is optional — pass an empty string to omit it.
func (c *Client) Refund(ctx context.Context, paymentIntentID, reason string) (string, error) {
	p := &gostripe.RefundParams{
		PaymentIntent: gostripe.String(paymentIntentID),
	}
	if reason != "" {
		p.Reason = gostripe.String(reason)
	}

	r, err := refund.New(p)
	if err != nil {
		return "", fmt.Errorf("stripe refund: %w", err)
	}

	return r.ID, nil
}
