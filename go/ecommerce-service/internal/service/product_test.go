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

type mockProductRepo struct {
	products []model.Product
}

func (m *mockProductRepo) List(ctx context.Context, params model.ProductListParams) ([]model.Product, int, error) {
	var filtered []model.Product
	for _, p := range m.products {
		if params.Category != "" && p.Category != params.Category {
			continue
		}
		filtered = append(filtered, p)
	}
	return filtered, len(filtered), nil
}

func (m *mockProductRepo) FindByID(ctx context.Context, id uuid.UUID) (*model.Product, error) {
	for _, p := range m.products {
		if p.ID == id {
			return &p, nil
		}
	}
	return nil, fmt.Errorf("product not found")
}

func (m *mockProductRepo) Categories(ctx context.Context) ([]string, error) {
	seen := make(map[string]bool)
	var cats []string
	for _, p := range m.products {
		if !seen[p.Category] {
			seen[p.Category] = true
			cats = append(cats, p.Category)
		}
	}
	return cats, nil
}

func (m *mockProductRepo) DecrementStock(ctx context.Context, productID uuid.UUID, qty int) error {
	for i, p := range m.products {
		if p.ID == productID {
			if p.Stock < qty {
				return fmt.Errorf("insufficient stock")
			}
			m.products[i].Stock -= qty
			return nil
		}
	}
	return fmt.Errorf("product not found")
}

func newMockProductRepo() *mockProductRepo {
	return &mockProductRepo{
		products: []model.Product{
			{
				ID:       uuid.MustParse("00000000-0000-0000-0000-000000000001"),
				Name:     "Laptop",
				Price:    99900,
				Category: "Electronics",
				Stock:    10,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			},
			{
				ID:       uuid.MustParse("00000000-0000-0000-0000-000000000002"),
				Name:     "T-Shirt",
				Price:    2500,
				Category: "Clothing",
				Stock:    50,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			},
		},
	}
}

func TestListProducts(t *testing.T) {
	repo := newMockProductRepo()
	svc := service.NewProductService(repo, nil)

	products, total, err := svc.List(context.Background(), model.ProductListParams{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 2 {
		t.Errorf("expected total 2, got %d", total)
	}
	if len(products) != 2 {
		t.Errorf("expected 2 products, got %d", len(products))
	}
}

func TestListProductsByCategory(t *testing.T) {
	repo := newMockProductRepo()
	svc := service.NewProductService(repo, nil)

	products, total, err := svc.List(context.Background(), model.ProductListParams{Category: "Electronics"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 1 {
		t.Errorf("expected total 1, got %d", total)
	}
	if len(products) != 1 {
		t.Errorf("expected 1 product, got %d", len(products))
	}
	if products[0].Name != "Laptop" {
		t.Errorf("expected Laptop, got %s", products[0].Name)
	}
}

func TestGetCategories(t *testing.T) {
	repo := newMockProductRepo()
	svc := service.NewProductService(repo, nil)

	cats, err := svc.Categories(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cats) != 2 {
		t.Errorf("expected 2 categories, got %d", len(cats))
	}
}
