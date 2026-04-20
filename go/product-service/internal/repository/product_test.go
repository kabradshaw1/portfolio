package repository

import (
	"testing"

	"github.com/kabradshaw1/portfolio/go/pkg/resilience"
)

func TestNewProductRepository(t *testing.T) {
	breaker := resilience.NewBreaker(resilience.BreakerConfig{Name: "test"})
	repo := NewProductRepository(nil, breaker)
	if repo == nil {
		t.Fatal("expected non-nil repository")
	}
}
