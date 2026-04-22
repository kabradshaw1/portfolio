package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kabradshaw1/portfolio/go/order-service/internal/kafka"
	"github.com/kabradshaw1/portfolio/go/order-service/internal/model"
	"github.com/kabradshaw1/portfolio/go/order-service/internal/saga"
	"github.com/kabradshaw1/portfolio/go/order-service/internal/service"
)

// mockOrderRepo is an in-memory order repository for tests.
type mockOrderRepo struct {
	orders map[uuid.UUID]*model.Order
}

func newMockOrderRepo() *mockOrderRepo {
	return &mockOrderRepo{orders: make(map[uuid.UUID]*model.Order)}
}

func (m *mockOrderRepo) Create(_ context.Context, userID uuid.UUID, total int, items []model.OrderItem) (*model.Order, error) {
	order := &model.Order{
		ID:        uuid.New(),
		UserID:    userID,
		Status:    model.OrderStatusPending,
		SagaStep:  saga.StepCreated,
		Total:     total,
		Items:     items,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	m.orders[order.ID] = order
	return order, nil
}

func (m *mockOrderRepo) FindByID(_ context.Context, id uuid.UUID) (*model.Order, error) {
	order, ok := m.orders[id]
	if !ok {
		return nil, errors.New("order not found")
	}
	return order, nil
}

func (m *mockOrderRepo) ListByUser(_ context.Context, userID uuid.UUID, _ model.OrderListParams) ([]model.Order, error) {
	var result []model.Order
	for _, o := range m.orders {
		if o.UserID == userID {
			result = append(result, *o)
		}
	}
	return result, nil
}

func (m *mockOrderRepo) UpdateStatus(_ context.Context, orderID uuid.UUID, status model.OrderStatus) error {
	order, ok := m.orders[orderID]
	if !ok {
		return errors.New("order not found")
	}
	order.Status = status
	return nil
}

func (m *mockOrderRepo) UpdateSagaStep(_ context.Context, orderID uuid.UUID, step string) error {
	order, ok := m.orders[orderID]
	if !ok {
		return errors.New("order not found")
	}
	order.SagaStep = step
	return nil
}

func (m *mockOrderRepo) UpdateCheckoutURL(_ context.Context, orderID uuid.UUID, url string) error {
	order, ok := m.orders[orderID]
	if !ok {
		return errors.New("order not found")
	}
	order.CheckoutURL = url
	return nil
}

// mockCartClient satisfies the CartClient interface for order tests.
type mockCartClient struct {
	items []model.CartItem
}

func (m *mockCartClient) GetByUser(_ context.Context, _ uuid.UUID) ([]model.CartItem, error) {
	return m.items, nil
}

func (m *mockCartClient) ClearCart(_ context.Context, _ uuid.UUID) error {
	m.items = nil
	return nil
}

// mockSagaPub captures published saga commands.
type mockSagaPub struct {
	commands []saga.Command
}

func (m *mockSagaPub) PublishCommand(_ context.Context, cmd saga.Command) error {
	m.commands = append(m.commands, cmd)
	return nil
}

// mockStockChecker always returns available.
type mockStockChecker struct {
	available bool
}

func (m *mockStockChecker) CheckAvailability(_ context.Context, _ uuid.UUID, _ int) (bool, error) {
	return m.available, nil
}

// nopKafka is a no-op Kafka producer for tests.
type nopKafka struct{}

func (nopKafka) Publish(context.Context, string, string, kafka.Event) error { return nil }
func (nopKafka) Close() error                                              { return nil }

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
	sagaPub := &mockSagaPub{}
	orch := saga.NewOrchestrator(orderRepo, sagaPub, &mockStockChecker{available: true}, nil, nopKafka{}, "http://localhost:3000")
	svc := service.NewOrderService(orderRepo, cartClient, orch)

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

	// Saga is kicked off asynchronously — we verify the order was created correctly.
	// Saga orchestrator tests cover the full state machine.
}

func TestCheckoutEmptyCart(t *testing.T) {
	cartClient := &mockCartClient{}
	orderRepo := newMockOrderRepo()
	sagaPub := &mockSagaPub{}
	orch := saga.NewOrchestrator(orderRepo, sagaPub, &mockStockChecker{available: true}, nil, nopKafka{}, "http://localhost:3000")
	svc := service.NewOrderService(orderRepo, cartClient, orch)

	userID := uuid.New()

	_, err := svc.Checkout(context.Background(), userID)
	if !errors.Is(err, service.ErrEmptyCart) {
		t.Fatalf("expected ErrEmptyCart, got %v", err)
	}
}
