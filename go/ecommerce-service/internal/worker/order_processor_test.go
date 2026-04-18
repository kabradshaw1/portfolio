package worker_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kabradshaw1/portfolio/go/ecommerce-service/internal/kafka"
	"github.com/kabradshaw1/portfolio/go/ecommerce-service/internal/model"
	"github.com/kabradshaw1/portfolio/go/ecommerce-service/internal/worker"
)

type mockOrderRepoForWorker struct {
	orders map[uuid.UUID]*model.Order
}

func (m *mockOrderRepoForWorker) FindByID(ctx context.Context, id uuid.UUID) (*model.Order, error) {
	order, ok := m.orders[id]
	if !ok {
		return nil, fmt.Errorf("order not found")
	}
	return order, nil
}

func (m *mockOrderRepoForWorker) UpdateStatus(ctx context.Context, orderID uuid.UUID, status model.OrderStatus) error {
	order, ok := m.orders[orderID]
	if !ok {
		return fmt.Errorf("order not found")
	}
	order.Status = status
	order.UpdatedAt = time.Now()
	return nil
}

type mockProductRepoForWorker struct {
	stock map[uuid.UUID]int
}

func (m *mockProductRepoForWorker) DecrementStock(ctx context.Context, productID uuid.UUID, qty int) error {
	current, ok := m.stock[productID]
	if !ok {
		return fmt.Errorf("product not found")
	}
	if current < qty {
		return fmt.Errorf("insufficient stock")
	}
	m.stock[productID] = current - qty
	return nil
}

type mockCacheInvalidator struct {
	called bool
}

func (m *mockCacheInvalidator) InvalidateCache(ctx context.Context) error {
	m.called = true
	return nil
}

func TestProcessOrder_Success(t *testing.T) {
	productID := uuid.New()
	orderID := uuid.New()

	orderRepo := &mockOrderRepoForWorker{
		orders: map[uuid.UUID]*model.Order{
			orderID: {
				ID:     orderID,
				UserID: uuid.New(),
				Status: model.OrderStatusPending,
				Total:  10000,
				Items: []model.OrderItem{
					{
						ProductID:       productID,
						Quantity:        2,
						PriceAtPurchase: 5000,
					},
				},
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			},
		},
	}

	productRepo := &mockProductRepoForWorker{
		stock: map[uuid.UUID]int{
			productID: 10,
		},
	}

	cache := &mockCacheInvalidator{}
	processor := worker.NewOrderProcessor(orderRepo, productRepo, cache, kafka.NopProducer{})

	err := processor.ProcessOrder(context.Background(), orderID.String())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	order := orderRepo.orders[orderID]
	if order.Status != model.OrderStatusCompleted {
		t.Errorf("expected status completed, got %s", order.Status)
	}

	if productRepo.stock[productID] != 8 {
		t.Errorf("expected stock 8, got %d", productRepo.stock[productID])
	}

	if !cache.called {
		t.Error("expected cache invalidation to be called")
	}
}

func TestProcessOrder_InsufficientStock(t *testing.T) {
	productID := uuid.New()
	orderID := uuid.New()

	orderRepo := &mockOrderRepoForWorker{
		orders: map[uuid.UUID]*model.Order{
			orderID: {
				ID:     orderID,
				UserID: uuid.New(),
				Status: model.OrderStatusPending,
				Total:  50000,
				Items: []model.OrderItem{
					{
						ProductID:       productID,
						Quantity:        20,
						PriceAtPurchase: 2500,
					},
				},
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			},
		},
	}

	productRepo := &mockProductRepoForWorker{
		stock: map[uuid.UUID]int{
			productID: 5,
		},
	}

	cache := &mockCacheInvalidator{}
	processor := worker.NewOrderProcessor(orderRepo, productRepo, cache, kafka.NopProducer{})

	err := processor.ProcessOrder(context.Background(), orderID.String())
	if err == nil {
		t.Fatal("expected error for insufficient stock, got nil")
	}

	order := orderRepo.orders[orderID]
	if order.Status != model.OrderStatusFailed {
		t.Errorf("expected status failed, got %s", order.Status)
	}
}
