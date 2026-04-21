package service_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kabradshaw1/portfolio/go/cart-service/internal/kafka"
	"github.com/kabradshaw1/portfolio/go/cart-service/internal/model"
	"github.com/kabradshaw1/portfolio/go/cart-service/internal/service"
)

type mockCartRepo struct {
	items []model.CartItem
}

func (m *mockCartRepo) GetByUser(_ context.Context, userID uuid.UUID) ([]model.CartItem, error) {
	var result []model.CartItem
	for _, item := range m.items {
		if item.UserID == userID {
			result = append(result, item)
		}
	}
	return result, nil
}

func (m *mockCartRepo) AddItem(_ context.Context, userID, productID uuid.UUID, quantity int) (*model.CartItem, error) {
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

func (m *mockCartRepo) UpdateQuantity(_ context.Context, itemID, userID uuid.UUID, quantity int) error {
	for i, item := range m.items {
		if item.ID == itemID && item.UserID == userID {
			m.items[i].Quantity = quantity
			return nil
		}
	}
	return fmt.Errorf("cart item not found")
}

func (m *mockCartRepo) RemoveItem(_ context.Context, itemID, userID uuid.UUID) error {
	for i, item := range m.items {
		if item.ID == itemID && item.UserID == userID {
			m.items = append(m.items[:i], m.items[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("cart item not found")
}

func (m *mockCartRepo) Reserve(_ context.Context, _ uuid.UUID) error { return nil }

func (m *mockCartRepo) Release(_ context.Context, _ uuid.UUID) error { return nil }

func (m *mockCartRepo) ClearCart(_ context.Context, userID uuid.UUID) error {
	var remaining []model.CartItem
	for _, item := range m.items {
		if item.UserID != userID {
			remaining = append(remaining, item)
		}
	}
	m.items = remaining
	return nil
}

type mockProductClient struct{}

func (m *mockProductClient) ValidateProduct(_ context.Context, _ uuid.UUID) error {
	return nil
}

func (m *mockProductClient) EnrichCartItems(_ context.Context, items []model.CartItem) []model.CartItem {
	for i := range items {
		items[i].ProductName = "Test Product"
		items[i].ProductPrice = 999
		items[i].ProductImage = "https://example.com/image.png"
	}
	return items
}

func TestAddToCart(t *testing.T) {
	repo := &mockCartRepo{}
	svc := service.NewCartService(repo, kafka.NopProducer{}, &mockProductClient{})

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
	svc := service.NewCartService(repo, kafka.NopProducer{}, &mockProductClient{})

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
	if items[0].ProductName != "Test Product" {
		t.Errorf("expected enriched product name, got %q", items[0].ProductName)
	}
}

func TestRemoveFromCart(t *testing.T) {
	repo := &mockCartRepo{}
	svc := service.NewCartService(repo, kafka.NopProducer{}, &mockProductClient{})

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

func TestClearCart(t *testing.T) {
	repo := &mockCartRepo{}
	svc := service.NewCartService(repo, kafka.NopProducer{}, &mockProductClient{})

	userID := uuid.New()

	_, _ = svc.AddItem(context.Background(), userID, uuid.New(), 1)
	_, _ = svc.AddItem(context.Background(), userID, uuid.New(), 2)

	err := svc.ClearCart(context.Background(), userID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	items, _ := svc.GetCart(context.Background(), userID)
	if len(items) != 0 {
		t.Errorf("expected 0 items after clear, got %d", len(items))
	}
}
