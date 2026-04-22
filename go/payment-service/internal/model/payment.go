package model

import (
	"time"

	"github.com/google/uuid"
)

type PaymentStatus string

const (
	PaymentStatusPending   PaymentStatus = "pending"
	PaymentStatusSucceeded PaymentStatus = "succeeded"
	PaymentStatusFailed    PaymentStatus = "failed"
	PaymentStatusRefunded  PaymentStatus = "refunded"
)

type Payment struct {
	ID                      uuid.UUID     `json:"id"`
	OrderID                 uuid.UUID     `json:"orderId"`
	StripePaymentIntentID   string        `json:"stripePaymentIntentId,omitempty"`
	StripeCheckoutSessionID string        `json:"stripeCheckoutSessionId,omitempty"`
	AmountCents             int           `json:"amountCents"`
	Currency                string        `json:"currency"`
	Status                  PaymentStatus `json:"status"`
	IdempotencyKey          string        `json:"idempotencyKey"`
	CreatedAt               time.Time     `json:"createdAt"`
	UpdatedAt               time.Time     `json:"updatedAt"`
}
