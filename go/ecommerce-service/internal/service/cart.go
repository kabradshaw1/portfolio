package service

import (
	"context"

	"github.com/google/uuid"
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
	repo CartRepo
}

func NewCartService(repo CartRepo) *CartService {
	return &CartService{repo: repo}
}

func (s *CartService) GetCart(ctx context.Context, userID uuid.UUID) ([]model.CartItem, error) {
	return s.repo.GetByUser(ctx, userID)
}

func (s *CartService) AddItem(ctx context.Context, userID, productID uuid.UUID, quantity int) (*model.CartItem, error) {
	return s.repo.AddItem(ctx, userID, productID, quantity)
}

func (s *CartService) UpdateQuantity(ctx context.Context, itemID, userID uuid.UUID, quantity int) error {
	return s.repo.UpdateQuantity(ctx, itemID, userID, quantity)
}

func (s *CartService) RemoveItem(ctx context.Context, itemID, userID uuid.UUID) error {
	return s.repo.RemoveItem(ctx, itemID, userID)
}

func (s *CartService) ClearCart(ctx context.Context, userID uuid.UUID) error {
	return s.repo.ClearCart(ctx, userID)
}
