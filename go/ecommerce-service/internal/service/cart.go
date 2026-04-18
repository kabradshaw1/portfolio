package service

import (
	"context"

	"github.com/google/uuid"
	"github.com/kabradshaw1/portfolio/go/ecommerce-service/internal/kafka"
	"github.com/kabradshaw1/portfolio/go/ecommerce-service/internal/model"
)

type CartRepo interface {
	GetByUser(ctx context.Context, userID uuid.UUID) ([]model.CartItem, error)
	AddItem(ctx context.Context, userID, productID uuid.UUID, quantity int) (*model.CartItem, error)
	UpdateQuantity(ctx context.Context, itemID, userID uuid.UUID, quantity int) error
	RemoveItem(ctx context.Context, itemID, userID uuid.UUID) error
	ClearCart(ctx context.Context, userID uuid.UUID) error
}

type CartService struct {
	repo           CartRepo
	kafkaPublisher kafka.Producer
}

func NewCartService(repo CartRepo, kafkaPub kafka.Producer) *CartService {
	return &CartService{repo: repo, kafkaPublisher: kafkaPub}
}

func (s *CartService) GetCart(ctx context.Context, userID uuid.UUID) ([]model.CartItem, error) {
	return s.repo.GetByUser(ctx, userID)
}

func (s *CartService) AddItem(ctx context.Context, userID, productID uuid.UUID, quantity int) (*model.CartItem, error) {
	item, err := s.repo.AddItem(ctx, userID, productID, quantity)
	if err != nil {
		return nil, err
	}
	kafka.SafePublish(ctx, s.kafkaPublisher, "ecommerce.cart", userID.String(), kafka.Event{
		Type: "cart.item_added",
		Data: map[string]any{
			"userID":      userID.String(),
			"productID":   productID.String(),
			"quantity":    quantity,
			"productName": item.ProductName,
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
