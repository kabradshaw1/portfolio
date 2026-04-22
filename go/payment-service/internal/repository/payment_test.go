package repository

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/kabradshaw1/portfolio/go/payment-service/internal/model"
	"github.com/kabradshaw1/portfolio/go/pkg/resilience"
)

func TestIdempotencyKey(t *testing.T) {
	id := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	key := IdempotencyKey(id)
	expected := "payment:00000000-0000-0000-0000-000000000001"
	if key != expected {
		t.Errorf("expected %q, got %q", expected, key)
	}
}

func TestPaymentStatusConstants(t *testing.T) {
	cases := []struct {
		status   model.PaymentStatus
		expected string
	}{
		{model.PaymentStatusPending, "pending"},
		{model.PaymentStatusSucceeded, "succeeded"},
		{model.PaymentStatusFailed, "failed"},
		{model.PaymentStatusRefunded, "refunded"},
	}
	for _, c := range cases {
		if string(c.status) != c.expected {
			t.Errorf("expected %q, got %q", c.expected, string(c.status))
		}
	}
}

func TestPaymentRepository_NilPool_Create(t *testing.T) {
	breaker := resilience.NewBreaker(resilience.BreakerConfig{Name: "test-payment-create"})
	repo := NewPaymentRepository(nil, breaker)
	_, err := repo.Create(context.Background(), uuid.New(), 1000, "usd")
	if err == nil {
		t.Error("expected error for nil pool, got nil")
	}
}

func TestPaymentRepository_NilPool_FindByOrderID(t *testing.T) {
	breaker := resilience.NewBreaker(resilience.BreakerConfig{Name: "test-payment-find"})
	repo := NewPaymentRepository(nil, breaker)
	_, err := repo.FindByOrderID(context.Background(), uuid.New())
	if err == nil {
		t.Error("expected error for nil pool, got nil")
	}
}
