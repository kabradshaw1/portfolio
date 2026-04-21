package service

import (
	"context"

	"github.com/google/uuid"
	"github.com/kabradshaw1/portfolio/go/order-service/internal/metrics"
	"github.com/kabradshaw1/portfolio/go/order-service/internal/model"
	"github.com/kabradshaw1/portfolio/go/order-service/internal/saga"
	"github.com/kabradshaw1/portfolio/go/pkg/apperror"
)

var ErrEmptyCart = apperror.BadRequest("EMPTY_CART", "cart is empty")

type OrderRepo interface {
	Create(ctx context.Context, userID uuid.UUID, total int, items []model.OrderItem) (*model.Order, error)
	FindByID(ctx context.Context, id uuid.UUID) (*model.Order, error)
	ListByUser(ctx context.Context, userID uuid.UUID, params model.OrderListParams) ([]model.Order, error)
	UpdateStatus(ctx context.Context, orderID uuid.UUID, status model.OrderStatus) error
	UpdateSagaStep(ctx context.Context, orderID uuid.UUID, step string) error
}

// CartClient abstracts cart-service gRPC calls for order checkout.
type CartClient interface {
	GetByUser(ctx context.Context, userID uuid.UUID) ([]model.CartItem, error)
	ClearCart(ctx context.Context, userID uuid.UUID) error
}

type OrderService struct {
	orderRepo    OrderRepo
	cartClient   CartClient
	orchestrator *saga.Orchestrator
}

func NewOrderService(orderRepo OrderRepo, cartClient CartClient, orch *saga.Orchestrator) *OrderService {
	return &OrderService{
		orderRepo:    orderRepo,
		cartClient:   cartClient,
		orchestrator: orch,
	}
}

func (s *OrderService) Checkout(ctx context.Context, userID uuid.UUID) (*model.Order, error) {
	cartItems, err := s.cartClient.GetByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	if len(cartItems) == 0 {
		return nil, ErrEmptyCart
	}

	var total int
	var orderItems []model.OrderItem
	for _, ci := range cartItems {
		total += ci.ProductPrice * ci.Quantity
		orderItems = append(orderItems, model.OrderItem{
			ProductID:       ci.ProductID,
			Quantity:        ci.Quantity,
			PriceAtPurchase: ci.ProductPrice,
		})
	}

	order, err := s.orderRepo.Create(ctx, userID, total, orderItems)
	if err != nil {
		return nil, err
	}

	metrics.OrdersPlaced.WithLabelValues("created").Inc()
	metrics.OrderValue.Observe(float64(total) / 100)

	// Kick off the saga asynchronously — order is returned as PENDING.
	go func() {
		if err := s.orchestrator.Advance(context.Background(), order.ID); err != nil {
			// Logged by orchestrator; saga recovery will pick up on restart.
			_ = err
		}
	}()

	return order, nil
}

func (s *OrderService) GetOrder(ctx context.Context, orderID uuid.UUID) (*model.Order, error) {
	return s.orderRepo.FindByID(ctx, orderID)
}

func (s *OrderService) ListOrders(ctx context.Context, userID uuid.UUID, params model.OrderListParams) ([]model.Order, error) {
	return s.orderRepo.ListByUser(ctx, userID, params)
}
