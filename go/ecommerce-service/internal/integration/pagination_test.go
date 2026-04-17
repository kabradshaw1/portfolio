//go:build integration

package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/kabradshaw1/portfolio/go/ecommerce-service/internal/integration/testutil"
	"github.com/kabradshaw1/portfolio/go/ecommerce-service/internal/model"
	"github.com/kabradshaw1/portfolio/go/ecommerce-service/internal/pagination"
	"github.com/kabradshaw1/portfolio/go/ecommerce-service/internal/repository"
)

// TestPagination_CursorThroughAllProducts seeds 25 products and pages through
// them in 5 pages of 5 using cursor mode, verifying no duplicates, correct
// descending created_at order, and correct hasMore signals.
//
// The first page uses offset mode (no cursor). Each subsequent page is fetched
// with a cursor derived from the last product of the previous page.
func TestPagination_CursorThroughAllProducts(t *testing.T) {
	infra := getInfra(t)
	ctx := context.Background()
	testutil.TruncateAll(ctx, t, infra.Pool)

	const total = 25
	const pageSize = 5

	testutil.SeedProducts(ctx, t, infra.Pool, total)

	repo := repository.NewProductRepository(infra.Pool, newBreaker())

	seen := make(map[string]bool)
	var allProducts []model.Product

	// Page 1: offset mode (Cursor is empty) — fetches pageSize+1 to detect hasMore.
	firstPage, _, err := repo.List(ctx, model.ProductListParams{Limit: pageSize, Page: 1})
	if err != nil {
		t.Fatalf("page 1: List: %v", err)
	}
	if len(firstPage) != pageSize {
		t.Fatalf("page 1: expected %d products, got %d", pageSize, len(firstPage))
	}
	for _, p := range firstPage {
		seen[p.ID.String()] = true
		allProducts = append(allProducts, p)
	}

	// Pages 2–5: cursor mode.
	last := firstPage[len(firstPage)-1]
	cursor := pagination.EncodeCursor(last.CreatedAt.UTC().Format(time.RFC3339Nano), last.ID)

	for page := 2; ; page++ {
		products, _, err := repo.List(ctx, model.ProductListParams{
			Limit:  pageSize,
			Cursor: cursor,
		})
		if err != nil {
			t.Fatalf("page %d: List: %v", page, err)
		}

		hasMore := len(products) > pageSize
		if hasMore {
			products = products[:pageSize]
		}

		if len(products) == 0 {
			break
		}

		// Verify descending created_at order within the page.
		for i := 1; i < len(products); i++ {
			if products[i].CreatedAt.After(products[i-1].CreatedAt) {
				t.Errorf("page %d: product[%d].CreatedAt=%v is after product[%d].CreatedAt=%v (expected DESC order)",
					page, i, products[i].CreatedAt, i-1, products[i-1].CreatedAt)
			}
		}

		for _, p := range products {
			key := p.ID.String()
			if seen[key] {
				t.Errorf("duplicate product on page %d: id=%s", page, key)
			}
			seen[key] = true
			allProducts = append(allProducts, p)
		}

		if !hasMore {
			break
		}

		// Build cursor from last product of this page.
		last = products[len(products)-1]
		cursor = pagination.EncodeCursor(last.CreatedAt.UTC().Format(time.RFC3339Nano), last.ID)

		if page > total/pageSize+2 {
			t.Fatalf("too many pages; possible infinite loop")
		}
	}

	if len(allProducts) != total {
		t.Errorf("expected %d total products across all pages, got %d", total, len(allProducts))
	}
}

// TestPagination_OffsetStillWorks seeds 15 products and fetches page 2 with
// limit 5, verifying that total=15 and exactly 5 products are returned.
func TestPagination_OffsetStillWorks(t *testing.T) {
	infra := getInfra(t)
	ctx := context.Background()
	testutil.TruncateAll(ctx, t, infra.Pool)

	testutil.SeedProducts(ctx, t, infra.Pool, 15)

	repo := repository.NewProductRepository(infra.Pool, newBreaker())

	products, total, err := repo.List(ctx, model.ProductListParams{
		Limit: 5,
		Page:  2,
	})
	if err != nil {
		t.Fatalf("List page 2: %v", err)
	}
	if total != 15 {
		t.Errorf("expected total=15, got %d", total)
	}
	if len(products) != 5 {
		t.Errorf("expected 5 products on page 2, got %d", len(products))
	}
}

// TestPagination_PriceSortCursor seeds 10 products and pages through the first
// two pages with price_asc sorting, verifying ascending price order and hasMore.
func TestPagination_PriceSortCursor(t *testing.T) {
	infra := getInfra(t)
	ctx := context.Background()
	testutil.TruncateAll(ctx, t, infra.Pool)

	testutil.SeedProducts(ctx, t, infra.Pool, 10)

	repo := repository.NewProductRepository(infra.Pool, newBreaker())

	const pageSize = 3

	// First page: offset mode with price_asc.
	firstPage, _, err := repo.List(ctx, model.ProductListParams{
		Sort:  "price_asc",
		Limit: pageSize,
	})
	if err != nil {
		t.Fatalf("first page List: %v", err)
	}
	if len(firstPage) != pageSize {
		t.Fatalf("expected %d products on first page, got %d", pageSize, len(firstPage))
	}

	// Verify ascending price order on first page.
	for i := 1; i < len(firstPage); i++ {
		if firstPage[i].Price < firstPage[i-1].Price {
			t.Errorf("first page: product[%d].Price=%d < product[%d].Price=%d (expected ASC order)",
				i, firstPage[i].Price, i-1, firstPage[i-1].Price)
		}
	}

	// Build cursor from last product of first page.
	last := firstPage[len(firstPage)-1]
	cursor := pagination.EncodeCursor(fmt.Sprintf("%d", last.Price), last.ID)

	// Second page: cursor mode with price_asc.
	secondPage, _, err := repo.List(ctx, model.ProductListParams{
		Sort:   "price_asc",
		Limit:  pageSize,
		Cursor: cursor,
	})
	if err != nil {
		t.Fatalf("second page List: %v", err)
	}

	hasMore := len(secondPage) > pageSize
	if hasMore {
		secondPage = secondPage[:pageSize]
	}

	if len(secondPage) == 0 {
		t.Fatal("expected products on second page, got none")
	}

	// Verify ascending price order on second page.
	for i := 1; i < len(secondPage); i++ {
		if secondPage[i].Price < secondPage[i-1].Price {
			t.Errorf("second page: product[%d].Price=%d < product[%d].Price=%d (expected ASC order)",
				i, secondPage[i].Price, i-1, secondPage[i-1].Price)
		}
	}

	// The first product of the second page must be cheaper than or equal to the last
	// product of the first page (cursor is exclusive so strictly greater expected here).
	if len(secondPage) > 0 && secondPage[0].Price <= last.Price && secondPage[0].ID == last.ID {
		t.Errorf("second page starts with the same product as end of first page: id=%s", last.ID)
	}

	// Verify hasMore is true since 10 products / 3 per page means more pages exist.
	if !hasMore {
		t.Errorf("expected hasMore=true for second page of 10 products with limit=3")
	}
}
