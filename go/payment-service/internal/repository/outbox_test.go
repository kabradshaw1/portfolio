package repository

import (
	"context"
	"testing"

	"github.com/kabradshaw1/portfolio/go/pkg/resilience"
)

func TestOutboxRepository_NilPool_Insert(t *testing.T) {
	breaker := resilience.NewBreaker(resilience.BreakerConfig{Name: "test-outbox-insert"})
	repo := NewOutboxRepository(nil, breaker)
	// nil tx forces the resilience.Do path which will panic/error on nil pool
	err := repo.Insert(context.Background(), nil, "payments", "payment.created", []byte(`{}`))
	if err == nil {
		t.Error("expected error for nil pool, got nil")
	}
}
