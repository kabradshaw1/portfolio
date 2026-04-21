package service

import (
	"context"
	"log"

	"github.com/google/uuid"
	"github.com/kabradshaw1/portfolio/go/order-service/internal/kafka"
	"github.com/kabradshaw1/portfolio/go/order-service/internal/metrics"
	"github.com/kabradshaw1/portfolio/go/order-service/internal/model"
	"github.com/kabradshaw1/portfolio/go/pkg/apperror"
)

var ErrEmptyCart = apperror.BadRequest("EMPTY_CART", "cart is empty")

type OrderRepo interface {
	Create(ctx context.Context, userID uuid.UUID, total int, items []model.OrderItem) (*model.Order, error)
	FindByID(ctx context.Context, id uuid.UUID) (*model.Order, error)
	ListByUser(ctx context.Context, userID uuid.UUID, params model.OrderListParams) ([]model.Order, error)
	UpdateStatus(ctx context.Context, orderID uuid.UUID, status model.OrderStatus) error
}

type OrderPublisher interface {
	PublishOrderCreated(orderID string) error
}

// CartClient abstracts cart-service gRPC calls for order checkout.
type CartClient interface {
	GetByUser(ctx context.Context, userID uuid.UUID) ([]model.CartItem, error)
	ClearCart(ctx context.Context, userID uuid.UUID) error
}

type OrderService struct {
	orderRepo      OrderRepo
	cartClient     CartClient
	publisher      OrderPublisher
	kafkaPublisher kafka.Producer
}

func NewOrderService(orderRepo OrderRepo, cartClient CartClient, publisher OrderPublisher, kafkaPub kafka.Producer) *OrderService {
	return &OrderService{
		orderRepo:      orderRepo,
		cartClient:     cartClient,
		publisher:      publisher,
		kafkaPublisher: kafkaPub,
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

	if err := s.cartClient.ClearCart(ctx, userID); err != nil {
		return nil, err
	}

	metrics.OrdersPlaced.WithLabelValues("created").Inc()
	metrics.OrderValue.Observe(float64(total) / 100) // cents → dollars

	if err := s.publisher.PublishOrderCreated(order.ID.String()); err != nil {
		log.Printf("WARN: failed to publish order created event for %s: %v", order.ID, err)
		metrics.RabbitMQPublish.WithLabelValues("order.created", "error").Inc()
	} else {
		metrics.RabbitMQPublish.WithLabelValues("order.created", "success").Inc()
	}

	// Publish analytics event to Kafka (fire-and-forget).
	type itemData struct {
		ProductID  string `json:"productID"`
		Quantity   int    `json:"quantity"`
		PriceCents int    `json:"priceCents"`
	}
	items := make([]itemData, len(orderItems))
	for i, oi := range orderItems {
		items[i] = itemData{
			ProductID:  oi.ProductID.String(),
			Quantity:   oi.Quantity,
			PriceCents: oi.PriceAtPurchase,
		}
	}
	kafka.SafePublish(ctx, s.kafkaPublisher, "ecommerce.orders", order.ID.String(), kafka.Event{
		Type: "order.created",
		Data: map[string]any{
			"orderID":    order.ID.String(),
			"userID":     userID.String(),
			"totalCents": order.Total,
			"items":      items,
		},
	})

	return order, nil
}

func (s *OrderService) GetOrder(ctx context.Context, orderID uuid.UUID) (*model.Order, error) {
	return s.orderRepo.FindByID(ctx, orderID)
}

func (s *OrderService) ListOrders(ctx context.Context, userID uuid.UUID, params model.OrderListParams) ([]model.Order, error) {
	return s.orderRepo.ListByUser(ctx, userID, params)
}
