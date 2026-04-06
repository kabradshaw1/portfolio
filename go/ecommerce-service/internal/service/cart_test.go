package service_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kabradshaw1/portfolio/go/ecommerce-service/internal/model"
	"github.com/kabradshaw1/portfolio/go/ecommerce-service/internal/service"
)

type mockCartRepo struct {
	items []model.CartItem
}

func (m *mockCartRepo) GetByUser(ctx context.Context, userID uuid.UUID) ([]model.CartItem, error) {
	var result []model.CartItem
	for _, item := range m.items {
		if item.UserID == userID {
			result = append(result, item)
		}
	}
	return result, nil
}

func (m *mockCartRepo) AddItem(ctx context.Context, userID, productID uuid.UUID, quantity int) (*model.CartItem, error) {
	for i, item := range m.items {
		if item.UserID == userID && item.ProductID == productID {
			m.items[i].Quantity += quantity
			return &m.items[i], nil
		}
	}
	newItem := model.CartItem{
		ID:        uuid.New(),
		UserID:    userID,
		ProductID: productID,
		Quantity:  quantity,
		CreatedAt: time.Now(),
	}
	m.items = append(m.items, newItem)
	return &newItem, nil
}

func (m *mockCartRepo) UpdateQuantity(ctx context.Context, itemID, userID uuid.UUID, quantity int) error {
	for i, item := range m.items {
		if item.ID == itemID && item.UserID == userID {
			m.items[i].Quantity = quantity
			return nil
		}
	}
	return fmt.Errorf("cart item not found")
}

func (m *mockCartRepo) RemoveItem(ctx context.Context, itemID, userID uuid.UUID) error {
	for i, item := range m.items {
		if item.ID == itemID && item.UserID == userID {
			m.items = append(m.items[:i], m.items[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("cart item not found")
}

func (m *mockCartRepo) ClearCart(ctx context.Context, userID uuid.UUID) error {
	var remaining []model.CartItem
	for _, item := range m.items {
		if item.UserID != userID {
			remaining = append(remaining, item)
		}
	}
	m.items = remaining
	return nil
}

func TestAddToCart(t *testing.T) {
	repo := &mockCartRepo{}
	svc := service.NewCartService(repo)

	userID := uuid.New()
	productID := uuid.New()

	item, err := svc.AddItem(context.Background(), userID, productID, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if item.Quantity != 2 {
		t.Errorf("expected quantity 2, got %d", item.Quantity)
	}
}

func TestGetCart(t *testing.T) {
	repo := &mockCartRepo{}
	svc := service.NewCartService(repo)

	userID := uuid.New()

	_, err := svc.AddItem(context.Background(), userID, uuid.New(), 1)
	if err != nil {
		t.Fatalf("unexpected error adding item 1: %v", err)
	}
	_, err = svc.AddItem(context.Background(), userID, uuid.New(), 1)
	if err != nil {
		t.Fatalf("unexpected error adding item 2: %v", err)
	}

	items, err := svc.GetCart(context.Background(), userID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 2 {
		t.Errorf("expected 2 items, got %d", len(items))
	}
}

func TestRemoveFromCart(t *testing.T) {
	repo := &mockCartRepo{}
	svc := service.NewCartService(repo)

	userID := uuid.New()

	item, err := svc.AddItem(context.Background(), userID, uuid.New(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	err = svc.RemoveItem(context.Background(), item.ID, userID)
	if err != nil {
		t.Fatalf("unexpected error removing: %v", err)
	}

	items, err := svc.GetCart(context.Background(), userID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("expected 0 items, got %d", len(items))
	}
}
