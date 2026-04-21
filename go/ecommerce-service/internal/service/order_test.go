package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kabradshaw1/portfolio/go/ecommerce-service/internal/kafka"
	"github.com/kabradshaw1/portfolio/go/ecommerce-service/internal/model"
	"github.com/kabradshaw1/portfolio/go/ecommerce-service/internal/service"
)

// nopKafka is a no-op Kafka producer for tests.
type nopKafka struct{}

func (nopKafka) Publish(context.Context, string, string, kafka.Event) error { return nil }
func (nopKafka) Close() error                                              { return nil }

// mockOrderRepo is an in-memory order repository for tests.
type mockOrderRepo struct {
	orders map[uuid.UUID]*model.Order
}

func newMockOrderRepo() *mockOrderRepo {
	return &mockOrderRepo{orders: make(map[uuid.UUID]*model.Order)}
}

func (m *mockOrderRepo) Create(ctx context.Context, userID uuid.UUID, total int, items []model.OrderItem) (*model.Order, error) {
	order := &model.Order{
		ID:        uuid.New(),
		UserID:    userID,
		Status:    model.OrderStatusPending,
		Total:     total,
		Items:     items,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	m.orders[order.ID] = order
	return order, nil
}

func (m *mockOrderRepo) FindByID(ctx context.Context, id uuid.UUID) (*model.Order, error) {
	order, ok := m.orders[id]
	if !ok {
		return nil, errors.New("order not found")
	}
	return order, nil
}

func (m *mockOrderRepo) ListByUser(ctx context.Context, userID uuid.UUID, params model.OrderListParams) ([]model.Order, error) {
	var result []model.Order
	for _, o := range m.orders {
		if o.UserID == userID {
			result = append(result, *o)
		}
	}
	return result, nil
}

func (m *mockOrderRepo) UpdateStatus(ctx context.Context, orderID uuid.UUID, status model.OrderStatus) error {
	order, ok := m.orders[orderID]
	if !ok {
		return errors.New("order not found")
	}
	order.Status = status
	order.UpdatedAt = time.Now()
	return nil
}

// mockCartClient satisfies the CartClient interface for order tests.
type mockCartClient struct {
	items   []model.CartItem
	cleared bool
}

func (m *mockCartClient) GetByUser(_ context.Context, _ uuid.UUID) ([]model.CartItem, error) {
	return m.items, nil
}

func (m *mockCartClient) ClearCart(_ context.Context, _ uuid.UUID) error {
	m.items = nil
	m.cleared = true
	return nil
}

// mockPublisher tracks published order IDs.
type mockPublisher struct {
	publishedIDs []string
}

func (m *mockPublisher) PublishOrderCreated(orderID string) error {
	m.publishedIDs = append(m.publishedIDs, orderID)
	return nil
}

func TestCheckout(t *testing.T) {
	userID := uuid.New()
	productID := uuid.MustParse("00000000-0000-0000-0000-000000000001")

	cartClient := &mockCartClient{
		items: []model.CartItem{
			{
				ID:           uuid.New(),
				UserID:       userID,
				ProductID:    productID,
				Quantity:     2,
				ProductPrice: 5000,
				CreatedAt:    time.Now(),
			},
		},
	}
	orderRepo := newMockOrderRepo()
	publisher := &mockPublisher{}
	svc := service.NewOrderService(orderRepo, cartClient, publisher, nopKafka{})

	order, err := svc.Checkout(context.Background(), userID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if order.Status != model.OrderStatusPending {
		t.Errorf("expected status pending, got %s", order.Status)
	}
	expectedTotal := 5000 * 2
	if order.Total != expectedTotal {
		t.Errorf("expected total %d, got %d", expectedTotal, order.Total)
	}
	if len(publisher.publishedIDs) != 1 {
		t.Errorf("expected 1 published event, got %d", len(publisher.publishedIDs))
	}
	if publisher.publishedIDs[0] != order.ID.String() {
		t.Errorf("expected published ID %s, got %s", order.ID.String(), publisher.publishedIDs[0])
	}

	// Cart should be cleared after checkout.
	if !cartClient.cleared {
		t.Error("expected cart to be cleared after checkout")
	}
}

func TestCheckoutEmptyCart(t *testing.T) {
	cartClient := &mockCartClient{}
	orderRepo := newMockOrderRepo()
	publisher := &mockPublisher{}
	svc := service.NewOrderService(orderRepo, cartClient, publisher, nopKafka{})

	userID := uuid.New()

	_, err := svc.Checkout(context.Background(), userID)
	if !errors.Is(err, service.ErrEmptyCart) {
		t.Fatalf("expected ErrEmptyCart, got %v", err)
	}
}
