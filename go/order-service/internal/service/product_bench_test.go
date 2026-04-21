package service_test

import (
	"context"
	"testing"

	"github.com/kabradshaw1/portfolio/go/order-service/internal/model"
	"github.com/kabradshaw1/portfolio/go/order-service/internal/service"
)

func BenchmarkProductList(b *testing.B) {
	svc := service.NewProductService(newMockProductRepo(), nil)
	ctx := context.Background()
	params := model.ProductListParams{Page: 1, Limit: 20}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = svc.List(ctx, params)
	}
}

func BenchmarkProductListByCategory(b *testing.B) {
	svc := service.NewProductService(newMockProductRepo(), nil)
	ctx := context.Background()
	params := model.ProductListParams{Category: "Electronics", Page: 1, Limit: 20}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = svc.List(ctx, params)
	}
}
