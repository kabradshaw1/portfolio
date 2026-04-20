package grpc

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/kabradshaw1/portfolio/go/product-service/internal/model"
	pb "github.com/kabradshaw1/portfolio/go/product-service/internal/pb/product/v1"
)

type fakeProduct struct {
	stock int
	price int
}

type fakeService struct {
	stockProduct *fakeProduct
}

func (f *fakeService) List(ctx context.Context, params model.ProductListParams) ([]model.Product, int, error) {
	return nil, 0, nil
}

func (f *fakeService) GetByID(ctx context.Context, id uuid.UUID) (*model.Product, error) {
	if f.stockProduct != nil {
		return &model.Product{
			ID:    id,
			Name:  "Test Product",
			Stock: f.stockProduct.stock,
			Price: f.stockProduct.price,
		}, nil
	}
	return nil, fmt.Errorf("product not found")
}

func (f *fakeService) Categories(ctx context.Context) ([]string, error) {
	return nil, nil
}

func (f *fakeService) InvalidateCache(ctx context.Context) error {
	return nil
}

func TestGetProduct_NotFound(t *testing.T) {
	svc := &fakeService{}
	srv := NewProductGRPCServer(svc)

	_, err := srv.GetProduct(context.Background(), &pb.GetProductRequest{Id: "00000000-0000-0000-0000-000000000099"})
	if err == nil {
		t.Fatal("expected error for nonexistent product")
	}
}

func TestGetProducts_Empty(t *testing.T) {
	svc := &fakeService{}
	srv := NewProductGRPCServer(svc)

	resp, err := srv.GetProducts(context.Background(), &pb.GetProductsRequest{Limit: 10})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Products) != 0 {
		t.Errorf("expected 0 products, got %d", len(resp.Products))
	}
}

func TestCheckAvailability_InStock(t *testing.T) {
	svc := &fakeService{
		stockProduct: &fakeProduct{stock: 10, price: 999},
	}
	srv := NewProductGRPCServer(svc)

	resp, err := srv.CheckAvailability(context.Background(), &pb.CheckAvailabilityRequest{
		ProductId: "00000000-0000-0000-0000-000000000001",
		Quantity:  5,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Available {
		t.Error("expected available=true")
	}
	if resp.CurrentStock != 10 {
		t.Errorf("expected stock 10, got %d", resp.CurrentStock)
	}
}
