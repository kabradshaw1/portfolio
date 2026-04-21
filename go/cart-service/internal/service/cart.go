package service

import (
	"context"

	"github.com/google/uuid"
	"github.com/kabradshaw1/portfolio/go/cart-service/internal/kafka"
	"github.com/kabradshaw1/portfolio/go/cart-service/internal/metrics"
	"github.com/kabradshaw1/portfolio/go/cart-service/internal/model"
)

type CartRepo interface {
	GetByUser(ctx context.Context, userID uuid.UUID) ([]model.CartItem, error)
	AddItem(ctx context.Context, userID, productID uuid.UUID, quantity int) (*model.CartItem, error)
	UpdateQuantity(ctx context.Context, itemID, userID uuid.UUID, quantity int) error
	RemoveItem(ctx context.Context, itemID, userID uuid.UUID) error
	ClearCart(ctx context.Context, userID uuid.UUID) error
}

type ProductClient interface {
	ValidateProduct(ctx context.Context, productID uuid.UUID) error
	EnrichCartItems(ctx context.Context, items []model.CartItem) []model.CartItem
}

type CartService struct {
	repo           CartRepo
	kafkaPublisher kafka.Producer
	productClient  ProductClient
}

func NewCartService(repo CartRepo, kafkaPub kafka.Producer, productClient ProductClient) *CartService {
	return &CartService{repo: repo, kafkaPublisher: kafkaPub, productClient: productClient}
}

func (s *CartService) GetCart(ctx context.Context, userID uuid.UUID) ([]model.CartItem, error) {
	items, err := s.repo.GetByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return items, nil
	}
	return s.productClient.EnrichCartItems(ctx, items), nil
}

func (s *CartService) GetCartRaw(ctx context.Context, userID uuid.UUID) ([]model.CartItem, error) {
	return s.repo.GetByUser(ctx, userID)
}

func (s *CartService) AddItem(ctx context.Context, userID, productID uuid.UUID, quantity int) (*model.CartItem, error) {
	if err := s.productClient.ValidateProduct(ctx, productID); err != nil {
		return nil, err
	}

	item, err := s.repo.AddItem(ctx, userID, productID, quantity)
	if err != nil {
		return nil, err
	}

	metrics.CartItemsAdded.Inc()
	metrics.ProductValidation.WithLabelValues("success").Inc()

	kafka.SafePublish(ctx, s.kafkaPublisher, "ecommerce.cart", userID.String(), kafka.Event{
		Type: "cart.item_added",
		Data: map[string]any{
			"userID":    userID.String(),
			"productID": productID.String(),
			"quantity":  quantity,
		},
	})
	return item, nil
}

func (s *CartService) UpdateQuantity(ctx context.Context, itemID, userID uuid.UUID, quantity int) error {
	return s.repo.UpdateQuantity(ctx, itemID, userID, quantity)
}

func (s *CartService) RemoveItem(ctx context.Context, itemID, userID uuid.UUID) error {
	if err := s.repo.RemoveItem(ctx, itemID, userID); err != nil {
		return err
	}

	metrics.CartItemsRemoved.Inc()

	kafka.SafePublish(ctx, s.kafkaPublisher, "ecommerce.cart", userID.String(), kafka.Event{
		Type: "cart.item_removed",
		Data: map[string]any{
			"userID": userID.String(),
			"itemID": itemID.String(),
		},
	})
	return nil
}

func (s *CartService) ClearCart(ctx context.Context, userID uuid.UUID) error {
	return s.repo.ClearCart(ctx, userID)
}
