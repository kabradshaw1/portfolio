package repository

import (
	"context"
	"testing"

	"github.com/kabradshaw1/portfolio/go/pkg/resilience"
)

func TestProcessedEventRepository_NilPool_TryInsert(t *testing.T) {
	breaker := resilience.NewBreaker(resilience.BreakerConfig{Name: "test-processed-event-insert"})
	repo := NewProcessedEventRepository(nil, breaker)
	// nil tx forces the resilience.Call path which will error on nil pool
	_, err := repo.TryInsert(context.Background(), nil, "evt_test_123", "payment_intent.succeeded")
	if err == nil {
		t.Error("expected error for nil pool, got nil")
	}
}
