//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/kabradshaw1/portfolio/go/ecommerce-service/internal/integration/testutil"
	"github.com/kabradshaw1/portfolio/go/ecommerce-service/internal/repository"
	"github.com/kabradshaw1/portfolio/go/ecommerce-service/internal/service"
)

// testPublisher is a simple in-memory OrderPublisher stub that records
// every order ID published so tests can assert on them.
type testPublisher struct {
	messages []string
}

func (p *testPublisher) PublishOrderCreated(orderID string) error {
	p.messages = append(p.messages, orderID)
	return nil
}

func TestCheckoutFlow_EndToEnd(t *testing.T) {
	infra := getInfra(t)
	ctx := context.Background()
	testutil.TruncateAll(ctx, t, infra.Pool)

	// Seed 3 products so the cart items can reference real product rows.
	ids := testutil.SeedProducts(ctx, t, infra.Pool, 3)

	productID1, err := uuid.Parse(ids[0])
	if err != nil {
		t.Fatalf("parse product ID 0: %v", err)
	}
	productID2, err := uuid.Parse(ids[1])
	if err != nil {
		t.Fatalf("parse product ID 1: %v", err)
	}

	breaker := newBreaker()
	cartRepo := repository.NewCartRepository(infra.Pool, breaker)
	orderRepo := repository.NewOrderRepository(infra.Pool, breaker)
	publisher := &testPublisher{}
	orderSvc := service.NewOrderService(orderRepo, cartRepo, publisher)

	userID := uuid.New()

	// Add 2 distinct items to the cart.
	if _, err := cartRepo.AddItem(ctx, userID, productID1, 2); err != nil {
		t.Fatalf("AddItem product1: %v", err)
	}
	if _, err := cartRepo.AddItem(ctx, userID, productID2, 1); err != nil {
		t.Fatalf("AddItem product2: %v", err)
	}

	// Execute checkout.
	order, err := orderSvc.Checkout(ctx, userID)
	if err != nil {
		t.Fatalf("Checkout: %v", err)
	}

	// Order status must be "pending".
	if order.Status != "pending" {
		t.Errorf("expected order status=pending, got %s", order.Status)
	}

	// Order must contain 2 items (one per distinct product).
	if len(order.Items) != 2 {
		t.Errorf("expected 2 order items, got %d", len(order.Items))
	}

	// Cart must be empty after checkout.
	remaining, err := cartRepo.GetByUser(ctx, userID)
	if err != nil {
		t.Fatalf("GetByUser after checkout: %v", err)
	}
	if len(remaining) != 0 {
		t.Errorf("expected empty cart after checkout, got %d items", len(remaining))
	}

	// Publisher must have received exactly 1 message with the order's ID.
	if len(publisher.messages) != 1 {
		t.Fatalf("expected 1 published message, got %d", len(publisher.messages))
	}
	if publisher.messages[0] != order.ID.String() {
		t.Errorf("published order ID mismatch: want %s, got %s", order.ID.String(), publisher.messages[0])
	}

	// Verify the order is retrievable from the DB by ID.
	found, err := orderRepo.FindByID(ctx, order.ID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if found.ID != order.ID {
		t.Errorf("FindByID returned wrong order: want %s, got %s", order.ID, found.ID)
	}
}

func TestCheckoutFlow_EmptyCart(t *testing.T) {
	infra := getInfra(t)
	ctx := context.Background()
	testutil.TruncateAll(ctx, t, infra.Pool)

	breaker := newBreaker()
	cartRepo := repository.NewCartRepository(infra.Pool, breaker)
	orderRepo := repository.NewOrderRepository(infra.Pool, breaker)
	publisher := &testPublisher{}
	orderSvc := service.NewOrderService(orderRepo, cartRepo, publisher)

	userID := uuid.New()

	// No cart items have been added — Checkout must return an error.
	_, err := orderSvc.Checkout(ctx, userID)
	if err == nil {
		t.Error("expected error when checking out with empty cart, got nil")
	}
}
